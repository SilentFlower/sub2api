package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	defaultOpenAIResponsesLiteHeaderBlockedModelsJSON = `["gpt-5.4","gpt-5.4-mini","gpt-5.5"]`
	openAIResponsesLiteHeaderPolicyCacheTTL           = 60 * time.Second
	openAIResponsesLiteHeaderPolicyErrorTTL           = 5 * time.Second
	openAIResponsesLiteHeaderPolicyDBTimeout          = 5 * time.Second
	openAIResponsesLiteHeaderPolicyRefreshKey         = "openai_responses_lite_header_policy"
)

var defaultOpenAIResponsesLiteHeaderBlockedModels = []string{
	"gpt-5.4",
	"gpt-5.4-mini",
	"gpt-5.5",
}

type cachedOpenAIResponsesLiteHeaderPolicy struct {
	blockedModels []string
	expiresAt     int64
}

func cloneOpenAIResponsesLiteHeaderBlockedModels(models []string) []string {
	if len(models) == 0 {
		return []string{}
	}
	return append([]string(nil), models...)
}

func defaultOpenAIResponsesLiteHeaderBlockedModelsCopy() []string {
	return cloneOpenAIResponsesLiteHeaderBlockedModels(defaultOpenAIResponsesLiteHeaderBlockedModels)
}

// NormalizeOpenAIResponsesLiteHeaderBlockedModels 归一化并校验 Responses Lite 阻止模型规则。
//
// @param models 原始模型规则列表。
// @return 去除首尾空白、稳定去重后的规则列表；规则非法时返回错误。
func NormalizeOpenAIResponsesLiteHeaderBlockedModels(models []string) ([]string, error) {
	normalized := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, raw := range models {
		model := strings.TrimSpace(raw)
		if model == "" {
			return nil, errors.New("responses Lite blocked model rule must not be empty")
		}
		if wildcardCount := strings.Count(model, "*"); wildcardCount > 1 ||
			(wildcardCount == 1 && !strings.HasSuffix(model, "*")) {
			return nil, fmt.Errorf("responses Lite blocked model rule %q only supports one trailing wildcard", model)
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		normalized = append(normalized, model)
	}
	return normalized, nil
}

func parseOpenAIResponsesLiteHeaderBlockedModelsSetting(raw string, exists bool) ([]string, error) {
	if !exists {
		return defaultOpenAIResponsesLiteHeaderBlockedModelsCopy(), nil
	}
	var models []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &models); err != nil {
		return nil, fmt.Errorf("parse responses Lite blocked models: %w", err)
	}
	if models == nil {
		return nil, errors.New("responses Lite blocked models must be a JSON array")
	}
	return NormalizeOpenAIResponsesLiteHeaderBlockedModels(models)
}

func (s *SettingService) getOpenAIResponsesLiteHeaderBlockedModelsCached(ctx context.Context) []string {
	fallback := defaultOpenAIResponsesLiteHeaderBlockedModelsCopy()
	if s == nil || s.settingRepo == nil {
		return fallback
	}
	if cached, ok := s.openAIResponsesLiteHeaderPolicyCache.Load().(*cachedOpenAIResponsesLiteHeaderPolicy); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cloneOpenAIResponsesLiteHeaderBlockedModels(cached.blockedModels)
		}
	}

	result, _, _ := s.openAIResponsesLiteHeaderPolicySF.Do(openAIResponsesLiteHeaderPolicyRefreshKey, func() (any, error) {
		if cached, ok := s.openAIResponsesLiteHeaderPolicyCache.Load().(*cachedOpenAIResponsesLiteHeaderPolicy); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.blockedModels, nil
			}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIResponsesLiteHeaderPolicyDBTimeout)
		defer cancel()

		raw, err := s.settingRepo.GetValue(dbCtx, SettingKeyOpenAIResponsesLiteHeaderBlockedModels)
		exists := err == nil
		if err != nil && !errors.Is(err, ErrSettingNotFound) {
			slog.Warn("读取 Responses Lite 阻止模型配置失败，回退默认值", "error", err)
			entry := &cachedOpenAIResponsesLiteHeaderPolicy{
				blockedModels: fallback,
				expiresAt:     time.Now().Add(openAIResponsesLiteHeaderPolicyErrorTTL).UnixNano(),
			}
			s.openAIResponsesLiteHeaderPolicyCache.Store(entry)
			return entry.blockedModels, nil
		}

		models, parseErr := parseOpenAIResponsesLiteHeaderBlockedModelsSetting(raw, exists)
		if parseErr != nil {
			slog.Warn("Responses Lite 阻止模型配置非法，回退默认值",
				"setting_key", SettingKeyOpenAIResponsesLiteHeaderBlockedModels,
				"error", parseErr,
			)
			models = fallback
		}
		ttl := openAIResponsesLiteHeaderPolicyCacheTTL
		if parseErr != nil {
			ttl = openAIResponsesLiteHeaderPolicyErrorTTL
		}
		entry := &cachedOpenAIResponsesLiteHeaderPolicy{
			blockedModels: cloneOpenAIResponsesLiteHeaderBlockedModels(models),
			expiresAt:     time.Now().Add(ttl).UnixNano(),
		}
		s.openAIResponsesLiteHeaderPolicyCache.Store(entry)
		return entry.blockedModels, nil
	})
	if models, ok := result.([]string); ok {
		return cloneOpenAIResponsesLiteHeaderBlockedModels(models)
	}
	return fallback
}

// ShouldBlockOpenAIResponsesLite 判断最终上游模型是否禁止透传 Responses Lite 协议标记。
//
// @param ctx 请求上下文。
// @param finalModel 完成账号映射和上游归一化后的最终模型。
// @return 命中精确规则或末尾通配符规则时返回 true。
func (s *SettingService) ShouldBlockOpenAIResponsesLite(ctx context.Context, finalModel string) bool {
	finalModel = strings.TrimSpace(finalModel)
	if finalModel == "" {
		return false
	}
	for _, pattern := range s.getOpenAIResponsesLiteHeaderBlockedModelsCached(ctx) {
		if matchModelPattern(pattern, finalModel) {
			return true
		}
	}
	return false
}

func (s *OpenAIGatewayService) resolveOpenAIResponsesLitePolicyModel(
	ctx context.Context,
	account *Account,
	requestedModel string,
	compact bool,
) string {
	if account == nil {
		return strings.TrimSpace(requestedModel)
	}

	finalModel := resolveOpenAIAccountUpstreamModelForRequest(account, requestedModel, compact)
	if !compact && isOpenAIImageGenerationModel(finalModel) {
		finalModel = s.openAIImageGenerationMainModel(ctx)
	}
	return strings.TrimSpace(finalModel)
}

func (s *OpenAIGatewayService) shouldForwardOpenAIResponsesLite(
	ctx context.Context,
	account *Account,
	finalModel string,
) bool {
	if account == nil || account.Platform != PlatformOpenAI {
		return false
	}
	var settingService *SettingService
	if s != nil {
		settingService = s.settingService
	}
	return !settingService.ShouldBlockOpenAIResponsesLite(ctx, finalModel)
}

func (s *OpenAIGatewayService) applyOpenAIResponsesLiteHTTPBodyPolicy(
	ctx context.Context,
	account *Account,
	body []byte,
	finalModel string,
	headerValue string,
) ([]byte, bool, error) {
	if !isOpenAIResponsesLiteHeader(headerValue) {
		return body, false, nil
	}
	if !s.shouldForwardOpenAIResponsesLite(ctx, account, finalModel) {
		return body, false, nil
	}

	normalized, _, err := normalizeOpenAIResponsesLiteToolsPayload(body)
	if err != nil {
		return body, false, err
	}
	return normalized, true, nil
}

func (s *OpenAIGatewayService) applyOpenAIResponsesLiteHTTPIngressPolicy(
	ctx context.Context,
	account *Account,
	body []byte,
	headerValue string,
	compact bool,
) ([]byte, error) {
	if !isOpenAIResponsesLiteHeader(headerValue) {
		return body, nil
	}
	finalModel := s.resolveOpenAIResponsesLitePolicyModel(
		ctx,
		account,
		strings.TrimSpace(gjson.GetBytes(body, "model").String()),
		compact,
	)
	updated, _, err := s.applyOpenAIResponsesLiteHTTPBodyPolicy(ctx, account, body, finalModel, headerValue)
	return updated, err
}

func (s *OpenAIGatewayService) applyOpenAIResponsesLiteWebSocketPolicy(
	ctx context.Context,
	account *Account,
	body []byte,
	finalModel string,
) ([]byte, bool, error) {
	if !isOpenAIResponsesLiteWebSocketPayload(body) {
		return body, false, nil
	}
	if !s.shouldForwardOpenAIResponsesLite(ctx, account, finalModel) {
		stripped, err := stripOpenAIResponsesLiteWebSocketMetadata(body)
		return stripped, false, err
	}

	normalized, _, err := normalizeOpenAIResponsesLiteToolsPayload(body)
	if err != nil {
		return body, false, err
	}
	return normalized, true, nil
}

func (s *OpenAIGatewayService) applyOpenAIResponsesLiteWebSocketPayloadPolicy(
	ctx context.Context,
	account *Account,
	body []byte,
	finalModel string,
) ([]byte, error) {
	updated, _, err := s.applyOpenAIResponsesLiteWebSocketPolicy(ctx, account, body, finalModel)
	return updated, err
}

func (s *OpenAIGatewayService) applyOpenAIResponsesLiteWSHTTPBridgePolicy(
	ctx context.Context,
	req *http.Request,
	account *Account,
	wsPayload []byte,
	httpBody []byte,
	originalModel string,
) {
	if req == nil {
		return
	}
	finalModel := strings.TrimSpace(gjson.GetBytes(httpBody, "model").String())
	if finalModel == "" && originalModel != "" {
		finalModel = normalizeOpenAIModelForUpstream(account, account.GetMappedModel(originalModel))
	}
	if isOpenAIResponsesLiteWebSocketPayload(wsPayload) && s.shouldForwardOpenAIResponsesLite(ctx, account, finalModel) {
		req.Header.Set(responsesLiteHeader, "true")
		return
	}
	req.Header.Del(responsesLiteHeader)
}

func stripOpenAIResponsesLiteWebSocketMetadata(body []byte) ([]byte, error) {
	updated, err := sjson.DeleteBytes(body, "client_metadata."+responsesLiteWSMetadataKey)
	if err != nil {
		return body, fmt.Errorf("delete responses Lite websocket metadata: %w", err)
	}
	metadata := gjson.GetBytes(updated, "client_metadata")
	if metadata.IsObject() && len(metadata.Map()) == 0 {
		updated, err = sjson.DeleteBytes(updated, "client_metadata")
		if err != nil {
			return body, fmt.Errorf("delete empty websocket client metadata: %w", err)
		}
	}
	return updated, nil
}

func (s *OpenAIGatewayService) enforceOpenAIResponsesLiteHTTPHeader(
	ctx context.Context,
	req *http.Request,
	account *Account,
	finalModel string,
) {
	if req == nil || !isOpenAIResponsesLiteHeader(req.Header.Get(responsesLiteHeader)) {
		if req != nil {
			req.Header.Del(responsesLiteHeader)
		}
		return
	}
	if !s.shouldForwardOpenAIResponsesLite(ctx, account, finalModel) {
		req.Header.Del(responsesLiteHeader)
		return
	}
	req.Header.Set(responsesLiteHeader, "true")
}
