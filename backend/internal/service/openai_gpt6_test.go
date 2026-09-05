package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

// TestNormalizeCodexModelGPT6Astra 验证 Astra 别名、后缀和未知型号的路由边界。
// @param t 测试上下文。
// @return 无。
func TestNormalizeCodexModelGPT6Astra(t *testing.T) {
	t.Parallel()

	for _, model := range []string{
		"gpt-6-astra", "gpt-6", "gpt6", "openai/gpt-6",
		"OpenAI/GPT_6_ASTRA", "gpt-6-high", "gpt-6-max", "gpt-6-ultra",
		"gpt-6-astra-max", "gpt-6-astra-ultra", "gpt-6-astra-2026-07-09",
		"gpt-6-astra-openai-compact",
	} {
		t.Run(model, func(t *testing.T) {
			require.Equal(t, "gpt-6-astra", normalizeCodexModel(model))
			require.Equal(t, "max", normalizeOpenAIReasoningEffortForModel("max", model))
		})
	}

	for _, model := range []string{"gpt-6-pro", "gpt-6-astra-custom", "gpt-60", "gpt-5.5"} {
		t.Run(model, func(t *testing.T) {
			require.Equal(t, model, normalizeCodexModel(model))
			require.False(t, isOpenAIGPT6AstraModel(model))
			require.Equal(t, "xhigh", normalizeOpenAIReasoningEffortForModel("max", model))
		})
	}

	require.Equal(t,
		[]string{"openai/gpt-6", "gpt-6", "gpt-6-astra"},
		usageBillingModelCandidates("openai/gpt-6"),
	)
}

// TestBuildCodexModelsManifestGPT6Astra 验证默认列表和客户端目录中的 Astra 能力。
// @param t 测试上下文。
// @return 无。
func TestBuildCodexModelsManifestGPT6Astra(t *testing.T) {
	t.Parallel()

	modelIDs := []string{"gpt-6-astra", "gpt-6"}
	for _, model := range modelIDs {
		require.Contains(t, openai.DefaultModelIDs(), model)
	}
	body, err := BuildCodexModelsManifest(modelIDs)
	require.NoError(t, err)
	models := decodeCodexManifestModels(t, body)
	require.Len(t, models, 2)
	for i, model := range models {
		require.Equal(t, modelIDs[i], model["slug"])
		require.Equal(t, "medium", model["default_reasoning_level"])
		require.Equal(t, []string{"low", "medium", "high", "xhigh", "max", "ultra"}, effortsFromManifestModel(t, model))
		require.EqualValues(t, 272_000, model["context_window"])
		require.EqualValues(t, 872_000, model["max_context_window"])
		require.Equal(t, true, model["support_verbosity"])
		require.Equal(t, true, model["supports_parallel_tool_calls"])
		require.Equal(t, "list", model["visibility"])
		require.Equal(t, false, model["use_responses_lite"])
		require.Equal(t, []any{map[string]any{
			"id": "priority", "name": "Fast", "description": "Priority processing for lower latency.",
		}}, model["service_tiers"])
	}
}

// TestCompleteAPIKeyCodexModelsManifestGPT6Astra 验证图像能力补齐及上游显式限制的优先级。
// @param t 测试上下文。
// @return 无。
func TestCompleteAPIKeyCodexModelsManifestGPT6Astra(t *testing.T) {
	t.Parallel()

	for _, baseURL := range []string{"", "https://openai-compatible.example.test/v1"} {
		t.Run(baseURL, func(t *testing.T) {
			svc := &OpenAIGatewayService{}
			manifest := &CodexModelsManifest{Body: []byte(`{"models":[{"slug":"gpt-6-astra"},{"slug":"gpt-6"}]}`)}
			account := newCodexModelsAPIKeyTestAccount(baseURL)
			require.NoError(t, svc.CompleteAPIKeyCodexModelsManifestForClient(manifest, account))
			models := decodeCodexManifestModels(t, manifest.Body)
			require.Len(t, models, 2)
			for _, model := range models {
				require.Equal(t, []any{"text", "image"}, model["input_modalities"])
			}
		})
	}

	svc := &OpenAIGatewayService{}
	manifest := &CodexModelsManifest{Body: []byte(`{"models":[{"slug":"gpt-6-astra","input_modalities":["text"]}]}`)}
	account := newCodexModelsAPIKeyTestAccount("https://openai-compatible.example.test/v1")
	require.NoError(t, svc.CompleteAPIKeyCodexModelsManifestForClient(manifest, account))
	models := decodeCodexManifestModels(t, manifest.Body)
	require.Len(t, models, 1)
	require.Equal(t, []any{"text"}, models[0]["input_modalities"])
}

// TestAdjustAPIKeyCodexModelsManifestGPT6Astra 验证 API Key 目录使用完整 Responses 并保留其它元数据。
// @param t 测试上下文。
// @return 无。
func TestAdjustAPIKeyCodexModelsManifestGPT6Astra(t *testing.T) {
	t.Parallel()

	body := []byte(`{"models":[{"slug":"gpt-6-astra","use_responses_lite":true,"custom":1},{"slug":"gpt-6","use_responses_lite":true},{"slug":"gpt-6-pro","use_responses_lite":true}],"revision":2}`)
	adjusted, err := adjustAPIKeyCodexModelsManifest(body)
	require.NoError(t, err)
	require.JSONEq(t, `{"models":[{"slug":"gpt-6-astra","use_responses_lite":false,"custom":1},{"slug":"gpt-6","use_responses_lite":false},{"slug":"gpt-6-pro","use_responses_lite":true}],"revision":2}`, string(adjusted))
	unchanged, err := adjustAPIKeyCodexModelsManifest(adjusted)
	require.NoError(t, err)
	require.Equal(t, adjusted, unchanged)
}
