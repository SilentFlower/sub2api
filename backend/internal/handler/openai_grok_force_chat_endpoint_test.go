//go:build unit

package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResolveOpenAIUpstreamEndpointForGrokMessagesForceChat(t *testing.T) {
	tests := []struct {
		name            string
		path            string
		account         *service.Account
		runtimeEndpoint string
		want            string
	}{
		{
			name: "messages forced chat",
			path: "/v1/messages",
			account: &service.Account{
				Platform: service.PlatformGrok,
				Type:     service.AccountTypeOAuth,
				Extra: map[string]any{
					openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
				},
			},
			want: EndpointChatCompletions,
		},
		{
			name:    "messages auto stays responses",
			path:    "/v1/messages",
			account: &service.Account{Platform: service.PlatformGrok, Type: service.AccountTypeOAuth},
			want:    EndpointResponses,
		},
		{
			name: "responses stays responses even when forced",
			path: "/v1/responses",
			account: &service.Account{
				Platform: service.PlatformGrok,
				Type:     service.AccountTypeOAuth,
				Extra: map[string]any{
					openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
				},
			},
			want: EndpointResponses,
		},
		{
			name:            "native chat completions records runtime chat completions",
			path:            "/v1/chat/completions",
			account:         &service.Account{Platform: service.PlatformGrok, Type: service.AccountTypeOAuth},
			runtimeEndpoint: EndpointChatCompletions,
			want:            EndpointChatCompletions,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, tt.path, nil)
			service.SetActualOpenAIUpstreamEndpoint(c, tt.runtimeEndpoint)

			require.Equal(t, tt.want, resolveOpenAIUpstreamEndpoint(c, tt.account, nil))
		})
	}
}
