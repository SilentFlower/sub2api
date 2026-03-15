//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestCanUseAntigravityCreditsOverages_仅绕过模型级限流
// 验证 overages 只允许绕过模型级限流，不会绕过账号级不可调度状态。
func TestCanUseAntigravityCreditsOverages_仅绕过模型级限流(t *testing.T) {
	modelResetAt := time.Now().Add(10 * time.Minute).Format(time.RFC3339)
	accountRateLimitResetAt := time.Now().Add(5 * time.Minute)
	overloadUntil := time.Now().Add(3 * time.Minute)
	expiredAt := time.Now().Add(-1 * time.Minute)

	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{
			name: "账号正常且开启 overages 时允许继续调度",
			account: &Account{
				ID:          1,
				Platform:    PlatformAntigravity,
				Status:      StatusActive,
				Schedulable: true,
				Extra: map[string]any{
					"allow_overages": true,
					"model_rate_limits": map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": modelResetAt,
						},
					},
				},
			},
			want: true,
		},
		{
			name: "账号级限流时不能通过 overages 放行",
			account: &Account{
				ID:               2,
				Platform:         PlatformAntigravity,
				Status:           StatusActive,
				Schedulable:      true,
				RateLimitResetAt: &accountRateLimitResetAt,
				Extra: map[string]any{
					"allow_overages": true,
					"model_rate_limits": map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": modelResetAt,
						},
					},
				},
			},
			want: false,
		},
		{
			name: "过载时不能通过 overages 放行",
			account: &Account{
				ID:            3,
				Platform:      PlatformAntigravity,
				Status:        StatusActive,
				Schedulable:   true,
				OverloadUntil: &overloadUntil,
				Extra: map[string]any{
					"allow_overages": true,
					"model_rate_limits": map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": modelResetAt,
						},
					},
				},
			},
			want: false,
		},
		{
			name: "过期账号不能通过 overages 放行",
			account: &Account{
				ID:                 4,
				Platform:           PlatformAntigravity,
				Status:             StatusActive,
				Schedulable:        true,
				AutoPauseOnExpired: true,
				ExpiresAt:          &expiredAt,
				Extra: map[string]any{
					"allow_overages": true,
					"model_rate_limits": map[string]any{
						"claude-sonnet-4-5": map[string]any{
							"rate_limit_reset_at": modelResetAt,
						},
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, canUseAntigravityCreditsOverages(context.Background(), tt.account, "claude-sonnet-4-5"))
		})
	}
}

// TestIsAccountSchedulableForRequestedModel_Overages不绕过账号级不可调度
// 验证统一调度判断在 overages 场景下仍会尊重账号级可调度状态。
func TestIsAccountSchedulableForRequestedModel_Overages不绕过账号级不可调度(t *testing.T) {
	modelResetAt := time.Now().Add(10 * time.Minute).Format(time.RFC3339)
	accountRateLimitResetAt := time.Now().Add(5 * time.Minute)

	account := &Account{
		ID:               10,
		Platform:         PlatformAntigravity,
		Status:           StatusActive,
		Schedulable:      true,
		RateLimitResetAt: &accountRateLimitResetAt,
		Extra: map[string]any{
			"allow_overages": true,
			"model_rate_limits": map[string]any{
				"gemini-2.5-flash": map[string]any{
					"rate_limit_reset_at": modelResetAt,
				},
			},
		},
	}

	require.False(t, isAccountSchedulableForRequestedModel(context.Background(), account, "gemini-2.5-flash"))
}

// TestResolveCreditsOveragesModelKey_优先上游模型名否则回退请求模型
// 验证 credits 成功回写状态时，缺失上游模型名也能稳定写回最终模型 key。
func TestResolveCreditsOveragesModelKey_优先上游模型名否则回退请求模型(t *testing.T) {
	account := &Account{
		Platform: PlatformAntigravity,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"claude-opus-4-6": "claude-sonnet-4-5",
			},
		},
	}

	t.Run("优先使用上游模型名", func(t *testing.T) {
		got := resolveCreditsOveragesModelKey(context.Background(), account, "gemini-2.5-pro", "claude-opus-4-6")
		require.Equal(t, "gemini-2.5-pro", got)
	})

	t.Run("上游缺失时回退最终映射模型", func(t *testing.T) {
		got := resolveCreditsOveragesModelKey(context.Background(), account, "", "claude-opus-4-6")
		require.Equal(t, "claude-sonnet-4-5", got)
	})
}
