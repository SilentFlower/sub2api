package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGatewayService_BuildUpstreamRequest_AppliesAnthropicCCHFixedMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("User-Agent", "claude-cli/2.3.4 (external, cli)")

	body := []byte(`{"model":"claude-3-7-sonnet-20250219","system":[{"type":"text","text":"Original system"},{"type":"text","text":"x-anthropic-billing-header: cc_version=0.0.0; cc_entrypoint=cli; cch=abcde;"}],"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
	svc := &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				MaxLineSize: defaultMaxLineSize,
			},
		},
	}
	account := &Account{
		ID:          401,
		Name:        "anthropic-oauth-cch-fixed",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "oauth-token",
		},
		Extra: map[string]any{
			"anthropic_cch_enabled":            true,
			"anthropic_cch_mode":               anthropicCCHModeFixed,
			"anthropic_cch_fixed_version":      "2.1.90",
			"anthropic_cch_rewrite_user_agent": true,
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	req, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "oauth-token", "oauth", "claude-3-7-sonnet-20250219", false, false)
	require.NoError(t, err)

	upstreamBody := readRequestBodyForTest(t, req)
	systemText := gjson.GetBytes(upstreamBody, "system.0.text").String()
	require.Equal(t, "claude-cli/2.1.90 (external, cli)", getHeaderRaw(req.Header, "User-Agent"))
	require.Equal(t, getHeaderRaw(req.Header, "x-anthropic-billing-header"), strings.TrimPrefix(systemText, "x-anthropic-billing-header: "))
	require.Contains(t, systemText, "cc_version=2.1.90;")
	require.Contains(t, gjson.GetBytes(upstreamBody, "system.1.text").String(), "Original system")
	require.Len(t, gjson.GetBytes(upstreamBody, "system").Array(), 2)

	placeholderBody, ok := overwriteAnthropicBillingHeaderInBody(upstreamBody, "2.1.90", anthropicCCHPlaceholder)
	require.True(t, ok)
	require.Equal(t, computeAnthropicCCH(placeholderBody), extractCCHFromSystemText(systemText))
}

func TestGatewayService_BuildUpstreamRequest_AppliesAnthropicCCHUserAgentMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("User-Agent", "claude-cli/2.8.6 (external, cli)")

	body := []byte(`{"model":"claude-3-7-sonnet-20250219","system":"Keep this system","messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
	svc := &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				MaxLineSize: defaultMaxLineSize,
			},
		},
	}
	account := &Account{
		ID:          402,
		Name:        "anthropic-oauth-cch-ua",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeSetupToken,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "oauth-token",
		},
		Extra: map[string]any{
			"anthropic_cch_enabled":       true,
			"anthropic_cch_mode":          anthropicCCHModeUserAgent,
			"anthropic_cch_fixed_version": "2.1.90",
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	req, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "oauth-token", "oauth", "claude-3-7-sonnet-20250219", false, false)
	require.NoError(t, err)

	upstreamBody := readRequestBodyForTest(t, req)
	require.Equal(t, "claude-cli/2.8.6 (external, cli)", getHeaderRaw(req.Header, "User-Agent"))
	require.Equal(t, gjson.String, gjson.GetBytes(upstreamBody, "system").Type)
	require.Contains(t, gjson.GetBytes(upstreamBody, "system").String(), "cc_version=2.8.6;")
	require.Contains(t, gjson.GetBytes(upstreamBody, "system").String(), "Keep this system")
	require.Equal(t, "cc_version=2.8.6; cc_entrypoint=cli; cch="+extractCCHFromSystemText(gjson.GetBytes(upstreamBody, "system").String())+";", getHeaderRaw(req.Header, "x-anthropic-billing-header"))
}

func TestGatewayService_BuildUpstreamRequest_AppliesAnthropicCCHWhenSystemMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("User-Agent", "claude-cli/2.3.4 (external, cli)")

	body := []byte(`{"model":"claude-3-7-sonnet-20250219","messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)
	svc := &GatewayService{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				MaxLineSize: defaultMaxLineSize,
			},
		},
	}
	account := &Account{
		ID:          403,
		Name:        "anthropic-oauth-cch-missing-system",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "oauth-token",
		},
		Extra: map[string]any{
			"anthropic_cch_enabled":       true,
			"anthropic_cch_fixed_version": "2.1.90",
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	req, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "oauth-token", "oauth", "claude-3-7-sonnet-20250219", false, false)
	require.NoError(t, err)

	upstreamBody := readRequestBodyForTest(t, req)
	require.True(t, gjson.GetBytes(upstreamBody, "system").IsArray())
	require.Len(t, gjson.GetBytes(upstreamBody, "system").Array(), 1)
	require.Contains(t, gjson.GetBytes(upstreamBody, "system.0.text").String(), "x-anthropic-billing-header:")
}

func readRequestBodyForTest(t *testing.T, req *http.Request) []byte {
	t.Helper()
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))
	return body
}

func extractCCHFromSystemText(text string) string {
	const prefix = "cch="
	idx := strings.Index(text, prefix)
	if idx < 0 || len(text) < idx+len(prefix)+5 {
		return ""
	}
	return text[idx+len(prefix) : idx+len(prefix)+5]
}
