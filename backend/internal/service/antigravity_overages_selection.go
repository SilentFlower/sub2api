package service

import "context"

// canUseAntigravityCreditsOverages 判断当前请求是否可以通过 AI Credits 超量继续使用。
// 命中模型限流时，只要 antigravity 账号开启了 allow_overages 且 credits 未耗尽，
// 就不应在调度阶段被提前过滤，而应继续走请求前注入 enabledCreditTypes 的链路。
func canUseAntigravityCreditsOverages(ctx context.Context, account *Account, requestedModel string) bool {
	if account == nil || account.Platform != PlatformAntigravity {
		return false
	}
	if !account.IsOveragesEnabled() || isCreditsExhausted(account.ID) {
		return false
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
