package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// accountExtraKeyOpenAIAlphaSearchViaResponses 是账号级"Alpha Search 经上游 Responses
// 执行"开关在 accounts.extra 中的键名。
const accountExtraKeyOpenAIAlphaSearchViaResponses = "openai_alpha_search_via_responses"

// IsOpenAIAlphaSearchViaResponsesEnabled 报告账号是否开启"Alpha Search 经上游 Responses
// web_search 执行"。
//
// 仅 openai 平台的 API Key 账号可开启：alpha/search 只在 OpenAI 分组内调度，国产平台
// 账号进不了该入口；OAuth 与 PAT 账号保持既有的 chatgpt.com 路径。
//
// @return extra 中该键为布尔 true 时返回 true。
func (a *Account) IsOpenAIAlphaSearchViaResponsesEnabled() bool {
	if a == nil || !a.IsOpenAIApiKey() || a.Extra == nil {
		return false
	}
	enabled, ok := a.Extra[accountExtraKeyOpenAIAlphaSearchViaResponses].(bool)
	return ok && enabled
}

// forwardAlphaSearchViaUpstreamResponsesWebSearch 把 Codex 独立搜索请求翻译成带
// web_search 工具的流式 Responses 请求交给账号上游执行，再把结果转回 alpha/search
// 响应形态。
//
// 上游 2xx 但没有真实搜索证据（无 web_search_call 输出项、无 url_citation）时，说明
// 上游像 DeepSeek 一样忽略了 web_search 工具，模型只是凭记忆作答；此时若账号具备
// 本地模拟资格则改走 Web Search Emulation 供应商，否则返回 502 且不计费。上游非 2xx
// 时同样优先本地模拟，模拟不可用再沿用 PAT 路径的 failover / 透传分类。
//
// @param ctx 请求上下文。
// @param c Gin 请求上下文，用于读取入站头并写入下游响应。
// @param account 已开启开关的 API Key 账号。
// @param alphaBody 原始 alpha/search 请求体（模型已映射）。
// @param token 账号访问令牌。
// @param proxyURL 账号代理地址，空表示直连。
// @param requestedModel 客户端请求的模型名。
// @param upstreamModel 映射后的上游模型名。
// @return 真实搜索成功时返回 WebSearchCalls=1 的结果；已写下游的非计费响应返回 (nil, nil)；
// 可切换错误与传输错误通过 error 返回。
func (s *OpenAIGatewayService) forwardAlphaSearchViaUpstreamResponsesWebSearch(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	alphaBody []byte,
	token string,
	proxyURL string,
	requestedModel string,
	upstreamModel string,
) (*OpenAIForwardResult, error) {
	if upstreamModel == "" {
		upstreamModel = requestedModel
	}
	responsesBody, err := buildOpenAIAlphaSearchResponsesWebSearchBody(alphaBody, upstreamModel)
	if err != nil {
		return nil, err
	}
	req, err := s.buildOpenAIAlphaSearchAPIKeyResponsesRequest(ctx, c, account, responsesBody, token)
	if err != nil {
		return nil, err
	}
	SetActualOpenAIUpstreamEndpoint(c, openAIResponsesEndpoint)

	upstreamStart := time.Now()
	resp, err := s.doOpenAIUpstream(req, proxyURL, account)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, true)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, fmt.Errorf("read alpha search upstream responses web_search response: %w", err)
	}

	emulationEligible := s.alphaSearchEmulationEligible(ctx, c, account)
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		output, results, searched := parseOpenAIResponsesSSEForAlphaSearch(respBody)
		if searched {
			alphaRespBody, encodeErr := encodeOpenAIAlphaSearchResponse(output, results)
			if encodeErr != nil {
				return nil, encodeErr
			}
			c.Data(http.StatusOK, "application/json", alphaRespBody)
			return &OpenAIForwardResult{
				RequestID:        strings.TrimSpace(resp.Header.Get("x-request-id")),
				UpstreamHeaders:  resp.Header,
				Model:            requestedModel,
				UpstreamModel:    upstreamModel,
				UpstreamEndpoint: openAIResponsesEndpoint,
				ResponseHeaders:  resp.Header.Clone(),
				Duration:         time.Since(upstreamStart),
				WebSearchCalls:   1,
			}, nil
		}
		logger.L().Info("openai alpha search: upstream responses did not perform web search",
			zap.Int64("account_id", account.ID),
			zap.String("model", upstreamModel),
			zap.Bool("emulation_eligible", emulationEligible),
		)
		if emulationEligible {
			return s.emulateOpenAIAlphaSearch(ctx, c, account, alphaBody, requestedModel, upstreamModel)
		}
		writeOpenAIAlphaSearchFailed(c, "upstream did not perform web search")
		return nil, errors.New("alpha search upstream responses did not perform web search")
	}

	if emulationEligible {
		logger.L().Warn("openai alpha search: upstream responses web_search failed; falling back to emulation",
			zap.Int64("account_id", account.ID),
			zap.String("model", upstreamModel),
			zap.Int("upstream_status", resp.StatusCode),
		)
		return s.emulateOpenAIAlphaSearch(ctx, c, account, alphaBody, requestedModel, upstreamModel)
	}

	// 模拟不可用：与 PAT 路径保持同一 failover / 透传分类，端点缺失（404/405）同样换号。
	upstreamMessage := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
	if s.shouldFailoverOpenAIUpstreamResponse(account, resp.StatusCode, upstreamMessage, respBody) ||
		isOpenAIAlphaSearchEndpointUnsupported(account, resp.StatusCode) {
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		shouldDisable := false
		if shouldApplyOpenAIAlphaSearchAccountErrorSideEffects(resp.StatusCode) {
			shouldDisable = s.handleFailoverSideEffects(ctx, resp, account, respBody, openAIAlphaSearchSchedulingModel(account, requestedModel))
		}
		retryableOnSameAccount := !shouldDisable && account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode)
		if isOpenAIHTTPUpstreamAccessStateError(resp.StatusCode, upstreamMessage, respBody) {
			return nil, newOpenAIUpstreamFailoverError(resp.StatusCode, resp.Header, respBody, upstreamMessage, retryableOnSameAccount)
		}
		return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBody, RetryableOnSameAccount: retryableOnSameAccount}
	}
	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, respBody)
	return nil, nil
}

// buildOpenAIAlphaSearchAPIKeyResponsesRequest 为 API Key 账号构造发往账号上游 Responses
// 端点的搜索请求。与 PAT 版不同，这里不带 ChatGPT 账号头、Codex 身份头与 Lite 头，
// 只保留 Bearer 鉴权、账号自定义 UA 与 header 覆盖。
//
// @param ctx 请求上下文。
// @param c Gin 请求上下文，用于回退读取入站 User-Agent。
// @param account API Key 账号。
// @param body 已构造好的 Responses 请求体。
// @param token 账号访问令牌。
// @return 可直接发送的上游请求；base_url 校验或鉴权头构造失败时返回错误。
func (s *OpenAIGatewayService) buildOpenAIAlphaSearchAPIKeyResponsesRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token string) (*http.Request, error) {
	targetURL := openaiPlatformAPIURL
	if baseURL := account.GetOpenAIBaseURL(); baseURL != "" {
		validatedURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return nil, err
		}
		targetURL = buildOpenAIResponsesURLForPlatform(account.Platform, validatedURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))

	authHeaders, err := s.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		return nil, fmt.Errorf("build openai authentication headers: %w", err)
	}
	for key, values := range authHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if customUA := account.GetOpenAIUserAgent(); customUA != "" {
		req.Header.Set("User-Agent", customUA)
	} else if userAgent := openAIAlphaSearchInboundHeader(c, "User-Agent"); userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	account.ApplyHeaderOverrides(req.Header)
	return req, nil
}

// openAIAlphaSearchValueHasWebSearchCall 递归判断 SSE 事件或 completed 响应中是否出现
// web_search_call 输出项，作为上游真实执行过搜索的证据之一。
//
// @param value 已解码的事件或响应片段。
// @return 任意层级存在 {"type":"web_search_call"} 时返回 true。
func openAIAlphaSearchValueHasWebSearchCall(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if typed["type"] == "web_search_call" {
			return true
		}
		for _, child := range typed {
			if openAIAlphaSearchValueHasWebSearchCall(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if openAIAlphaSearchValueHasWebSearchCall(child) {
				return true
			}
		}
	}
	return false
}
