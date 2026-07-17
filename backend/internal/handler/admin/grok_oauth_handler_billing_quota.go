package admin

import (
	"net/http"
	"strconv"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// QueryBillingQuota 刷新并返回独立 Grok 套餐额度快照。
//
// @param c Gin 请求上下文。
// @return 无返回值，结果写入 HTTP response。
func (h *GrokOAuthHandler) QueryBillingQuota(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	if h.billingQuotaService == nil {
		response.ErrorFrom(c, infraerrors.New(http.StatusInternalServerError, "GROK_BILLING_QUOTA_NOT_CONFIGURED", "Grok billing quota service is not configured"))
		return
	}
	result, err := h.billingQuotaService.QueryBillingQuota(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
