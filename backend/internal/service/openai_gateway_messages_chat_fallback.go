package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// forwardAnthropicViaRawChatCompletions 使用只支持 /v1/chat/completions 的上游
// 服务 /v1/messages（Anthropic）客户端。请求走 Anthropic<->Chat 直连桥接，
// 不经过 Responses 中转层，避免 Responses 两段转换扰动 Chat prefix cache。
//
// 上游始终用流式请求并请求 usage；客户端原始 stream 偏好只决定网关是转发
// Anthropic SSE，还是把上游流折叠成单个 Anthropic JSON 响应。
func (s *OpenAIGatewayService) forwardAnthropicViaRawChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	defaultMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	var anthropicReq apicompat.AnthropicRequest
	if err := json.Unmarshal(body, &anthropicReq); err != nil {
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return nil, fmt.Errorf("parse anthropic request: %w", err)
	}
	originalModel := strings.TrimSpace(anthropicReq.Model)
	if originalModel == "" {
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("missing model in request")
	}
	derivedReasoningEffort := applyOpenAICompatModelNormalization(&anthropicReq)
	clientStream := anthropicReq.Stream
	debugLogGatewaySnapshot(&s.debugGatewayBodyFile, "CLIENT_ORIGINAL_MESSAGES_RAW_CHAT", c.Request.Header, body, map[string]string{
		"account":      fmt.Sprintf("%d(%s)", account.ID, account.Name),
		"account_type": string(account.Type),
		"model":        originalModel,
		"stream":       fmt.Sprintf("%t", clientStream),
	})

	chatReq, err := apicompat.AnthropicToChatCompletions(&anthropicReq)
	if err != nil {
		writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, fmt.Errorf("convert anthropic to chat completions: %w", err)
	}
	if derivedReasoningEffort != "" {
		chatReq.ReasoningEffort = derivedReasoningEffort
	}

	billingModel := resolveOpenAIForwardModel(account, anthropicReq.Model, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	chatReq.Model = upstreamModel

	// 部分 OpenAI-compatible 上游只实现 SSE 响应；这里始终向上游请求流式，
	// 非流式客户端由本地折叠，避免同一路径出现 JSON/SSE 两种上游形态。
	chatReq.Stream = true
	chatReq.StreamOptions = &apicompat.ChatStreamOptions{IncludeUsage: true}

	// BetaFastMode -> service_tier: "priority"，与 Responses fallback 保持一致。
	if containsBetaToken(c.GetHeader("anthropic-beta"), claude.BetaFastMode) {
		chatReq.ServiceTier = "priority"
	}

	chatBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("marshal chat completions request: %w", err)
	}
	if normalizedBody, normalized := normalizeOpenAIReasoningEffortForProvider(chatBody, upstreamModel); normalized {
		chatBody = normalizedBody
	}
	chatBody, err = s.applyOpenAIFastPolicyToBody(ctx, account, upstreamModel, chatBody)
	if err != nil {
		var blocked *OpenAIFastBlockedError
		if errors.As(err, &blocked) {
			writeOpenAIFastPolicyBlockedResponse(c, blocked)
		}
		return nil, err
	}
	reasoningEffort := extractFinalOpenAIReasoningEffort(chatBody)
	serviceTier := extractOpenAIServiceTierFromBody(chatBody)

	logger.L().Debug("openai messages: forwarding via raw chat completions",
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("billing_model", billingModel),
		zap.String("upstream_model", upstreamModel),
		zap.Bool("stream", clientStream),
	)

	apiKey, targetURL, err := s.resolveCCFallbackTarget(ctx, account)
	if err != nil {
		return nil, err
	}
	customUA := account.GetOpenAIUserAgent()
	if customUA == "" && account.Platform == PlatformGrok {
		customUA = "sub2api-grok/1.0"
	}
	upstreamDebug := map[string]string{
		"account":        fmt.Sprintf("%d(%s)", account.ID, account.Name),
		"account_type":   string(account.Type),
		"billing_model":  billingModel,
		"client_stream":  fmt.Sprintf("%t", clientStream),
		"original_model": originalModel,
		"upstream_model": upstreamModel,
		"url":            targetURL,
	}
	if serviceTier != nil {
		upstreamDebug["service_tier"] = *serviceTier
	}
	if reasoningEffort != nil {
		upstreamDebug["reasoning_effort"] = *reasoningEffort
	}
	debugLogGatewaySnapshot(
		&s.debugGatewayBodyFile,
		"UPSTREAM_FORWARD_MESSAGES_RAW_CHAT",
		buildMessagesRawChatDebugHeaders(c, account, apiKey, customUA),
		chatBody,
		upstreamDebug,
	)

	resp, err := s.sendCCUpstreamRequest(ctx, c, account, targetURL, chatBody, true, apiKey, customUA)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, upstreamMsg := s.readOpenAIUpstreamError(resp)
		if account.Platform == PlatformGrok {
			s.updateGrokUsageSnapshot(ctx, account.ID, xai.ParseQuotaHeaders(resp.Header, resp.StatusCode))
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")),
				Kind:               "failover",
				Message:            upstreamMsg,
			})
			s.handleGrokAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
			if s.shouldFailoverUpstreamError(resp.StatusCode) {
				return nil, &UpstreamFailoverError{
					StatusCode:             resp.StatusCode,
					ResponseBody:           respBody,
					RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
				}
			}
			return s.handleAnthropicErrorResponse(resp, c, account, billingModel)
		}
		if foErr := s.failoverOpenAIUpstreamHTTPError(ctx, c, account, resp, respBody, upstreamMsg, upstreamModel); foErr != nil {
			return nil, foErr
		}
		return s.handleAnthropicErrorResponse(resp, c, account, billingModel)
	}

	if account.Platform == PlatformGrok {
		s.updateGrokUsageSnapshot(ctx, account.ID, xai.ParseQuotaHeaders(resp.Header, resp.StatusCode))
	}

	if clientStream {
		return s.streamChatCompletionsAsAnthropic(c, resp, originalModel, billingModel, upstreamModel, reasoningEffort, serviceTier, startTime)
	}
	return s.bufferChatCompletionsAsAnthropic(c, resp, originalModel, billingModel, upstreamModel, reasoningEffort, serviceTier, startTime)
}

func buildMessagesRawChatDebugHeaders(c *gin.Context, account *Account, apiKey string, customUA string) http.Header {
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	headers.Set("Authorization", "Bearer "+apiKey)
	headers.Set("Accept", "text/event-stream")
	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			lowerKey := strings.ToLower(key)
			if openaiCCRawAllowedHeaders[lowerKey] {
				for _, v := range values {
					headers.Add(key, v)
				}
			}
		}
	}
	if customUA != "" {
		headers.Set("user-agent", customUA)
	}
	if account != nil {
		account.ApplyHeaderOverrides(headers)
	}
	return headers
}

// streamChatCompletionsAsAnthropic 将上游 Chat Completions SSE 直接转换为
// Anthropic Messages SSE。
func (s *OpenAIGatewayService) streamChatCompletionsAsAnthropic(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	serviceTier *string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestID := firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id"))
	writeStreamHeaders := s.newStreamHeaderWriter(c, resp.Header)

	state := apicompat.NewChatCompletionsToAnthropicStreamState(originalModel)
	clientDisconnected := false

	writeEvents := func(events []apicompat.AnthropicStreamEvent) {
		if clientDisconnected || len(events) == 0 {
			return
		}
		writeStreamHeaders()
		for _, event := range events {
			sse, err := apicompat.ResponsesAnthropicEventToSSE(event)
			if err != nil {
				logger.L().Warn("openai messages chat fallback: failed to marshal stream event",
					zap.Error(err),
					zap.String("request_id", requestID),
				)
				continue
			}
			if _, err := fmt.Fprint(c.Writer, sse); err != nil {
				clientDisconnected = true
				logger.L().Debug("openai messages chat fallback: client disconnected, continuing to drain upstream for billing",
					zap.Error(err),
					zap.String("request_id", requestID),
				)
				return
			}
		}
		c.Writer.Flush()
	}

	scan := s.scanCCStream(resp, "openai messages chat fallback", requestID, startTime, func(chunk *apicompat.ChatCompletionsChunk) {
		writeEvents(apicompat.ChatCompletionsChunkToAnthropicEvents(chunk, state))
	})
	usage := scan.Usage

	if scan.Err != nil {
		return &OpenAIForwardResult{
			RequestID:        requestID,
			Usage:            usage,
			Model:            originalModel,
			BillingModel:     billingModel,
			UpstreamModel:    upstreamModel,
			ReasoningEffort:  reasoningEffort,
			ServiceTier:      serviceTier,
			Stream:           true,
			Duration:         time.Since(startTime),
			FirstTokenMs:     scan.FirstTokenMs,
			ClientDisconnect: clientDisconnected,
		}, fmt.Errorf("stream usage incomplete: %w", scan.Err)
	}

	writeEvents(apicompat.FinalizeChatCompletionsAnthropicStream(state))
	if !clientDisconnected {
		c.Writer.Flush()
	}
	if !scan.SawDone {
		logCCStreamMissingDoneSentinel("openai messages chat fallback", requestID)
	}

	return &OpenAIForwardResult{
		RequestID:        requestID,
		Usage:            usage,
		Model:            originalModel,
		BillingModel:     billingModel,
		UpstreamModel:    upstreamModel,
		ReasoningEffort:  reasoningEffort,
		ServiceTier:      serviceTier,
		Stream:           true,
		Duration:         time.Since(startTime),
		FirstTokenMs:     scan.FirstTokenMs,
		ClientDisconnect: clientDisconnected,
	}, nil
}

// bufferChatCompletionsAsAnthropic 读取完整的上游 Chat Completions SSE，
// 并折叠为单个 Anthropic Messages JSON 响应，供非流式客户端使用。
func (s *OpenAIGatewayService) bufferChatCompletionsAsAnthropic(
	c *gin.Context,
	resp *http.Response,
	originalModel string,
	billingModel string,
	upstreamModel string,
	reasoningEffort *string,
	serviceTier *string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestID := firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id"))
	var chunks []*apicompat.ChatCompletionsChunk

	scan := s.scanCCStream(resp, "openai messages chat fallback", requestID, startTime, func(chunk *apicompat.ChatCompletionsChunk) {
		chunkCopy := *chunk
		chunks = append(chunks, &chunkCopy)
	})
	if scan.Err != nil {
		writeAnthropicError(c, http.StatusBadGateway, "api_error", "Upstream stream ended unexpectedly")
		return &OpenAIForwardResult{
			RequestID:       requestID,
			Usage:           scan.Usage,
			Model:           originalModel,
			BillingModel:    billingModel,
			UpstreamModel:   upstreamModel,
			ReasoningEffort: reasoningEffort,
			ServiceTier:     serviceTier,
			Stream:          false,
			Duration:        time.Since(startTime),
			FirstTokenMs:    scan.FirstTokenMs,
		}, fmt.Errorf("stream usage incomplete: %w", scan.Err)
	}
	if !scan.SawDone {
		logCCStreamMissingDoneSentinel("openai messages chat fallback", requestID)
	}

	anthropicResp := apicompat.ChatCompletionsStreamToAnthropicResponse(chunks, originalModel)

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	// 上游被强制流式，其响应头 Content-Type 为 text/event-stream，会经
	// WriteFilteredHeaders 透传进来；非流式客户端必须收到 JSON。
	c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.JSON(http.StatusOK, anthropicResp)

	return &OpenAIForwardResult{
		RequestID:       requestID,
		Usage:           scan.Usage,
		Model:           originalModel,
		BillingModel:    billingModel,
		UpstreamModel:   upstreamModel,
		ReasoningEffort: reasoningEffort,
		ServiceTier:     serviceTier,
		Stream:          false,
		Duration:        time.Since(startTime),
		FirstTokenMs:    scan.FirstTokenMs,
	}, nil
}
