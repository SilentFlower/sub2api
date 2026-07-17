package service

import "github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"

// ShouldForwardAnthropicMessagesViaRawChatCompletions 判断 OpenAI 兼容
// /v1/messages 入站是否应直接走 /v1/chat/completions。
//
// @param account 当前转发账号。
// @return OpenAI API Key 能力或 Grok 显式设置要求 Chat 路由时返回 true。
func ShouldForwardAnthropicMessagesViaRawChatCompletions(account *Account) bool {
	if account == nil {
		return false
	}
	if account.Platform == PlatformOpenAI {
		return account.Type == AccountTypeAPIKey && !openai_compat.ShouldUseResponsesAPI(account.Extra)
	}
	return shouldForwardGrokAnthropicMessagesViaRawChatCompletions(account)
}
