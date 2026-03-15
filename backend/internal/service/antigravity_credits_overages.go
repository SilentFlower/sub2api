package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// ============================================================
// AI Credits 超量请求（Overages）核心逻辑
//
// 当 Antigravity 账号的免费配额耗尽（429 QUOTA_EXHAUSTED）时，
// 如果账号开启了 allow_overages，则自动注入 enabledCreditTypes
// 使用 GOOGLE_ONE_AI credits 继续请求。
//
// 关键流程：
//   1. 预检查阶段（antigravityRetryLoop）：模型限流 + overages 启用 →
//      直接注入 credits 字段，跳过限流等待
//   2. 降级重试（handleSmartRetry）：首次 429 → 注入 credits 重试 →
//      成功则设限流标记（后续走预检查路径）；失败则标记 credits 耗尽
//   3. 调度过滤（isAccountSchedulableForRequestedModel）：
//      overages 账号即使限流也不被调度器过滤
// ============================================================

// creditsExhaustedCache 记录账号的 AI Credits 耗尽时间。
// 当 credits 重试失败时设置，避免后续请求重复尝试无效的 credits 重试。
// key: accountID (int64), value: time.Time (耗尽截止时间，与配额重置时间一致)
var creditsExhaustedCache sync.Map

// isCreditsExhausted 检查账号的 credits 是否已标记为耗尽
func isCreditsExhausted(accountID int64) bool {
	v, ok := creditsExhaustedCache.Load(accountID)
	if !ok {
		return false
	}
	until := v.(time.Time)
	if time.Now().After(until) {
		// 已过期，清除标记
		creditsExhaustedCache.Delete(accountID)
		return false
	}
	return true
}

// setCreditsExhausted 标记账号的 credits 已耗尽，直到指定时间
func setCreditsExhausted(accountID int64, until time.Time) {
	creditsExhaustedCache.Store(accountID, until)
}

// clearCreditsExhausted 清除账号的 credits 耗尽标记（如用户充值或重新开启 overages）
func clearCreditsExhausted(accountID int64) {
	creditsExhaustedCache.Delete(accountID)
}

// creditsExhaustedKeywords 用于判断上游响应是否明确表示 credits 余额不足
var creditsExhaustedKeywords = []string{
	"google_one_ai",
	"insufficient credit",
	"insufficient credits",
	"not enough credit",
	"not enough credits",
	"credit exhausted",
	"credits exhausted",
	"credit balance",
	"minimumcreditamountforusage",
	"minimum credit amount for usage",
	"minimum credit",
}

// shouldMarkCreditsExhausted 判断一次 credits 降级失败是否应标记为"credits 已耗尽"。
// 采用保守策略：只有在上游明确返回与 credits 相关的非瞬时错误时才打标，
// 避免把网络抖动、5xx、URL 级限流或普通模型限流误判成 credits 耗尽。
func shouldMarkCreditsExhausted(resp *http.Response, respBody []byte, reqErr error) bool {
	if reqErr != nil || resp == nil {
		return false
	}
	// 5xx / 超时属于瞬时错误，不标记
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusRequestTimeout {
		return false
	}
	// URL 级限流 / 智能重试信号不属于 credits 耗尽
	if isURLLevelRateLimit(respBody) {
		return false
	}
	if info := parseAntigravitySmartRetryInfo(respBody); info != nil {
		return false
	}

	bodyLower := strings.ToLower(string(respBody))
	if bodyLower == "" {
		return false
	}
	for _, keyword := range creditsExhaustedKeywords {
		if strings.Contains(bodyLower, keyword) {
			return true
		}
	}
	return false
}

// injectEnabledCreditTypes 在已序列化的 v1internal JSON body 中注入 enabledCreditTypes 字段。
// 用于 429 QUOTA_EXHAUSTED 时的 credits 降级重试。
func injectEnabledCreditTypes(body []byte) []byte {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil
	}
	obj["enabledCreditTypes"] = []string{"GOOGLE_ONE_AI"}
	result, err := json.Marshal(obj)
	if err != nil {
		return nil
	}
	return result
}

// resolveCreditsOveragesModelKey 解析 credits 成功后应写入的模型限流 key。
// 优先使用上游 429 响应中的模型名；若上游未返回，则回退到本次请求的最终模型 key，
// 确保“超量请求中”的状态能够稳定写回数据库并展示在后台状态列。
func resolveCreditsOveragesModelKey(ctx context.Context, account *Account, upstreamModelName, requestedModel string) string {
	modelKey := strings.TrimSpace(upstreamModelName)
	if modelKey != "" {
		return modelKey
	}
	if account == nil {
		return ""
	}
	modelKey = resolveFinalAntigravityModelKey(ctx, account, requestedModel)
	if strings.TrimSpace(modelKey) != "" {
		return modelKey
	}
	return resolveAntigravityModelKey(requestedModel)
}

// canUseAntigravityCreditsOverages 判断当前请求是否可以通过 AI Credits 超量继续使用。
// 命中模型限流时，只要 antigravity 账号开启了 allow_overages 且 credits 未耗尽，
// 就不应在调度阶段被提前过滤，而应继续走请求前注入 enabledCreditTypes 的链路。
func canUseAntigravityCreditsOverages(ctx context.Context, account *Account, requestedModel string) bool {
	if account == nil || account.Platform != PlatformAntigravity {
		return false
	}
	// 仅允许绕过模型级限流；账号级不可调度状态（error/disabled/过期/过载/账号级限流等）
	// 仍应按原逻辑拦截，避免 overages 把不可用账号重新放回调度器。
	if !account.IsSchedulable() {
		return false
	}
	if !account.IsOveragesEnabled() || isCreditsExhausted(account.ID) {
		return false
	}
	return account.GetRateLimitRemainingTimeWithContext(ctx, requestedModel) > 0
}

// isAccountSchedulableForRequestedModel 统一封装"模型可调度"判断。
// 普通账号沿用原有限流逻辑；仅对 antigravity 的 AI Credits 超量场景放行。
func isAccountSchedulableForRequestedModel(ctx context.Context, account *Account, requestedModel string) bool {
	if account == nil {
		return false
	}
	if account.IsSchedulableForModelWithContext(ctx, requestedModel) {
		return true
	}
	return canUseAntigravityCreditsOverages(ctx, account, requestedModel)
}

// creditsOveragesRetryResult credits 降级重试的结果
type creditsOveragesRetryResult struct {
	// handled 表示是否处理了 credits 降级（无论成功还是失败）
	handled bool
	// resp 如果 credits 重试成功，包含成功的响应
	resp *http.Response
}

// attemptCreditsOveragesRetry 尝试通过注入 enabledCreditTypes 进行 credits 降级重试。
//
// 前置条件：调用方已确认 resp.StatusCode == 429 && account.IsOveragesEnabled() && !isCreditsExhausted(account.ID)
//
// 返回值：
//   - handled=true, resp!=nil: credits 成功，调用方应直接使用该 resp
//   - handled=true, resp==nil: credits 失败，已做好标记，调用方继续原有逻辑
//   - handled=false: 无法注入 credits（如 body 解析失败），调用方继续原有逻辑
func (s *AntigravityGatewayService) attemptCreditsOveragesRetry(
	p antigravityRetryLoopParams,
	baseURL string,
	modelName string,
	waitDuration time.Duration,
	originalStatusCode int,
) *creditsOveragesRetryResult {

	creditsBody := injectEnabledCreditTypes(p.body)
	if creditsBody == nil {
		return &creditsOveragesRetryResult{handled: false}
	}
	modelKey := resolveCreditsOveragesModelKey(p.ctx, p.account, modelName, p.requestedModel)

	logger.LegacyPrintf("service.antigravity_gateway", "%s status=429 credit_overages_retry model=%s account=%d (injecting enabledCreditTypes)",
		p.prefix, modelKey, p.account.ID)

	creditsReq, err := antigravity.NewAPIRequestWithURL(p.ctx, baseURL, p.action, p.accessToken, creditsBody)
	if err != nil {
		logger.LegacyPrintf("service.antigravity_gateway", "%s credit_overages_failed model=%s account=%d build_request_err=%v",
			p.prefix, modelKey, p.account.ID, err)
		return &creditsOveragesRetryResult{handled: true}
	}

	creditsResp, err := p.httpUpstream.Do(creditsReq, p.proxyURL, p.account.ID, p.account.Concurrency)

	// 成功
	if err == nil && creditsResp != nil && creditsResp.StatusCode < 400 {
		logger.LegacyPrintf("service.antigravity_gateway", "%s status=%d credit_overages_success model=%s account=%d",
			p.prefix, creditsResp.StatusCode, modelKey, p.account.ID)

		clearCreditsExhausted(p.account.ID)

		// 设置模型限流标记，后续请求在预检查中直接注入 enabledCreditTypes，
		// 避免每次都先 429 再降级。配额重置后限流自然过期。
		resetAt := s.resolveCreditsRateLimitResetAt(waitDuration)
		if setModelRateLimitByModelName(p.ctx, p.accountRepo, p.account.ID, modelKey, p.prefix, originalStatusCode, resetAt, false) {
			s.updateAccountModelRateLimitInCache(p.ctx, p.account, modelKey, resetAt)
		}

		return &creditsOveragesRetryResult{handled: true, resp: creditsResp}
	}

	// 失败：读取响应体并判断是否应标记为 credits 耗尽
	s.handleCreditsRetryFailure(p.prefix, modelKey, p.account.ID, waitDuration, creditsResp, err)

	return &creditsOveragesRetryResult{handled: true}
}

// handleCreditsRetryFailure 处理 credits 重试失败，判断是否标记 credits 为耗尽
func (s *AntigravityGatewayService) handleCreditsRetryFailure(
	prefix string,
	modelName string,
	accountID int64,
	waitDuration time.Duration,
	creditsResp *http.Response,
	reqErr error,
) {
	var creditsRespBody []byte
	creditsStatusCode := 0
	if creditsResp != nil {
		creditsStatusCode = creditsResp.StatusCode
		if creditsResp.Body != nil {
			creditsRespBody, _ = io.ReadAll(io.LimitReader(creditsResp.Body, 64<<10))
			_ = creditsResp.Body.Close()
		}
	}

	if shouldMarkCreditsExhausted(creditsResp, creditsRespBody, reqErr) {
		exhaustedUntil := s.resolveCreditsRateLimitResetAt(waitDuration)
		setCreditsExhausted(accountID, exhaustedUntil)
		logger.LegacyPrintf("service.antigravity_gateway", "%s credit_overages_failed model=%s account=%d marked_exhausted=true status=%d exhausted_until=%v body=%s",
			prefix, modelName, accountID, creditsStatusCode, exhaustedUntil, truncateForLog(creditsRespBody, 200))
	} else {
		logger.LegacyPrintf("service.antigravity_gateway", "%s credit_overages_failed model=%s account=%d marked_exhausted=false status=%d err=%v body=%s",
			prefix, modelName, accountID, creditsStatusCode, reqErr, truncateForLog(creditsRespBody, 200))
	}
}

// resolveCreditsRateLimitResetAt 计算 credits 限流/耗尽的过期时间
func (s *AntigravityGatewayService) resolveCreditsRateLimitResetAt(waitDuration time.Duration) time.Time {
	if waitDuration <= 0 {
		waitDuration = antigravityDefaultRateLimitDuration
	}
	return time.Now().Add(waitDuration)
}
