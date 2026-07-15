package routes

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGrokBillingQuotaRouteIsRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	admin := router.Group("/api/v1/admin")
	registerGrokOAuthRoutes(admin, &handler.Handlers{
		Admin: &handler.AdminHandlers{GrokOAuth: &adminhandler.GrokOAuthHandler{}},
	})

	registered := false
	for _, route := range router.Routes() {
		if route.Method == http.MethodGet && route.Path == "/api/v1/admin/grok/accounts/:id/billing-quota" {
			registered = true
			break
		}
	}

	require.True(t, registered, "GET Grok Billing 额度路由应完成注册")
}
