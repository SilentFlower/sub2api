//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestApplyOpenAIJSONSchemaDowngrade(t *testing.T) {
	enabled := &Account{ID: 1001, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{openai_compat.ExtraKeyJSONSchemaToJSONObject: true}}

	t.Run("responses shape", func(t *testing.T) {
		body := []byte(`{"instructions":"keep","text":{"format":{"type":"json_schema","schema":{"type":"object"}}}}`)
		out, err := applyOpenAIJSONSchemaDowngrade(nil, enabled, body, openAIJSONSchemaRequestShapeResponses, openAIResponsesEndpoint)
		require.NoError(t, err)
		require.Equal(t, "json_object", gjson.GetBytes(out, "text.format.type").String())
		require.Contains(t, gjson.GetBytes(out, "instructions").String(), "keep")
	})

	t.Run("chat shape", func(t *testing.T) {
		body := []byte(`{"messages":[{"role":"user","content":"hi"}],"response_format":{"type":"json_schema","json_schema":{"schema":{"type":"object"}}}}`)
		out, err := applyOpenAIJSONSchemaDowngrade(nil, enabled, body, openAIJSONSchemaRequestShapeChat, openAIChatCompletionsEndpoint)
		require.NoError(t, err)
		require.Equal(t, "json_object", gjson.GetBytes(out, "response_format.type").String())
		require.Equal(t, "system", gjson.GetBytes(out, "messages.0.role").String())
	})

	t.Run("disabled account is unchanged", func(t *testing.T) {
		body := []byte(`{"text":{"format":{"type":"json_schema","schema":{}}}}`)
		account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
		out, err := applyOpenAIJSONSchemaDowngrade(nil, account, body, openAIJSONSchemaRequestShapeResponses, openAIResponsesEndpoint)
		require.NoError(t, err)
		require.Equal(t, body, out)
	})
}
