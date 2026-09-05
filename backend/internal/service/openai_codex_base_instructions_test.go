package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

// TestCodexBaseInstructionsModelAliases 验证目录与请求补全对已知别名使用相同模板。
// @param t 测试上下文。
// @return 无。
func TestCodexBaseInstructionsModelAliases(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		model     string
		wantModel string
	}{
		{"gpt-5.6", "gpt-5.6-sol"},
		{"gpt-5.6-sol", "gpt-5.6-sol"},
		{"gpt-5.6-terra", "gpt-5.6-terra"},
		{"gpt-5.6-luna", "gpt-5.6-luna"},
		{"openai/gpt-5.6", "gpt-5.6-sol"},
		{" OpenAI/GPT_5.6_TERRA ", "gpt-5.6-terra"},
		{"gpt5.6-luna", "gpt-5.6-luna"},
		{"gpt-5.6-high", "gpt-5.6-sol"},
		{"gpt-5.6-max", "gpt-5.6-sol"},
		{"gpt-5.6-sol-2026-07-09", "gpt-5.6-sol"},
		{"gpt-5.6-terra-max", "gpt-5.6-terra"},
		{"gpt-5.6-luna-openai-compact", "gpt-5.6-luna"},
		{"gpt-6", "gpt-6-astra"},
		{"gpt-6-astra", "gpt-6-astra"},
		{"gpt6", "gpt-6-astra"},
		{"openai/gpt-6", "gpt-6-astra"},
		{" OpenAI/GPT_6_ASTRA ", "gpt-6-astra"},
		{"gpt-6-high", "gpt-6-astra"},
		{"gpt-6-max", "gpt-6-astra"},
		{"gpt-6-astra-ultra", "gpt-6-astra"},
		{"gpt-6-astra-2026-07-09", "gpt-6-astra"},
		{"gpt-6-astra-openai-compact", "gpt-6-astra"},
		{"gpt-6-pro", "gpt-5.5"},
		{"gpt-6-astra-custom", "gpt-5.5"},
		{"gpt-60", "gpt-5.5"},
		{"gpt-5.6-pro", "gpt-5.5"},
		{"gpt-5.60", "gpt-5.5"},
		{"unknown-model", "gpt-5.5"},
		{"gpt-5.1", "gpt-5.1"},
		{"gpt-5.2", "gpt-5.2"},
		{"gpt-5.3", "gpt-5.5"},
		{"gpt-5.4", "gpt-5.5"},
		{"gpt-5.5", "gpt-5.5"},
		{"gpt-5.2-codex", "gpt-5-codex"},
		{"gpt-5.6-codex", "gpt-5-codex"},
		{"gpt-5.6-sol-codex", "gpt-5-codex"},
		{"gpt-6-codex", "gpt-5-codex"},
	} {
		t.Run(tc.model, func(t *testing.T) {
			want := openai.CodexBaseInstructionsForModel(tc.wantModel)
			require.Equal(t, want, codexBaseInstructionsForModel(tc.model))
			require.Equal(t, strings.TrimSpace(want), defaultCodexSynthInstructions(tc.model))

			body, err := BuildCodexModelsManifest([]string{tc.model})
			require.NoError(t, err)
			models := decodeCodexManifestModels(t, body)
			require.Len(t, models, 1)
			require.Equal(t, strings.TrimSpace(tc.model), models[0]["slug"])
			messages, ok := models[0]["model_messages"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, want, messages["instructions_template"])
		})
	}
}

// TestApplyCodexOAuthTransformNewModelInstructions 验证空提示词补全、客户端优先级和跳过开关。
// @param t 测试上下文。
// @return 无。
func TestApplyCodexOAuthTransformNewModelInstructions(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-6-astra", "openai/gpt-6"} {
		for _, isCLI := range []bool{false, true} {
			for _, tc := range []struct {
				name         string
				instructions any
				missing      bool
				skip         bool
			}{
				{name: "missing", missing: true},
				{name: "null"},
				{name: "empty", instructions: ""},
				{name: "whitespace", instructions: " \n\t"},
				{name: "existing", instructions: "  客户端原有提示词\n"},
				{name: "skip", missing: true, skip: true},
			} {
				t.Run(fmt.Sprintf("%s/cli=%t/%s", model, isCLI, tc.name), func(t *testing.T) {
					body := map[string]any{"model": model, "input": []any{}}
					if !tc.missing {
						body["instructions"] = tc.instructions
					}
					result := applyCodexOAuthTransformWithOptions(body, codexOAuthTransformOptions{
						IsCodexCLI: isCLI, SkipDefaultInstructions: tc.skip,
					})
					require.NoError(t, result.Error)
					require.Equal(t, model, body["model"])
					switch tc.name {
					case "skip":
						require.NotContains(t, body, "instructions")
					case "existing":
						require.Equal(t, tc.instructions, body["instructions"])
					default:
						wantModel := model
						if model == "openai/gpt-6" {
							wantModel = "gpt-6-astra"
						}
						require.Equal(t, strings.TrimSpace(openai.CodexBaseInstructionsForModel(wantModel)), body["instructions"])
					}
				})
			}
		}
	}
}

// TestCompleteAPIKeyCodexModelsManifestNewModelInstructions 验证目录补全采用新模板且保留上游内容。
// @param t 测试上下文。
// @return 无。
func TestCompleteAPIKeyCodexModelsManifestNewModelInstructions(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-6-astra", "gpt-6"} {
		for _, supplied := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/supplied=%t", model, supplied), func(t *testing.T) {
				messages := map[string]any{"provider_field": "原有元数据"}
				want := openai.CodexBaseInstructionsForModel(model)
				if supplied {
					want = "  上游专用提示词\n"
					messages["instructions_template"] = want
				}
				body, err := json.Marshal(map[string]any{
					"models": []any{map[string]any{"slug": model, "model_messages": messages}},
				})
				require.NoError(t, err)
				manifest := &CodexModelsManifest{Body: body}
				service := &OpenAIGatewayService{}
				account := newCodexModelsAPIKeyTestAccount("https://upstream.example/v1")
				require.NoError(t, service.CompleteAPIKeyCodexModelsManifestForClient(manifest, account))
				models := decodeCodexManifestModels(t, manifest.Body)
				require.Len(t, models, 1)
				actual, ok := models[0]["model_messages"].(map[string]any)
				require.True(t, ok)
				require.Equal(t, want, actual["instructions_template"])
				require.Equal(t, "原有元数据", actual["provider_field"])

				previous := append([]byte(nil), manifest.Body...)
				require.NoError(t, service.CompleteAPIKeyCodexModelsManifestForClient(manifest, account))
				require.Equal(t, previous, manifest.Body)
			})
		}
	}
}
