//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/stretchr/testify/require"
)

func TestShouldForwardAnthropicMessagesViaRawChatCompletions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{name: "nil account", account: nil, want: false},
		{
			name:    "OpenAI API Key with supported Responses",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{openai_compat.ExtraKeyResponsesSupported: true}},
			want:    false,
		},
		{
			name:    "OpenAI API Key with unsupported Responses",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{openai_compat.ExtraKeyResponsesSupported: false}},
			want:    true,
		},
		{
			name:    "OpenAI API Key without probe result keeps Responses",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
			want:    false,
		},
		{
			name:    "OpenAI OAuth ignores API Key capability",
			account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{openai_compat.ExtraKeyResponsesSupported: false}},
			want:    false,
		},
		{
			name:    "Grok force chat",
			account: forceChatGrokMessagesFallbackAccount(AccountTypeOAuth),
			want:    true,
		},
		{
			name:    "Grok default Responses",
			account: &Account{Platform: PlatformGrok, Type: AccountTypeOAuth},
			want:    false,
		},
		{
			name:    "other platform",
			account: &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, ShouldForwardAnthropicMessagesViaRawChatCompletions(tt.account))
		})
	}
}
