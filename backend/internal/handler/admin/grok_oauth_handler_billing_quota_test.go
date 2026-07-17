//go:build unit

package admin

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGrokOAuthHandlerQueryBillingQuotaUsesIndependentService(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &grokQuotaHandlerAccountRepo{account: &service.Account{
		ID:          44,
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		},
	}}
	upstream := &grokQuotaHandlerUpstream{}
	billingQuotaService := service.NewGrokBillingQuotaService(repo, nil, service.NewGrokTokenProvider(repo, nil), upstream)
	handler := NewGrokOAuthHandler(nil, nil, nil, billingQuotaService, nil)

	router := gin.New()
	router.GET("/api/v1/admin/grok/accounts/:id/billing-quota", handler.QueryBillingQuota)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/grok/accounts/44/billing-quota", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"source":"grok_cli_billing_quota"`)
	require.Contains(t, recorder.Body.String(), `"snapshot":`)
	require.NotContains(t, recorder.Body.String(), "access-token")
	upstream.mu.Lock()
	requests := append([]*http.Request(nil), upstream.requests...)
	upstream.mu.Unlock()
	require.Len(t, requests, 2)
	require.Contains(t, repo.updates[44], "grok_billing_quota_snapshot")
	require.NotContains(t, repo.updates[44], "grok_billing_snapshot")
}

func TestGrokOAuthHandlerQueryBillingQuotaRedactsTransportError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &grokQuotaHandlerAccountRepo{account: &service.Account{
		ID:          45,
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		},
	}}
	upstream := &grokQuotaHandlerUpstream{err: errors.New("access_token=super-secret")}
	billingQuotaService := service.NewGrokBillingQuotaService(repo, nil, service.NewGrokTokenProvider(repo, nil), upstream)
	handler := NewGrokOAuthHandler(nil, nil, nil, billingQuotaService, nil)

	router := gin.New()
	router.GET("/api/v1/admin/grok/accounts/:id/billing-quota", handler.QueryBillingQuota)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/grok/accounts/45/billing-quota", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"reason":"GROK_BILLING_QUOTA_REQUEST_FAILED"`)
	require.Contains(t, recorder.Body.String(), `"message":"billing request failed"`)
	require.NotContains(t, recorder.Body.String(), "super-secret")
	require.NotContains(t, recorder.Body.String(), "access_token")
}
