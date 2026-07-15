package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/websearch"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	openAIResponsesWebRunNamespace       = "web"
	openAIResponsesWebRunName            = "run"
	openAIResponsesWebRunMaxQueries      = 4
	openAIResponsesWebRunMaxRounds       = 2
	openAIResponsesWebRunDefaultLength   = "medium"
	openAIResponsesWebRunMaxTitleBytes   = 512
	openAIResponsesWebRunMaxURLBytes     = 2048
	openAIResponsesWebRunMaxSnippetBytes = 4096
	openAIResponsesWebRunToolDescription = "Search the public web. Only search_query is supported; use it for current information, including weather queries."
	openAIResponsesWebRunToolSchema      = `{"type":"object","properties":{"search_query":{"type":"array","minItems":1,"maxItems":4,"items":{"type":"object","properties":{"q":{"type":"string","minLength":1},"recency":{"type":"integer","minimum":0}},"required":["q"],"additionalProperties":false}},"response_length":{"type":"string","enum":["short","medium","long"]}},"required":["search_query"],"additionalProperties":false}`
)

type openAIResponsesWebRunLoopOptions struct {
	OriginalModel   string
	BillingModel    string
	UpstreamModel   string
	ReasoningEffort *string
	ServiceTier     *string
	ClientStream    bool
	CustomTools     map[string]bool
	ToolSearch      bool
	NamespaceTools  map[string]apicompat.NamespacedToolName
	WebRunToolName  string
	StartTime       time.Time
}

type openAIResponsesWebRunQuery struct {
	Q       string `json:"q"`
	Recency *int   `json:"recency,omitempty"`
}

type openAIResponsesWebRunArguments struct {
	SearchQuery    []openAIResponsesWebRunQuery `json:"search_query"`
	ResponseLength string                       `json:"response_length,omitempty"`
}

type openAIResponsesWebRunError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type openAIResponsesWebRunWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type openAIResponsesWebRunSearchGroup struct {
	Query    string                         `json:"query"`
	Provider string                         `json:"provider,omitempty"`
	Results  []websearch.SearchResult       `json:"results,omitempty"`
	Warnings []openAIResponsesWebRunWarning `json:"warnings,omitempty"`
	Error    *openAIResponsesWebRunError    `json:"error,omitempty"`
}

type openAIResponsesWebRunOutput struct {
	SearchQuery []openAIResponsesWebRunSearchGroup `json:"search_query,omitempty"`
	Error       *openAIResponsesWebRunError        `json:"error,omitempty"`
}

func findOpenAIResponsesWebRunTool(
	req *apicompat.ChatCompletionsRequest,
	namespaceTools map[string]apicompat.NamespacedToolName,
) (string, bool) {
	if req == nil {
		return "", false
	}
	for flattened, namespaced := range namespaceTools {
		if namespaced.Namespace != openAIResponsesWebRunNamespace || namespaced.Name != openAIResponsesWebRunName {
			continue
		}
		for _, tool := range req.Tools {
			if tool.Function != nil && tool.Function.Name == flattened {
				return flattened, true
			}
		}
	}
	return "", false
}

func narrowOpenAIResponsesWebRunTool(req *apicompat.ChatCompletionsRequest, toolName string) {
	if req == nil || toolName == "" {
		return
	}
	for i := range req.Tools {
		tool := &req.Tools[i]
		if tool.Function == nil || tool.Function.Name != toolName {
			continue
		}
		tool.Function.Description = openAIResponsesWebRunToolDescription
		tool.Function.Parameters = json.RawMessage(openAIResponsesWebRunToolSchema)
		tool.Function.Strict = nil
		parallel := false
		req.ParallelToolCalls = &parallel
		return
	}
}

func parseOpenAIResponsesWebRunArguments(arguments string, remainingQueries int) (*openAIResponsesWebRunArguments, *openAIResponsesWebRunError) {
	if remainingQueries < 1 {
		return nil, &openAIResponsesWebRunError{Code: "search_limit_exceeded", Message: "The request has reached the maximum of 4 web search queries"}
	}
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return nil, &openAIResponsesWebRunError{Code: "invalid_tool_arguments", Message: "web.run requires a JSON object containing search_query"}
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return nil, &openAIResponsesWebRunError{Code: "invalid_tool_arguments", Message: "web.run arguments must be valid JSON"}
	}
	for key := range raw {
		if key != "search_query" && key != "response_length" {
			return nil, &openAIResponsesWebRunError{Code: "unsupported_command", Message: "Only web.run search_query is supported by this gateway"}
		}
	}

	var parsed openAIResponsesWebRunArguments
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parsed); err != nil {
		return nil, &openAIResponsesWebRunError{Code: "invalid_tool_arguments", Message: "web.run search_query arguments are invalid"}
	}
	if len(parsed.SearchQuery) == 0 {
		return nil, &openAIResponsesWebRunError{Code: "invalid_tool_arguments", Message: "web.run search_query must contain at least one query"}
	}
	if len(parsed.SearchQuery) > remainingQueries {
		return nil, &openAIResponsesWebRunError{Code: "search_limit_exceeded", Message: "The request supports at most 4 web search queries"}
	}
	for i := range parsed.SearchQuery {
		parsed.SearchQuery[i].Q = strings.TrimSpace(parsed.SearchQuery[i].Q)
		if parsed.SearchQuery[i].Q == "" {
			return nil, &openAIResponsesWebRunError{Code: "invalid_tool_arguments", Message: "Each web.run search_query item requires a non-empty q"}
		}
		if parsed.SearchQuery[i].Recency != nil && *parsed.SearchQuery[i].Recency < 0 {
			return nil, &openAIResponsesWebRunError{Code: "invalid_tool_arguments", Message: "web.run search_query recency must not be negative"}
		}
	}
	parsed.ResponseLength = strings.TrimSpace(parsed.ResponseLength)
	if parsed.ResponseLength == "" {
		parsed.ResponseLength = openAIResponsesWebRunDefaultLength
	}
	if _, err := openAIResponsesWebRunMaxResults(parsed.ResponseLength); err != nil {
		return nil, &openAIResponsesWebRunError{Code: "invalid_tool_arguments", Message: err.Error()}
	}
	return &parsed, nil
}

func openAIResponsesWebRunMaxResults(responseLength string) (int, error) {
	switch responseLength {
	case "short":
		return 3, nil
	case "medium":
		return 5, nil
	case "long":
		return 10, nil
	default:
		return 0, fmt.Errorf("web.run response_length must be short, medium, or long")
	}
}

func marshalOpenAIResponsesWebRunOutput(output openAIResponsesWebRunOutput) string {
	body, err := json.Marshal(output)
	if err != nil {
		return `{"error":{"code":"tool_output_error","message":"Failed to encode web search results"}}`
	}
	return string(body)
}

func openAIResponsesWebRunToolError(code, message string) string {
	return marshalOpenAIResponsesWebRunOutput(openAIResponsesWebRunOutput{
		Error: &openAIResponsesWebRunError{Code: code, Message: message},
	})
}

func (s *OpenAIGatewayService) executeOpenAIResponsesWebRunSearch(
	ctx context.Context,
	account *Account,
	parsed *openAIResponsesWebRunArguments,
) (string, int, error) {
	if parsed == nil {
		return openAIResponsesWebRunToolError("invalid_tool_arguments", "web.run search_query arguments are missing"), 0, nil
	}
	if s.settingService == nil || !s.settingService.IsWebSearchEmulationEnabled(ctx) || getWebSearchManager() == nil {
		return openAIResponsesWebRunToolError("web_search_unavailable", "No global web search provider is available"), 0, nil
	}
	maxResults, err := openAIResponsesWebRunMaxResults(parsed.ResponseLength)
	if err != nil {
		return openAIResponsesWebRunToolError("invalid_tool_arguments", err.Error()), 0, nil
	}

	searchExecutor := doWebSearchWithMaxResults
	if s.openAIWebSearchExecutor != nil {
		searchExecutor = s.openAIWebSearchExecutor
	}
	output := openAIResponsesWebRunOutput{SearchQuery: make([]openAIResponsesWebRunSearchGroup, 0, len(parsed.SearchQuery))}
	successfulCalls := 0
	for _, query := range parsed.SearchQuery {
		group := openAIResponsesWebRunSearchGroup{Query: query.Q}
		if query.Recency != nil {
			group.Warnings = append(group.Warnings, openAIResponsesWebRunWarning{
				Code:    "recency_not_enforced",
				Message: "The configured search provider does not guarantee strict recency filtering",
			})
		}
		response, provider, searchErr := searchExecutor(ctx, account, query.Q, maxResults)
		if searchErr != nil {
			if errors.Is(searchErr, websearch.ErrProxyUnavailable) {
				return "", successfulCalls, &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: []byte("web search account proxy unavailable")}
			}
			group.Error = &openAIResponsesWebRunError{Code: "web_search_failed", Message: "All configured web search providers failed"}
			output.SearchQuery = append(output.SearchQuery, group)
			continue
		}
		if response == nil {
			group.Error = &openAIResponsesWebRunError{Code: "web_search_failed", Message: "The web search provider returned an empty response"}
			output.SearchQuery = append(output.SearchQuery, group)
			continue
		}
		group.Provider = provider
		results := response.Results
		if len(results) > maxResults {
			results = results[:maxResults]
		}
		group.Results = sanitizeOpenAIResponsesWebRunResults(results)
		output.SearchQuery = append(output.SearchQuery, group)
		successfulCalls++
	}
	return marshalOpenAIResponsesWebRunOutput(output), successfulCalls, nil
}

func sanitizeOpenAIResponsesWebRunResults(results []websearch.SearchResult) []websearch.SearchResult {
	if len(results) == 0 {
		return nil
	}
	out := make([]websearch.SearchResult, 0, len(results))
	for _, result := range results {
		out = append(out, websearch.SearchResult{
			URL:     truncateString(result.URL, openAIResponsesWebRunMaxURLBytes),
			Title:   truncateString(result.Title, openAIResponsesWebRunMaxTitleBytes),
			Snippet: truncateString(result.Snippet, openAIResponsesWebRunMaxSnippetBytes),
			PageAge: truncateString(result.PageAge, openAIResponsesWebRunMaxTitleBytes),
		})
	}
	return out
}

func (s *OpenAIGatewayService) forwardResponsesViaWebRunChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	chatReq *apicompat.ChatCompletionsRequest,
	options openAIResponsesWebRunLoopOptions,
) (*OpenAIForwardResult, error) {
	apiKey, targetURL, err := s.resolveCCFallbackTarget(ctx, account)
	if err != nil {
		return nil, err
	}

	var aggregateUsage OpenAIUsage
	webSearchCalls := 0
	queryCount := 0
	webRunRounds := 0
	requestID := ""
	var responseHeaders http.Header
	for {
		chatBody, marshalErr := json.Marshal(chatReq)
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal web.run chat completions request: %w", marshalErr)
		}
		resp, sendErr := s.sendCCUpstreamRequest(ctx, c, account, targetURL, chatBody, false, apiKey, account.GetOpenAIUserAgent(), "")
		if sendErr != nil {
			return nil, sendErr
		}
		requestID = strings.TrimSpace(resp.Header.Get("x-request-id"))
		responseHeaders = resp.Header.Clone()
		if resp.StatusCode >= http.StatusBadRequest {
			respBody, upstreamMsg := s.readOpenAIUpstreamError(resp)
			if failoverErr := s.failoverOpenAIUpstreamHTTPError(ctx, c, account, resp, respBody, upstreamMsg, options.UpstreamModel); failoverErr != nil {
				_ = resp.Body.Close()
				return nil, failoverErr
			}
			result, handleErr := s.handleErrorResponse(ctx, resp, c, account, chatBody, options.BillingModel)
			_ = resp.Body.Close()
			return result, handleErr
		}

		ccResp, usage, readErr := s.readCCUpstreamJSONResponse(c, resp, writeOpenAIResponsesFallbackError)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		addOpenAIUsage(&aggregateUsage, usage)
		choice, webRunCall, callErr := selectOpenAIResponsesWebRunCall(ccResp, options.WebRunToolName)
		if callErr != nil {
			writeOpenAIResponsesFallbackError(c, http.StatusBadGateway, "api_error", callErr.Error())
			return nil, callErr
		}
		if webRunCall == nil {
			applyOpenAIResponsesWebRunAggregateUsage(ccResp, aggregateUsage)
			fields := []zap.Field{
				zap.Int64("account_id", account.ID),
				zap.String("model", options.OriginalModel),
				zap.Int("search_rounds", webRunRounds),
				zap.Int("search_calls", webSearchCalls),
			}
			if webRunRounds > 0 {
				logger.L().Info("openai web.run search loop completed", fields...)
			} else {
				logger.L().Debug("openai web.run was available but not selected", fields...)
			}
			return s.writeOpenAIResponsesWebRunResult(c, ccResp, responseHeaders, requestID, aggregateUsage, webSearchCalls, options)
		}
		if webRunRounds >= openAIResponsesWebRunMaxRounds {
			err := errors.New("web.run exceeded the maximum of 2 search tool rounds")
			writeOpenAIResponsesFallbackError(c, http.StatusBadGateway, "api_error", err.Error())
			return nil, err
		}
		webRunRounds++

		remainingQueries := openAIResponsesWebRunMaxQueries - queryCount
		parsed, argumentErr := parseOpenAIResponsesWebRunArguments(webRunCall.Function.Arguments, remainingQueries)
		toolOutput := ""
		if argumentErr != nil {
			toolOutput = marshalOpenAIResponsesWebRunOutput(openAIResponsesWebRunOutput{Error: argumentErr})
		} else {
			queryCount += len(parsed.SearchQuery)
			var successfulCalls int
			toolOutput, successfulCalls, err = s.executeOpenAIResponsesWebRunSearch(ctx, account, parsed)
			if err != nil {
				return nil, err
			}
			webSearchCalls += successfulCalls
		}
		appendOpenAIResponsesWebRunMessages(chatReq, choice.Message, webRunCall, toolOutput, webRunRounds)
	}
}

func selectOpenAIResponsesWebRunCall(
	resp *apicompat.ChatCompletionsResponse,
	webRunToolName string,
) (*apicompat.ChatChoice, *apicompat.ChatToolCall, error) {
	if resp == nil || len(resp.Choices) == 0 {
		return nil, nil, nil
	}
	choice := &resp.Choices[0]
	webRunIndex := -1
	for i := range choice.Message.ToolCalls {
		if choice.Message.ToolCalls[i].Function.Name == webRunToolName {
			if webRunIndex >= 0 || len(choice.Message.ToolCalls) != 1 {
				return nil, nil, errors.New("web.run cannot be combined with parallel client tool calls")
			}
			webRunIndex = i
		}
	}
	if webRunIndex < 0 {
		return choice, nil, nil
	}
	return choice, &choice.Message.ToolCalls[webRunIndex], nil
}

func appendOpenAIResponsesWebRunMessages(
	req *apicompat.ChatCompletionsRequest,
	assistant apicompat.ChatMessage,
	toolCall *apicompat.ChatToolCall,
	toolOutput string,
	round int,
) {
	if req == nil || toolCall == nil {
		return
	}
	callID := strings.TrimSpace(toolCall.ID)
	if callID == "" {
		callID = deterministicOpenAIResponsesWebRunCallID(round, toolCall.Function.Name, toolCall.Function.Arguments)
		toolCall.ID = callID
	}
	toolCall.Type = "function"
	assistant.Role = "assistant"
	assistant.ToolCalls = []apicompat.ChatToolCall{*toolCall}
	content, _ := json.Marshal(toolOutput)
	req.Messages = append(req.Messages, assistant, apicompat.ChatMessage{
		Role:       "tool",
		Content:    content,
		ToolCallID: callID,
	})
}

func deterministicOpenAIResponsesWebRunCallID(round int, name, arguments string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\n%s\n%s", round, name, arguments)))
	return "call_web_" + hex.EncodeToString(sum[:8])
}

func applyOpenAIResponsesWebRunAggregateUsage(resp *apicompat.ChatCompletionsResponse, usage OpenAIUsage) {
	if resp == nil {
		return
	}
	promptDetails := &apicompat.ChatTokenDetails{
		CachedTokens:        usage.CacheReadInputTokens,
		CacheCreationTokens: usage.CacheCreationInputTokens,
	}
	if promptDetails.CachedTokens == 0 && promptDetails.CacheCreationTokens == 0 {
		promptDetails = nil
	}
	resp.Usage = &apicompat.ChatUsage{
		PromptTokens:        usage.InputTokens,
		CompletionTokens:    usage.OutputTokens,
		TotalTokens:         usage.InputTokens + usage.OutputTokens,
		PromptTokensDetails: promptDetails,
	}
}

func (s *OpenAIGatewayService) writeOpenAIResponsesWebRunResult(
	c *gin.Context,
	ccResp *apicompat.ChatCompletionsResponse,
	upstreamHeaders http.Header,
	requestID string,
	usage OpenAIUsage,
	webSearchCalls int,
	options openAIResponsesWebRunLoopOptions,
) (*OpenAIForwardResult, error) {
	if options.ClientStream {
		return s.writeOpenAIResponsesWebRunStream(c, ccResp, upstreamHeaders, requestID, usage, webSearchCalls, options)
	}
	responsesResp := apicompat.ChatCompletionsResponseToResponses(ccResp, options.OriginalModel, options.CustomTools, options.ToolSearch, options.NamespaceTools)
	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), upstreamHeaders, s.responseHeaderFilter)
	}
	c.JSON(http.StatusOK, responsesResp)
	return &OpenAIForwardResult{
		RequestID:       requestID,
		ResponseID:      responsesResp.ID,
		Usage:           usage,
		Model:           options.OriginalModel,
		BillingModel:    options.BillingModel,
		UpstreamModel:   options.UpstreamModel,
		ReasoningEffort: options.ReasoningEffort,
		ServiceTier:     options.ServiceTier,
		Stream:          false,
		Duration:        time.Since(options.StartTime),
		WebSearchCalls:  webSearchCalls,
	}, nil
}

func (s *OpenAIGatewayService) writeOpenAIResponsesWebRunStream(
	c *gin.Context,
	ccResp *apicompat.ChatCompletionsResponse,
	upstreamHeaders http.Header,
	requestID string,
	usage OpenAIUsage,
	webSearchCalls int,
	options openAIResponsesWebRunLoopOptions,
) (*OpenAIForwardResult, error) {
	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), upstreamHeaders, s.responseHeaderFilter)
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	events := apicompat.ChatCompletionsResponseToResponsesEvents(ccResp, options.OriginalModel, options.CustomTools, options.ToolSearch, options.NamespaceTools)
	clientDisconnected := false
	firstTokenMs := int(time.Since(options.StartTime).Milliseconds())
	for _, event := range events {
		frame, err := apicompat.ResponsesEventToSSE(event)
		if err != nil {
			return nil, err
		}
		if _, err := io.WriteString(c.Writer, frame); err != nil {
			clientDisconnected = true
			break
		}
	}
	if !clientDisconnected {
		if _, err := io.WriteString(c.Writer, "data: [DONE]\n\n"); err != nil {
			clientDisconnected = true
		}
	}
	c.Writer.Flush()

	responseID := ""
	if ccResp != nil {
		responseID = ccResp.ID
	}
	return &OpenAIForwardResult{
		RequestID:        requestID,
		ResponseID:       responseID,
		Usage:            usage,
		Model:            options.OriginalModel,
		BillingModel:     options.BillingModel,
		UpstreamModel:    options.UpstreamModel,
		ReasoningEffort:  options.ReasoningEffort,
		ServiceTier:      options.ServiceTier,
		Stream:           true,
		Duration:         time.Since(options.StartTime),
		FirstTokenMs:     &firstTokenMs,
		ClientDisconnect: clientDisconnected,
		WebSearchCalls:   webSearchCalls,
	}, nil
}
