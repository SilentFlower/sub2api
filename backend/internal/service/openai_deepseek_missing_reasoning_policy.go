package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

const (
	deepSeekMissingReasoningPolicyCacheTTL   = 60 * time.Second
	deepSeekMissingReasoningPolicyErrorTTL   = 5 * time.Second
	deepSeekMissingReasoningPolicyDBTimeout  = 5 * time.Second
	deepSeekMissingReasoningPolicyRefreshKey = "deepseek_missing_reasoning_policy"

	deepSeekMissingReasoningSourceChatCompletions    = "chat_completions"
	deepSeekMissingReasoningSourceResponsesFallback  = "responses_chat_fallback"
	deepSeekMissingReasoningSourceResponsesWebRun    = "responses_web_run"
	deepSeekMissingReasoningSourceAnthropicFallback  = "anthropic_chat_fallback"
	deepSeekMissingReasoningReasonAssistantToolCalls = "assistant_tool_calls_missing_reasoning"
)

type cachedDeepSeekMissingReasoningPolicy struct {
	enabled   bool
	expiresAt int64
}

type deepSeekMissingReasoningPolicyResult struct {
	body         []byte
	changed      bool
	missingCount int
}

// IsDeepSeekMissingReasoningAutoDowngradeEnabled 判断 DeepSeek 缺失推理内容自动降级是否开启。
//
// @param ctx 请求上下文。
// @return 配置缺失或读取失败时返回默认值 true。
func (s *SettingService) IsDeepSeekMissingReasoningAutoDowngradeEnabled(ctx context.Context) bool {
	if s == nil || s.settingRepo == nil {
		return true
	}
	if cached := s.deepSeekMissingReasoningPolicyCache.Load(); cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.enabled
		}
	}

	result, _, _ := s.deepSeekMissingReasoningPolicySF.Do(deepSeekMissingReasoningPolicyRefreshKey, func() (any, error) {
		expected := s.deepSeekMissingReasoningPolicyCache.Load()
		if expected != nil {
			if time.Now().UnixNano() < expected.expiresAt {
				return expected.enabled, nil
			}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), deepSeekMissingReasoningPolicyDBTimeout)
		defer cancel()

		raw, err := s.settingRepo.GetValue(dbCtx, SettingKeyEnableDeepSeekMissingReasoningAutoDowngrade)
		enabled := true
		ttl := deepSeekMissingReasoningPolicyCacheTTL
		if err == nil {
			raw = strings.TrimSpace(raw)
			if raw != "" {
				enabled = raw == "true"
			}
		} else if !errors.Is(err, ErrSettingNotFound) {
			slog.Warn("读取 DeepSeek 缺失推理内容自动降级配置失败，回退默认开启", "error", err)
			ttl = deepSeekMissingReasoningPolicyErrorTTL
		}

		entry := &cachedDeepSeekMissingReasoningPolicy{
			enabled:   enabled,
			expiresAt: time.Now().Add(ttl).UnixNano(),
		}
		// 保存设置会直接替换缓存。CAS 可以阻止更早开始的数据库读取在稍后完成时覆盖新值。
		if !s.deepSeekMissingReasoningPolicyCache.CompareAndSwap(expected, entry) {
			if current := s.deepSeekMissingReasoningPolicyCache.Load(); current != nil {
				return current.enabled, nil
			}
		}
		return enabled, nil
	})
	if enabled, ok := result.(bool); ok {
		return enabled
	}
	return true
}

func isDeepSeekUpstreamModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "deepseek-")
}

func hasUsableDeepSeekReasoning(message gjson.Result) bool {
	for _, field := range []string{"reasoning_content", "reasoning"} {
		value := message.Get(field)
		if value.Type == gjson.String && strings.TrimSpace(value.String()) != "" {
			return true
		}
	}
	return false
}

func applyDeepSeekMissingReasoningPolicy(body []byte, upstreamModel string, enabled bool) (deepSeekMissingReasoningPolicyResult, error) {
	result := deepSeekMissingReasoningPolicyResult{body: body}
	if !enabled || !isDeepSeekUpstreamModel(upstreamModel) {
		return result, nil
	}
	if !gjson.ValidBytes(body) {
		return result, errors.New("invalid chat completions request JSON")
	}

	for _, message := range gjson.GetBytes(body, "messages").Array() {
		if message.Get("role").String() != "assistant" {
			continue
		}
		toolCalls := message.Get("tool_calls")
		if !toolCalls.IsArray() || len(toolCalls.Array()) == 0 {
			continue
		}
		if !hasUsableDeepSeekReasoning(message) {
			result.missingCount++
		}
	}
	if result.missingCount == 0 {
		return result, nil
	}

	root := gjson.ParseBytes(body)
	thinkingType := root.Get("thinking.type")
	thinkingDisabled := thinkingType.Type == gjson.String && strings.EqualFold(strings.TrimSpace(thinkingType.String()), "disabled")
	reasoningEffortExists := root.Get("reasoning_effort").Exists()
	if thinkingDisabled && !reasoningEffortExists {
		return result, nil
	}

	updated := body
	var err error
	if !thinkingDisabled {
		updated, err = sjson.SetBytes(updated, "thinking.type", "disabled")
		if err != nil {
			return result, fmt.Errorf("disable DeepSeek thinking: %w", err)
		}
	}
	if reasoningEffortExists {
		updated, err = sjson.DeleteBytes(updated, "reasoning_effort")
		if err != nil {
			return result, fmt.Errorf("remove DeepSeek reasoning_effort: %w", err)
		}
	}

	result.body = updated
	result.changed = true
	return result, nil
}

func (s *OpenAIGatewayService) applyDeepSeekMissingReasoningAutoDowngrade(
	ctx context.Context,
	account *Account,
	upstreamModel string,
	body []byte,
	sourcePath string,
) ([]byte, error) {
	var settingService *SettingService
	if s != nil {
		settingService = s.settingService
	}
	result, err := applyDeepSeekMissingReasoningPolicy(
		body,
		upstreamModel,
		settingService.IsDeepSeekMissingReasoningAutoDowngradeEnabled(ctx),
	)
	if err != nil {
		return body, err
	}
	if !result.changed {
		return result.body, nil
	}

	accountID := int64(0)
	if account != nil {
		accountID = account.ID
	}
	logger.FromContext(ctx).Info("DeepSeek 工具调用历史缺失推理内容，已自动关闭 thinking",
		zap.String("component", "openai.deepseek_missing_reasoning_policy"),
		zap.Int64("account_id", accountID),
		zap.String("upstream_model", upstreamModel),
		zap.String("source_path", sourcePath),
		zap.Int("missing_assistant_tool_call_messages", result.missingCount),
		zap.String("reason", deepSeekMissingReasoningReasonAssistantToolCalls),
	)
	return result.body, nil
}
