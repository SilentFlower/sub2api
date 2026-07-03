package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 非流式 Anthropic /v1/messages 客户端经 chat-completions 桥接时，必须收到
// application/json 响应。桥接层始终流式请求上游，因此上游的 text/event-stream
// Content-Type 会被 WriteFilteredHeaders 透传；gin 的 c.JSON 只会在响应头不存在时
// 设置 Content-Type，无法覆盖既有值，所以缓冲路径必须显式重置。
func TestBufferChatCompletionsAsAnthropic_NonStreamUsesJSONContentType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	const upstreamSSE = "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hi\"}}]}\n" +
		"data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n" +
		"data: [DONE]\n"

	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	s := &OpenAIGatewayService{
		cfg:                  cfg,
		responseHeaderFilter: compileResponseHeaderFilter(cfg),
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamSSE)),
	}

	_, err := s.bufferChatCompletionsAsAnthropic(
		c, resp,
		"claude-3-5-sonnet-20241022",
		"gpt-4o",
		"gpt-4o",
		nil,
		nil,
		time.Now(),
	)
	require.NoError(t, err)

	require.Truef(t,
		strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json"),
		"non-stream response must be application/json, got %q", rec.Header().Get("Content-Type"),
	)
	require.Truef(t,
		strings.HasPrefix(strings.TrimSpace(rec.Body.String()), "{"),
		"expected JSON object body, got %q", rec.Body.String(),
	)
}
