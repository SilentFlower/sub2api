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

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMessagesChatFallbackClampsActualOllamaOutboundTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		baseURL string
		input   int
		cap     any
		want    int64
	}{
		{name: "默认上限", baseURL: "https://ollama.com/v1", input: 256000, want: 65535},
		{name: "自定义上限", baseURL: "https://ollama.com/v1", input: 256000, cap: 32768, want: 32768},
		{name: "显式禁用", baseURL: "https://ollama.com/v1", input: 256000, cap: 0, want: 256000},
		{name: "上限以内保持", baseURL: "https://ollama.com/v1", input: 4096, want: 4096},
		{name: "非 Ollama 不裁剪", baseURL: "https://api.deepseek.com/v1", input: 256000, want: 256000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":"public","max_tokens":%d,"messages":[{"role":"user","content":"hello"}]}`, tt.input))
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":"chatcmpl_tokens","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`)),
			}}
			account := forceChatMessagesFallbackAccount()
			account.Credentials["base_url"] = tt.baseURL
			account.Credentials["model_mapping"] = map[string]any{"public": "deepseek-v4-pro"}
			if tt.cap != nil {
				account.Extra[OllamaCloudMaxTokensCapExtraKey] = tt.cap
			}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, "/v1/chat/completions", upstream.lastReq.URL.Path)
			require.Equal(t, "deepseek-v4-pro", gjson.GetBytes(upstream.lastBody, "model").String())
			require.Equal(t, tt.want, gjson.GetBytes(upstream.lastBody, "max_completion_tokens").Int())
			require.Equal(t, int64(tt.input), gjson.GetBytes(body, "max_tokens").Int())
		})
	}
}
