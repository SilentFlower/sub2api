package service

import (
	"context"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestOpenAIImageGenerationModelPrecedence 验证后台配置、环境变量和默认值在管理与转发链路中的优先级。
// @param t 测试上下文。
// @return 无。
func TestOpenAIImageGenerationModelPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		env  string
		want string
	}{
		{"内置默认", "", "", "gpt-5.6-luna"},
		{"空白环境变量", " ", " \t", "gpt-5.6-luna"},
		{"环境变量回退", "", " gpt-5.6-terra ", "gpt-5.6-terra"},
		{"后台配置优先", " gpt-5.6-sol ", "gpt-5.6-terra", "gpt-5.6-sol"},
		{"存量显式配置保留", "gpt-5.4-mini", "gpt-5.6-terra", "gpt-5.4-mini"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SUB2API_IMAGES_MAIN_MODEL", tc.env)
			ctx := context.Background()
			repo := &settingValuesRepoStub{values: map[string]string{
				SettingKeyOpenAIImageGenerationMainModel:       tc.raw,
				SettingKeyOpenAIImageGenerationReasoningEffort: "high",
			}}
			settings := NewSettingService(repo, &config.Config{})
			gateway := &OpenAIGatewayService{settingService: settings}
			require.Equal(t, tc.want, settings.GetOpenAIImageGenerationMainModel(ctx))
			require.Equal(t, tc.want, gateway.openAIImageGenerationMainModel(ctx))

			body, err := buildOpenAIImagesResponsesRequest(&OpenAIImagesRequest{
				Endpoint: openAIImagesGenerationsEndpoint,
				Model:    "gpt-image-2.5-flare",
				Prompt:   "画一个红色杯子",
			}, "gpt-image-2.5-flare", gateway.openAIImagesResponsesRequestOptions(ctx))
			require.NoError(t, err)
			require.Equal(t, tc.want, gjson.GetBytes(body, "model").String())
			require.Equal(t, "high", gjson.GetBytes(body, "reasoning.effort").String())
			req := map[string]any{"model": "gpt-image-2.5-flare", "input": "画一个红色杯子"}
			require.True(t, normalizeOpenAIResponsesImageOnlyModel(req, gateway.openAIImageGenerationMainModel(ctx)))
			require.Equal(t, tc.want, req["model"])

			view, err := settings.GetAllSettings(ctx)
			require.NoError(t, err)
			require.Equal(t, strings.TrimSpace(tc.raw), view.OpenAIImageGenerationMainModel)
			require.NoError(t, settings.UpdateSettings(ctx, view))
			require.Equal(t, strings.TrimSpace(tc.raw), repo.updates[SettingKeyOpenAIImageGenerationMainModel])
			require.Equal(t, tc.want, settings.GetOpenAIImageGenerationMainModel(ctx))
		})
	}
}

// TestOpenAIImageGenerationDefaultConfiguration 验证新安装保留自动选择，并在无设置服务时使用环境变量。
// @param t 测试上下文。
// @return 无。
func TestOpenAIImageGenerationDefaultConfiguration(t *testing.T) {
	t.Setenv("SUB2API_IMAGES_MAIN_MODEL", "gpt-5.6-terra")
	ctx := context.Background()
	repo := &settingValuesRepoStub{values: map[string]string{}}
	settings := NewSettingService(repo, &config.Config{})
	require.NoError(t, settings.InitializeDefaultSettings(ctx))
	require.Contains(t, repo.updates, SettingKeyOpenAIImageGenerationMainModel)
	require.Empty(t, repo.updates[SettingKeyOpenAIImageGenerationMainModel])
	require.Equal(t, "gpt-5.6-terra", settings.GetOpenAIImageGenerationMainModel(ctx))
	var absent *SettingService
	require.Equal(t, "gpt-5.6-terra", absent.GetOpenAIImageGenerationMainModel(ctx))
	gateway := &OpenAIGatewayService{}
	require.Equal(t, "gpt-5.6-terra", gateway.openAIImageGenerationMainModel(ctx))
	require.Equal(t, "gpt-5.6-terra", gateway.openAIImagesResponsesRequestOptions(ctx).MainModel)
}
