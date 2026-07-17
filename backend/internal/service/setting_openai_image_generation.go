package service

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

// cachedOpenAIImageGenerationSettings 缓存 OpenAI OAuth 生图 Responses 请求设置。
type cachedOpenAIImageGenerationSettings struct {
	mainModel       string
	reasoningEffort string
	expiresAt       int64
}

const (
	openAIImageGenerationSettingsCacheTTL       = 60 * time.Second
	openAIImageGenerationSettingsErrorTTL       = 5 * time.Second
	openAIImageGenerationSettingsDBTimeout      = 5 * time.Second
	openAIImageGenerationSettingsRefreshKey     = "openai_image_generation_settings"
	openAIImageGenerationReasoningEffortDefault = "medium"
)

type openAIImageGenerationSettingsResult struct {
	mainModel       string
	reasoningEffort string
}

func defaultOpenAIImageGenerationSettings() openAIImageGenerationSettingsResult {
	return openAIImageGenerationSettingsResult{
		mainModel:       openAIImagesResponsesMainModel,
		reasoningEffort: openAIImageGenerationReasoningEffortDefault,
	}
}

func normalizeOpenAIImageGenerationMainModel(value string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return openAIImagesResponsesMainModel
}

// NormalizeOpenAIImageGenerationReasoningEffort 归一化 OpenAI 生图请求的思考预算。
//
// @param value 后台设置中的原始值。
// @return 合法档位；空值或非法值回退 medium。
func NormalizeOpenAIImageGenerationReasoningEffort(value string) string {
	return normalizeOpenAIImageGenerationReasoningEffort(value, false)
}

func normalizeOpenAIImageGenerationReasoningEffort(value string, warnInvalid bool) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "low", "medium", "high", "xhigh", "max":
		return normalized
	case "":
		return openAIImageGenerationReasoningEffortDefault
	default:
		if warnInvalid {
			slog.Warn("invalid openai image generation reasoning effort setting, defaulting to medium",
				"setting_key", SettingKeyOpenAIImageGenerationReasoningEffort,
				"value", normalized)
		}
		return openAIImageGenerationReasoningEffortDefault
	}
}

func (s *SettingService) getOpenAIImageGenerationSettingsCached(ctx context.Context) openAIImageGenerationSettingsResult {
	fallback := defaultOpenAIImageGenerationSettings()
	if s == nil || s.settingRepo == nil {
		return fallback
	}
	if cached, ok := s.openAIImageGenerationCache.Load().(*cachedOpenAIImageGenerationSettings); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return openAIImageGenerationSettingsResult{
				mainModel:       cached.mainModel,
				reasoningEffort: cached.reasoningEffort,
			}
		}
	}

	result, _, _ := s.openAIImageGenerationSF.Do(openAIImageGenerationSettingsRefreshKey, func() (any, error) {
		if cached, ok := s.openAIImageGenerationCache.Load().(*cachedOpenAIImageGenerationSettings); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return openAIImageGenerationSettingsResult{
					mainModel:       cached.mainModel,
					reasoningEffort: cached.reasoningEffort,
				}, nil
			}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), openAIImageGenerationSettingsDBTimeout)
		defer cancel()
		values, err := s.settingRepo.GetMultiple(dbCtx, []string{
			SettingKeyOpenAIImageGenerationMainModel,
			SettingKeyOpenAIImageGenerationReasoningEffort,
		})
		if err != nil {
			slog.Warn("failed to get openai image generation settings", "error", err)
			s.openAIImageGenerationCache.Store(&cachedOpenAIImageGenerationSettings{
				mainModel:       fallback.mainModel,
				reasoningEffort: fallback.reasoningEffort,
				expiresAt:       time.Now().Add(openAIImageGenerationSettingsErrorTTL).UnixNano(),
			})
			return fallback, nil
		}
		next := openAIImageGenerationSettingsResult{
			mainModel:       normalizeOpenAIImageGenerationMainModel(values[SettingKeyOpenAIImageGenerationMainModel]),
			reasoningEffort: normalizeOpenAIImageGenerationReasoningEffort(values[SettingKeyOpenAIImageGenerationReasoningEffort], true),
		}
		s.openAIImageGenerationCache.Store(&cachedOpenAIImageGenerationSettings{
			mainModel:       next.mainModel,
			reasoningEffort: next.reasoningEffort,
			expiresAt:       time.Now().Add(openAIImageGenerationSettingsCacheTTL).UnixNano(),
		})
		return next, nil
	})
	if settings, ok := result.(openAIImageGenerationSettingsResult); ok {
		return settings
	}
	return fallback
}

// GetOpenAIImageGenerationMainModel 返回 OpenAI OAuth 生图请求使用的主模型。
//
// @param ctx 请求上下文。
// @return 后台设置值；为空、缺失或读取失败时回退内置默认模型。
func (s *SettingService) GetOpenAIImageGenerationMainModel(ctx context.Context) string {
	return s.getOpenAIImageGenerationSettingsCached(ctx).mainModel
}

// GetOpenAIImageGenerationReasoningEffort 返回 OpenAI OAuth 生图请求使用的思考预算。
//
// @param ctx 请求上下文。
// @return 后台设置值；为空、缺失或非法时回退 medium。
func (s *SettingService) GetOpenAIImageGenerationReasoningEffort(ctx context.Context) string {
	return s.getOpenAIImageGenerationSettingsCached(ctx).reasoningEffort
}
