//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesShapedChatFixedProtocolPreservesReasoningHistory(t *testing.T) {
	for _, platform := range []string{PlatformOpenAI, PlatformDeepseek, PlatformOpenCodeGo} {
		t.Run(platform, func(t *testing.T) {
			body := deepSeekChatHistoryWithEncryptedReasoning()
			account := adaptiveProtocolTestAccount(platform, map[string]any{APIProtocolChatCompletions: "http://chat.example"})
			account.Credentials["api_protocol"] = APIProtocolChatCompletions
			account.Credentials["base_url"] = "http://chat.example"
			account.Extra = map[string]any{"openai_responses_mode": "force_chat_completions"}
			upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
			svc := &OpenAIGatewayService{
				cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream,
				cache: &reasoningHitCache{values: map[string]string{"item_enc1": "cached thinking"}},
			}
			_, err := svc.ForwardAsChatCompletions(context.Background(), adaptiveProtocolTestContext("/v1/chat/completions", body), account, body, "", "")
			require.Error(t, err)
			require.NotNil(t, upstream.lastReq)
			require.Contains(t, upstream.lastReq.URL.Path, "/chat/completions")
			require.False(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
			require.Equal(t, "cached thinking", gjson.GetBytes(upstream.lastBody, "messages.0.reasoning_content").String())
			require.Equal(t, "call_1", gjson.GetBytes(upstream.lastBody, "messages.0.tool_calls.0.id").String())
		})
	}
}

func TestResponsesShapedChatRejectsUnexecutableWebSearchBeforeTransport(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":"search","tools":[{"type":"web_search"}]}`)
	upstream := &httpUpstreamRecorder{err: errors.New("unexpected upstream request")}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	_, err := svc.ForwardAsChatCompletions(context.Background(), adaptiveProtocolTestContext("/v1/chat/completions", body), forceChatResponsesFallbackAccount(), body, "", "")
	require.ErrorContains(t, err, "cannot execute the requested web_search")
	require.Nil(t, upstream.lastReq)
}
