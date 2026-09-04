package handler

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func resolveOpenAIMessagesSessionHash(gatewayService *service.OpenAIGatewayService, c *gin.Context, reqModel string, body []byte) (string, string) {
	if gatewayService == nil {
		return resolveOpenAIMessagesMetadataSession(c, "", "", reqModel, body)
	}
	// Anthropic metadata.user_id 是 Claude Code 会话内稳定的账号粘性信号。
	// 只有显式 session_id/conversation_id/prompt_cache_key 可以覆盖它；内容
	// fallback 放到最后，避免首条用户消息或工具定义变化导致同一会话漂移。
	sessionHash := gatewayService.GenerateExplicitSessionHash(c, body)
	promptCacheKey := gatewayService.ExtractSessionID(c, body)
	sessionHash, promptCacheKey = resolveOpenAIMessagesMetadataSession(c, sessionHash, promptCacheKey, reqModel, body)
	if sessionHash == "" {
		sessionHash = gatewayService.GenerateSessionHash(c, body)
	}
	return sessionHash, promptCacheKey
}

func resolveOpenAIMessagesMetadataSession(c *gin.Context, sessionHash, promptCacheKey, reqModel string, body []byte) (string, string) {
	// metadata.user_id 只作为账号粘性信号，上游缓存键继续由完整消息语义派生，
	// 避免固定 metadata 值压住后续轮次的缓存滚动。
	if promptCacheKey == "" {
		if claudeSessionID := service.ClaudeCodeSessionIDFromHeader(c); claudeSessionID != "" {
			return service.DeriveSessionHashFromSeed(claudeSessionID), promptCacheKey
		}
	}
	if sessionHash != "" {
		return sessionHash, promptCacheKey
	}
	if userID := strings.TrimSpace(gjson.GetBytes(body, "metadata.user_id").String()); userID != "" {
		sessionHash = service.DeriveSessionHashFromSeed(reqModel + "-" + userID)
	}
	return sessionHash, promptCacheKey
}
