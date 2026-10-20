package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

// CyberSessionBlockStore 是 cyber 会话屏蔽表的存取接口。
// repository 层 gatewayCache 附带实现（类型断言探测接入，不改 GatewayCache
// 共享接口）；测试 stub 不实现时屏蔽能力自动降级关闭。
type CyberSessionBlockStore interface {
	SetCyberSessionBlocked(ctx context.Context, scopeKey string, keys []string, ttl time.Duration) error
	IsCyberSessionScopeActive(ctx context.Context, scopeKey string) (bool, error)
	FindCyberSessionBlocked(ctx context.Context, keys []string) (string, error)
}

// CyberSessionExplicitBlockKey 仅按认证 API Key 与明确会话请求头生成屏蔽键。
// 缓存提示、粘性标记和历史相似度均不能证明会话身份；共享 Key 的网关需为各会话提供独立 ID。
//
// @param apiKeyID 认证后的网关 API Key ID。
// @param c 包含原始会话请求头的请求上下文。
// @param _ 为保持调用兼容而保留的请求体，不参与会话身份判断。
// @return v2 会话哈希；身份缺失时返回空字符串。
func CyberSessionExplicitBlockKey(apiKeyID int64, c *gin.Context, _ []byte) string {
	if apiKeyID <= 0 || c == nil || c.Request == nil {
		return ""
	}
	for _, header := range []string{
		"session-id", "session_id", "conversation_id",
		openCodeSessionIDHeader, openCodeNativeSessionHeader, codeBuddyConversationHeader,
	} {
		if sessionID := strings.TrimSpace(c.GetHeader(header)); sessionID != "" {
			// 独立命名空间避免滚动升级时把旧缓存键封禁误认为明确会话封禁。
			raw := "cyber-explicit-session:v2|api_key=" + strconv.FormatInt(apiKeyID, 10) + "|session=" + sessionID
			sum := sha256.Sum256([]byte(raw))
			return hex.EncodeToString(sum[:])
		}
	}
	return ""
}

// cyberSessionBlockStore 探测 cache 是否具备屏蔽存储能力。
// 注意：若未来以装饰器包装 GatewayCache（如日志/指标装饰器），该装饰器必须同时实现
// CyberSessionBlockStore，否则会话屏蔽能力将静默降级关闭
// （编译断言 var _ service.CyberSessionBlockStore = (*gatewayCache)(nil) 只覆盖
// *gatewayCache 本体，无法覆盖其外层包装）。
func (s *OpenAIGatewayService) cyberSessionBlockStore() CyberSessionBlockStore {
	if s == nil || s.cache == nil {
		return nil
	}
	store, ok := s.cache.(CyberSessionBlockStore)
	if !ok {
		return nil
	}
	return store
}

// CyberSessionBlockRuntime 返回 (开关, TTL)。开关默认关。
// 委托给 SettingService.GetCyberSessionBlockRuntime，进程内缓存避免热路径 DB 往返。
func (s *OpenAIGatewayService) CyberSessionBlockRuntime(ctx context.Context) (bool, time.Duration) {
	if s == nil || s.settingService == nil {
		return false, time.Hour
	}
	return s.settingService.GetCyberSessionBlockRuntime(ctx)
}

// MarkCyberSessionBlocked 把会话写入屏蔽表（写入点：cyber 命中后）。
// 开关关闭、key 为空或存储不可用时静默跳过。
func (s *OpenAIGatewayService) MarkCyberSessionBlocked(ctx context.Context, scopeKey string, keys []string) {
	if s == nil || len(keys) == 0 {
		return
	}
	enabled, ttl := s.CyberSessionBlockRuntime(ctx)
	if !enabled {
		return
	}
	store := s.cyberSessionBlockStore()
	if store == nil {
		return
	}
	if err := store.SetCyberSessionBlocked(ctx, scopeKey, keys, ttl); err != nil {
		logger.LegacyPrintf("service.openai_gateway", "cyber session block write failed: err=%v", err)
	}
}

// FindCyberSessionBlockedForRequest 只查询与当前明确会话身份完全一致的封禁。
// 身份缺失或存储失败时交给上游审核，来源元数据与历史长度不参与封禁判断。
//
// @param ctx 查询上下文。
// @param apiKeyID 认证后的网关 API Key ID。
// @param c 包含原始会话请求头的请求上下文。
// @param body 为保持调用兼容而保留的请求体。
// @param _ 为保持调用兼容而保留的来源元数据。
// @return 命中的会话屏蔽键；未命中或无法读取时返回空字符串。
func (s *OpenAIGatewayService) FindCyberSessionBlockedForRequest(ctx context.Context, apiKeyID int64, c *gin.Context, body []byte, _ string, _ string) string {
	enabled, _ := s.CyberSessionBlockRuntime(ctx)
	if !enabled {
		return ""
	}
	key := CyberSessionExplicitBlockKey(apiKeyID, c, body)
	if key == "" {
		return ""
	}
	store := s.cyberSessionBlockStore()
	if store == nil {
		return ""
	}
	matched, err := store.FindCyberSessionBlocked(ctx, []string{key})
	if err != nil {
		logger.LegacyPrintf("service.openai_gateway", "cyber explicit session read failed: err=%v", err)
		return ""
	}
	return matched
}
