package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type updateAntigravityGIFSettingsRequest struct {
	Enabled         bool `json:"enabled"`
	MaxFramesPerGIF int  `json:"max_frames_per_gif"`
}

// GetAntigravityGIFCompatibilitySettings 获取反重力 GIF 多帧兼容设置。
//
// @param c Gin 请求上下文。
// @return 无；结果写入 HTTP 响应。
func (h *SettingHandler) GetAntigravityGIFCompatibilitySettings(c *gin.Context) {
	settings, err := h.settingService.GetAntigravityGIFCompatibilitySettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.AntigravityGIFCompatibilitySettings{
		Enabled:         settings.Enabled,
		MaxFramesPerGIF: settings.MaxFramesPerGIF,
	})
}

// UpdateAntigravityGIFCompatibilitySettings 更新反重力 GIF 多帧兼容设置。
//
// @param c Gin 请求上下文。
// @return 无；结果写入 HTTP 响应。
func (h *SettingHandler) UpdateAntigravityGIFCompatibilitySettings(c *gin.Context) {
	var request updateAntigravityGIFSettingsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest(
			"ANTIGRAVITY_GIF_SETTINGS_INVALID_REQUEST",
			"invalid antigravity GIF settings request",
		).WithCause(err))
		return
	}

	settings := &service.AntigravityGIFCompatibilitySettings{
		Enabled:         request.Enabled,
		MaxFramesPerGIF: request.MaxFramesPerGIF,
	}
	if err := h.settingService.SetAntigravityGIFCompatibilitySettings(c.Request.Context(), settings); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.AntigravityGIFCompatibilitySettings{
		Enabled:         settings.Enabled,
		MaxFramesPerGIF: settings.MaxFramesPerGIF,
	})
}
