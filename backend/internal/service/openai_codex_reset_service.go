package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	openAICodexResetReferralKey     = "codex_referral_persistent_invite"
	openAICodexResetMaxInviteEmails = 5
)

var (
	openAICodexResetEmailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

	// ErrOpenAICodexResetUnsupportedAccount 表示账号不是 OpenAI OAuth 账号。
	ErrOpenAICodexResetUnsupportedAccount = infraerrors.BadRequest("OPENAI_CODEX_RESET_ACCOUNT_UNSUPPORTED", "仅 OpenAI OAuth 账号支持 Codex 邀请重置")
	// ErrOpenAICodexResetAccessTokenMissing 表示账号缺少 OpenAI access_token。
	ErrOpenAICodexResetAccessTokenMissing = infraerrors.BadRequest("OPENAI_CODEX_RESET_ACCESS_TOKEN_MISSING", "OpenAI OAuth access token 缺失")
	// ErrOpenAICodexResetNoAvailableCredit 表示没有可用的 reset credit。
	ErrOpenAICodexResetNoAvailableCredit = infraerrors.BadRequest("OPENAI_CODEX_RESET_NO_AVAILABLE_CREDIT", "当前没有可用的 Codex reset credit")
	// ErrOpenAICodexResetConfirmationRequired 表示邀请操作缺少收件人同意确认。
	ErrOpenAICodexResetConfirmationRequired = infraerrors.BadRequest("OPENAI_CODEX_RESET_CONFIRMATION_REQUIRED", "发送邀请前必须确认已获得收件人同意")
	// ErrOpenAICodexResetInvalidEmail 表示邮箱格式无效。
	ErrOpenAICodexResetInvalidEmail = infraerrors.BadRequest("OPENAI_CODEX_RESET_INVALID_EMAIL", "邀请邮箱格式无效")
	// ErrOpenAICodexResetTooManyEmails 表示单次邀请邮箱数量超过上限。
	ErrOpenAICodexResetTooManyEmails = infraerrors.BadRequest("OPENAI_CODEX_RESET_TOO_MANY_EMAILS", "单次邀请邮箱数量过多")
)

// OpenAICodexResetAccountSummary 是前端展示用的 OpenAI OAuth 账号摘要。
type OpenAICodexResetAccountSummary struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

// OpenAICodexResetCreditStatus 是 ChatGPT backend 返回的 reset credit 非敏感摘要。
type OpenAICodexResetCreditStatus struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

// OpenAICodexResetCreditsResult 表示 reset credit 查询结果。
type OpenAICodexResetCreditsResult struct {
	AvailableCount     int                            `json:"available_count"`
	CreditCount        int                            `json:"credit_count"`
	AvailableCreditIDs []string                       `json:"available_credit_ids"`
	CreditStatuses     []OpenAICodexResetCreditStatus `json:"credit_statuses"`
}

// OpenAICodexResetStatus 表示单个账号的 Codex reset / invite 状态。
type OpenAICodexResetStatus struct {
	Account            OpenAICodexResetAccountSummary `json:"account"`
	AvailableCount     int                            `json:"available_count"`
	CreditCount        int                            `json:"credit_count"`
	AvailableCreditIDs []string                       `json:"available_credit_ids"`
	CreditStatuses     []OpenAICodexResetCreditStatus `json:"credit_statuses"`
	Eligibility        map[string]any                 `json:"eligibility,omitempty"`
	Rules              map[string]any                 `json:"rules,omitempty"`
}

// OpenAICodexResetConsumeRequest 表示消耗 reset credit 的请求。
type OpenAICodexResetConsumeRequest struct {
	CreditID string `json:"credit_id"`
}

// OpenAICodexResetConsumeResult 表示消耗 reset credit 的结果。
type OpenAICodexResetConsumeResult struct {
	Account              OpenAICodexResetAccountSummary `json:"account"`
	CreditID             string                         `json:"credit_id"`
	Code                 string                         `json:"code,omitempty"`
	AvailableCount       *int                           `json:"available_count,omitempty"`
	RemainingCreditCount *int                           `json:"remaining_credit_count,omitempty"`
}

// OpenAICodexInviteRequest 表示发送 Codex 邀请的请求。
type OpenAICodexInviteRequest struct {
	Emails           []string `json:"emails"`
	ConsentConfirmed bool     `json:"consent_confirmed"`
}

// OpenAICodexInviteResult 表示发送 Codex 邀请的结果。
type OpenAICodexInviteResult struct {
	Account      OpenAICodexResetAccountSummary `json:"account"`
	Emails       []string                       `json:"emails"`
	InvitedCount *int                           `json:"invited_count,omitempty"`
	FailedEmails []string                       `json:"failed_emails,omitempty"`
	Message      string                         `json:"message,omitempty"`
}

// OpenAICodexResetClientAccount 是调用 ChatGPT backend 所需的最小账号凭证。
type OpenAICodexResetClientAccount struct {
	ID               int64
	Name             string
	Email            string
	AccessToken      string
	ChatGPTAccountID string
	ProxyURL         string
	UserAgent        string
}

// OpenAICodexResetClient 封装 ChatGPT backend 的 Codex invite/reset 调用。
type OpenAICodexResetClient interface {
	// GetCredits 查询账号可用的 Codex reset credit。
	//
	// @param ctx 请求上下文。
	// @param account ChatGPT backend 调用所需的最小账号凭证。
	// @return Codex reset credit 查询结果。
	GetCredits(ctx context.Context, account OpenAICodexResetClientAccount) (*OpenAICodexResetCreditsResult, error)
	// GetEligibility 查询账号当前 Codex 邀请资格。
	//
	// @param ctx 请求上下文。
	// @param account ChatGPT backend 调用所需的最小账号凭证。
	// @param referralKey Codex 邀请使用的 referral key。
	// @return Codex 邀请资格原始摘要。
	GetEligibility(ctx context.Context, account OpenAICodexResetClientAccount, referralKey string) (map[string]any, error)
	// GetRules 查询 Codex 邀请规则。
	//
	// @param ctx 请求上下文。
	// @param account ChatGPT backend 调用所需的最小账号凭证。
	// @param referralKey Codex 邀请使用的 referral key。
	// @return Codex 邀请规则原始摘要。
	GetRules(ctx context.Context, account OpenAICodexResetClientAccount, referralKey string) (map[string]any, error)
	// ConsumeCredit 消耗指定 Codex reset credit。
	//
	// @param ctx 请求上下文。
	// @param account ChatGPT backend 调用所需的最小账号凭证。
	// @param creditID 要消耗的 reset credit ID。
	// @param redeemRequestID 本次消耗请求的唯一 ID。
	// @return reset credit 消耗结果。
	ConsumeCredit(ctx context.Context, account OpenAICodexResetClientAccount, creditID, redeemRequestID string) (*OpenAICodexResetConsumeResult, error)
	// SendInvites 发送 Codex 邀请邮件。
	//
	// @param ctx 请求上下文。
	// @param account ChatGPT backend 调用所需的最小账号凭证。
	// @param referralKey Codex 邀请使用的 referral key。
	// @param emails 去重并校验后的邀请邮箱。
	// @return Codex 邀请发送结果。
	SendInvites(ctx context.Context, account OpenAICodexResetClientAccount, referralKey string, emails []string) (*OpenAICodexInviteResult, error)
}

type openAICodexResetAccountReader interface {
	GetByID(ctx context.Context, id int64) (*Account, error)
}

type openAICodexResetProxyReader interface {
	GetByID(ctx context.Context, id int64) (*Proxy, error)
}

// OpenAICodexResetService 提供 OpenAI OAuth 账号的 Codex 邀请和 reset credit 操作。
type OpenAICodexResetService struct {
	accountRepo openAICodexResetAccountReader
	proxyRepo   openAICodexResetProxyReader
	client      OpenAICodexResetClient
	uaProvider  interface {
		GetOpenAICodexUserAgent(ctx context.Context) string
	}
}

// NewOpenAICodexResetService 创建 OpenAICodexResetService。
//
// @param accountRepo 账号仓储，用于读取管理员指定的账号。
// @param proxyRepo 代理仓储，用于读取账号绑定的代理配置。
// @param client ChatGPT backend 客户端。
// @param uaProvider OpenAI Codex User-Agent 配置提供者。
// @return OpenAI Codex reset 服务实例。
func NewOpenAICodexResetService(accountRepo AccountRepository, proxyRepo ProxyRepository, client OpenAICodexResetClient, uaProvider *SettingService) *OpenAICodexResetService {
	return &OpenAICodexResetService{
		accountRepo: accountRepo,
		proxyRepo:   proxyRepo,
		client:      client,
		uaProvider:  uaProvider,
	}
}

// GetStatus 查询账号的 Codex reset credit、邀请资格和邀请规则。
//
// @param ctx 请求上下文。
// @param accountID 管理员明确选中的账号 ID。
// @return 单个账号的 Codex reset 状态。
func (s *OpenAICodexResetService) GetStatus(ctx context.Context, accountID int64) (*OpenAICodexResetStatus, error) {
	account, clientAccount, err := s.resolveClientAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}

	credits, err := s.client.GetCredits(ctx, clientAccount)
	if err != nil {
		return nil, err
	}
	if credits == nil {
		credits = &OpenAICodexResetCreditsResult{}
	}
	status := &OpenAICodexResetStatus{
		Account:            openAICodexResetAccountSummary(account),
		AvailableCount:     credits.AvailableCount,
		CreditCount:        credits.CreditCount,
		AvailableCreditIDs: append([]string(nil), credits.AvailableCreditIDs...),
		CreditStatuses:     append([]OpenAICodexResetCreditStatus(nil), credits.CreditStatuses...),
	}

	if eligibility, err := s.client.GetEligibility(ctx, clientAccount, openAICodexResetReferralKey); err == nil {
		status.Eligibility = eligibility
	}
	if rules, err := s.client.GetRules(ctx, clientAccount, openAICodexResetReferralKey); err == nil {
		status.Rules = rules
	}

	return status, nil
}

// ConsumeCredit 消耗账号的一个 Codex reset credit。
//
// @param ctx 请求上下文。
// @param accountID 管理员明确选中的账号 ID。
// @param req 消耗 reset credit 的请求，credit_id 为空时使用第一个可用 credit。
// @return reset credit 消耗结果。
func (s *OpenAICodexResetService) ConsumeCredit(ctx context.Context, accountID int64, req OpenAICodexResetConsumeRequest) (*OpenAICodexResetConsumeResult, error) {
	account, clientAccount, err := s.resolveClientAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}

	credits, err := s.client.GetCredits(ctx, clientAccount)
	if err != nil {
		return nil, err
	}
	if credits == nil {
		credits = &OpenAICodexResetCreditsResult{}
	}
	creditID := strings.TrimSpace(req.CreditID)
	if creditID == "" {
		if len(credits.AvailableCreditIDs) == 0 {
			return nil, ErrOpenAICodexResetNoAvailableCredit
		}
		creditID = credits.AvailableCreditIDs[0]
	} else if !openAICodexResetCreditAvailable(credits.AvailableCreditIDs, creditID) {
		return nil, ErrOpenAICodexResetNoAvailableCredit
	}

	redeemRequestID, err := newOpenAICodexResetRedeemRequestID()
	if err != nil {
		return nil, fmt.Errorf("generate redeem request id: %w", err)
	}
	result, err := s.client.ConsumeCredit(ctx, clientAccount, creditID, redeemRequestID)
	if err != nil {
		return nil, err
	}
	if result == nil {
		result = &OpenAICodexResetConsumeResult{}
	}
	result.Account = openAICodexResetAccountSummary(account)
	result.CreditID = creditID
	return result, nil
}

// SendInvites 使用指定 OpenAI OAuth 账号发送 Codex 邀请邮件。
//
// @param ctx 请求上下文。
// @param accountID 管理员明确选中的账号 ID。
// @param req 邀请请求，必须包含邮箱列表和收件人同意确认。
// @return Codex 邀请发送结果。
func (s *OpenAICodexResetService) SendInvites(ctx context.Context, accountID int64, req OpenAICodexInviteRequest) (*OpenAICodexInviteResult, error) {
	if !req.ConsentConfirmed {
		return nil, ErrOpenAICodexResetConfirmationRequired
	}
	account, clientAccount, err := s.resolveClientAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	emails, err := NormalizeOpenAICodexInviteEmails(req.Emails)
	if err != nil {
		return nil, err
	}

	result, err := s.client.SendInvites(ctx, clientAccount, openAICodexResetReferralKey, emails)
	if err != nil {
		return nil, err
	}
	if result == nil {
		result = &OpenAICodexInviteResult{}
	}
	result.Account = openAICodexResetAccountSummary(account)
	result.Emails = append([]string(nil), emails...)
	return result, nil
}

// NormalizeOpenAICodexInviteEmails 规范化、去重并校验 Codex 邀请邮箱。
//
// @param rawEmails 原始邮箱列表。
// @return 去重后的合法邮箱列表。
func NormalizeOpenAICodexInviteEmails(rawEmails []string) ([]string, error) {
	seen := make(map[string]struct{}, len(rawEmails))
	emails := make([]string, 0, len(rawEmails))
	for _, raw := range rawEmails {
		email := strings.TrimSpace(raw)
		if email == "" {
			continue
		}
		if !openAICodexResetEmailPattern.MatchString(email) {
			return nil, ErrOpenAICodexResetInvalidEmail.WithMetadata(map[string]string{"email": email})
		}
		key := strings.ToLower(email)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		emails = append(emails, email)
	}
	if len(emails) == 0 {
		return nil, ErrOpenAICodexResetInvalidEmail
	}
	if len(emails) > openAICodexResetMaxInviteEmails {
		return nil, ErrOpenAICodexResetTooManyEmails.WithMetadata(map[string]string{
			"max": fmt.Sprintf("%d", openAICodexResetMaxInviteEmails),
		})
	}
	return emails, nil
}

func (s *OpenAICodexResetService) resolveClientAccount(ctx context.Context, accountID int64) (*Account, OpenAICodexResetClientAccount, error) {
	if s == nil || s.accountRepo == nil || s.client == nil {
		return nil, OpenAICodexResetClientAccount{}, infraerrors.New(http.StatusServiceUnavailable, "OPENAI_CODEX_RESET_SERVICE_UNAVAILABLE", "OpenAI Codex reset 服务不可用")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, OpenAICodexResetClientAccount{}, fmt.Errorf("get account: %w", err)
	}
	if account == nil {
		return nil, OpenAICodexResetClientAccount{}, ErrAccountNotFound
	}
	if account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
		return nil, OpenAICodexResetClientAccount{}, ErrOpenAICodexResetUnsupportedAccount
	}
	accessToken := strings.TrimSpace(account.GetCredential("access_token"))
	if accessToken == "" {
		return nil, OpenAICodexResetClientAccount{}, ErrOpenAICodexResetAccessTokenMissing
	}

	clientAccount := OpenAICodexResetClientAccount{
		ID:               account.ID,
		Name:             account.Name,
		Email:            openAICodexResetAccountEmail(account),
		AccessToken:      accessToken,
		ChatGPTAccountID: strings.TrimSpace(account.GetCredential("chatgpt_account_id")),
		UserAgent:        s.openAICodexResetUserAgent(ctx),
	}
	if account.ProxyID != nil && s.proxyRepo != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
		if err != nil {
			return nil, OpenAICodexResetClientAccount{}, fmt.Errorf("get proxy: %w", err)
		}
		if proxy == nil {
			return nil, OpenAICodexResetClientAccount{}, ErrProxyNotFound
		}
		clientAccount.ProxyURL = proxy.URL()
	}
	return account, clientAccount, nil
}

func (s *OpenAICodexResetService) openAICodexResetUserAgent(ctx context.Context) string {
	if s != nil && s.uaProvider != nil {
		if ua := strings.TrimSpace(s.uaProvider.GetOpenAICodexUserAgent(ctx)); ua != "" {
			return ua
		}
	}
	return DefaultOpenAICodexUserAgent
}

func openAICodexResetAccountSummary(account *Account) OpenAICodexResetAccountSummary {
	if account == nil {
		return OpenAICodexResetAccountSummary{}
	}
	return OpenAICodexResetAccountSummary{
		ID:    account.ID,
		Name:  account.Name,
		Email: openAICodexResetAccountEmail(account),
	}
}

func openAICodexResetAccountEmail(account *Account) string {
	if account == nil {
		return ""
	}
	for _, candidate := range []string{
		account.GetCredential("email"),
		account.GetExtraString("email"),
		account.GetExtraString("email_address"),
	} {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func openAICodexResetCreditAvailable(availableIDs []string, creditID string) bool {
	for _, id := range availableIDs {
		if id == creditID {
			return true
		}
	}
	return false
}

func newOpenAICodexResetRedeemRequestID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	), nil
}
