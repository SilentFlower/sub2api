package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccount_AnthropicCCHConfig(t *testing.T) {
	t.Run("默认值与固定模式回退", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"anthropic_cch_enabled": true,
			},
		}

		require.True(t, account.IsAnthropicCCHEnabled())
		require.Equal(t, anthropicCCHModeFixed, account.GetAnthropicCCHMode())
		require.Equal(t, anthropicCCHDefaultFixedVersion, account.GetAnthropicCCHFixedVersion())
		require.True(t, account.ShouldRewriteAnthropicCCHUserAgent())
	})

	t.Run("user_agent 模式不重写 UA", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeSetupToken,
			Extra: map[string]any{
				"anthropic_cch_enabled":            true,
				"anthropic_cch_mode":               anthropicCCHModeUserAgent,
				"anthropic_cch_rewrite_user_agent": true,
			},
		}

		require.Equal(t, anthropicCCHModeUserAgent, account.GetAnthropicCCHMode())
		require.False(t, account.ShouldRewriteAnthropicCCHUserAgent())
	})

	t.Run("非法配置对非支持账号无效", func(t *testing.T) {
		account := &Account{
			Platform: PlatformAnthropic,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"anthropic_cch_enabled":            true,
				"anthropic_cch_mode":               "invalid",
				"anthropic_cch_fixed_version":      "9.9.9",
				"anthropic_cch_rewrite_user_agent": false,
			},
		}

		require.False(t, account.IsAnthropicCCHEnabled())
		require.Empty(t, account.GetAnthropicCCHMode())
		require.Empty(t, account.GetAnthropicCCHFixedVersion())
		require.False(t, account.ShouldRewriteAnthropicCCHUserAgent())
	})
}
