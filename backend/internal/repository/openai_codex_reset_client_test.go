//go:build unit

package repository

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAICodexResetClient_CreditsAndConsume(t *testing.T) {
	t.Parallel()

	type capturedRequest struct {
		method        string
		path          string
		authorization string
		accountID     string
		userAgent     string
		body          map[string]any
	}
	var captured []capturedRequest
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		item := capturedRequest{
			method:        r.Method,
			path:          r.URL.RequestURI(),
			authorization: r.Header.Get("Authorization"),
			accountID:     r.Header.Get("chatgpt-account-id"),
			userAgent:     r.Header.Get("User-Agent"),
		}
		if r.Body != nil {
			body, _ := io.ReadAll(r.Body)
			if len(body) > 0 {
				_ = json.Unmarshal(body, &item.body)
			}
		}
		captured = append(captured, item)

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/wham/rate-limit-reset-credits":
			_, _ = io.WriteString(w, `{"available_count":1,"credits":[{"id":"credit-1","status":"available","title":"Reset"},{"id":"credit-2","status":"used"}]}`)
		case "/wham/rate-limit-reset-credits/consume":
			_, _ = io.WriteString(w, `{"code":"ok","available_count":0,"credits":[]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := newOpenAICodexResetClientWithBaseURL(srv.URL)
	account := service.OpenAICodexResetClientAccount{
		AccessToken:      "token-secret",
		ChatGPTAccountID: "acct-secret",
		UserAgent:        "Codex Test UA",
	}

	credits, err := client.GetCredits(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, 1, credits.AvailableCount)
	require.Equal(t, []string{"credit-1"}, credits.AvailableCreditIDs)

	result, err := client.ConsumeCredit(context.Background(), account, "credit-1", "redeem-id")
	require.NoError(t, err)
	require.Equal(t, "ok", result.Code)
	require.NotNil(t, result.AvailableCount)
	require.Equal(t, 0, *result.AvailableCount)

	require.Len(t, captured, 2)
	require.Equal(t, "GET", captured[0].method)
	require.Equal(t, "/wham/rate-limit-reset-credits", captured[0].path)
	require.Equal(t, "Bearer token-secret", captured[0].authorization)
	require.Equal(t, "acct-secret", captured[0].accountID)
	require.Equal(t, "Codex Test UA", captured[0].userAgent)
	require.Equal(t, "POST", captured[1].method)
	require.Equal(t, "/wham/rate-limit-reset-credits/consume", captured[1].path)
	require.Equal(t, "credit-1", captured[1].body["credit_id"])
	require.Equal(t, "redeem-id", captured[1].body["redeem_request_id"])
}

func TestOpenAICodexResetClient_EligibilityRulesAndInvite(t *testing.T) {
	t.Parallel()

	var inviteBody map[string]any
	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/referrals/invite/eligibility":
			require.Equal(t, "codex_referral_persistent_invite", r.URL.Query().Get("referral_key"))
			_, _ = io.WriteString(w, `{"eligible":true}`)
		case "/wham/referrals/eligibility_rules":
			require.Equal(t, "codex_referral_persistent_invite", r.URL.Query().Get("referral_key"))
			_, _ = io.WriteString(w, `{"max":5}`)
		case "/wham/referrals/invite":
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &inviteBody)
			_, _ = io.WriteString(w, `{"invites":[{"email":"a@example.com"},{"email":"b@example.com"}],"failed_emails":["c@example.com"],"message":"partial"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := newOpenAICodexResetClientWithBaseURL(srv.URL)
	account := service.OpenAICodexResetClientAccount{AccessToken: "token-secret"}

	eligibility, err := client.GetEligibility(context.Background(), account, "codex_referral_persistent_invite")
	require.NoError(t, err)
	require.Equal(t, true, eligibility["eligible"])

	rules, err := client.GetRules(context.Background(), account, "codex_referral_persistent_invite")
	require.NoError(t, err)
	require.Equal(t, float64(5), rules["max"])

	result, err := client.SendInvites(context.Background(), account, "codex_referral_persistent_invite", []string{"a@example.com", "b@example.com"})
	require.NoError(t, err)
	require.Equal(t, "codex_referral_persistent_invite", inviteBody["referral_key"])
	require.Equal(t, []any{"a@example.com", "b@example.com"}, inviteBody["emails"])
	require.NotNil(t, result.InvitedCount)
	require.Equal(t, 2, *result.InvitedCount)
	require.Equal(t, []string{"c@example.com"}, result.FailedEmails)
	require.Equal(t, "partial", result.Message)
}

func TestOpenAICodexResetClient_UpstreamErrorDoesNotExposeBody(t *testing.T) {
	t.Parallel()

	srv := newLocalTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"error":"contains-token-secret"}`)
	}))
	defer srv.Close()

	client := newOpenAICodexResetClientWithBaseURL(srv.URL)
	_, err := client.GetCredits(context.Background(), service.OpenAICodexResetClientAccount{AccessToken: "token-secret"})

	require.Error(t, err)
	require.Equal(t, "OPENAI_CODEX_RESET_UPSTREAM_FAILED", infraerrors.Reason(err))
	require.Contains(t, infraerrors.Message(err), "状态码 502")
	require.NotContains(t, infraerrors.Message(err), "contains-token-secret")
}
