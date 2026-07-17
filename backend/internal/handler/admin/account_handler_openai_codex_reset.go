package admin

import (
	"errors"
	"io"
	"strconv"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GetOpenAICodexResetStatus 查询账号的 Codex reset credit 和邀请状态。
//
// @param c Gin 请求上下文。
// @return 通过统一响应体返回 Codex reset 状态。
// GET /api/v1/admin/accounts/:id/openai-codex-reset/status
func (h *AccountHandler) GetOpenAICodexResetStatus(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "账号 ID 无效")
		return
	}

	if h.openaiCodexResetService == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("OPENAI_CODEX_RESET_SERVICE_UNAVAILABLE", "OpenAI Codex reset 服务不可用"))
		return
	}

	status, err := h.openaiCodexResetService.GetStatus(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, status)
}

// ConsumeOpenAICodexResetCredit 为 OpenAI OAuth 账号消耗一个 Codex reset credit。
//
// @param c Gin 请求上下文。
// @return 通过统一响应体返回 reset credit 消耗结果。
// POST /api/v1/admin/accounts/:id/openai-codex-reset/consume
func (h *AccountHandler) ConsumeOpenAICodexResetCredit(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "账号 ID 无效")
		return
	}

	if h.openaiCodexResetService == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("OPENAI_CODEX_RESET_SERVICE_UNAVAILABLE", "OpenAI Codex reset 服务不可用"))
		return
	}

	var req service.OpenAICodexResetConsumeRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(c, "请求参数无效："+err.Error())
		return
	}

	result, err := h.openaiCodexResetService.ConsumeCredit(c.Request.Context(), accountID, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, result)
}

// SendOpenAICodexInvites 使用 OpenAI OAuth 账号发送 Codex 邀请邮件。
//
// @param c Gin 请求上下文。
// @return 通过统一响应体返回邀请发送结果。
// POST /api/v1/admin/accounts/:id/openai-codex-reset/invite
func (h *AccountHandler) SendOpenAICodexInvites(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "账号 ID 无效")
		return
	}

	if h.openaiCodexResetService == nil {
		response.ErrorFrom(c, infraerrors.ServiceUnavailable("OPENAI_CODEX_RESET_SERVICE_UNAVAILABLE", "OpenAI Codex reset 服务不可用"))
		return
	}

	var req service.OpenAICodexInviteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数无效："+err.Error())
		return
	}

	result, err := h.openaiCodexResetService.SendInvites(c.Request.Context(), accountID, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, result)
}
