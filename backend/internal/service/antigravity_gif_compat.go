package service

import (
	"context"
	"log/slog"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

func (s *AntigravityGatewayService) applyAntigravityGIFCompatibility(ctx context.Context, body []byte) ([]byte, error) {
	if !antigravity.ContainsGIFInlineDataCandidate(body) {
		return body, nil
	}

	settings := DefaultAntigravityGIFCompatibilitySettings()
	if s != nil && s.settingService != nil {
		configured, err := s.settingService.GetAntigravityGIFCompatibilitySettings(ctx)
		if err != nil {
			// 设置读取失败时保持默认开启，避免存储抖动重新暴露上游 GIF 500。
			slog.Warn("antigravity_gif_settings_read_failed", "error", err)
		} else {
			settings = configured
		}
	}
	if !settings.Enabled {
		return body, nil
	}

	return antigravity.TransformGIFInlineData(body, settings.MaxFramesPerGIF)
}

func (s *AntigravityGatewayService) wrapV1InternalRequestWithGIFCompatibility(ctx context.Context, projectID, model string, originalBody []byte) ([]byte, error) {
	wrappedBody, err := s.wrapV1InternalRequest(projectID, model, originalBody)
	if err != nil {
		return nil, err
	}
	return s.applyAntigravityGIFCompatibility(ctx, wrappedBody)
}

func (s *AntigravityGatewayService) transformClaudeRequestWithGIFCompatibility(
	ctx context.Context,
	request *antigravity.ClaudeRequest,
	projectID string,
	mappedModel string,
	options antigravity.TransformOptions,
) ([]byte, error) {
	body, err := antigravity.TransformClaudeToGeminiWithOptions(request, projectID, mappedModel, options)
	if err != nil {
		return nil, err
	}
	return s.applyAntigravityGIFCompatibility(ctx, body)
}

func antigravityGIFClientErrorMessage(err error, fallback string) string {
	if antigravity.IsGIFCompatibilityError(err) {
		return err.Error()
	}
	return fallback
}
