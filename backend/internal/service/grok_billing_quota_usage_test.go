//go:build unit

package service

import (
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai/billingquota"
	"github.com/stretchr/testify/require"
)

func TestGrokQuotaFetcherAttachesIndependentBillingQuotaSnapshot(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	tests := []struct {
		name      string
		updatedAt string
		wantStale bool
	}{
		{name: "fresh", updatedAt: now.Add(-time.Minute).Format(time.RFC3339), wantStale: false},
		{name: "stale", updatedAt: now.Add(-31 * time.Minute).Format(time.RFC3339), wantStale: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			account := &Account{
				Platform: PlatformGrok,
				Type:     AccountTypeOAuth,
				Extra: map[string]any{
					grokBillingQuotaSnapshotExtraKey: &billingquota.Snapshot{
						MonthlyLimitCents: grokInt64PtrForTest(15_000),
						PlanLabel:         "supergrok",
						UpdatedAt:         test.updatedAt,
					},
				},
			}

			usage := NewGrokQuotaFetcher().BuildUsageInfo(account)
			require.NotNil(t, usage.GrokBillingQuota)
			require.Equal(t, "supergrok", usage.GrokBillingQuota.PlanLabel)
			require.Equal(t, test.wantStale, usage.GrokBillingQuota.Stale)
			require.Nil(t, usage.GrokBilling)
		})
	}
}

func TestGrokQuotaFetcherDoesNotProjectMainBillingPlanIntoSubscriptionTier(t *testing.T) {
	t.Parallel()

	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"subscription_tier":  " FREE ",
			"entitlement_status": " active ",
		},
		Extra: map[string]any{
			grokBillingExtraKey: &xai.BillingSummary{
				PeriodType: "weekly",
				Plan:       "SuperGrok Heavy",
				StatusCode: http.StatusOK,
				UpdatedAt:  "2030-01-01T00:00:00Z",
			},
		},
	}

	usage := NewGrokQuotaFetcher().BuildUsageInfo(account)

	require.NotNil(t, usage.GrokBilling)
	require.Equal(t, "FREE", usage.SubscriptionTier)
	require.Equal(t, "FREE", usage.SubscriptionTierRaw)
	require.Equal(t, "active", usage.GrokEntitlementStatus)
}
