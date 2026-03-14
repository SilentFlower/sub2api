package service

import "context"

// canUseAntigravityCreditsOverages 判断当前请求是否可以通过 AI Credits 超量继续使用。
// 只有在模型已经进入 credits 超量状态，且账号未被标记为 credits 耗尽时，
// 才允许在调度阶段继续放行并走 enabledCreditTypes 链路。
func canUseAntigravityCreditsOverages(ctx context.Context, account *Account, requestedModel string) bool {
	if account == nil || account.Platform != PlatformAntigravity {
		return false
	}
	if !account.IsOveragesEnabled() || isAntigravityCreditsExhausted(account) {
		return false
	}
	return account.IsAntigravityCreditOveragesActive(ctx, requestedModel)
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
