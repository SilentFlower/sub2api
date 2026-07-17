package service

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func writeCodexClientRestrictionForbidden(c *gin.Context, result CodexClientRestrictionDetectionResult) {
	if c == nil {
		return
	}
	message := CodexClientRestrictionMessage(result)
	errorPayload := gin.H{
		"type":    "forbidden_error",
		"message": message,
	}
	if userAgent := codexCLIOnlyRejectedRequestUserAgent(c); userAgent != "" {
		errorPayload["request_user_agent"] = userAgent
		errorPayload["message"] = message + ". Request User-Agent: " + userAgent
	}
	c.JSON(http.StatusForbidden, gin.H{"error": errorPayload})
}

func codexCLIOnlyRejectedRequestUserAgent(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	return truncateString(strings.TrimSpace(c.Request.Header.Get("User-Agent")), codexCLIOnlyHeaderValueMaxBytes)
}
