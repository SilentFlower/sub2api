//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type openAICodexResetAccountRepoStub struct {
	account *Account
	err     error
}

func (s *openAICodexResetAccountRepoStub) GetByID(context.Context, int64) (*Account, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.account, nil
}

type openAICodexResetProxyRepoStub struct {
	proxy *Proxy
	err   error
}

func (s *openAICodexResetProxyRepoStub) GetByID(context.Context, int64) (*Proxy, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.proxy, nil
}

type openAICodexResetClientStub struct {
	credits        *OpenAICodexResetCreditsResult
	consumeCredit  string
	consumeRequest string
	inviteKey      string
	inviteEmails   []string
}

func (s *openAICodexResetClientStub) GetCredits(context.Context, OpenAICodexResetClientAccount) (*OpenAICodexResetCreditsResult, error) {
	if s.credits != nil {
		return s.credits, nil
	}
	return &OpenAICodexResetCreditsResult{}, nil
}

func (s *openAICodexResetClientStub) GetEligibility(context.Context, OpenAICodexResetClientAccount, string) (map[string]any, error) {
	return map[string]any{"eligible": true}, nil
}

func (s *openAICodexResetClientStub) GetRules(context.Context, OpenAICodexResetClientAccount, string) (map[string]any, error) {
	return map[string]any{"max": float64(5)}, nil
}

func (s *openAICodexResetClientStub) ConsumeCredit(_ context.Context, _ OpenAICodexResetClientAccount, creditID, redeemRequestID string) (*OpenAICodexResetConsumeResult, error) {
	s.consumeCredit = creditID
	s.consumeRequest = redeemRequestID
	return &OpenAICodexResetConsumeResult{Code: "ok"}, nil
}

func (s *openAICodexResetClientStub) SendInvites(_ context.Context, _ OpenAICodexResetClientAccount, referralKey string, emails []string) (*OpenAICodexInviteResult, error) {
	s.inviteKey = referralKey
	s.inviteEmails = append([]string(nil), emails...)
	count := len(emails)
	return &OpenAICodexInviteResult{InvitedCount: &count}, nil
}

func newOpenAICodexResetTestService(account *Account, client *openAICodexResetClientStub) *OpenAICodexResetService {
	return &OpenAICodexResetService{
		accountRepo: &openAICodexResetAccountRepoStub{account: account},
		client:      client,
	}
}

func openAICodexResetOAuthAccount() *Account {
	return &Account{
		ID:       42,
		Name:     "openai-oauth",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":       "access-token",
			"chatgpt_account_id": "acct-secret",
			"email":              "user@example.com",
		},
	}
}

func TestOpenAICodexResetService_GetStatusRejectsNonOpenAIOAuth(t *testing.T) {
	t.Parallel()

	svc := newOpenAICodexResetTestService(&Account{
		ID:       1,
		Platform: PlatformGemini,
		Type:     AccountTypeOAuth,
	}, &openAICodexResetClientStub{})

	_, err := svc.GetStatus(context.Background(), 1)

	require.ErrorIs(t, err, ErrOpenAICodexResetUnsupportedAccount)
	require.Equal(t, "OPENAI_CODEX_RESET_ACCOUNT_UNSUPPORTED", infraerrors.Reason(err))
}

func TestOpenAICodexResetService_GetStatusRejectsMissingAccessToken(t *testing.T) {
	t.Parallel()

	account := openAICodexResetOAuthAccount()
	account.Credentials = map[string]any{"email": "user@example.com"}
	svc := newOpenAICodexResetTestService(account, &openAICodexResetClientStub{})

	_, err := svc.GetStatus(context.Background(), account.ID)

	require.ErrorIs(t, err, ErrOpenAICodexResetAccessTokenMissing)
	require.Equal(t, "OPENAI_CODEX_RESET_ACCESS_TOKEN_MISSING", infraerrors.Reason(err))
}

func TestOpenAICodexResetService_GetStatusReturnsCreditsAndRules(t *testing.T) {
	t.Parallel()

	client := &openAICodexResetClientStub{
		credits: &OpenAICodexResetCreditsResult{
			AvailableCount:     1,
			CreditCount:        2,
			AvailableCreditIDs: []string{"credit-1"},
			CreditStatuses: []OpenAICodexResetCreditStatus{
				{ID: "credit-1", Status: "available", Title: "Reset"},
				{ID: "credit-2", Status: "used", Title: "Used"},
			},
		},
	}
	svc := newOpenAICodexResetTestService(openAICodexResetOAuthAccount(), client)

	status, err := svc.GetStatus(context.Background(), 42)

	require.NoError(t, err)
	require.Equal(t, int64(42), status.Account.ID)
	require.Equal(t, "user@example.com", status.Account.Email)
	require.Equal(t, 1, status.AvailableCount)
	require.Equal(t, []string{"credit-1"}, status.AvailableCreditIDs)
	require.Equal(t, map[string]any{"eligible": true}, status.Eligibility)
	require.Equal(t, map[string]any{"max": float64(5)}, status.Rules)
}

func TestOpenAICodexResetService_GetStatusReturnsProxyReadError(t *testing.T) {
	t.Parallel()

	proxyID := int64(7)
	account := openAICodexResetOAuthAccount()
	account.ProxyID = &proxyID
	svc := newOpenAICodexResetTestService(account, &openAICodexResetClientStub{})
	svc.proxyRepo = &openAICodexResetProxyRepoStub{err: fmt.Errorf("proxy read failed")}

	_, err := svc.GetStatus(context.Background(), 42)

	require.Error(t, err)
	require.Contains(t, err.Error(), "get proxy")
}

func TestOpenAICodexResetService_ConsumeCreditRejectsNoAvailableCredit(t *testing.T) {
	t.Parallel()

	svc := newOpenAICodexResetTestService(openAICodexResetOAuthAccount(), &openAICodexResetClientStub{})

	_, err := svc.ConsumeCredit(context.Background(), 42, OpenAICodexResetConsumeRequest{})

	require.ErrorIs(t, err, ErrOpenAICodexResetNoAvailableCredit)
	require.Equal(t, "OPENAI_CODEX_RESET_NO_AVAILABLE_CREDIT", infraerrors.Reason(err))
}

func TestOpenAICodexResetService_ConsumeCreditUsesFirstAvailableCredit(t *testing.T) {
	t.Parallel()

	client := &openAICodexResetClientStub{
		credits: &OpenAICodexResetCreditsResult{
			AvailableCreditIDs: []string{"credit-a", "credit-b"},
		},
	}
	svc := newOpenAICodexResetTestService(openAICodexResetOAuthAccount(), client)

	result, err := svc.ConsumeCredit(context.Background(), 42, OpenAICodexResetConsumeRequest{})

	require.NoError(t, err)
	require.Equal(t, "credit-a", client.consumeCredit)
	require.NotEmpty(t, client.consumeRequest)
	require.Equal(t, "credit-a", result.CreditID)
}

func TestOpenAICodexResetService_SendInvitesRequiresConsent(t *testing.T) {
	t.Parallel()

	svc := newOpenAICodexResetTestService(openAICodexResetOAuthAccount(), &openAICodexResetClientStub{})

	_, err := svc.SendInvites(context.Background(), 42, OpenAICodexInviteRequest{
		Emails: []string{"a@example.com"},
	})

	require.ErrorIs(t, err, ErrOpenAICodexResetConfirmationRequired)
	require.Equal(t, "OPENAI_CODEX_RESET_CONFIRMATION_REQUIRED", infraerrors.Reason(err))
}

func TestOpenAICodexResetService_SendInvitesNormalizesEmails(t *testing.T) {
	t.Parallel()

	client := &openAICodexResetClientStub{}
	svc := newOpenAICodexResetTestService(openAICodexResetOAuthAccount(), client)

	result, err := svc.SendInvites(context.Background(), 42, OpenAICodexInviteRequest{
		Emails:           []string{" A@example.com ", "a@example.com", "b@example.com"},
		ConsentConfirmed: true,
	})

	require.NoError(t, err)
	require.Equal(t, openAICodexResetReferralKey, client.inviteKey)
	require.Equal(t, []string{"A@example.com", "b@example.com"}, client.inviteEmails)
	require.Equal(t, []string{"A@example.com", "b@example.com"}, result.Emails)
}

func TestNormalizeOpenAICodexInviteEmailsRejectsInvalidAndTooMany(t *testing.T) {
	t.Parallel()

	_, err := NormalizeOpenAICodexInviteEmails([]string{"bad-email"})
	require.ErrorIs(t, err, ErrOpenAICodexResetInvalidEmail)

	_, err = NormalizeOpenAICodexInviteEmails([]string{"User <user@example.com>"})
	require.ErrorIs(t, err, ErrOpenAICodexResetInvalidEmail)

	_, err = NormalizeOpenAICodexInviteEmails([]string{
		"a1@example.com",
		"a2@example.com",
		"a3@example.com",
		"a4@example.com",
		"a5@example.com",
		"a6@example.com",
	})
	require.ErrorIs(t, err, ErrOpenAICodexResetTooManyEmails)
}
