//go:build unit

package billingquota

import (
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

func TestBuildSnapshotMergesWeeklyAndMonthly(t *testing.T) {
	t.Parallel()

	weeklyPayload, err := ParsePayload([]byte(`{
		"config": {
			"currentPeriod": {"type": "weekly", "start": "2026-07-06T00:00:00Z", "end": "2026-07-13T00:00:00Z"},
			"creditUsagePercent": "25.5",
			"productUsage": [{"product": "grok-code", "usagePercent": "40"}]
		}
	}`))
	require.NoError(t, err)
	monthlyPayload, err := ParsePayload([]byte(`{
		"config": {
			"monthlyLimit": {"val": "15000"},
			"used": {"val": "5000"},
			"onDemandCap": {"val": "10000"},
			"billingPeriodStart": "2026-07-01T00:00:00Z",
			"billingPeriodEnd": "2026-08-01T00:00:00Z"
		}
	}`))
	require.NoError(t, err)

	snapshot := BuildSnapshot(weeklyPayload, monthlyPayload, time.Date(2026, 7, 9, 8, 0, 0, 0, time.UTC))
	require.NotNil(t, snapshot)
	require.Equal(t, "weekly", snapshot.PeriodType)
	require.InDelta(t, 25.5, *snapshot.WeeklyUsedPercent, 0.001)
	require.Equal(t, "2026-07-13T00:00:00Z", snapshot.WeeklyResetAt)
	require.Len(t, snapshot.ProductUsage, 1)
	require.Equal(t, "grok-code", snapshot.ProductUsage[0].Product)
	require.InDelta(t, 40, *snapshot.ProductUsage[0].UsagePercent, 0.001)
	require.EqualValues(t, 15000, *snapshot.MonthlyLimitCents)
	require.EqualValues(t, 5000, *snapshot.MonthlyUsedCents)
	require.EqualValues(t, 10000, *snapshot.MonthlyRemainingCents)
	require.InDelta(t, 33.333, *snapshot.MonthlyUsedPercent, 0.01)
	require.Equal(t, "supergrok", snapshot.PlanLabel)
	require.Equal(t, "2026-07-09T08:00:00Z", snapshot.UpdatedAt)
}

func TestBuildSnapshotSupportsSnakeCaseAndDerivesOnDemandUsed(t *testing.T) {
	t.Parallel()

	monthlyPayload, err := ParsePayload([]byte(`{
		"config": {
			"monthly_limit": {"val": 15000},
			"used": {"val": 17000},
			"on_demand_cap": {"val": 10000},
			"billing_period_end": "2026-08-01T00:00:00Z"
		}
	}`))
	require.NoError(t, err)

	snapshot := BuildSnapshot(nil, monthlyPayload, time.Date(2026, 7, 9, 8, 0, 0, 0, time.UTC))
	require.NotNil(t, snapshot)
	require.EqualValues(t, 15000, *snapshot.MonthlyUsedCents)
	require.EqualValues(t, 0, *snapshot.MonthlyRemainingCents)
	require.EqualValues(t, 2000, *snapshot.OnDemandUsedCents)
	require.EqualValues(t, 8000, *snapshot.OnDemandRemainingCents)
	require.InDelta(t, 20, *snapshot.OnDemandUsedPercent, 0.001)
}

func TestBuildSnapshotSkipsProductUsageWithoutName(t *testing.T) {
	t.Parallel()

	weeklyPayload, err := ParsePayload([]byte(`{
		"config": {
			"productUsage": [
				{"usagePercent": 10},
				{"product": "grok-code", "usagePercent": 20}
			]
		}
	}`))
	require.NoError(t, err)

	snapshot := BuildSnapshot(weeklyPayload, nil, time.Now())
	require.NotNil(t, snapshot)
	require.Len(t, snapshot.ProductUsage, 1)
	require.Equal(t, "grok-code", snapshot.ProductUsage[0].Product)
	require.InDelta(t, 20, *snapshot.ProductUsage[0].UsagePercent, 0.001)
}

func TestBuildSnapshotReturnsNilForEmptyConfig(t *testing.T) {
	t.Parallel()

	payload, err := ParsePayload([]byte(`{"config":{}}`))
	require.NoError(t, err)
	require.Nil(t, BuildSnapshot(payload, nil, time.Now()))
}

func TestParsePayloadRejectsEmptyAndInvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := ParsePayload(nil)
	require.Error(t, err)
	_, err = ParsePayload([]byte(`{"config":`))
	require.Error(t, err)
}

func TestBuildURLAndApplyHeaders(t *testing.T) {
	t.Parallel()

	weeklyURL, err := BuildURL(xai.DefaultCLIBaseURL, true)
	require.NoError(t, err)
	require.Equal(t, xai.DefaultCLIBaseURL+"/billing?format=credits", weeklyURL)
	monthlyURL, err := BuildURL(xai.DefaultCLIBaseURL, false)
	require.NoError(t, err)
	require.Equal(t, xai.DefaultCLIBaseURL+"/billing", monthlyURL)

	req, err := http.NewRequest(http.MethodGet, weeklyURL, nil)
	require.NoError(t, err)
	ApplyHeaders(req, "secret-token")
	require.Equal(t, "Bearer secret-token", req.Header.Get("Authorization"))
	require.Equal(t, "xai-grok-cli", req.Header.Get("x-xai-token-auth"))
	require.NotEmpty(t, req.Header.Get("x-grok-client-version"))
	require.NotEmpty(t, req.Header.Get("User-Agent"))
}
