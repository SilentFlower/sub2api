package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
)

const featureKeyCodexWebSearchBridge = "codex_web_search_bridge"

type codexWebSearchBridgeDecision struct {
	Enabled    bool
	Reason     string
	ToolChoice openAIResponsesToolChoiceKind
}

// CodexWebSearchBridgeOverride 返回渠道级 Codex Web Search 桥接覆盖值。
//
// @param platform 渠道平台。
// @return 显式布尔覆盖值；nil 表示未配置。
func (c *Channel) CodexWebSearchBridgeOverride(platform string) *bool {
	if c == nil {
		return nil
	}
	return platformBoolOverride(c.FeaturesConfig, featureKeyCodexWebSearchBridge, platform)
}

// CodexWebSearchBridgeOverride 返回 OpenAI APIKey 账号级 Codex Web Search 桥接覆盖值。
//
// @return 显式布尔覆盖值；nil 表示跟随渠道。
func (a *Account) CodexWebSearchBridgeOverride() *bool {
	if a == nil || !a.IsOpenAIApiKey() || a.Extra == nil {
		return nil
	}
	if override := boolOverrideFromMap(a.Extra, featureKeyCodexWebSearchBridge); override != nil {
		return override
	}
	openaiConfig, _ := a.Extra[PlatformOpenAI].(map[string]any)
	return boolOverrideFromMap(openaiConfig, featureKeyCodexWebSearchBridge)
}

func (s *OpenAIGatewayService) isCodexWebSearchBridgeEnabled(
	ctx context.Context,
	c *gin.Context,
	account *Account,
) bool {
	if override := account.CodexWebSearchBridgeOverride(); override != nil {
		return *override
	}
	if s == nil || s.channelService == nil {
		return false
	}
	apiKey := getAPIKeyFromContext(c)
	if apiKey == nil || apiKey.GroupID == nil {
		return false
	}
	channel, err := s.channelService.GetChannelForGroup(ctx, *apiKey.GroupID)
	if err != nil || channel == nil {
		return false
	}
	override := channel.CodexWebSearchBridgeOverride(PlatformOpenAI)
	return override != nil && *override
}

func (s *OpenAIGatewayService) resolveCodexWebSearchBridgeDecision(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	choice openAIResponsesToolChoiceKind,
	hasExplicitWebTool bool,
	hasClientTool bool,
) codexWebSearchBridgeDecision {
	decision := codexWebSearchBridgeDecision{Reason: "not_eligible", ToolChoice: choice}
	if account == nil || !account.IsOpenAIApiKey() || openai_compat.ShouldUseResponsesAPI(account.Extra) {
		decision.Reason = "not_chat_fallback"
		return decision
	}
	if c == nil || GetOpenAIClientTransport(c) != OpenAIClientTransportHTTP || isOpenAIResponsesCompactPath(c) {
		decision.Reason = "unsupported_transport"
		return decision
	}
	if !isOpenAIResponsesLiteHeader(c.GetHeader(responsesLiteHeader)) {
		decision.Reason = "not_responses_lite"
		return decision
	}
	isCodexClient := openai.IsCodexOfficialClientByHeaders(c.GetHeader("User-Agent"), c.GetHeader("originator"))
	if !isCodexClient && (s == nil || s.cfg == nil || !s.cfg.Gateway.ForceCodexCLI) {
		decision.Reason = "not_codex_client"
		return decision
	}
	if hasExplicitWebTool {
		decision.Reason = "explicit_web_tool"
		return decision
	}
	switch choice {
	case openAIResponsesToolChoiceAbsent, openAIResponsesToolChoiceAuto:
	case openAIResponsesToolChoiceRequired:
		if !hasClientTool {
			decision.Reason = "required_without_client_tool"
			return decision
		}
	default:
		decision.Reason = "tool_choice_excluded"
		return decision
	}
	if !s.isCodexWebSearchBridgeEnabled(ctx, c, account) {
		decision.Reason = "bridge_disabled"
		return decision
	}
	if !s.isOpenAIWebSearchEmulationEnabled(ctx, c, account) {
		decision.Reason = "web_search_policy_disabled"
		return decision
	}
	manager := getWebSearchManager()
	if s.settingService == nil || !s.settingService.IsWebSearchEmulationEnabled(ctx) ||
		manager == nil || !manager.HasAvailableProvider(ctx, resolveAccountProxyURL(account)) {
		decision.Reason = "provider_unavailable"
		return decision
	}
	decision.Enabled = true
	decision.Reason = "enabled"
	return decision
}

func defaultCodexWebSearchBridgeToolConfig() openAIResponsesInternalWebToolConfig {
	return openAIResponsesInternalWebToolConfig{
		Name:       openAIResponsesTypedWebSearchToolName,
		Kind:       openAIResponsesInternalWebToolTypedSearch,
		MaxResults: webSearchDefaultMaxResults,
		MaxRounds:  openAIResponsesWebRunMaxRounds,
	}
}
