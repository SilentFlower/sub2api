package service

import (
	"context"
	"log/slog"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
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

	transformed, err := antigravity.TransformGIFInlineData(body, settings.MaxFramesPerGIF)
	if err != nil {
		logAntigravityGIFCompatibilityError(ctx, err)
	}
	return transformed, err
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

func logAntigravityGIFCompatibilityError(ctx context.Context, err error) {
	if !antigravity.IsGIFCompatibilityError(err) {
		return
	}
	fields := []zap.Field{zap.Error(err)}
	if diagnostic, ok := antigravity.GIFCompatibilityDiagnosticsFromError(err); ok {
		fields = append(fields,
			zap.String("gif_stage", diagnostic.Stage),
			zap.Int("gif_input_length", diagnostic.InputLength),
			zap.Int("gif_trimmed_length", diagnostic.TrimmedLength),
			zap.Int("gif_payload_length", diagnostic.PayloadLength),
			zap.Int("gif_normalized_payload_length", diagnostic.NormalizedPayloadLength),
			zap.Bool("gif_has_outer_base64_prefix", diagnostic.HasOuterBase64Prefix),
			zap.Bool("gif_has_data_uri", diagnostic.HasDataURI),
			zap.String("gif_data_uri_mime", diagnostic.DataURIMime),
			zap.Bool("gif_data_uri_has_base64", diagnostic.DataURIHasBase64),
			zap.Bool("gif_had_url_escape", diagnostic.HadURLEscape),
			zap.Bool("gif_url_unescape_failed", diagnostic.URLUnescapeFailed),
			zap.Bool("gif_removed_whitespace", diagnostic.RemovedWhitespace),
			zap.Bool("gif_has_url_safe_alphabet", diagnostic.HasURLSafeAlphabet),
			zap.Int("gif_padding_remainder", diagnostic.PaddingRemainder),
			zap.String("gif_decode_error", diagnostic.DecodeError),
		)
	}
	logger.FromContext(ctx).Warn("antigravity_gif_transform_failed", fields...)
}
