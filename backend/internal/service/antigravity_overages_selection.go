package service

import "context"

// canUseAntigravityCreditsOverages 判断当前请求是否可以通过 AI Credits 超量继续使用。
// 兼容两种状态：
// 1. 新状态：模型已经记录了明确的 credits 超量标记；
// 2. 旧状态：只有 model_rate_limits，没有持久化的超量标记。
// 只要账号开启了 allow_overages、credits 未耗尽，且当前模型仍处于限流窗口，
// 就允许继续走 enabledCreditTypes 链路，避免老数据在升级后被卡死。
func canUseAntigravityCreditsOverages(ctx context.Context, account *Account, requestedModel string) bool {
	if account == nil || account.Platform != PlatformAntigravity {
		return false
	}
	if !account.IsOveragesEnabled() || isAntigravityCreditsExhausted(account) {
		return false
	}
	if account.IsAntigravityCreditOveragesActive(ctx, requestedModel) {
		return true
	}
	return account.GetRateLimitRemainingTimeWithContext(ctx, requestedModel) > 0
}

// isAccountSchedulableForRequestedModel 统一封装“模型可调度”判断。
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

func isAntigravityCreditsExhausted(account *Account) bool {
	if account == nil {
		return false
	}
	if isCreditsExhausted(account.ID) {
		return true
	}
	return account.IsAntigravityCreditsExhausted()
}
