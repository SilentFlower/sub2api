package service

import "github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"

// shouldForwardGrokAnthropicMessagesViaRawChatCompletions 判断 Grok 账号是否显式要求
// 将 /v1/messages 入站转到 xAI /chat/completions。
func shouldForwardGrokAnthropicMessagesViaRawChatCompletions(account *Account) bool {
	if account == nil || account.Platform != PlatformGrok {
		return false
	}
	if account.Type != AccountTypeOAuth && account.Type != AccountTypeAPIKey {
		return false
	}
	mode, _ := account.Extra[openai_compat.ExtraKeyResponsesMode].(string)
	return openai_compat.NormalizeResponsesSupportMode(mode) == openai_compat.ResponsesSupportModeForceChatCompletions
}
