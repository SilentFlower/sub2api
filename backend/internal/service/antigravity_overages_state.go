package service

import (
	"context"
	"time"
)

const (
	antigravityCreditsExhaustedUntilExtraKey   = "antigravity_credits_exhausted_until"
	antigravityCreditOveragesUntilExtraKeyBase = "antigravity_credit_overages_until__"
)

func antigravityCreditOveragesUntilExtraKey(modelName string) string {
	modelName = normalizeAntigravityModelName(modelName)
	if modelName == "" {
		return ""
	}
	return antigravityCreditOveragesUntilExtraKeyBase + modelName
}

// AntigravityCreditsExhaustedUntil 返回 AI Credits 耗尽状态的截止时间。
func (a *Account) AntigravityCreditsExhaustedUntil() *time.Time {
	if a == nil {
		return nil
	}
	until := a.getExtraTime(antigravityCreditsExhaustedUntilExtraKey)
	if until.IsZero() {
		return nil
	}
	return &until
}

// IsAntigravityCreditsExhausted 返回账号的 AI Credits 是否仍处于耗尽状态。
func (a *Account) IsAntigravityCreditsExhausted() bool {
	until := a.AntigravityCreditsExhaustedUntil()
	return until != nil && time.Now().Before(*until)
}

// AntigravityCreditOveragesUntil 返回指定模型的 credits 超量状态截止时间。
func (a *Account) AntigravityCreditOveragesUntil(ctx context.Context, requestedModel string) *time.Time {
	if a == nil || a.Platform != PlatformAntigravity {
		return nil
	}
	modelKey := resolveFinalAntigravityModelKey(ctx, a, requestedModel)
	if modelKey == "" {
		return nil
	}
	until := a.getExtraTime(antigravityCreditOveragesUntilExtraKey(modelKey))
	if until.IsZero() {
		return nil
	}
	return &until
}

// IsAntigravityCreditOveragesActive 返回指定模型是否处于 credits 超量状态。
func (a *Account) IsAntigravityCreditOveragesActive(ctx context.Context, requestedModel string) bool {
	until := a.AntigravityCreditOveragesUntil(ctx, requestedModel)
	return until != nil && time.Now().Before(*until)
}
