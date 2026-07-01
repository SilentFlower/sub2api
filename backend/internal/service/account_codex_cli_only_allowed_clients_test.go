package service

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccount_GetCodexCLIOnlyCustomUserAgentPrefixes(t *testing.T) {
	t.Run("OAuth 账号读取 []any 字符串列表", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only_custom_user_agent_prefixes": []any{"my-client/*"}},
		}
		require.Equal(t, []string{"my-client/*"}, account.GetCodexCLIOnlyCustomUserAgentPrefixes())
	})

	t.Run("OAuth 账号读取 []string 列表", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only_custom_user_agent_prefixes": []string{"my-client/*"}},
		}
		require.Equal(t, []string{"my-client/*"}, account.GetCodexCLIOnlyCustomUserAgentPrefixes())
	})

	t.Run("跳过非字符串与空白元素", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only_custom_user_agent_prefixes": []any{"my-client/*", 123, "", "  "}},
		}
		require.Equal(t, []string{"my-client/*"}, account.GetCodexCLIOnlyCustomUserAgentPrefixes())
	})

	t.Run("去重并限制数量和长度", func(t *testing.T) {
		values := make([]any, 0, codexCLIOnlyCustomUserAgentPrefixMaxCount+4)
		values = append(values, " My-Client/* ", "my-client/*")
		values = append(values, strings.Repeat("x", codexCLIOnlyCustomUserAgentPrefixMaxLength+1))
		for i := 0; i < codexCLIOnlyCustomUserAgentPrefixMaxCount+2; i++ {
			values = append(values, "client-"+strconv.Itoa(i)+"/*")
		}
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only_custom_user_agent_prefixes": values},
		}

		got := account.GetCodexCLIOnlyCustomUserAgentPrefixes()
		require.Len(t, got, codexCLIOnlyCustomUserAgentPrefixMaxCount)
		require.Equal(t, "My-Client/*", got[0])
		require.NotContains(t, got, "my-client/*")
		require.NotContains(t, got, strings.Repeat("x", codexCLIOnlyCustomUserAgentPrefixMaxLength+1))
	})

	t.Run("非 OAuth 账号返回空", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra:    map[string]any{"codex_cli_only_custom_user_agent_prefixes": []any{"my-client/*"}},
		}
		require.Empty(t, account.GetCodexCLIOnlyCustomUserAgentPrefixes())
	})

	t.Run("字段缺失返回空", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{},
		}
		require.Empty(t, account.GetCodexCLIOnlyCustomUserAgentPrefixes())
	})
}
