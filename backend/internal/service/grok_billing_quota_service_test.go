//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type grokBillingQuotaTestUpstream struct {
	httpUpstreamRecorder
	weeklyStatus  int
	monthlyStatus int
	requests      []*http.Request
	proxyURLs     []string
	concurrencies []int
	requestErr    error
}

func (u *grokBillingQuotaTestUpstream) Do(req *http.Request, proxyURL string, _ int64, concurrency int) (*http.Response, error) {
	u.requests = append(u.requests, req)
	u.proxyURLs = append(u.proxyURLs, proxyURL)
	u.concurrencies = append(u.concurrencies, concurrency)
	if u.requestErr != nil {
		return nil, u.requestErr
	}
	status := u.monthlyStatus
	payload := `{"config":{"monthlyLimit":{"val":15000},"used":{"val":17000},"onDemandCap":{"val":10000},"billingPeriodEnd":"2026-08-01T00:00:00Z"}}`
	if req.URL.RawQuery == "format=credits" {
		status = u.weeklyStatus
		payload = `{"config":{"currentPeriod":{"type":"weekly","end":"2026-07-16T00:00:00Z"},"creditUsagePercent":25}}`
	}
	if status == 0 {
		status = http.StatusOK
	}
	if status >= http.StatusBadRequest {
		payload = `{"error":{"message":"secret upstream detail"}}`
	}
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(payload)),
	}, nil
}

func TestGrokBillingQuotaServiceStoresOnlyIndependentSnapshot(t *testing.T) {
	t.Parallel()

	account := &Account{
		ID:          71,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Concurrency: 0,
		Credentials: map[string]any{
			"access_token": "access-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &grokBillingQuotaTestUpstream{}
	service := NewGrokBillingQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream)

	result, err := service.QueryBillingQuota(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "grok_cli_billing_quota", result.Source)
	require.NotNil(t, result.Snapshot)
	require.False(t, result.Snapshot.Partial)
	require.EqualValues(t, 15000, *result.Snapshot.MonthlyUsedCents)
	require.EqualValues(t, 2000, *result.Snapshot.OnDemandUsedCents)
	require.Len(t, upstream.requests, 2)
	for index, request := range upstream.requests {
		require.Equal(t, "Bearer access-token", request.Header.Get("Authorization"))
		require.Equal(t, "xai-grok-cli", request.Header.Get("x-xai-token-auth"))
		require.Equal(t, 1, upstream.concurrencies[index])
	}
	require.Contains(t, repo.updates[account.ID], grokBillingQuotaSnapshotExtraKey)
	require.NotContains(t, repo.updates[account.ID], grokBillingExtraKey)
	require.NotContains(t, repo.updates[account.ID], grokQuotaSnapshotExtraKey)
}

func TestGrokBillingQuotaServiceKeepsPartialWindow(t *testing.T) {
	t.Parallel()

	account := &Account{
		ID:       72,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "access-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &grokBillingQuotaTestUpstream{monthlyStatus: http.StatusServiceUnavailable}
	service := NewGrokBillingQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream)

	result, err := service.QueryBillingQuota(context.Background(), account.ID)
	require.NoError(t, err)
	require.True(t, result.Snapshot.Partial)
	require.Equal(t, []string{"monthly"}, result.Snapshot.FailedWindows)
	require.NotNil(t, result.Snapshot.WeeklyUsedPercent)
	require.Nil(t, result.Snapshot.MonthlyLimitCents)
}

func TestGrokBillingQuotaServiceLoadsAccountProxy(t *testing.T) {
	t.Parallel()

	proxyID := int64(17)
	account := &Account{
		ID:          74,
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		ProxyID:     &proxyID,
		Concurrency: 3,
		Credentials: map[string]any{
			"access_token": "access-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	proxyRepo := &grokQuotaProxyRepo{proxies: map[int64]*Proxy{
		proxyID: {
			ID:       proxyID,
			Protocol: "http",
			Host:     "proxy.test",
			Port:     3128,
		},
	}}
	upstream := &grokBillingQuotaTestUpstream{}
	service := NewGrokBillingQuotaService(repo, proxyRepo, NewGrokTokenProvider(repo, nil), upstream)

	_, err := service.QueryBillingQuota(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, 1, proxyRepo.calls)
	require.Equal(t, []string{"http://proxy.test:3128", "http://proxy.test:3128"}, upstream.proxyURLs)
	require.Equal(t, []int{3, 3}, upstream.concurrencies)
}

func TestGrokBillingQuotaServiceReturnsStableErrorWhenBothWindowsFail(t *testing.T) {
	t.Parallel()

	account := &Account{
		ID:       75,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "access-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &grokBillingQuotaTestUpstream{
		weeklyStatus:  http.StatusTooManyRequests,
		monthlyStatus: http.StatusServiceUnavailable,
	}
	service := NewGrokBillingQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream)

	_, err := service.QueryBillingQuota(context.Background(), account.ID)
	require.Error(t, err)
	require.Equal(t, "GROK_BILLING_QUOTA_PARTS_FAILED", infraerrors.Reason(err))
	require.Equal(t, "weekly and monthly billing requests failed", infraerrors.Message(err))
	require.Empty(t, repo.updates)
}

func TestGrokBillingQuotaServiceDoesNotExposeTransportError(t *testing.T) {
	t.Parallel()

	account := &Account{
		ID:       76,
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "access-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &grokBillingQuotaTestUpstream{requestErr: errors.New("access_token=super-secret")}
	service := NewGrokBillingQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream)

	_, err := service.QueryBillingQuota(context.Background(), account.ID)
	require.Error(t, err)
	require.Equal(t, "GROK_BILLING_QUOTA_REQUEST_FAILED", infraerrors.Reason(err))
	require.Equal(t, "billing request failed", infraerrors.Message(err))
	require.NotContains(t, err.Error(), "super-secret")
}

func TestGrokBillingQuotaServiceRejectsNonGrokAccountBeforeUpstream(t *testing.T) {
	t.Parallel()

	account := &Account{ID: 73, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	repo := &grokQuotaAccountRepo{mockAccountRepoForPlatform: &mockAccountRepoForPlatform{
		accountsByID: map[int64]*Account{account.ID: account},
	}}
	upstream := &grokBillingQuotaTestUpstream{}
	service := NewGrokBillingQuotaService(repo, nil, NewGrokTokenProvider(repo, nil), upstream)

	_, err := service.QueryBillingQuota(context.Background(), account.ID)
	require.Error(t, err)
	require.Equal(t, "GROK_BILLING_QUOTA_INVALID_PLATFORM", infraerrors.Reason(err))
	require.Empty(t, upstream.requests)
}
