package service

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai/billingquota"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

const (
	grokBillingQuotaSnapshotExtraKey = "grok_billing_quota_snapshot"
	grokBillingQuotaBodyLimit        = 1 << 20
)

// GrokBillingQuotaResult 表示独立 Grok 套餐额度刷新结果。
type GrokBillingQuotaResult struct {
	Source    string                 `json:"source"`
	Snapshot  *billingquota.Snapshot `json:"snapshot,omitempty"`
	FetchedAt int64                  `json:"fetched_at"`
}

// GrokBillingQuotaService 负责与 main quota 隔离的 Grok 套餐额度查询。
type GrokBillingQuotaService struct {
	accountRepo   AccountRepository
	proxyRepo     ProxyRepository
	tokenProvider *GrokTokenProvider
	httpUpstream  HTTPUpstream
}

// NewGrokBillingQuotaService 创建独立 Grok 套餐额度服务。
// @param accountRepo 账号仓储。
// @param proxyRepo 代理仓储。
// @param tokenProvider Grok OAuth Token Provider。
// @param httpUpstream 上游 HTTP transport。
// @return 初始化后的独立套餐额度服务。
func NewGrokBillingQuotaService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	tokenProvider *GrokTokenProvider,
	httpUpstream HTTPUpstream,
) *GrokBillingQuotaService {
	return &GrokBillingQuotaService{
		accountRepo:   accountRepo,
		proxyRepo:     proxyRepo,
		tokenProvider: tokenProvider,
		httpUpstream:  httpUpstream,
	}
}

// QueryBillingQuota 刷新独立 Grok CLI Billing 套餐额度并写入专用快照。
// @param ctx 请求上下文。
// @param accountID Grok OAuth 账号 ID。
// @return 独立套餐额度刷新结果和业务错误。
func (s *GrokBillingQuotaService) QueryBillingQuota(ctx context.Context, accountID int64) (*GrokBillingQuotaResult, error) {
	account, token, proxyURL, err := s.prepare(ctx, accountID)
	if err != nil {
		return nil, err
	}

	weeklyURL, err := billingquota.BuildURL(xai.DefaultCLIBaseURL, true)
	if err != nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "GROK_BILLING_QUOTA_URL_INVALID", "failed to build weekly billing URL")
	}
	monthlyURL, err := billingquota.BuildURL(xai.DefaultCLIBaseURL, false)
	if err != nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "GROK_BILLING_QUOTA_URL_INVALID", "failed to build monthly billing URL")
	}

	probeCtx, cancel := context.WithTimeout(ctx, grokQuotaUpstreamTimeout)
	defer cancel()
	now := time.Now().UTC()
	weeklyPayload, weeklyStatus, weeklyErr := s.fetch(probeCtx, account, token, proxyURL, weeklyURL, "weekly")
	monthlyPayload, monthlyStatus, monthlyErr := s.fetch(probeCtx, account, token, proxyURL, monthlyURL, "monthly")

	weeklyOK := weeklyErr == nil && billingquota.BuildSnapshot(weeklyPayload, nil, now) != nil
	monthlyOK := monthlyErr == nil && billingquota.BuildSnapshot(nil, monthlyPayload, now) != nil
	if !weeklyOK && weeklyErr == nil {
		weeklyErr = infraerrors.New(http.StatusBadGateway, "GROK_BILLING_QUOTA_EMPTY", "weekly billing response did not include usable quota data")
	}
	if !monthlyOK && monthlyErr == nil {
		monthlyErr = infraerrors.New(http.StatusBadGateway, "GROK_BILLING_QUOTA_EMPTY", "monthly billing response did not include usable quota data")
	}
	if !weeklyOK && !monthlyOK {
		return nil, mergeGrokBillingQuotaErrors(weeklyStatus, monthlyStatus, weeklyErr, monthlyErr)
	}
	if !weeklyOK {
		weeklyPayload = nil
	}
	if !monthlyOK {
		monthlyPayload = nil
	}

	snapshot := billingquota.BuildSnapshot(weeklyPayload, monthlyPayload, now)
	if snapshot == nil {
		return nil, infraerrors.New(http.StatusBadGateway, "GROK_BILLING_QUOTA_EMPTY", "upstream billing response did not include usable quota data")
	}
	snapshot.Partial = !weeklyOK || !monthlyOK
	if !weeklyOK {
		snapshot.FailedWindows = append(snapshot.FailedWindows, "weekly")
	}
	if !monthlyOK {
		snapshot.FailedWindows = append(snapshot.FailedWindows, "monthly")
	}
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
		grokBillingQuotaSnapshotExtraKey: snapshot,
	}); err != nil {
		slog.Warn("grok_billing_quota_cache_update_failed", "account_id", account.ID)
		return nil, infraerrors.New(http.StatusInternalServerError, "GROK_BILLING_QUOTA_CACHE_UPDATE_FAILED", "failed to update billing quota snapshot")
	}

	return &GrokBillingQuotaResult{
		Source:    "grok_cli_billing_quota",
		Snapshot:  snapshot,
		FetchedAt: now.Unix(),
	}, nil
}

func (s *GrokBillingQuotaService) prepare(ctx context.Context, accountID int64) (*Account, string, string, error) {
	if s == nil || s.accountRepo == nil || s.tokenProvider == nil || s.httpUpstream == nil {
		return nil, "", "", infraerrors.New(http.StatusInternalServerError, "GROK_BILLING_QUOTA_NOT_CONFIGURED", "Grok billing quota service is not configured")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		if infraerrors.Code(err) == http.StatusNotFound {
			return nil, "", "", infraerrors.New(http.StatusNotFound, "GROK_BILLING_QUOTA_ACCOUNT_NOT_FOUND", "account not found")
		}
		return nil, "", "", infraerrors.New(http.StatusInternalServerError, "GROK_BILLING_QUOTA_ACCOUNT_LOOKUP_FAILED", "failed to load account")
	}
	if account == nil {
		return nil, "", "", infraerrors.New(http.StatusNotFound, "GROK_BILLING_QUOTA_ACCOUNT_NOT_FOUND", "account not found")
	}
	if account.Platform != PlatformGrok {
		return nil, "", "", infraerrors.New(http.StatusBadRequest, "GROK_BILLING_QUOTA_INVALID_PLATFORM", "account is not a Grok account")
	}
	if account.Type != AccountTypeOAuth {
		return nil, "", "", infraerrors.New(http.StatusBadRequest, "GROK_BILLING_QUOTA_INVALID_TYPE", "account is not an OAuth account")
	}
	token, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, "", "", infraerrors.New(http.StatusBadGateway, "GROK_BILLING_QUOTA_TOKEN_UNAVAILABLE", "failed to acquire access token")
	}
	if strings.TrimSpace(token) == "" {
		return nil, "", "", infraerrors.New(http.StatusBadGateway, "GROK_BILLING_QUOTA_TOKEN_UNAVAILABLE", "access token is empty")
	}
	return account, token, s.resolveProxyURL(ctx, account), nil
}

func (s *GrokBillingQuotaService) resolveProxyURL(ctx context.Context, account *Account) string {
	if account == nil || account.ProxyID == nil {
		return ""
	}
	if account.Proxy != nil {
		return account.Proxy.URL()
	}
	if s.proxyRepo == nil {
		return ""
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
	if err != nil || proxy == nil {
		return ""
	}
	return proxy.URL()
}

func (s *GrokBillingQuotaService) fetch(
	ctx context.Context,
	account *Account,
	token string,
	proxyURL string,
	targetURL string,
	window string,
) (*billingquota.Payload, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, 0, infraerrors.New(http.StatusInternalServerError, "GROK_BILLING_QUOTA_REQUEST_BUILD_FAILED", "failed to build billing request")
	}
	billingquota.ApplyHeaders(req, token)
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, maxInt(account.Concurrency, 1))
	if err != nil {
		return nil, 0, infraerrors.New(http.StatusBadGateway, "GROK_BILLING_QUOTA_REQUEST_FAILED", "billing request failed")
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, grokBillingQuotaBodyLimit))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		bodyText := logredact.RedactText(truncate(strings.TrimSpace(string(bodyBytes)), 240), "authorization")
		slog.Warn("grok_billing_quota_request_failed", "account_id", account.ID, "window", window, "status", resp.StatusCode, "body", bodyText)
		return nil, resp.StatusCode, infraerrors.Newf(mapUpstreamStatus(resp.StatusCode), "GROK_BILLING_QUOTA_UPSTREAM_ERROR", "billing upstream returned %d", resp.StatusCode)
	}
	payload, err := billingquota.ParsePayload(bodyBytes)
	if err != nil {
		return nil, resp.StatusCode, infraerrors.New(http.StatusBadGateway, "GROK_BILLING_QUOTA_PARSE_FAILED", "failed to parse billing response")
	}
	return payload, resp.StatusCode, nil
}

func mergeGrokBillingQuotaErrors(weeklyStatus, monthlyStatus int, weeklyErr, monthlyErr error) error {
	if weeklyErr != nil && monthlyErr != nil &&
		infraerrors.Code(weeklyErr) == infraerrors.Code(monthlyErr) &&
		infraerrors.Reason(weeklyErr) == infraerrors.Reason(monthlyErr) {
		return weeklyErr
	}
	slog.Warn("grok_billing_quota_parts_failed", "weekly_status", weeklyStatus, "monthly_status", monthlyStatus)
	return infraerrors.New(http.StatusBadGateway, "GROK_BILLING_QUOTA_PARTS_FAILED", "weekly and monthly billing requests failed").WithMetadata(map[string]string{
		"weekly_status":  strconv.Itoa(weeklyStatus),
		"monthly_status": strconv.Itoa(monthlyStatus),
	})
}
