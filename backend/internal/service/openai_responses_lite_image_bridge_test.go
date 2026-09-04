//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestOpenAIGatewayServiceForward_ResponsesLiteImageBridgePreservesHeaderPolicy 验证禁用 Lite 图片桥接时仍保留模型阻止规则。
//
// @param t 测试上下文。
// @return 无。
func TestOpenAIGatewayServiceForward_ResponsesLiteImageBridgePreservesHeaderPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name          string
		model         string
		lite          bool
		blockedModels string
		wantHeader    bool
		wantHosted    bool
	}{
		{name: "普通请求保留桥接", model: "gpt-5.6-sol", wantHosted: true},
		{name: "Lite允许模型禁用桥接", model: "gpt-5.6-sol", lite: true, wantHeader: true},
		{name: "Lite默认阻止模型禁用桥接", model: "gpt-5.4", lite: true},
		{name: "Lite自定义阻止规则禁用桥接", model: "gpt-5.6-sol", lite: true, blockedModels: `["gpt-5.6*"]`},
		{name: "Lite空阻止列表禁用桥接", model: "gpt-5.4", lite: true, blockedModels: `[]`, wantHeader: true},
	}
	for _, accountType := range []string{AccountTypeAPIKey, AccountTypeOAuth} {
		for _, tt := range tests {
			t.Run(accountType+"/"+tt.name, func(t *testing.T) {
				upstream := &httpUpstreamRecorder{resp: &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
					Body: io.NopCloser(strings.NewReader(
						"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_lite_bridge\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n" +
							"data: [DONE]\n\n",
					)),
				}}
				svc := newOpenAIImageGenerationControlTestService(upstream)
				svc.cfg.Gateway.CodexImageGenerationBridgeEnabled = true
				if tt.blockedModels != "" {
					svc.settingService = NewSettingService(&responsesLitePolicySettingRepoStub{values: map[string]string{
						SettingKeyOpenAIResponsesLiteHeaderBlockedModels: tt.blockedModels,
					}}, svc.cfg)
				}
				c, _ := newOpenAIImageGenerationControlTestContext(true, "codex_cli_rs/0.147.0")
				if tt.lite {
					c.Request.Header.Set(responsesLiteHeader, "true")
				}
				account := newOpenAIImageGenerationControlTestAccount()
				account.Type = accountType
				account.Extra = map[string]any{"use_responses_api": true, "openai_passthrough": false}
				if accountType == AccountTypeOAuth {
					account.Credentials = map[string]any{"access_token": "test-token"}
				}
				body, err := json.Marshal(map[string]any{
					"model": tt.model, "stream": true, "instructions": "检查代码", "input": "检查代码", "tool_choice": "none",
				})
				require.NoError(t, err)

				result, err := svc.Forward(context.Background(), c, account, body)

				require.NoError(t, err)
				require.NotNil(t, result)
				require.NotNil(t, upstream.lastReq)
				require.Equal(t, tt.wantHeader, isOpenAIResponsesLiteHeader(upstream.lastReq.Header.Get(responsesLiteHeader)))
				require.Equal(t, tt.wantHosted, gjson.GetBytes(upstream.lastBody, `tools.#(type=="image_generation")`).Exists())
				require.Equal(t, tt.wantHosted, strings.Contains(gjson.GetBytes(upstream.lastBody, "instructions").String(), codexImageGenerationBridgeMarker))
				if tt.lite {
					require.Equal(t, "none", gjson.GetBytes(upstream.lastBody, "tool_choice").String())
				}
			})
		}
	}
}
