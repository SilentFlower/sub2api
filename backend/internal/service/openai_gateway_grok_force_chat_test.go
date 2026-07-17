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
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func forceChatGrokMessagesFallbackAccount(accountType string) *Account {
	credentials := map[string]any{
		"base_url": xai.DefaultCLIBaseURL,
	}
	if accountType == AccountTypeAPIKey {
		credentials["api_key"] = "xai-api-key"
	} else {
		credentials["access_token"] = "grok-access-token"
	}
	return &Account{
		ID:          202,
		Name:        "grok-force-chat",
		Platform:    PlatformGrok,
		Type:        accountType,
		Concurrency: 1,
		Credentials: credentials,
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
		},
	}
}

func TestShouldForwardGrokAnthropicMessagesViaRawChatCompletions(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{
			name:    "missing mode stays responses",
			account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth},
			want:    false,
		},
		{
			name: "auto stays responses",
			account: &Account{
				Platform: PlatformGrok,
				Type:     AccountTypeOAuth,
				Extra: map[string]any{
					openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeAuto),
				},
			},
			want: false,
		},
		{
			name: "force responses stays responses",
			account: &Account{
				Platform: PlatformGrok,
				Type:     AccountTypeOAuth,
				Extra: map[string]any{
					openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceResponses),
				},
			},
			want: false,
		},
		{
			name:    "oauth force chat uses raw chat",
			account: forceChatGrokMessagesFallbackAccount(AccountTypeOAuth),
			want:    true,
		},
		{
			name:    "apikey force chat uses raw chat",
			account: forceChatGrokMessagesFallbackAccount(AccountTypeAPIKey),
			want:    true,
		},
		{
			name: "unsupported type ignores force chat",
			account: &Account{
				Platform: PlatformGrok,
				Type:     AccountTypeSetupToken,
				Extra: map[string]any{
					openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
				},
			},
			want: false,
		},
		{
			name:    "non grok account ignores force chat",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
			want:    false,
		},
		{
			name:    "nil account stays responses",
			account: nil,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldForwardGrokAnthropicMessagesViaRawChatCompletions(tt.account))
		})
	}
}

func TestForwardAsAnthropic_GrokOAuthForceChatCompletions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"grok","max_tokens":32,"output_config":{"effort":"max"},"messages":[{"role":"user","content":"hello"}],"stream":false}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_grok_messages","object":"chat.completion.chunk","model":"grok-4.3","choices":[{"index":0,"delta":{"role":"assistant","content":"ok"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_grok_messages","object":"chat.completion.chunk","model":"grok-4.3","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10,"prompt_tokens_details":{"cached_tokens":2}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":                   []string{"text/event-stream"},
			"Xai-Request-Id":                 []string{"xai-msg-chat"},
			"X-Ratelimit-Limit-Requests":     []string{"10"},
			"X-Ratelimit-Remaining-Requests": []string{"9"},
		},
		Body: io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	repo := &grokQuotaAccountRepo{
		mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
			accountsByID: map[int64]*Account{202: forceChatGrokMessagesFallbackAccount(AccountTypeOAuth)},
		},
	}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
		accountRepo:  repo,
	}

	result, err := svc.ForwardAsAnthropic(context.Background(), c, forceChatGrokMessagesFallbackAccount(AccountTypeOAuth), body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, xai.DefaultCLIBaseURL+"/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer grok-access-token", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "sub2api-grok/1.0", upstream.lastReq.Header.Get("User-Agent"))
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream_options.include_usage").Bool())
	require.Equal(t, "high", gjson.GetBytes(upstream.lastBody, "reasoning_effort").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
	require.Equal(t, "ok", gjson.Get(recorder.Body.String(), "content.0.text").String())
	require.Equal(t, "xai-msg-chat", result.RequestID)
	require.Equal(t, 7, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.Equal(t, 2, result.Usage.CacheReadInputTokens)
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "high", *result.ReasoningEffort)
	require.NotNil(t, repo.updates[202][grokQuotaSnapshotExtraKey])
}

func TestForwardAsAnthropic_GrokAPIKeyForceChatCompletions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"grok","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":true}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_grok_apikey","object":"chat.completion.chunk","model":"grok-4.3","choices":[{"index":0,"delta":{"role":"assistant","content":"ok"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_grok_apikey","object":"chat.completion.chunk","model":"grok-4.3","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "Xai-Request-Id": []string{"xai-msg-apikey"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.ForwardAsAnthropic(context.Background(), c, forceChatGrokMessagesFallbackAccount(AccountTypeAPIKey), body, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, xai.DefaultCLIBaseURL+"/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer xai-api-key", upstream.lastReq.Header.Get("Authorization"))
	require.True(t, result.Stream)
	require.Contains(t, recorder.Body.String(), "event: message_start")
	require.Contains(t, recorder.Body.String(), `"text":"ok"`)
	require.Equal(t, 5, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
}
