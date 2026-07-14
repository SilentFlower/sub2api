//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIJSONSchemaDowngradeGatewayPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name             string
		inboundEndpoint  string
		body             string
		responsesSupport bool
		forward          func(*OpenAIGatewayService, *gin.Context, *Account, []byte) error
		assertUpstream   func(*testing.T, []byte)
	}{
		{
			name:             "native responses to responses",
			inboundEndpoint:  openAIResponsesEndpoint,
			body:             `{"model":"gpt-5.4","input":"hello","stream":false,"text":{"format":{"type":"json_schema","schema":{"type":"object","required":["answer"]}}}}`,
			responsesSupport: true,
			forward: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) error {
				_, err := svc.Forward(context.Background(), c, account, body)
				return err
			},
			assertUpstream: func(t *testing.T, body []byte) {
				require.Equal(t, "json_object", gjson.GetBytes(body, "text.format.type").String())
				require.Contains(t, gjson.GetBytes(body, "instructions").String(), `"required":["answer"]`)
			},
		},
		{
			name:             "responses to raw chat",
			inboundEndpoint:  openAIResponsesEndpoint,
			body:             `{"model":"deepseek-v4-pro","input":"hello","stream":false,"text":{"format":{"type":"json_schema","schema":{"type":"object","properties":{"answer":{"type":"string"}}}}}}`,
			responsesSupport: false,
			forward: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) error {
				_, err := svc.Forward(context.Background(), c, account, body)
				return err
			},
			assertUpstream: func(t *testing.T, body []byte) {
				require.Equal(t, "json_object", gjson.GetBytes(body, "response_format.type").String())
				require.Equal(t, "system", gjson.GetBytes(body, "messages.0.role").String())
				require.Contains(t, gjson.GetBytes(body, "messages.0.content").String(), `"answer":{"type":"string"}`)
			},
		},
		{
			name:             "raw chat to raw chat",
			inboundEndpoint:  openAIChatCompletionsEndpoint,
			body:             `{"model":"deepseek-v4-pro","messages":[{"role":"user","content":"hello"}],"stream":false,"response_format":{"type":"json_schema","json_schema":{"schema":{"type":"object","required":["answer"]}}}}`,
			responsesSupport: false,
			forward: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) error {
				_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
				return err
			},
			assertUpstream: func(t *testing.T, body []byte) {
				require.Equal(t, "json_object", gjson.GetBytes(body, "response_format.type").String())
				require.Equal(t, "system", gjson.GetBytes(body, "messages.0.role").String())
				require.Equal(t, "user", gjson.GetBytes(body, "messages.1.role").String())
			},
		},
		{
			name:             "chat to responses",
			inboundEndpoint:  openAIChatCompletionsEndpoint,
			body:             `{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false,"response_format":{"type":"json_schema","json_schema":{"schema":{"type":"object","required":["answer"]}}}}`,
			responsesSupport: true,
			forward: func(svc *OpenAIGatewayService, c *gin.Context, account *Account, body []byte) error {
				_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
				return err
			},
			assertUpstream: func(t *testing.T, body []byte) {
				require.Equal(t, "json_object", gjson.GetBytes(body, "text.format.type").String())
				require.Contains(t, string(body), `\"required\":[\"answer\"]`)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, tc.inboundEndpoint, bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"stop after request capture"}}`)),
			}}
			svc := &OpenAIGatewayService{
				cfg:          rawChatCompletionsTestConfig(),
				httpUpstream: upstream,
			}
			account := rawChatCompletionsTestAccount()
			account.Extra = map[string]any{
				openai_compat.ExtraKeyResponsesSupported:     tc.responsesSupport,
				openai_compat.ExtraKeyJSONSchemaToJSONObject: true,
			}

			err := tc.forward(svc, c, account, body)
			require.Error(t, err)
			require.NotNil(t, upstream.lastReq)
			tc.assertUpstream(t, upstream.lastBody)
		})
	}
}
