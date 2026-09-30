//go:build unit

package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForwardMessagesChatDisabledThinkingUsesFinalProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		requested  string
		mapped     string
		baseURL    string
		wantEffort string
	}{
		{name: "原生模型后缀不得覆盖关闭", requested: "gpt-5.6-luna-xhigh", mapped: "gpt-5.6-luna", baseURL: "http://upstream.example", wantEffort: "none"},
		{name: "公开 GPT 别名映射到 DeepSeek", requested: "gpt-5.4-xhigh", mapped: "deepseek-v4-pro", baseURL: "http://upstream.example"},
		{name: "GLM 不得把关闭映射成 high", requested: "public", mapped: "glm-5.3", baseURL: "http://upstream.example"},
		{name: "兼容别名映射到原生模型", requested: "public", mapped: "gpt-5.6-luna", baseURL: "http://upstream.example", wantEffort: "none"},
		{name: "官方端点采用 OpenAI 字段", requested: "public", mapped: "custom-model", baseURL: "https://api.openai.com/v1", wantEffort: "none"},
		{name: "未知兼容模型保留开关", requested: "public", mapped: "custom-model", baseURL: "http://upstream.example"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":%q,"max_tokens":1024,"thinking":{"type":"disabled"},"output_config":{"effort":"max"},"messages":[{"role":"user","content":"hello"}]}`, tt.requested))
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":"chatcmpl_disabled","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`)),
			}}
			account := forceChatMessagesFallbackAccount()
			account.Credentials["base_url"] = tt.baseURL
			account.Credentials["model_mapping"] = map[string]any{NormalizeOpenAICompatRequestedModel(tt.requested): tt.mapped}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, tt.mapped, gjson.GetBytes(upstream.lastBody, "model").String())
			if tt.wantEffort == "" {
				require.Equal(t, "disabled", gjson.GetBytes(upstream.lastBody, "thinking.type").String())
				require.False(t, gjson.GetBytes(upstream.lastBody, "reasoning_effort").Exists())
				require.Nil(t, result.ReasoningEffort)
			} else {
				require.False(t, gjson.GetBytes(upstream.lastBody, "thinking").Exists())
				require.Equal(t, tt.wantEffort, gjson.GetBytes(upstream.lastBody, "reasoning_effort").String())
				require.NotNil(t, result.ReasoningEffort)
				require.Equal(t, tt.wantEffort, *result.ReasoningEffort)
			}
		})
	}
}

func TestCompatModelNormalizationDisabledThinkingDoesNotDeriveEffort(t *testing.T) {
	for _, model := range []string{"gpt-5.4-xhigh", "gpt-5.6-luna-max", "openai/gpt-6.1-sol-max"} {
		t.Run(model, func(t *testing.T) {
			req := &apicompat.AnthropicRequest{Model: model, Thinking: &apicompat.AnthropicThinking{Type: "disabled"}}
			require.Empty(t, applyOpenAICompatModelNormalization(req))
			require.Nil(t, req.OutputConfig)
			require.Equal(t, "none", openAICompatAnthropicReasoningEffort(req, req.Model, "xhigh"))
		})
	}
}
