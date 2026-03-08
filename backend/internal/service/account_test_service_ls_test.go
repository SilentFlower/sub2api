package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRouteAntigravityTest_LSEnabledUsesLSGateway(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &AccountTestService{}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/admin/accounts/test", nil)

	account := &Account{
		Platform: PlatformAntigravity,
		Extra: map[string]any{
			"use_ls": true,
		},
	}

	err := svc.routeAntigravityTest(ctx, account, "claude-sonnet-4-5")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Antigravity LS gateway service not configured")
}

func TestRouteAntigravityTest_DisabledLSUsesClassicGateway(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &AccountTestService{}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/admin/accounts/test", nil)

	account := &Account{
		Platform: PlatformAntigravity,
		Extra: map[string]any{
			"use_ls": false,
		},
	}

	err := svc.routeAntigravityTest(ctx, account, "claude-sonnet-4-5")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Antigravity gateway service not configured")
}
