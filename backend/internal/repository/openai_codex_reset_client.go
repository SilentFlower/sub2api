package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/imroc/req/v3"
)

const openAICodexResetBackendBaseURL = "https://chatgpt.com/backend-api"

type openAICodexResetClient struct {
	baseURL string
}

// NewOpenAICodexResetClient 创建 ChatGPT Codex reset/invite 后端客户端。
//
// @return ChatGPT backend 客户端实例。
func NewOpenAICodexResetClient() service.OpenAICodexResetClient {
	return &openAICodexResetClient{baseURL: openAICodexResetBackendBaseURL}
}

func newOpenAICodexResetClientWithBaseURL(baseURL string) *openAICodexResetClient {
	return &openAICodexResetClient{baseURL: strings.TrimRight(baseURL, "/")}
}

func (c *openAICodexResetClient) GetCredits(ctx context.Context, account service.OpenAICodexResetClientAccount) (*service.OpenAICodexResetCreditsResult, error) {
	var payload openAICodexResetCreditsPayload
	if err := c.requestJSON(ctx, account, http.MethodGet, "/wham/rate-limit-reset-credits", nil, &payload); err != nil {
		return nil, err
	}
	return payload.toService(), nil
}

func (c *openAICodexResetClient) GetEligibility(ctx context.Context, account service.OpenAICodexResetClientAccount, referralKey string) (map[string]any, error) {
	var payload map[string]any
	path := "/referrals/invite/eligibility?referral_key=" + url.QueryEscape(referralKey)
	if err := c.requestJSON(ctx, account, http.MethodGet, path, nil, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *openAICodexResetClient) GetRules(ctx context.Context, account service.OpenAICodexResetClientAccount, referralKey string) (map[string]any, error) {
	var payload map[string]any
	path := "/wham/referrals/eligibility_rules?referral_key=" + url.QueryEscape(referralKey)
	if err := c.requestJSON(ctx, account, http.MethodGet, path, nil, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *openAICodexResetClient) ConsumeCredit(ctx context.Context, account service.OpenAICodexResetClientAccount, creditID, redeemRequestID string) (*service.OpenAICodexResetConsumeResult, error) {
	body := map[string]string{
		"credit_id":         creditID,
		"redeem_request_id": redeemRequestID,
	}
	var payload openAICodexResetConsumePayload
	if err := c.requestJSON(ctx, account, http.MethodPost, "/wham/rate-limit-reset-credits/consume", body, &payload); err != nil {
		return nil, err
	}
	result := &service.OpenAICodexResetConsumeResult{
		Code:                 payload.Code,
		AvailableCount:       payload.AvailableCount,
		RemainingCreditCount: payload.remainingCreditCount(),
	}
	return result, nil
}

func (c *openAICodexResetClient) SendInvites(ctx context.Context, account service.OpenAICodexResetClientAccount, referralKey string, emails []string) (*service.OpenAICodexInviteResult, error) {
	body := map[string]any{
		"referral_key": referralKey,
		"emails":       emails,
	}
	var payload openAICodexInvitePayload
	if err := c.requestJSON(ctx, account, http.MethodPost, "/wham/referrals/invite", body, &payload); err != nil {
		return nil, err
	}
	result := &service.OpenAICodexInviteResult{
		InvitedCount: payload.invitedCount(),
		FailedEmails: append([]string(nil), payload.FailedEmails...),
		Message:      payload.Message,
	}
	return result, nil
}

func (c *openAICodexResetClient) requestJSON(ctx context.Context, account service.OpenAICodexResetClientAccount, method, path string, body any, result any) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client, err := CreatePrivacyReqClient(account.ProxyURL)
	if err != nil {
		return infraerrors.New(http.StatusBadGateway, "OPENAI_CODEX_RESET_CLIENT_INIT_FAILED", "创建 ChatGPT backend 客户端失败")
	}

	request := client.R().
		SetContext(ctx).
		SetHeaders(openAICodexResetHeaders(account)).
		SetSuccessResult(result)
	if body != nil {
		request.SetBody(body)
	}

	var resp *req.Response
	baseURL := strings.TrimRight(c.baseURL, "/")
	if baseURL == "" {
		baseURL = openAICodexResetBackendBaseURL
	}
	url := baseURL + path
	switch method {
	case http.MethodGet:
		resp, err = request.Get(url)
	case http.MethodPost:
		resp, err = request.Post(url)
	default:
		return fmt.Errorf("unsupported method: %s", method)
	}
	if err != nil {
		return infraerrors.New(http.StatusBadGateway, "OPENAI_CODEX_RESET_UPSTREAM_FAILED", "ChatGPT backend 请求失败")
	}
	if !resp.IsSuccessState() {
		message := fmt.Sprintf("ChatGPT backend 请求失败：状态码 %d", resp.StatusCode)
		if reason := openAICodexResetUpstreamErrorReason(resp.Bytes()); reason != "" {
			message = fmt.Sprintf("%s，原因：%s", message, reason)
		}
		return infraerrors.New(http.StatusBadGateway, "OPENAI_CODEX_RESET_UPSTREAM_FAILED", message)
	}
	return nil
}

func openAICodexResetUpstreamErrorReason(body []byte) string {
	var payload map[string]any
	if len(body) == 0 || json.Unmarshal(body, &payload) != nil {
		return ""
	}
	for _, path := range [][]string{
		{"detail"},
		{"message"},
		{"error", "message"},
		{"error", "code"},
		{"error"},
	} {
		if value := openAICodexResetStringAt(payload, path...); value != "" {
			return value
		}
	}
	return ""
}

func openAICodexResetStringAt(payload map[string]any, path ...string) string {
	var current any = payload
	for _, key := range path {
		obj, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = obj[key]
	}
	value, ok := current.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func openAICodexResetHeaders(account service.OpenAICodexResetClientAccount) map[string]string {
	ua := strings.TrimSpace(account.UserAgent)
	if ua == "" {
		ua = "Codex Desktop/0.0.0 (Linux; x86_64)"
	}
	headers := map[string]string{
		"Authorization":                   "Bearer " + account.AccessToken,
		"Accept":                          "application/json",
		"Content-Type":                    "application/json",
		"OAI-Language":                    "zh-CN",
		"originator":                      "Codex Desktop",
		"X-OpenAI-Attach-Auth":            "1",
		"X-OpenAI-Attach-Integrity-State": "1",
		"User-Agent":                      ua,
	}
	if strings.TrimSpace(account.ChatGPTAccountID) != "" {
		headers["chatgpt-account-id"] = strings.TrimSpace(account.ChatGPTAccountID)
	}
	return headers
}

type openAICodexResetCreditPayload struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type openAICodexResetCreditsPayload struct {
	AvailableCount *int                            `json:"available_count"`
	Credits        []openAICodexResetCreditPayload `json:"credits"`
}

func (p openAICodexResetCreditsPayload) toService() *service.OpenAICodexResetCreditsResult {
	creditStatuses := make([]service.OpenAICodexResetCreditStatus, 0, len(p.Credits))
	availableIDs := make([]string, 0, len(p.Credits))
	for _, credit := range p.Credits {
		if strings.TrimSpace(credit.ID) == "" {
			continue
		}
		status := strings.TrimSpace(credit.Status)
		creditStatuses = append(creditStatuses, service.OpenAICodexResetCreditStatus{
			ID:          credit.ID,
			Status:      status,
			Title:       credit.Title,
			Description: credit.Description,
		})
		if status == "" || strings.EqualFold(status, "available") {
			availableIDs = append(availableIDs, credit.ID)
		}
	}
	availableCount := len(availableIDs)
	if p.AvailableCount != nil {
		availableCount = *p.AvailableCount
	}
	return &service.OpenAICodexResetCreditsResult{
		AvailableCount:     availableCount,
		CreditCount:        len(p.Credits),
		AvailableCreditIDs: availableIDs,
		CreditStatuses:     creditStatuses,
	}
}

type openAICodexResetConsumePayload struct {
	Code           string                          `json:"code"`
	AvailableCount *int                            `json:"available_count"`
	Credits        []openAICodexResetCreditPayload `json:"credits"`
}

func (p openAICodexResetConsumePayload) remainingCreditCount() *int {
	if p.Credits == nil {
		return nil
	}
	count := len(p.Credits)
	return &count
}

type openAICodexInvitePayload struct {
	Invites      []any    `json:"invites"`
	FailedEmails []string `json:"failed_emails"`
	Message      string   `json:"message"`
}

func (p openAICodexInvitePayload) invitedCount() *int {
	if p.Invites == nil {
		return nil
	}
	count := len(p.Invites)
	return &count
}
