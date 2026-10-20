//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMessagesChatConfiguredFloorRespectsDisabledThinkingAndOllamaCap(t *testing.T) {
	cfg := rawChatCompletionsTestConfig()
	cfg.Gateway.MinOutputTokens = 512
	account := forceChatMessagesFallbackAccount()
	account.Credentials["base_url"] = "https://ollama.com"
	account.Credentials["model_mapping"] = map[string]any{"public-model": "deepseek-v4-pro"}
	account.Extra[OllamaCloudMaxTokensCapExtraKey] = 256
	body := []byte(`{"model":"public-model","max_tokens":16,"thinking":{"type":"disabled"},"messages":[{"role":"user","content":"hello"}]}`)
	upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	_, err := svc.ForwardAsAnthropic(context.Background(), adaptiveProtocolTestContext("/v1/messages", body), account, body, "", "")
	require.Error(t, err)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "deepseek-v4-pro", gjson.GetBytes(upstream.lastBody, "model").String())
	require.EqualValues(t, 256, gjson.GetBytes(upstream.lastBody, "max_completion_tokens").Int())
	require.Equal(t, "disabled", gjson.GetBytes(upstream.lastBody, "thinking.type").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "reasoning_effort").Exists())
	require.EqualValues(t, 16, gjson.GetBytes(body, "max_tokens").Int())
}
