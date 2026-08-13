//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/websearch"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCodexWebSearchBridgeOverridePrecedence(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{featureKeyCodexWebSearchBridge: false},
	}
	channel := &Channel{FeaturesConfig: map[string]any{
		featureKeyCodexWebSearchBridge: map[string]any{PlatformOpenAI: true},
	}}

	require.NotNil(t, account.CodexWebSearchBridgeOverride())
	require.False(t, *account.CodexWebSearchBridgeOverride())
	require.NotNil(t, channel.CodexWebSearchBridgeOverride(PlatformOpenAI))
	require.True(t, *channel.CodexWebSearchBridgeOverride(PlatformOpenAI))

	delete(account.Extra, featureKeyCodexWebSearchBridge)
	require.Nil(t, account.CodexWebSearchBridgeOverride())
	account.Extra[featureKeyCodexWebSearchBridge] = "true"
	require.Nil(t, account.CodexWebSearchBridgeOverride())
	account.Type = AccountTypeSetupToken
	account.Extra[featureKeyCodexWebSearchBridge] = true
	require.Nil(t, account.CodexWebSearchBridgeOverride())
}

func TestResolveCodexWebSearchBridgeDecision(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)

	groupID := int64(92)
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyCodexWebSearchBridge] = true
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled
	svc := &OpenAIGatewayService{settingService: &SettingService{}}

	newContext := func() *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		c.Request.Header.Set("User-Agent", "codex_cli_rs/0.147.0")
		c.Request.Header.Set(responsesLiteHeader, "true")
		c.Set("api_key", &APIKey{GroupID: &groupID})
		SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
		return c
	}

	tests := []struct {
		name          string
		change        func(*gin.Context, *Account)
		choice        openAIResponsesToolChoiceKind
		explicitWeb   bool
		hasClientTool bool
		wantEnabled   bool
		wantReason    string
	}{
		{name: "auto enabled", choice: openAIResponsesToolChoiceAuto, wantEnabled: true, wantReason: "enabled"},
		{name: "required with client tool", choice: openAIResponsesToolChoiceRequired, hasClientTool: true, wantEnabled: true, wantReason: "enabled"},
		{name: "required without client tool", choice: openAIResponsesToolChoiceRequired, wantReason: "required_without_client_tool"},
		{name: "none", choice: openAIResponsesToolChoiceNone, wantReason: "tool_choice_excluded"},
		{name: "explicit web tool", choice: openAIResponsesToolChoiceAuto, explicitWeb: true, wantReason: "explicit_web_tool"},
		{name: "non lite", choice: openAIResponsesToolChoiceAuto, change: func(c *gin.Context, _ *Account) {
			c.Request.Header.Del(responsesLiteHeader)
		}, wantReason: "not_responses_lite"},
		{name: "websocket transport", choice: openAIResponsesToolChoiceAuto, change: func(c *gin.Context, _ *Account) {
			SetOpenAIClientTransport(c, OpenAIClientTransportWS)
		}, wantReason: "unsupported_transport"},
		{name: "compact path", choice: openAIResponsesToolChoiceAuto, change: func(c *gin.Context, _ *Account) {
			c.Request.URL.Path = "/v1/responses/compact"
		}, wantReason: "unsupported_transport"},
		{name: "non codex", choice: openAIResponsesToolChoiceAuto, change: func(c *gin.Context, _ *Account) {
			c.Request.Header.Set("User-Agent", "curl/8.0")
		}, wantReason: "not_codex_client"},
		{name: "native responses", choice: openAIResponsesToolChoiceAuto, change: func(_ *gin.Context, account *Account) {
			account.Extra[openai_compat.ExtraKeyResponsesMode] = string(openai_compat.ResponsesSupportModeForceResponses)
		}, wantReason: "not_chat_fallback"},
		{name: "non openai account", choice: openAIResponsesToolChoiceAuto, change: func(_ *gin.Context, account *Account) {
			account.Platform = PlatformAnthropic
		}, wantReason: "not_chat_fallback"},
		{name: "bridge disabled", choice: openAIResponsesToolChoiceAuto, change: func(_ *gin.Context, account *Account) {
			account.Extra[featureKeyCodexWebSearchBridge] = false
		}, wantReason: "bridge_disabled"},
		{name: "bridge missing", choice: openAIResponsesToolChoiceAuto, change: func(_ *gin.Context, account *Account) {
			delete(account.Extra, featureKeyCodexWebSearchBridge)
		}, wantReason: "bridge_disabled"},
		{name: "search policy disabled", choice: openAIResponsesToolChoiceAuto, change: func(_ *gin.Context, account *Account) {
			account.Extra[featureKeyWebSearchEmulation] = WebSearchModeDisabled
		}, wantReason: "web_search_policy_disabled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newContext()
			candidate := *account
			candidate.Extra = map[string]any{
				openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
				featureKeyCodexWebSearchBridge:      true,
				featureKeyWebSearchEmulation:        WebSearchModeEnabled,
			}
			if tt.change != nil {
				tt.change(c, &candidate)
			}
			decision := svc.resolveCodexWebSearchBridgeDecision(
				context.Background(),
				c,
				&candidate,
				tt.choice,
				tt.explicitWeb,
				tt.hasClientTool,
			)
			require.Equal(t, tt.wantEnabled, decision.Enabled)
			require.Equal(t, tt.wantReason, decision.Reason)
		})
	}
}

func TestResolveCodexWebSearchBridgeDecisionRejectsUnavailableManager(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setGlobalWebSearchConfig(&WebSearchEmulationConfig{
		Enabled:   true,
		Providers: []WebSearchProviderConfig{{Type: websearch.ProviderTypeAnySearch}},
	})
	t.Cleanup(func() {
		SetWebSearchManager(nil)
		clearGlobalWebSearchConfig()
	})

	groupID := int64(92)
	newContext := func() *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		c.Request.Header.Set("User-Agent", "codex_cli_rs/0.147.0")
		c.Request.Header.Set(responsesLiteHeader, "true")
		c.Set("api_key", &APIKey{GroupID: &groupID})
		SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
		return c
	}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyCodexWebSearchBridge] = true
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled
	svc := &OpenAIGatewayService{settingService: &SettingService{}}
	past := time.Now().Add(-time.Hour).Unix()

	tests := []struct {
		name    string
		manager *websearch.Manager
	}{
		{name: "empty manager", manager: websearch.NewManager(nil, nil)},
		{name: "expired provider", manager: websearch.NewManager([]websearch.ProviderConfig{{
			Type: websearch.ProviderTypeBrave, APIKey: "k", ExpiresAt: &past,
		}}, nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetWebSearchManager(tt.manager)
			decision := svc.resolveCodexWebSearchBridgeDecision(
				context.Background(), newContext(), account, openAIResponsesToolChoiceAuto, false, false,
			)
			require.False(t, decision.Enabled)
			require.Equal(t, "provider_unavailable", decision.Reason)
		})
	}
}

func TestCodexWebSearchBridgeFollowsChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)

	groupID := int64(73)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.147.0")
	c.Request.Header.Set(responsesLiteHeader, "true")
	c.Set("api_key", &APIKey{GroupID: &groupID})
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	channel := &Channel{ID: 8, Status: StatusActive, FeaturesConfig: map[string]any{
		featureKeyCodexWebSearchBridge: map[string]any{PlatformOpenAI: true},
		featureKeyWebSearchEmulation:   map[string]any{PlatformOpenAI: true},
	}}
	svc := &OpenAIGatewayService{
		channelService: newChannelServiceWithCache(groupID, channel),
		settingService: &SettingService{},
	}
	account := forceChatResponsesFallbackAccount()

	decision := svc.resolveCodexWebSearchBridgeDecision(
		context.Background(),
		c,
		account,
		openAIResponsesToolChoiceAuto,
		false,
		false,
	)
	require.True(t, decision.Enabled)
	require.Equal(t, "enabled", decision.Reason)

	account.Extra[featureKeyCodexWebSearchBridge] = false
	decision = svc.resolveCodexWebSearchBridgeDecision(
		context.Background(),
		c,
		account,
		openAIResponsesToolChoiceAuto,
		false,
		false,
	)
	require.False(t, decision.Enabled)
	require.Equal(t, "bridge_disabled", decision.Reason)
}

func TestResolveCodexWebSearchBridgeDecisionAllowsForceCodexCLI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-codex-wrapper/1.0")
	c.Request.Header.Set(responsesLiteHeader, "true")
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	cfg := rawChatCompletionsTestConfig()
	cfg.Gateway.ForceCodexCLI = true
	svc := &OpenAIGatewayService{cfg: cfg, settingService: &SettingService{}}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyCodexWebSearchBridge] = true
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	decision := svc.resolveCodexWebSearchBridgeDecision(
		context.Background(),
		c,
		account,
		openAIResponsesToolChoiceAuto,
		false,
		false,
	)
	require.True(t, decision.Enabled)
	require.Equal(t, "enabled", decision.Reason)
}
