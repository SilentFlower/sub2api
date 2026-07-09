//go:build unit

package admin

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type grokQuotaHandlerAccountRepo struct {
	service.AccountRepository
	account *service.Account
	updates map[int64]map[string]any
}

func (r *grokQuotaHandlerAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	if r.account != nil && r.account.ID == id {
		return r.account, nil
	}
	return nil, service.ErrAccountNotFound
}

func (r *grokQuotaHandlerAccountRepo) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	if r.updates == nil {
		r.updates = make(map[int64]map[string]any)
	}
	r.updates[id] = updates
	return nil
}

type grokQuotaHandlerUpstream struct {
	resp      *http.Response
	responses []*http.Response
	lastReq   *http.Request
	lastBody  []byte
	calls     int
}

func (u *grokQuotaHandlerUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls++
	u.lastReq = req
	if req.Body != nil {
		u.lastBody, _ = io.ReadAll(req.Body)
	}
	if len(u.responses) > 0 {
		resp := u.responses[0]
		u.responses = u.responses[1:]
		return resp, nil
	}
	return u.resp, nil
}

func (u *grokQuotaHandlerUpstream) DoWithTLS(
	req *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestGrokOAuthHandlerQueryQuotaProbesUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &grokQuotaHandlerAccountRepo{account: &service.Account{
		ID:          42,
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "access-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}}
	upstream := &grokQuotaHandlerUpstream{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"X-Ratelimit-Limit-Requests":     []string{"10"},
			"X-Ratelimit-Remaining-Requests": []string{"8"},
		},
		Body: io.NopCloser(strings.NewReader(`{"id":"resp_probe"}`)),
	}}
	quotaService := service.NewGrokQuotaService(repo, nil, service.NewGrokTokenProvider(repo, nil), upstream)
	handler := NewGrokOAuthHandler(nil, nil, quotaService)

	router := gin.New()
	router.GET("/api/v1/admin/grok/accounts/:id/quota", handler.QueryQuota)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/grok/accounts/42/quota", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"source":"active_probe"`)
	require.Contains(t, rec.Body.String(), `"headers_observed":true`)
	require.NotContains(t, rec.Body.String(), "access-token")
	require.Equal(t, xai.DefaultBaseURL+"/responses", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer access-token", upstream.lastReq.Header.Get("Authorization"))
	require.Contains(t, string(upstream.lastBody), `"store":false`)
	require.NotNil(t, repo.updates[42])
}

func TestGrokOAuthHandlerQueryBillingQuotaRefreshesSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &grokQuotaHandlerAccountRepo{account: &service.Account{
		ID:          44,
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "access-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}}
	upstream := &grokQuotaHandlerUpstream{responses: []*http.Response{
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"config": {
					"currentPeriod": {"type": "weekly", "end": "2026-07-13T00:00:00Z"},
					"creditUsagePercent": 20
				}
			}`)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"config": {
					"monthlyLimit": {"val": 15000},
					"used": {"val": 5000},
					"billingPeriodEnd": "2026-08-01T00:00:00Z"
				}
			}`)),
		},
	}}
	quotaService := service.NewGrokQuotaService(repo, nil, service.NewGrokTokenProvider(repo, nil), upstream)
	handler := NewGrokOAuthHandler(nil, nil, quotaService)

	router := gin.New()
	router.GET("/api/v1/admin/grok/accounts/:id/billing-quota", handler.QueryBillingQuota)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/grok/accounts/44/billing-quota", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"source":"grok_cli_billing"`)
	require.Contains(t, rec.Body.String(), `"monthly_limit_cents":15000`)
	require.NotContains(t, rec.Body.String(), "access-token")
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/billing", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer access-token", upstream.lastReq.Header.Get("Authorization"))
	require.NotNil(t, repo.updates[44])
}

func TestGrokOAuthHandlerQueryBillingQuotaRejectsNonGrokAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &grokQuotaHandlerAccountRepo{account: &service.Account{
		ID:       45,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "access-token",
			"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}}
	upstream := &grokQuotaHandlerUpstream{}
	quotaService := service.NewGrokQuotaService(repo, nil, service.NewGrokTokenProvider(repo, nil), upstream)
	handler := NewGrokOAuthHandler(nil, nil, quotaService)

	router := gin.New()
	router.GET("/api/v1/admin/grok/accounts/:id/billing-quota", handler.QueryBillingQuota)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/grok/accounts/45/billing-quota", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), `"reason":"GROK_QUOTA_INVALID_PLATFORM"`)
	require.NotContains(t, rec.Body.String(), "access-token")
	require.Zero(t, upstream.calls)
	require.Nil(t, upstream.lastReq)
	require.Nil(t, repo.updates)
}

func TestGrokOAuthHandlerQueryBillingQuotaRejectsNonOAuthAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &grokQuotaHandlerAccountRepo{account: &service.Account{
		ID:       46,
		Platform: service.PlatformGrok,
		Type:     service.AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "grok-api-key",
		},
	}}
	upstream := &grokQuotaHandlerUpstream{}
	quotaService := service.NewGrokQuotaService(repo, nil, service.NewGrokTokenProvider(repo, nil), upstream)
	handler := NewGrokOAuthHandler(nil, nil, quotaService)

	router := gin.New()
	router.GET("/api/v1/admin/grok/accounts/:id/billing-quota", handler.QueryBillingQuota)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/grok/accounts/46/billing-quota", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), `"reason":"GROK_QUOTA_INVALID_TYPE"`)
	require.NotContains(t, rec.Body.String(), "grok-api-key")
	require.Zero(t, upstream.calls)
	require.Nil(t, upstream.lastReq)
	require.Nil(t, repo.updates)
}

func TestGrokOAuthHandlerQueryBillingQuotaRedactsUpstreamFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &grokQuotaHandlerAccountRepo{account: &service.Account{
		ID:          47,
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
			"expires_at":    time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}}
	upstreamBody := `{"error":"billing failed","Authorization":"Bearer access-token","access_token":"access-token","refresh_token":"refresh-token","details":"` +
		strings.Repeat("x", 320) + `tail-sensitive-marker"}`
	upstream := &grokQuotaHandlerUpstream{responses: []*http.Response{
		{
			StatusCode: http.StatusInternalServerError,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(upstreamBody)),
		},
		{
			StatusCode: http.StatusInternalServerError,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(upstreamBody)),
		},
	}}
	quotaService := service.NewGrokQuotaService(repo, nil, service.NewGrokTokenProvider(repo, nil), upstream)
	handler := NewGrokOAuthHandler(nil, nil, quotaService)

	router := gin.New()
	router.GET("/api/v1/admin/grok/accounts/:id/billing-quota", handler.QueryBillingQuota)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/grok/accounts/47/billing-quota", nil)
	router.ServeHTTP(rec, req)

	body := rec.Body.String()
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, body, `"reason":"GROK_BILLING_UPSTREAM_ERROR"`)
	require.Contains(t, body, `\"Authorization\":\"***\"`)
	require.Contains(t, body, `\"access_token\":\"***\"`)
	require.Contains(t, body, `\"refresh_token\":\"***\"`)
	require.NotContains(t, body, "Bearer access-token")
	require.NotContains(t, body, "access-token")
	require.NotContains(t, body, "refresh-token")
	require.NotContains(t, body, "tail-sensitive-marker")
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "Bearer access-token", upstream.lastReq.Header.Get("Authorization"))
	require.Nil(t, repo.updates)
}

func TestGrokOAuthHandlerResetQuotaReturnsUnsupported(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &grokQuotaHandlerAccountRepo{account: &service.Account{
		ID:       43,
		Platform: service.PlatformGrok,
		Type:     service.AccountTypeOAuth,
	}}
	quotaService := service.NewGrokQuotaService(repo, nil, nil, nil)
	handler := NewGrokOAuthHandler(nil, nil, quotaService)

	router := gin.New()
	router.POST("/api/v1/admin/grok/accounts/:id/reset-quota", handler.ResetQuota)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/grok/accounts/43/reset-quota", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotImplemented, rec.Code)
	require.Contains(t, rec.Body.String(), `"reason":"GROK_QUOTA_RESET_UNSUPPORTED"`)
	require.NotContains(t, rec.Body.String(), "access-token")
}

func TestGrokOAuthHandlerRuntimeSanityDoesNotExposeSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(xai.EnvBaseURL, "http://127.0.0.1:8080/v1?access_token=secret")
	t.Setenv(xai.EnvClientID, "client-secret-like-value")

	handler := NewGrokOAuthHandler(nil, nil, nil)
	router := gin.New()
	router.GET("/api/v1/admin/grok/runtime-sanity", handler.RuntimeSanity)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/grok/runtime-sanity", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"public_gateway_scope":"responses_only"`)
	require.Contains(t, rec.Body.String(), `"valid":false`)
	require.NotContains(t, rec.Body.String(), "access_token")
	require.NotContains(t, rec.Body.String(), "secret")
	require.NotContains(t, rec.Body.String(), "client-secret-like-value")
}
