//go:build unit

package xai

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildBillingSnapshotMergesWeeklyAndMonthly(t *testing.T) {
	t.Parallel()

	weeklyPayload, err := ParseBillingPayload([]byte(`{
		"config": {
			"currentPeriod": {
				"type": "weekly",
				"start": "2026-07-06T00:00:00Z",
				"end": "2026-07-13T00:00:00Z"
			},
			"creditUsagePercent": "25.5",
			"productUsage": [
				{"product": "grok-code", "usagePercent": "40"}
			]
		}
	}`))
	require.NoError(t, err)
	monthlyPayload, err := ParseBillingPayload([]byte(`{
		"config": {
			"monthlyLimit": {"val": "15000"},
			"used": {"val": "5000"},
			"onDemandCap": {"val": "10000"},
			"billingPeriodStart": "2026-07-01T00:00:00Z",
			"billingPeriodEnd": "2026-08-01T00:00:00Z"
		}
	}`))
	require.NoError(t, err)

	now := time.Date(2026, 7, 9, 8, 0, 0, 0, time.UTC)
	snapshot := BuildBillingSnapshot(weeklyPayload, monthlyPayload, now)
	require.NotNil(t, snapshot)
	require.Equal(t, "weekly", snapshot.PeriodType)
	require.NotNil(t, snapshot.WeeklyUsedPercent)
	require.InDelta(t, 25.5, *snapshot.WeeklyUsedPercent, 0.001)
	require.Equal(t, "2026-07-13T00:00:00Z", snapshot.WeeklyResetAt)
	require.Len(t, snapshot.ProductUsage, 1)
	require.Equal(t, "grok-code", snapshot.ProductUsage[0].Product)
	require.NotNil(t, snapshot.ProductUsage[0].UsagePercent)
	require.InDelta(t, 40, *snapshot.ProductUsage[0].UsagePercent, 0.001)
	require.NotNil(t, snapshot.MonthlyLimitCents)
	require.EqualValues(t, 15000, *snapshot.MonthlyLimitCents)
	require.NotNil(t, snapshot.MonthlyUsedCents)
	require.EqualValues(t, 5000, *snapshot.MonthlyUsedCents)
	require.NotNil(t, snapshot.MonthlyRemainingCents)
	require.EqualValues(t, 10000, *snapshot.MonthlyRemainingCents)
	require.NotNil(t, snapshot.MonthlyUsedPercent)
	require.InDelta(t, 33.333, *snapshot.MonthlyUsedPercent, 0.01)
	require.Equal(t, "supergrok", snapshot.PlanLabel)
	require.Equal(t, "2026-07-09T08:00:00Z", snapshot.UpdatedAt)
}

func TestBuildBillingSnapshotDerivesOnDemandUsed(t *testing.T) {
	t.Parallel()

	monthlyPayload, err := ParseBillingPayload([]byte(`{
		"config": {
			"monthly_limit": {"val": 15000},
			"used": {"val": 17000},
			"on_demand_cap": {"val": 10000},
			"billing_period_end": "2026-08-01T00:00:00Z"
		}
	}`))
	require.NoError(t, err)

	snapshot := BuildBillingSnapshot(nil, monthlyPayload, time.Date(2026, 7, 9, 8, 0, 0, 0, time.UTC))
	require.NotNil(t, snapshot)
	require.NotNil(t, snapshot.MonthlyUsedCents)
	require.EqualValues(t, 15000, *snapshot.MonthlyUsedCents)
	require.NotNil(t, snapshot.MonthlyRemainingCents)
	require.EqualValues(t, 0, *snapshot.MonthlyRemainingCents)
	require.NotNil(t, snapshot.OnDemandUsedCents)
	require.EqualValues(t, 2000, *snapshot.OnDemandUsedCents)
	require.NotNil(t, snapshot.OnDemandRemainingCents)
	require.EqualValues(t, 8000, *snapshot.OnDemandRemainingCents)
	require.NotNil(t, snapshot.OnDemandUsedPercent)
	require.InDelta(t, 20, *snapshot.OnDemandUsedPercent, 0.001)
}

func TestBuildBillingSnapshotReturnsNilForEmptyConfig(t *testing.T) {
	t.Parallel()

	payload, err := ParseBillingPayload([]byte(`{"config":{}}`))
	require.NoError(t, err)

	require.Nil(t, BuildBillingSnapshot(payload, nil, time.Now()))
}
