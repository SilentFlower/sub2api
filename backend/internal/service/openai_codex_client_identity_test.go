package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAICodexClientIdentity(t *testing.T) {
	require.Equal(t, "0.144.1", openAICodexClientVersion)
	require.Equal(t, openAICodexClientVersion, codexCLIVersion)
	require.Equal(t, openAICodexClientVersion, openAICodexProbeVersion)
	require.Contains(t, codexCLIUserAgent, "/"+openAICodexClientVersion)
	require.Equal(t, 2, strings.Count(DefaultOpenAICodexUserAgent, openAICodexClientVersion))
	require.NotContains(t, codexCLIUserAgent, "0.125.0")
	require.NotContains(t, DefaultOpenAICodexUserAgent, "0.125.0")
}

func TestOpenAIGatewayService_BuildOpenAIWSHeadersPreservesUserAgentPriority(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name          string
		accountUA     string
		requestUA     string
		forceCodex    bool
		wantUserAgent string
	}{
		{
			name:          "非 Codex OAuth 请求使用内置身份兜底",
			requestUA:     "curl/8.0",
			wantUserAgent: codexCLIUserAgent,
		},
		{
			name:          "账号自定义 Codex UA 保持优先",
			accountUA:     "codex_cli_rs/9.9.9 custom",
			requestUA:     "curl/8.0",
			wantUserAgent: "codex_cli_rs/9.9.9 custom",
		},
		{
			name:          "强制 Codex 覆盖账号自定义 UA",
			accountUA:     "codex_cli_rs/9.9.9 custom",
			requestUA:     "curl/8.0",
			forceCodex:    true,
			wantUserAgent: codexCLIUserAgent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("User-Agent", tt.requestUA)

			credentials := map[string]any{"chatgpt_account_id": "chatgpt-acc"}
			if tt.accountUA != "" {
				credentials["user_agent"] = tt.accountUA
			}
			account := &Account{
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Credentials: credentials,
			}
			svc := &OpenAIGatewayService{
				cfg: &config.Config{Gateway: config.GatewayConfig{ForceCodexCLI: tt.forceCodex}},
			}

			headers, _, err := svc.buildOpenAIWSHeaders(
				context.Background(),
				c,
				account,
				"oauth-token",
				OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
				false,
				"",
				"",
				"",
			)
			require.NoError(t, err)
			require.Equal(t, tt.wantUserAgent, headers.Get("User-Agent"))
		})
	}
}
