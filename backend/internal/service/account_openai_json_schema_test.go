//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/stretchr/testify/require"
)

func TestAccountIsOpenAIJSONSchemaToJSONObjectEnabled(t *testing.T) {
	enabledExtra := map[string]any{openai_compat.ExtraKeyJSONSchemaToJSONObject: true}

	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{"nil account", nil, false},
		{"openai api key enabled", &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: enabledExtra}, true},
		{"openai api key disabled", &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{openai_compat.ExtraKeyJSONSchemaToJSONObject: false}}, false},
		{"openai api key invalid value", &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{openai_compat.ExtraKeyJSONSchemaToJSONObject: "true"}}, false},
		{"openai oauth ignored", &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: enabledExtra}, false},
		{"openai setup token ignored", &Account{Platform: PlatformOpenAI, Type: AccountTypeSetupToken, Extra: enabledExtra}, false},
		{"grok api key ignored", &Account{Platform: PlatformGrok, Type: AccountTypeAPIKey, Extra: enabledExtra}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.account.IsOpenAIJSONSchemaToJSONObjectEnabled())
		})
	}
}
