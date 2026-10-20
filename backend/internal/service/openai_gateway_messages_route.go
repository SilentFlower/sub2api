package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
)

// resolveAnthropicUpstreamProtocol 在通用协议路由前应用 Grok 的显式路由约定。
// 探针结果不能覆盖 Grok 默认 Responses，只有管理员显式强制 Chat 时才改走 Chat。
func (s *OpenAIGatewayService) resolveAnthropicUpstreamProtocol(ctx context.Context, account *Account, model string) string {
	if account != nil && account.Platform == PlatformGrok {
		if shouldForwardGrokAnthropicMessagesViaRawChatCompletions(account) {
			return APIProtocolChatCompletions
		}
		return APIProtocolResponses
	}
	return s.resolveUpstreamProtocolFor(ctx, account, APIProtocolAnthropic, model)
}

// ShouldForwardAnthropicMessagesViaRawChatCompletions 判断 OpenAI 兼容
// /v1/messages 入站是否应直接走 /v1/chat/completions。
//
// @param account 当前转发账号。
// @return OpenAI API Key 能力、国产供应商 Chat 协议或 Grok 显式设置要求 Chat 路由时返回 true。
func ShouldForwardAnthropicMessagesViaRawChatCompletions(account *Account) bool {
	if account == nil {
		return false
	}
	if account.Platform == PlatformOpenAI {
		return account.Type == AccountTypeAPIKey && !openai_compat.ShouldUseResponsesAPI(account.Extra)
	}
	if account.IsCNProvider() {
		return account.Type == AccountTypeAPIKey && account.GetAPIProtocol() == APIProtocolChatCompletions
	}
	return shouldForwardGrokAnthropicMessagesViaRawChatCompletions(account)
}
