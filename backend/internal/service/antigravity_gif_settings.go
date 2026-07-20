package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	// SettingKeyAntigravityGIFCompatSettings 保存反重力 GIF 多帧兼容的全局 JSON 配置。
	SettingKeyAntigravityGIFCompatSettings = "antigravity_gif_compat_settings"
)

var (
	// ErrAntigravityGIFSettingsRequired 表示更新请求缺少设置对象。
	ErrAntigravityGIFSettingsRequired = infraerrors.BadRequest(
		"ANTIGRAVITY_GIF_SETTINGS_REQUIRED",
		"antigravity GIF settings are required",
	)
	// ErrAntigravityGIFMaxFramesInvalid 表示单 GIF 帧数上限超出允许范围。
	ErrAntigravityGIFMaxFramesInvalid = infraerrors.BadRequest(
		"ANTIGRAVITY_GIF_MAX_FRAMES_INVALID",
		"max_frames_per_gif must be between 1 and 16",
	)
)

// AntigravityGIFCompatibilitySettings 描述反重力 GIF 多帧兼容的全局设置。
type AntigravityGIFCompatibilitySettings struct {
	Enabled         bool `json:"enabled"`
	MaxFramesPerGIF int  `json:"max_frames_per_gif"`
}

// DefaultAntigravityGIFCompatibilitySettings 返回默认开启、单 GIF 最多 8 帧的配置。
//
// @return 可独立修改的默认配置对象。
func DefaultAntigravityGIFCompatibilitySettings() *AntigravityGIFCompatibilitySettings {
	return &AntigravityGIFCompatibilitySettings{
		Enabled:         true,
		MaxFramesPerGIF: antigravity.DefaultGIFFramesPerImage,
	}
}

// GetAntigravityGIFCompatibilitySettings 读取反重力 GIF 多帧兼容的全局设置。
//
// @param ctx 请求上下文。
// @return 设置不存在、为空或损坏时返回默认配置；仓储读取失败时返回错误。
func (s *SettingService) GetAntigravityGIFCompatibilitySettings(ctx context.Context) (*AntigravityGIFCompatibilitySettings, error) {
	if s == nil || s.settingRepo == nil {
		return nil, fmt.Errorf("get antigravity GIF settings: setting repository is unavailable")
	}

	value, err := s.settingRepo.GetValue(ctx, SettingKeyAntigravityGIFCompatSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return DefaultAntigravityGIFCompatibilitySettings(), nil
		}
		return nil, fmt.Errorf("get antigravity GIF settings: %w", err)
	}
	if value == "" {
		return DefaultAntigravityGIFCompatibilitySettings(), nil
	}

	settings := DefaultAntigravityGIFCompatibilitySettings()
	if err := json.Unmarshal([]byte(value), settings); err != nil {
		return DefaultAntigravityGIFCompatibilitySettings(), nil
	}
	if settings.MaxFramesPerGIF < antigravity.MinGIFFramesPerImage ||
		settings.MaxFramesPerGIF > antigravity.MaxGIFFramesPerImage {
		settings.MaxFramesPerGIF = antigravity.DefaultGIFFramesPerImage
	}
	return settings, nil
}

// SetAntigravityGIFCompatibilitySettings 保存反重力 GIF 多帧兼容的全局设置。
//
// @param ctx 请求上下文。
// @param settings 待保存的设置，帧数必须位于 1 到 16。
// @return 保存成功返回 nil；输入或持久化失败时返回错误。
func (s *SettingService) SetAntigravityGIFCompatibilitySettings(ctx context.Context, settings *AntigravityGIFCompatibilitySettings) error {
	if settings == nil {
		return ErrAntigravityGIFSettingsRequired
	}
	if settings.MaxFramesPerGIF < antigravity.MinGIFFramesPerImage ||
		settings.MaxFramesPerGIF > antigravity.MaxGIFFramesPerImage {
		return ErrAntigravityGIFMaxFramesInvalid
	}
	if s == nil || s.settingRepo == nil {
		return fmt.Errorf("set antigravity GIF settings: setting repository is unavailable")
	}

	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal antigravity GIF settings: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyAntigravityGIFCompatSettings, string(data)); err != nil {
		return fmt.Errorf("set antigravity GIF settings: %w", err)
	}
	return nil
}
