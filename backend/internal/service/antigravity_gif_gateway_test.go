//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAntigravityGatewayService_ForwardGemini_ConvertsGIFBeforeUpstream(t *testing.T) {
	body := serviceGIFRequestBody(t, serviceTestGIFBase64(t))
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{successfulAntigravityGIFResponse()}}
	service := newAntigravityGIFGatewayService(&antigravitySettingRepoStub{}, upstream)
	ginContext := antigravityGIFGinContext(t, "/antigravity/v1beta/models/gemini-2.5-flash:streamGenerateContent", body)

	result, err := service.ForwardGemini(
		context.Background(),
		ginContext,
		antigravityGIFTestAccount("gemini-2.5-flash", "gemini-2.5-flash"),
		"gemini-2.5-flash",
		"streamGenerateContent",
		true,
		body,
		false,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requestBodies, 1)
	require.NotContains(t, string(upstream.requestBodies[0]), "image/gif")
	require.Contains(t, string(upstream.requestBodies[0]), "image/png")
}

func TestAntigravityGatewayService_ForwardGemini_ModelFallbackRetryConvertsGIFBeforeUpstream(t *testing.T) {
	repository := newMockSettingRepo()
	repository.data[SettingKeyEnableModelFallback] = "true"
	repository.data[SettingKeyFallbackModelAntigravity] = "gemini-2.5-pro"
	body := serviceGIFRequestBody(t, serviceTestGIFBase64(t))
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{
		antigravityGIFErrorResponse(http.StatusNotFound, "model not found"),
		successfulAntigravityGIFResponse(),
	}}
	service := newAntigravityGIFGatewayService(repository, upstream)
	ginContext := antigravityGIFGinContext(t, "/antigravity/v1beta/models/gemini-missing:streamGenerateContent", body)

	result, err := service.ForwardGemini(
		context.Background(),
		ginContext,
		antigravityGIFTestAccount("gemini-missing", "gemini-missing-upstream"),
		"gemini-missing",
		"streamGenerateContent",
		true,
		body,
		false,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requestBodies, 2)
	for _, requestBody := range upstream.requestBodies {
		require.NotContains(t, string(requestBody), "image/gif")
		require.Contains(t, string(requestBody), "image/png")
	}
	require.Contains(t, string(upstream.requestBodies[1]), `"model":"gemini-2.5-pro"`)
}

func TestAntigravityGatewayService_ForwardGemini_SignatureRetryConvertsGIFBeforeUpstream(t *testing.T) {
	body := serviceGIFRequestBodyWithThoughtSignature(t, serviceTestGIFBase64(t))
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{
		antigravityGIFWrappedErrorResponse(http.StatusBadRequest, "Corrupted thought signature."),
		successfulAntigravityGIFResponse(),
	}}
	service := newAntigravityGIFGatewayService(&antigravitySettingRepoStub{}, upstream)
	ginContext := antigravityGIFGinContext(t, "/antigravity/v1beta/models/gemini-2.5-flash:streamGenerateContent", body)

	result, err := service.ForwardGemini(
		context.Background(),
		ginContext,
		antigravityGIFTestAccount("gemini-2.5-flash", "gemini-2.5-flash"),
		"gemini-2.5-flash",
		"streamGenerateContent",
		true,
		body,
		false,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requestBodies, 2)
	for _, requestBody := range upstream.requestBodies {
		require.NotContains(t, string(requestBody), "image/gif")
		require.Contains(t, string(requestBody), "image/png")
	}
	require.Contains(t, string(upstream.requestBodies[0]), `"thoughtSignature":"sig_bad"`)
	require.Contains(t, string(upstream.requestBodies[1]), `"thoughtSignature":"skip_thought_signature_validator"`)
	require.NotContains(t, string(upstream.requestBodies[1]), `"thoughtSignature":"sig_bad"`)
}

func TestAntigravityGatewayService_Forward_ConvertsGIFBeforeUpstream(t *testing.T) {
	body := antigravityGIFClaudeRequestBody(t, nil)
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{successfulAntigravityGIFResponse()}}
	service := newAntigravityGIFGatewayService(&antigravitySettingRepoStub{}, upstream)
	ginContext := antigravityGIFGinContext(t, "/v1/messages", body)

	result, err := service.Forward(
		context.Background(),
		ginContext,
		antigravityGIFTestAccount("claude-sonnet-4-5", "gemini-3-pro-high"),
		body,
		false,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requestBodies, 1)
	require.NotContains(t, string(upstream.requestBodies[0]), "image/gif")
	require.Contains(t, string(upstream.requestBodies[0]), "image/png")
}

func TestAntigravityGatewayService_Forward_ConvertsGIFOnClaudeRectifierRetries(t *testing.T) {
	tests := []struct {
		name         string
		requestPatch map[string]any
		firstError   string
	}{
		{
			name: "签名重试",
			requestPatch: map[string]any{
				"thinking": map[string]any{"type": "enabled", "budget_tokens": 1024},
			},
			firstError: "invalid thought_signature",
		},
		{
			name: "Budget 重试",
			requestPatch: map[string]any{
				"thinking": map[string]any{"type": "enabled", "budget_tokens": 1},
			},
			firstError: "thinking budget_tokens input should be greater than or equal to 1024",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := antigravityGIFClaudeRequestBody(t, test.requestPatch)
			upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{
				antigravityGIFErrorResponse(http.StatusBadRequest, test.firstError),
				successfulAntigravityGIFResponse(),
			}}
			service := newAntigravityGIFGatewayService(&antigravitySettingRepoStub{}, upstream)
			ginContext := antigravityGIFGinContext(t, "/v1/messages", body)

			result, err := service.Forward(
				context.Background(),
				ginContext,
				antigravityGIFTestAccount("claude-sonnet-4-5", "gemini-3-pro-high"),
				body,
				false,
			)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, upstream.requestBodies, 2)
			for _, requestBody := range upstream.requestBodies {
				require.NotContains(t, string(requestBody), "image/gif")
				require.Contains(t, string(requestBody), "image/png")
			}
		})
	}
}

func TestAntigravityGatewayService_Forward_UpstreamAccountPreservesGIFBody(t *testing.T) {
	body := antigravityGIFClaudeRequestBody(t, map[string]any{"stream": false})
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{successfulClaudeUpstreamGIFResponse()}}
	service := newAntigravityGIFGatewayService(&antigravitySettingRepoStub{}, upstream)
	ginContext := antigravityGIFGinContext(t, "/v1/messages", body)
	account := antigravityGIFTestAccount("claude-sonnet-4-5", "gemini-3-pro-high")
	account.Type = AccountTypeUpstream
	account.Credentials["base_url"] = "https://upstream.example"
	account.Credentials["api_key"] = "upstream-key"

	result, err := service.Forward(context.Background(), ginContext, account, body, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requestBodies, 1)
	require.Equal(t, body, upstream.requestBodies[0])
	require.Contains(t, string(upstream.requestBodies[0]), "image/gif")
	require.NotContains(t, string(upstream.requestBodies[0]), "image/png")
}

func TestAntigravityGatewayService_ForwardGemini_InvalidGIFStopsBeforeUpstream(t *testing.T) {
	body := serviceGIFRequestBody(t, "%%%")
	upstream := &queuedHTTPUpstreamStub{}
	service := newAntigravityGIFGatewayService(&antigravitySettingRepoStub{}, upstream)
	ginContext, writer := antigravityGIFGinContextWithRecorder(t, "/antigravity/v1beta/models/gemini-2.5-flash:streamGenerateContent", body)

	result, err := service.ForwardGemini(
		context.Background(),
		ginContext,
		antigravityGIFTestAccount("gemini-2.5-flash", "gemini-2.5-flash"),
		"gemini-2.5-flash",
		"streamGenerateContent",
		true,
		body,
		false,
	)

	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, writer.Code)
	require.Contains(t, writer.Body.String(), "Invalid GIF base64 data")
	require.Empty(t, upstream.requestBodies)
}

func TestAntigravityGatewayService_ForwardGemini_DisabledPreservesGIF(t *testing.T) {
	repository := &countingGIFSettingRepo{mockSettingRepo: newMockSettingRepo()}
	repository.data[SettingKeyAntigravityGIFCompatSettings] = `{"enabled":false,"max_frames_per_gif":8}`
	body := serviceGIFRequestBody(t, serviceTestGIFBase64(t))
	upstream := &queuedHTTPUpstreamStub{responses: []*http.Response{successfulAntigravityGIFResponse()}}
	service := newAntigravityGIFGatewayService(repository, upstream)
	ginContext := antigravityGIFGinContext(t, "/antigravity/v1beta/models/gemini-2.5-flash:streamGenerateContent", body)

	result, err := service.ForwardGemini(
		context.Background(),
		ginContext,
		antigravityGIFTestAccount("gemini-2.5-flash", "gemini-2.5-flash"),
		"gemini-2.5-flash",
		"streamGenerateContent",
		true,
		body,
		false,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requestBodies, 1)
	require.Contains(t, string(upstream.requestBodies[0]), "image/gif")
	require.NotContains(t, string(upstream.requestBodies[0]), "image/png")
}

func newAntigravityGIFGatewayService(repository SettingRepository, upstream HTTPUpstream) *AntigravityGatewayService {
	return &AntigravityGatewayService{
		settingService: NewSettingService(repository, &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}),
		tokenProvider:  &AntigravityTokenProvider{},
		httpUpstream:   upstream,
	}
}

func antigravityGIFTestAccount(requestedModel, mappedModel string) *Account {
	return &Account{
		ID:          901,
		Name:        "antigravity-gif-test",
		Platform:    PlatformAntigravity,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "token",
			"project_id":   "project-id",
			"model_mapping": map[string]any{
				requestedModel: mappedModel,
			},
		},
	}
}

func successfulAntigravityGIFResponse() *http.Response {
	body := []byte("data: {\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":1}}}\n\n")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func antigravityGIFErrorResponse(status int, message string) *http.Response {
	body, _ := json.Marshal(map[string]any{"error": map[string]any{"message": message}})
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func antigravityGIFWrappedErrorResponse(status int, message string) *http.Response {
	body, _ := json.Marshal(map[string]any{
		"response": map[string]any{
			"error": map[string]any{
				"code":    status,
				"message": message,
				"status":  "INVALID_ARGUMENT",
			},
		},
	})
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func successfulClaudeUpstreamGIFResponse() *http.Response {
	body := []byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"claude-sonnet-4-5","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func antigravityGIFClaudeRequestBody(t *testing.T, patch map[string]any) []byte {
	t.Helper()
	request := map[string]any{
		"model": "claude-sonnet-4-5",
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type": "image",
						"source": map[string]any{
							"type":       "base64",
							"media_type": "image/gif",
							"data":       serviceTestGIFBase64(t),
						},
					},
				},
			},
		},
		"max_tokens": 16,
		"stream":     true,
	}
	for key, value := range patch {
		request[key] = value
	}
	body, err := json.Marshal(request)
	require.NoError(t, err)
	return body
}

func serviceGIFRequestBodyWithThoughtSignature(t *testing.T, data string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"contents": []any{
			map[string]any{
				"role": "model",
				"parts": []any{
					map[string]any{
						"text":             "thinking",
						"thought":          true,
						"thoughtSignature": "sig_bad",
					},
				},
			},
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{
						"inlineData": map[string]any{
							"mimeType": "image/gif",
							"data":     data,
						},
					},
				},
			},
		},
	})
	require.NoError(t, err)
	return body
}

func antigravityGIFGinContext(t *testing.T, path string, body []byte) *gin.Context {
	t.Helper()
	context, _ := antigravityGIFGinContextWithRecorder(t, path, body)
	return context
}

func antigravityGIFGinContextWithRecorder(t *testing.T, path string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(writer)
	context.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	return context, writer
}
