//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const streamPolicyCreditError = `{"code":"credit_balance_exhausted","message":"You have no credits remaining. Add credits to continue using the API at https://platform.openai.com/settings/organization/billing/.","param":null,"type":"insufficient_quota"}`

// streamErrorPolicyAccountRepo 保存账号错误，分别验证持久化结果与调度状态通知。
type streamErrorPolicyAccountRepo struct {
	mockAccountRepoForGemini
	setErrorCalls int
}

// SetError 记录账号状态，供测试同时验证持久化结果和实时调度阻断。
// @param ctx 请求上下文。
// @param id 需要停用的账号编号。
// @param message 上游错误说明。
// @return 账号不存在时返回错误。
func (r *streamErrorPolicyAccountRepo) SetError(ctx context.Context, id int64, message string) error {
	account, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	account.Status = StatusError
	account.ErrorMessage = message
	r.setErrorCalls++
	return nil
}

func newStreamErrorPolicyService(t *testing.T) (*OpenAIGatewayService, *Account, *streamErrorPolicyAccountRepo) {
	t.Helper()
	account := &Account{
		ID:          1,
		Name:        "credit-account",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"custom_error_codes_enabled": true,
			"custom_error_codes":         []any{float64(http.StatusBadGateway)},
		},
	}
	healthy := *account
	healthy.ID = 2
	persisted := *account
	repo := &streamErrorPolicyAccountRepo{mockAccountRepoForGemini: mockAccountRepoForGemini{
		accountsByID: map[int64]*Account{account.ID: &persisted, healthy.ID: &healthy},
	}}
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	rateLimits := NewRateLimitService(repo, nil, cfg, nil, nil)
	svc := &OpenAIGatewayService{cfg: cfg, rateLimitService: rateLimits}
	rateLimits.SetAccountRuntimeBlocker(svc)
	return svc, account, repo
}

func streamErrorPolicyEvent(eventType, payload string) string {
	return "event: " + eventType + "\ndata: " + payload + "\n\n"
}

func runStreamErrorPolicy(t *testing.T, svc *OpenAIGatewayService, account *Account, path, body string) (*OpenAIUsage, *httptest.ResponseRecorder, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"rid-stream-policy"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
	defer func() {
		require.NoError(t, resp.Body.Close())
	}()
	if path == "passthrough" {
		result, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
		if result != nil {
			return result.usage, recorder, err
		}
		return nil, recorder, err
	}
	result, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
	if result != nil {
		return result.usage, recorder, err
	}
	return nil, recorder, err
}

// TestOpenAIStreamErrorPolicy_CreditExhaustionStopsScheduling 复现用户错误，并验证持久化状态与后续调度资格。
// @param t 测试上下文。
// @return 无返回值。
func TestOpenAIStreamErrorPolicy_CreditExhaustionStopsScheduling(t *testing.T) {
	for _, path := range []string{"native", "passthrough"} {
		for _, eventType := range []string{"error", "response.failed"} {
			for _, afterOutput := range []bool{false, true} {
				name := path + "/" + eventType + "/before_output"
				if afterOutput {
					name = path + "/" + eventType + "/after_output"
				}
				t.Run(name, func(t *testing.T) {
					svc, account, repo := newStreamErrorPolicyService(t)
					body := streamErrorPolicyEvent("response.created", `{"type":"response.created","response":{"id":"resp_credit","created_at":1700000000}}`)
					if afterOutput {
						body += streamErrorPolicyEvent("response.output_text.delta", `{"type":"response.output_text.delta","delta":"partial"}`)
					}
					payload := `{"type":"error","sequence_number":2,"error":` + streamPolicyCreditError + `}`
					if eventType == "response.failed" {
						payload = `{"type":"response.failed","response":{"id":"resp_credit","status":"failed","error":` + streamPolicyCreditError + `,"usage":{"input_tokens":9,"output_tokens":2}}}`
					}
					body += streamErrorPolicyEvent(eventType, payload)

					usage, recorder, err := runStreamErrorPolicy(t, svc, account, path, body)

					require.Error(t, err)
					require.Equal(t, StatusError, repo.accountsByID[account.ID].Status)
					require.Contains(t, repo.accountsByID[account.ID].ErrorMessage, "Custom error code 502")
					require.Contains(t, repo.accountsByID[account.ID].ErrorMessage, "no credits remaining")
					require.Equal(t, 1, repo.setErrorCalls)
					require.True(t, account.IsSchedulable(), "模拟错误写入后尚未更新的账号副本")
					require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
					require.Nil(t, svc.resolveFreshSchedulableOpenAIAccount(context.Background(), repo.accountsByID[account.ID], PlatformOpenAI, "model", false, ""))
					healthy := repo.accountsByID[2]
					require.NotNil(t, svc.resolveFreshSchedulableOpenAIAccount(context.Background(), healthy, PlatformOpenAI, "model", false, ""))
					require.Equal(t, StatusActive, healthy.Status)

					var failoverErr *UpstreamFailoverError
					require.Equal(t, !afterOutput, errors.As(err, &failoverErr))
					if !afterOutput {
						require.Empty(t, recorder.Body.String(), "首输出前换号不能先泄露错误帧")
					} else {
						require.Contains(t, recorder.Body.String(), "partial")
						if eventType == "response.failed" {
							require.NotNil(t, usage)
							require.Equal(t, 9, usage.InputTokens)
							require.Equal(t, 2, usage.OutputTokens)
						}
					}
				})
			}
		}
	}
}

// TestOpenAIStreamErrorPolicy_CustomCodeFilter 验证开关、列表和池模式仍由现有账号策略决定。
// @param t 测试上下文。
// @return 无返回值。
func TestOpenAIStreamErrorPolicy_CustomCodeFilter(t *testing.T) {
	for _, path := range []string{"native", "passthrough"} {
		for _, tc := range []struct {
			name      string
			enabled   bool
			codes     []any
			poolMode  bool
			wantError bool
		}{
			{name: "disabled", codes: []any{float64(502)}},
			{name: "unselected", enabled: true, codes: []any{float64(429)}},
			{name: "empty_list", enabled: true, codes: []any{}, wantError: true},
			{name: "pool_explicit_policy", enabled: true, codes: []any{float64(502)}, poolMode: true, wantError: true},
			{name: "pool_unselected", enabled: true, codes: []any{float64(429)}, poolMode: true},
		} {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				svc, account, repo := newStreamErrorPolicyService(t)
				account.Credentials["custom_error_codes_enabled"] = tc.enabled
				account.Credentials["custom_error_codes"] = tc.codes
				account.Credentials["pool_mode"] = tc.poolMode
				if tc.poolMode {
					account.Credentials["pool_mode_retry_status_codes"] = []any{float64(http.StatusBadGateway)}
				}
				body := streamErrorPolicyEvent("error", `{"type":"error","error":`+streamPolicyCreditError+`}`)
				_, _, err := runStreamErrorPolicy(t, svc, account, path, body)
				require.Error(t, err)
				require.Equal(t, tc.wantError, repo.accountsByID[account.ID].Status == StatusError)
				require.Equal(t, tc.wantError, svc.isOpenAIAccountRuntimeBlocked(account))
				var failoverErr *UpstreamFailoverError
				require.ErrorAs(t, err, &failoverErr)
				require.Equal(t, tc.poolMode && !tc.wantError, failoverErr.RetryableOnSameAccount)
			})
		}
	}
}

// TestOpenAIStreamErrorPolicy_BareErrorUsesPolicy 覆盖不触发换号的裸错误，防止只修复换号入口。
// @param t 测试上下文。
// @return 无返回值。
func TestOpenAIStreamErrorPolicy_BareErrorUsesPolicy(t *testing.T) {
	for _, path := range []string{"native", "passthrough"} {
		for _, afterOutput := range []bool{false, true} {
			name := path + "/before_output"
			if afterOutput {
				name = path + "/after_output"
			}
			t.Run(name, func(t *testing.T) {
				svc, account, repo := newStreamErrorPolicyService(t)
				body := ""
				if afterOutput {
					body = streamErrorPolicyEvent("response.output_text.delta", `{"type":"response.output_text.delta","delta":"partial"}`)
				}
				body += streamErrorPolicyEvent("error", `{"type":"error","error":{"code":"upstream_error","message":"upstream rejected request"}}`)
				_, recorder, err := runStreamErrorPolicy(t, svc, account, path, body)
				require.Error(t, err)
				var failoverErr *UpstreamFailoverError
				require.False(t, errors.As(err, &failoverErr), "停止规则不应自行改变裸错误的保守换号判定")
				require.Equal(t, StatusError, repo.accountsByID[account.ID].Status)
				require.Equal(t, 1, repo.setErrorCalls)
				require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
				if afterOutput {
					require.Contains(t, recorder.Body.String(), "partial")
				}
			})
		}
	}
}

// TestOpenAIStreamErrorPolicy_RequestScopedErrors 不把监控兜底的 502 当成账号故障。
// @param t 测试上下文。
// @return 无返回值。
func TestOpenAIStreamErrorPolicy_RequestScopedErrors(t *testing.T) {
	for _, path := range []string{"native", "passthrough"} {
		for _, afterOutput := range []bool{false, true} {
			for _, payload := range []string{
				`{"type":"response.failed","error":{"type":"invalid_request_error","code":"unknown_parameter","message":"Unknown parameter: foo"}}`,
				`{"type":"response.failed","error":{"type":"invalid_request_error","code":"context_length_exceeded","message":"Input exceeds the context window"}}`,
				`{"type":"response.failed","error":{"type":"content_policy_violation","code":"content_policy","message":"Request blocked by policy"}}`,
				`{"type":"response.failed","error":{"code":"server_is_overloaded","message":"Selected model is at capacity. Please try a different model."}}`,
				`{"type":"response.failed","error":{"type":"permission_error","code":"request_forbidden","message":"Access denied for this request"}}`,
			} {
				name := path + "/before_output/" + payload
				if afterOutput {
					name = path + "/after_output/" + payload
				}
				t.Run(name, func(t *testing.T) {
					svc, account, repo := newStreamErrorPolicyService(t)
					body := ""
					if afterOutput {
						body = streamErrorPolicyEvent("response.output_text.delta", `{"type":"response.output_text.delta","delta":"partial"}`)
					}
					body += streamErrorPolicyEvent("response.failed", payload)
					_, _, err := runStreamErrorPolicy(t, svc, account, path, body)
					require.Error(t, err)
					require.Equal(t, StatusActive, repo.accountsByID[account.ID].Status)
					require.Zero(t, repo.setErrorCalls)
					require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
				})
			}
		}
	}
}

// TestOpenAIStreamErrorPolicy_AuthoritativeTerminal 验证延迟处理裸错误不会重复停用或覆盖后续成功。
// @param t 测试上下文。
// @return 无返回值。
func TestOpenAIStreamErrorPolicy_AuthoritativeTerminal(t *testing.T) {
	for _, afterOutput := range []bool{false, true} {
		for _, terminal := range []string{"response.failed", "response.completed", "eof"} {
			name := terminal + "/before_output"
			if afterOutput {
				name = terminal + "/after_output"
			}
			t.Run(name, func(t *testing.T) {
				svc, account, repo := newStreamErrorPolicyService(t)
				body := ""
				if afterOutput {
					body = streamErrorPolicyEvent("response.output_text.delta", `{"type":"response.output_text.delta","delta":"partial"}`)
				}
				body += streamErrorPolicyEvent("error", `{"type":"error","error":{"code":"upstream_error","message":"upstream rejected request"}}`)
				switch terminal {
				case "response.failed":
					body += streamErrorPolicyEvent(terminal, `{"type":"response.failed","response":{"id":"resp_failed","status":"failed","error":{"code":"upstream_error","message":"upstream rejected request"},"usage":{"input_tokens":9,"output_tokens":2}}}`)
				case "response.completed":
					body += streamErrorPolicyEvent(terminal, `{"type":"response.completed","response":{"id":"resp_completed","status":"completed","output":[],"usage":{"input_tokens":7,"output_tokens":3}}}`)
				}
				usage, _, err := runStreamErrorPolicy(t, svc, account, "passthrough", body)
				if terminal == "response.completed" {
					require.NoError(t, err)
					require.Equal(t, StatusActive, repo.accountsByID[account.ID].Status)
					require.Zero(t, repo.setErrorCalls)
					require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
					require.Equal(t, 7, usage.InputTokens)
					require.Equal(t, 3, usage.OutputTokens)
					return
				}
				require.Error(t, err)
				require.Equal(t, StatusError, repo.accountsByID[account.ID].Status)
				require.Equal(t, 1, repo.setErrorCalls)
				require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
				if terminal == "response.failed" && afterOutput {
					require.Equal(t, 9, usage.InputTokens)
					require.Equal(t, 2, usage.OutputTokens)
				}
			})
		}
	}
}

// TestOpenAIStreamErrorPolicy_CreditFailover 只用结构化配额错误确认余额耗尽，避免根据回显文本换号。
// @param t 测试上下文。
// @return 无返回值。
func TestOpenAIStreamErrorPolicy_CreditFailover(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		want    bool
	}{
		{name: "nested_error", payload: `{"type":"error","error":` + streamPolicyCreditError + `}`, want: true},
		{name: "nested_response", payload: `{"type":"response.failed","response":{"error":` + streamPolicyCreditError + `}}`, want: true},
		{name: "top_level_code", payload: `{"type":"error","code":"credit_balance_exhausted","message":"No credits"}`, want: true},
		{name: "quota_code", payload: `{"type":"error","error":{"code":"insufficient_quota","message":"Quota exhausted"}}`, want: true},
		{name: "quota_type", payload: `{"type":"error","error":{"type":"insufficient_quota","message":"Quota exhausted"}}`, want: true},
		{name: "message_only", payload: `{"type":"error","error":{"message":"You have no credits remaining."}}`},
		{name: "echoed_request", payload: `{"type":"error","error":{"type":"invalid_request_error","message":"Unknown parameter: credit_balance_exhausted"}}`},
		{name: "invalid_json", payload: `{"type":"error","error":{"code":"credit_balance_exhausted"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := []byte(tc.payload)
			require.Equal(t, tc.want, openAIStreamErrorEventShouldFailover(payload, extractOpenAISSEErrorMessage(payload)))
			if tc.want {
				require.Equal(t, http.StatusBadGateway, openAIStreamFailureStatus(payload, extractOpenAISSEErrorMessage(payload)))
			}
		})
	}
}

// TestOpenAIStreamErrorPolicy_NonStreamingSSE 保证共用分类的 SSE 转 JSON 路径也能停用余额耗尽账号。
// @param t 测试上下文。
// @return 无返回值。
func TestOpenAIStreamErrorPolicy_NonStreamingSSE(t *testing.T) {
	for _, path := range []string{"native", "passthrough"} {
		t.Run(path, func(t *testing.T) {
			svc, account, repo := newStreamErrorPolicyService(t)
			c, recorder := newNonStreamingFailoverContext(t)
			resp := newNonStreamingSSEResponse()
			body := sseTerminalBody("error", `{"type":"error","error":`+streamPolicyCreditError+`}`)
			var err error
			if path == "passthrough" {
				_, err = svc.handlePassthroughSSEToJSON(resp, c, account, body, "model", "model")
			} else {
				_, err = svc.handleSSEToJSON(resp, c, account, body, "model", "model")
			}
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, StatusError, repo.accountsByID[account.ID].Status)
			require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
			require.Empty(t, recorder.Body.String())
		})
	}
}

// TestOpenAIStreamErrorPolicy_SyntheticFailureKeepsAccount 排除没有真实上游错误载荷的本地断流错误。
// @param t 测试上下文。
// @return 无返回值。
func TestOpenAIStreamErrorPolicy_SyntheticFailureKeepsAccount(t *testing.T) {
	svc, account, repo := newStreamErrorPolicyService(t)
	err := svc.newOpenAIStreamFailoverError(nil, account, false, "rid-disconnect", nil, "OpenAI stream ended before a terminal event")
	require.NotNil(t, err)
	require.Equal(t, StatusActive, repo.accountsByID[account.ID].Status)
	require.Zero(t, repo.setErrorCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}
