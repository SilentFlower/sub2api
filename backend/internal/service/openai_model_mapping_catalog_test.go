package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestProjectOAuthPassthroughAliasesUseFinalModelMetadata(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Extra:       map[string]any{"openai_passthrough": true},
		Credentials: map[string]any{"model_mapping": map[string]any{"sol": "gpt-5.6", "astra": "gpt-6"}},
	}
	body := []byte(`{"models":[{"slug":"gpt-5.6-sol","source":"sol-metadata"},{"slug":"gpt-6-astra","source":"astra-metadata"}]}`)
	projected, err := projectAccountModelsBody(body, account, nil, true)
	require.NoError(t, err)
	entries := gjson.GetBytes(projected, "models").Array()
	require.Len(t, entries, 4)
	byID := make(map[string]string, len(entries))
	for _, entry := range entries {
		byID[entry.Get("slug").String()] = entry.Get("source").String()
	}
	require.Equal(t, map[string]string{
		"sol": "sol-metadata", "astra": "astra-metadata",
		"gpt-5.6-sol": "sol-metadata", "gpt-6-astra": "astra-metadata",
	}, byID)
	require.JSONEq(t, `{"models":[{"slug":"gpt-5.6-sol","source":"sol-metadata"},{"slug":"gpt-6-astra","source":"astra-metadata"}]}`, string(body))
}

func TestProjectAPIKeyPassthroughChatFallbackPreservesNativeAndMappedModels(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Extra:       map[string]any{"openai_passthrough": true, "openai_responses_mode": "force_chat_completions"},
		Credentials: map[string]any{"model_mapping": map[string]any{"public": "target"}},
	}
	projected, err := projectAccountModelsBody([]byte(`{"data":[{"id":"target","owner":"upstream"},{"id":"other"}]}`), account, nil, false)
	require.NoError(t, err)
	entries := gjson.GetBytes(projected, "data").Array()
	require.Len(t, entries, 3)
	require.Equal(t, "target", entries[0].Get("id").String())
	require.Equal(t, "other", entries[1].Get("id").String())
	require.Equal(t, "public", entries[2].Get("id").String())
	require.Equal(t, "upstream", entries[2].Get("owner").String())
}
