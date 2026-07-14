package service

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const openAIChatCompletionsEndpoint = "/v1/chat/completions"

type openAIJSONSchemaRequestShape string

const (
	openAIJSONSchemaRequestShapeResponses openAIJSONSchemaRequestShape = "responses"
	openAIJSONSchemaRequestShapeChat      openAIJSONSchemaRequestShape = "chat_completions"
)

func applyOpenAIJSONSchemaDowngrade(
	c *gin.Context,
	account *Account,
	body []byte,
	shape openAIJSONSchemaRequestShape,
	upstreamEndpoint string,
) ([]byte, error) {
	if !account.IsOpenAIJSONSchemaToJSONObjectEnabled() {
		return body, nil
	}

	var (
		out     []byte
		changed bool
		err     error
	)
	switch shape {
	case openAIJSONSchemaRequestShapeResponses:
		out, changed, err = apicompat.DowngradeResponsesJSONSchemaToJSONObject(body)
	case openAIJSONSchemaRequestShapeChat:
		out, changed, err = apicompat.DowngradeChatJSONSchemaToJSONObject(body)
	default:
		return body, fmt.Errorf("unsupported OpenAI JSON Schema request shape: %s", shape)
	}
	if err != nil {
		// 兼容开关只处理合法请求；非法 JSON 保留给原入口返回既有协议错误。
		return body, nil
	}
	if !changed {
		return body, nil
	}

	inboundEndpoint := ""
	if c != nil && c.Request != nil && c.Request.URL != nil {
		inboundEndpoint = c.Request.URL.Path
	}
	logger.L().Info("openai structured output compatibility applied",
		zap.Int64("account_id", account.ID),
		zap.String("model", gjson.GetBytes(body, "model").String()),
		zap.String("inbound_endpoint", inboundEndpoint),
		zap.String("upstream_endpoint", upstreamEndpoint),
		zap.String("request_shape", string(shape)),
		zap.String("response_format_from", "json_schema"),
		zap.String("response_format_to", "json_object"),
	)
	return out, nil
}

func resolveOpenAIJSONSchemaUpstreamEndpoint(account *Account) string {
	if account != nil && account.Type == AccountTypeAPIKey && !openai_compat.ShouldUseResponsesAPI(account.Extra) {
		return openAIChatCompletionsEndpoint
	}
	return openAIResponsesEndpoint
}
