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
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/websearch"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	openAIResponsesWebRunNamespace         = "web"
	openAIResponsesWebRunName              = "run"
	openAIResponsesTypedWebSearchToolName  = "sub2api_web_search"
	openAIResponsesWebRunMaxQueries        = 5
	openAIResponsesWebRunMaxQueriesPerCall = 4
	openAIResponsesWebRunMaxRounds         = openAIResponsesWebRunMaxQueries
	openAIResponsesWebRunDefaultLength     = "medium"
	openAIResponsesWebRunMaxTitleBytes     = 512
	openAIResponsesWebRunMaxURLBytes       = 2048
	openAIResponsesWebRunMaxSnippetBytes   = 4096
	openAIResponsesWebRunToolDescription   = "Search the public web. Only search_query is supported; use it for current information, including weather queries."
	openAIResponsesWebSearchMaxSources     = 5
)

var (
	openAIResponsesWebRunToolSchema = fmt.Sprintf(
		`{"type":"object","properties":{"search_query":{"type":"array","minItems":1,"maxItems":%d,"items":{"type":"object","properties":{"q":{"type":"string","minLength":1},"recency":{"type":"integer","minimum":0}},"required":["q"],"additionalProperties":false}},"response_length":{"type":"string","enum":["short","medium","long"]}},"required":["search_query"],"additionalProperties":false}`,
		openAIResponsesWebRunMaxQueriesPerCall,
	)
	openAIResponsesTypedWebSearchToolSchema = fmt.Sprintf(
		`{"type":"object","properties":{"search_query":{"type":"array","minItems":1,"maxItems":%d,"items":{"type":"object","properties":{"q":{"type":"string","minLength":1}},"required":["q"],"additionalProperties":false}}},"required":["search_query"],"additionalProperties":false}`,
		openAIResponsesWebRunMaxQueriesPerCall,
	)
)

type openAIResponsesInternalWebToolKind string

const (
	openAIResponsesInternalWebToolWebRun      openAIResponsesInternalWebToolKind = "web_run"
	openAIResponsesInternalWebToolTypedSearch openAIResponsesInternalWebToolKind = "typed_web_search"
)

type openAIResponsesInternalWebToolConfig struct {
	Name           string
	Kind           openAIResponsesInternalWebToolKind
	MaxResults     int
	MaxRounds      int
	AllowedDomains []string
	BlockedDomains []string
}

type openAIResponsesWebRunLoopOptions struct {
	OriginalModel    string
	BillingModel     string
	UpstreamModel    string
	ReasoningEffort  *string
	ServiceTier      *string
	ClientStream     bool
	CustomTools      map[string]bool
	FunctionTools    map[string]bool
	ToolSearch       bool
	NamespaceTools   map[string]apicompat.NamespacedToolName
	InternalWebTools map[string]openAIResponsesInternalWebToolConfig
	AppendSources    bool
	StartTime        time.Time
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

type openAIResponsesWebSearchSourceProjection struct {
	OutputIndex  int
	ContentIndex int
	ItemID       string
	Suffix       string
	FinalText    string
	Annotations  []apicompat.ResponsesAnnotation
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

func addOpenAIResponsesTypedWebSearchTool(req *apicompat.ChatCompletionsRequest, config openAIResponsesInternalWebToolConfig) error {
	if req == nil || strings.TrimSpace(config.Name) == "" {
		return errors.New("typed web_search internal tool configuration is missing")
	}
	for _, tool := range req.Tools {
		if tool.Function != nil && tool.Function.Name == config.Name {
			return fmt.Errorf("typed web_search internal tool name %q conflicts with a declared client tool", config.Name)
		}
	}
	req.Tools = append(req.Tools, apicompat.ChatTool{
		Type: "function",
		Function: &apicompat.ChatFunction{
			Name:        config.Name,
			Description: openAIResponsesWebRunToolDescription,
			Parameters:  json.RawMessage(openAIResponsesTypedWebSearchToolSchema),
		},
	})
	parallel := false
	req.ParallelToolCalls = &parallel
	return nil
}

func parseOpenAIResponsesWebRunArguments(arguments string, remainingQueries int) (*openAIResponsesWebRunArguments, *openAIResponsesWebRunError) {
	if remainingQueries < 1 {
		return nil, &openAIResponsesWebRunError{
			Code:    "search_limit_exceeded",
			Message: fmt.Sprintf("The request has reached the maximum of %d web search queries", openAIResponsesWebRunMaxQueries),
		}
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
	if len(parsed.SearchQuery) > openAIResponsesWebRunMaxQueriesPerCall {
		return nil, &openAIResponsesWebRunError{
			Code:    "search_limit_exceeded",
			Message: fmt.Sprintf("Each web.run call supports at most %d web search queries", openAIResponsesWebRunMaxQueriesPerCall),
		}
	}
	if len(parsed.SearchQuery) > remainingQueries {
		return nil, &openAIResponsesWebRunError{
			Code:    "search_limit_exceeded",
			Message: fmt.Sprintf("The request supports at most %d web search queries", openAIResponsesWebRunMaxQueries),
		}
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

func parseOpenAIResponsesTypedWebSearchArguments(arguments string, remainingQueries int) (*openAIResponsesWebRunArguments, *openAIResponsesWebRunError) {
	if remainingQueries < 1 {
		return nil, &openAIResponsesWebRunError{
			Code:    "search_limit_exceeded",
			Message: fmt.Sprintf("The request has reached the maximum of %d web search queries", openAIResponsesWebRunMaxQueries),
		}
	}
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return nil, &openAIResponsesWebRunError{Code: "invalid_tool_arguments", Message: "web_search requires a JSON object containing search_query"}
	}
	var parsed struct {
		SearchQuery []struct {
			Q string `json:"q"`
		} `json:"search_query"`
	}
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parsed); err != nil {
		return nil, &openAIResponsesWebRunError{Code: "invalid_tool_arguments", Message: "web_search search_query arguments are invalid"}
	}
	if len(parsed.SearchQuery) == 0 {
		return nil, &openAIResponsesWebRunError{Code: "invalid_tool_arguments", Message: "web_search search_query must contain at least one query"}
	}
	if len(parsed.SearchQuery) > openAIResponsesWebRunMaxQueriesPerCall {
		return nil, &openAIResponsesWebRunError{
			Code:    "search_limit_exceeded",
			Message: fmt.Sprintf("Each web_search call supports at most %d web search queries", openAIResponsesWebRunMaxQueriesPerCall),
		}
	}
	if len(parsed.SearchQuery) > remainingQueries {
		return nil, &openAIResponsesWebRunError{
			Code:    "search_limit_exceeded",
			Message: fmt.Sprintf("The request supports at most %d web search queries", openAIResponsesWebRunMaxQueries),
		}
	}
	result := &openAIResponsesWebRunArguments{SearchQuery: make([]openAIResponsesWebRunQuery, 0, len(parsed.SearchQuery))}
	for _, query := range parsed.SearchQuery {
		query.Q = strings.TrimSpace(query.Q)
		if query.Q == "" {
			return nil, &openAIResponsesWebRunError{Code: "invalid_tool_arguments", Message: "Each web_search search_query item requires a non-empty q"}
		}
		result.SearchQuery = append(result.SearchQuery, openAIResponsesWebRunQuery{Q: query.Q})
	}
	return result, nil
}

func parseOpenAIResponsesInternalWebToolArguments(
	config openAIResponsesInternalWebToolConfig,
	arguments string,
	remainingQueries int,
) (*openAIResponsesWebRunArguments, *openAIResponsesWebRunError) {
	switch config.Kind {
	case openAIResponsesInternalWebToolWebRun:
		return parseOpenAIResponsesWebRunArguments(arguments, remainingQueries)
	case openAIResponsesInternalWebToolTypedSearch:
		return parseOpenAIResponsesTypedWebSearchArguments(arguments, remainingQueries)
	default:
		return nil, &openAIResponsesWebRunError{Code: "unsupported_tool", Message: "The selected internal web tool is unsupported"}
	}
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

func (s *OpenAIGatewayService) executeOpenAIResponsesWebRunSearch(
	ctx context.Context,
	account *Account,
	parsed *openAIResponsesWebRunArguments,
	config openAIResponsesInternalWebToolConfig,
) (openAIResponsesWebRunOutput, int, error) {
	if parsed == nil {
		return openAIResponsesWebRunOutput{
			Error: &openAIResponsesWebRunError{Code: "invalid_tool_arguments", Message: "web.run search_query arguments are missing"},
		}, 0, nil
	}
	if s.settingService == nil || !s.settingService.IsWebSearchEmulationEnabled(ctx) || getWebSearchManager() == nil {
		return openAIResponsesWebRunOutput{
			Error: &openAIResponsesWebRunError{Code: "web_search_unavailable", Message: "No global web search provider is available"},
		}, 0, nil
	}
	maxResults := config.MaxResults
	if config.Kind == openAIResponsesInternalWebToolWebRun {
		var err error
		maxResults, err = openAIResponsesWebRunMaxResults(parsed.ResponseLength)
		if err != nil {
			return openAIResponsesWebRunOutput{
				Error: &openAIResponsesWebRunError{Code: "invalid_tool_arguments", Message: err.Error()},
			}, 0, nil
		}
	}
	if maxResults < 1 {
		maxResults = webSearchDefaultMaxResults
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
				return openAIResponsesWebRunOutput{}, successfulCalls, &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: []byte("web search account proxy unavailable")}
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
		response.Results = filterOpenAIResponsesSearchResults(response.Results, config.AllowedDomains, config.BlockedDomains)
		group.Provider = provider
		results := response.Results
		if len(results) > maxResults {
			results = results[:maxResults]
		}
		group.Results = sanitizeOpenAIResponsesWebRunResults(results)
		output.SearchQuery = append(output.SearchQuery, group)
		successfulCalls++
	}
	return output, successfulCalls, nil
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

func collectOpenAIResponsesWebSearchSources(output openAIResponsesWebRunOutput) []websearch.SearchResult {
	var sources []websearch.SearchResult
	for _, group := range output.SearchQuery {
		if group.Error != nil {
			continue
		}
		sources = append(sources, group.Results...)
	}
	return sources
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
	var webSearchItems []apicompat.ResponsesOutput
	var webSearchSources []websearch.SearchResult
	toolRounds := make(map[string]int, len(options.InternalWebTools))
	searchModes := make(map[string]bool, len(options.InternalWebTools))
	searchProviders := make(map[string]bool)
	typedWebSearchExecuted := false
	for {
		chatBody, marshalErr := json.Marshal(chatReq)
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal web.run chat completions request: %w", marshalErr)
		}
		chatBody, err = s.applyDeepSeekMissingReasoningAutoDowngrade(
			ctx,
			account,
			options.UpstreamModel,
			chatBody,
			deepSeekMissingReasoningSourceResponsesWebRun,
		)
		if err != nil {
			return nil, fmt.Errorf("apply DeepSeek missing reasoning policy to web.run request: %w", err)
		}
		options.ReasoningEffort = extractOpenAIUpstreamReasoningEffort(
			chatBody,
			options.OriginalModel,
			options.UpstreamModel,
			options.BillingModel,
		)
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
		choice, webRunCall, toolConfig, callErr := selectOpenAIResponsesInternalWebToolCall(ccResp, options.InternalWebTools)
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
				zap.Strings("search_modes", sortedOpenAIResponsesWebSearchLogValues(searchModes)),
				zap.Strings("providers", sortedOpenAIResponsesWebSearchLogValues(searchProviders)),
			}
			if webRunRounds > 0 {
				logger.L().Info("openai internal web search loop completed", fields...)
			} else {
				logger.L().Debug("openai internal web search tool was available but not selected", fields...)
			}
			resultOptions := options
			resultOptions.AppendSources = options.AppendSources && typedWebSearchExecuted
			return s.writeOpenAIResponsesWebRunResult(c, ccResp, responseHeaders, requestID, aggregateUsage, webSearchCalls, webSearchItems, webSearchSources, resultOptions)
		}
		maxToolRounds := toolConfig.MaxRounds
		if maxToolRounds < 1 {
			maxToolRounds = openAIResponsesWebRunMaxRounds
		}
		globalRoundLimitReached := webRunRounds >= openAIResponsesWebRunMaxRounds
		toolRoundLimitReached := toolRounds[toolConfig.Name] >= maxToolRounds
		if globalRoundLimitReached || toolRoundLimitReached {
			// 搜索额度耗尽后仍需补齐当前 tool_call 的结果，再移除内部搜索工具继续生成。
			// 这样既避免无界循环，也不会把网关的内部保护机制作为 502 暴露给客户端。
			toolOutput := marshalOpenAIResponsesWebRunOutput(openAIResponsesWebRunOutput{
				Error: &openAIResponsesWebRunError{
					Code:    "search_limit_reached",
					Message: "Web search limit reached. Use the available search results to answer without calling web search again.",
				},
			})
			appendOpenAIResponsesWebRunMessages(chatReq, choice.Message, webRunCall, toolOutput, webRunRounds+1)
			disableOpenAIResponsesInternalWebTools(chatReq, options.InternalWebTools, toolConfig.Name, globalRoundLimitReached)
			logger.L().Info("openai internal web search limit reached; continuing without the exhausted tool",
				zap.Int64("account_id", account.ID),
				zap.String("model", options.OriginalModel),
				zap.String("tool", toolConfig.Name),
				zap.Int("search_rounds", webRunRounds),
				zap.Int("tool_rounds", toolRounds[toolConfig.Name]),
				zap.Bool("global_limit", globalRoundLimitReached),
			)
			continue
		}
		webRunRounds++
		toolRounds[toolConfig.Name]++
		searchModes[string(toolConfig.Kind)] = true

		remainingQueries := openAIResponsesWebRunMaxQueries - queryCount
		parsed, argumentErr := parseOpenAIResponsesInternalWebToolArguments(toolConfig, webRunCall.Function.Arguments, remainingQueries)
		toolOutput := ""
		if argumentErr != nil {
			toolOutput = marshalOpenAIResponsesWebRunOutput(openAIResponsesWebRunOutput{Error: argumentErr})
		} else {
			queryCount += len(parsed.SearchQuery)
			var successfulCalls int
			var searchOutput openAIResponsesWebRunOutput
			searchOutput, successfulCalls, err = s.executeOpenAIResponsesWebRunSearch(ctx, account, parsed, toolConfig)
			if err != nil {
				return nil, err
			}
			toolOutput = marshalOpenAIResponsesWebRunOutput(searchOutput)
			webSearchItems = append(webSearchItems, buildOpenAIResponsesWebRunSearchItems(parsed, searchOutput, webRunRounds)...)
			webSearchSources = append(webSearchSources, collectOpenAIResponsesWebSearchSources(searchOutput)...)
			for _, group := range searchOutput.SearchQuery {
				if provider := strings.TrimSpace(group.Provider); group.Error == nil && provider != "" {
					searchProviders[truncateString(provider, 64)] = true
				}
			}
			webSearchCalls += successfulCalls
			if toolConfig.Kind == openAIResponsesInternalWebToolTypedSearch && successfulCalls > 0 {
				typedWebSearchExecuted = true
			}
		}
		appendOpenAIResponsesWebRunMessages(chatReq, choice.Message, webRunCall, toolOutput, webRunRounds)
	}
}

func sortedOpenAIResponsesWebSearchLogValues(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func buildOpenAIResponsesWebRunSearchItems(
	parsed *openAIResponsesWebRunArguments,
	output openAIResponsesWebRunOutput,
	round int,
) []apicompat.ResponsesOutput {
	if parsed == nil || len(parsed.SearchQuery) == 0 {
		return nil
	}
	items := make([]apicompat.ResponsesOutput, 0, len(parsed.SearchQuery))
	for index, query := range parsed.SearchQuery {
		status := "completed"
		if output.Error != nil || index >= len(output.SearchQuery) || output.SearchQuery[index].Error != nil {
			status = "failed"
		}
		items = append(items, apicompat.ResponsesOutput{
			Type:   "web_search_call",
			ID:     deterministicOpenAIResponsesWebRunSearchID(round, index, query.Q),
			Status: status,
			Action: &apicompat.WebSearchAction{Type: "search", Query: query.Q},
		})
	}
	return items
}

func selectOpenAIResponsesInternalWebToolCall(
	resp *apicompat.ChatCompletionsResponse,
	internalTools map[string]openAIResponsesInternalWebToolConfig,
) (*apicompat.ChatChoice, *apicompat.ChatToolCall, openAIResponsesInternalWebToolConfig, error) {
	if resp == nil || len(resp.Choices) == 0 {
		return nil, nil, openAIResponsesInternalWebToolConfig{}, nil
	}
	choice := &resp.Choices[0]
	internalToolIndex := -1
	var selectedConfig openAIResponsesInternalWebToolConfig
	for i := range choice.Message.ToolCalls {
		config, ok := internalTools[choice.Message.ToolCalls[i].Function.Name]
		if !ok {
			continue
		}
		if internalToolIndex >= 0 || len(choice.Message.ToolCalls) != 1 {
			return nil, nil, openAIResponsesInternalWebToolConfig{}, errors.New("internal web search cannot be combined with parallel client tool calls")
		}
		internalToolIndex = i
		selectedConfig = config
	}
	if internalToolIndex < 0 {
		return choice, nil, openAIResponsesInternalWebToolConfig{}, nil
	}
	return choice, &choice.Message.ToolCalls[internalToolIndex], selectedConfig, nil
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

// disableOpenAIResponsesInternalWebTools 移除已耗尽的内部搜索工具，并把遗留的强制
// tool_choice 降级为 auto，避免下一轮引用已经不存在的工具而被上游拒绝。
func disableOpenAIResponsesInternalWebTools(
	req *apicompat.ChatCompletionsRequest,
	internalTools map[string]openAIResponsesInternalWebToolConfig,
	exhaustedToolName string,
	disableAll bool,
) {
	if req == nil {
		return
	}

	remainingTools := req.Tools[:0]
	for _, tool := range req.Tools {
		toolName := ""
		if tool.Function != nil {
			toolName = tool.Function.Name
		}
		_, internal := internalTools[toolName]
		if internal && (disableAll || toolName == exhaustedToolName) {
			delete(internalTools, toolName)
			continue
		}
		remainingTools = append(remainingTools, tool)
	}
	if disableAll {
		for toolName := range internalTools {
			delete(internalTools, toolName)
		}
	} else {
		delete(internalTools, exhaustedToolName)
	}

	req.Tools = remainingTools
	if len(req.Tools) == 0 {
		req.Tools = nil
		req.ToolChoice = nil
		req.ParallelToolCalls = nil
		return
	}
	req.ToolChoice = json.RawMessage(`"auto"`)
}

func deterministicOpenAIResponsesWebRunCallID(round int, name, arguments string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\n%s\n%s", round, name, arguments)))
	return "call_web_" + hex.EncodeToString(sum[:8])
}

func deterministicOpenAIResponsesWebRunSearchID(round, index int, query string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\n%d\n%s", round, index, query)))
	return "ws_" + hex.EncodeToString(sum[:8])
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
	webSearchItems []apicompat.ResponsesOutput,
	webSearchSources []websearch.SearchResult,
	options openAIResponsesWebRunLoopOptions,
) (*OpenAIForwardResult, error) {
	if options.ClientStream {
		return s.writeOpenAIResponsesWebRunStream(c, ccResp, upstreamHeaders, requestID, usage, webSearchCalls, webSearchItems, webSearchSources, options)
	}
	responsesResp := apicompat.ChatCompletionsResponseToResponses(ccResp, options.OriginalModel, options.CustomTools, options.FunctionTools, options.ToolSearch, options.NamespaceTools)
	prependOpenAIResponsesWebRunSearchItems(responsesResp, webSearchItems)
	if options.AppendSources {
		appendOpenAIResponsesWebSearchSources(responsesResp, webSearchSources)
	}
	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), upstreamHeaders, s.responseHeaderFilter)
	}
	c.JSON(http.StatusOK, responsesResp)
	return &OpenAIForwardResult{
		RequestID:                   requestID,
		UpstreamHeaders:             upstreamHeaders,
		ResponseID:                  responsesResp.ID,
		Usage:                       usage,
		Model:                       options.OriginalModel,
		BillingModel:                options.BillingModel,
		UpstreamModel:               options.UpstreamModel,
		ReasoningEffort:             options.ReasoningEffort,
		UpstreamResponseServiceTier: observedUpstreamResponseServiceTier(c),
		ServiceTier:                 resolvedOpenAIUpstreamServiceTier(c, options.ServiceTier),
		Stream:                      false,
		Duration:                    time.Since(options.StartTime),
		WebSearchCalls:              webSearchCalls,
	}, nil
}

func (s *OpenAIGatewayService) writeOpenAIResponsesWebRunStream(
	c *gin.Context,
	ccResp *apicompat.ChatCompletionsResponse,
	upstreamHeaders http.Header,
	requestID string,
	usage OpenAIUsage,
	webSearchCalls int,
	webSearchItems []apicompat.ResponsesOutput,
	webSearchSources []websearch.SearchResult,
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

	events := apicompat.ChatCompletionsResponseToResponsesEvents(ccResp, options.OriginalModel, options.CustomTools, options.FunctionTools, options.ToolSearch, options.NamespaceTools)
	events = addOpenAIResponsesWebRunSearchEvents(events, webSearchItems)
	if options.AppendSources {
		events = addOpenAIResponsesWebSearchSourceEvents(events, webSearchSources)
	}
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
		RequestID:                   requestID,
		UpstreamHeaders:             upstreamHeaders,
		ResponseID:                  responseID,
		Usage:                       usage,
		Model:                       options.OriginalModel,
		BillingModel:                options.BillingModel,
		UpstreamModel:               options.UpstreamModel,
		ReasoningEffort:             options.ReasoningEffort,
		UpstreamResponseServiceTier: observedUpstreamResponseServiceTier(c),
		ServiceTier:                 resolvedOpenAIUpstreamServiceTier(c, options.ServiceTier),
		Stream:                      true,
		Duration:                    time.Since(options.StartTime),
		FirstTokenMs:                &firstTokenMs,
		ClientDisconnect:            clientDisconnected,
		WebSearchCalls:              webSearchCalls,
	}, nil
}

func prependOpenAIResponsesWebRunSearchItems(response *apicompat.ResponsesResponse, searchItems []apicompat.ResponsesOutput) {
	if response == nil || len(searchItems) == 0 {
		return
	}
	output := make([]apicompat.ResponsesOutput, 0, len(searchItems)+len(response.Output))
	output = append(output, searchItems...)
	response.Output = append(output, response.Output...)
}

func addOpenAIResponsesWebRunSearchEvents(
	events []apicompat.ResponsesStreamEvent,
	searchItems []apicompat.ResponsesOutput,
) []apicompat.ResponsesStreamEvent {
	if len(events) == 0 || len(searchItems) == 0 {
		return events
	}

	// 内部 web.run 必须继续隐藏，但客户端需要标准搜索项才能展示搜索过程。
	output := make([]apicompat.ResponsesStreamEvent, 0, len(events)+len(searchItems)*2)
	sequence := 0
	searchEventsAdded := false
	appendEvent := func(event apicompat.ResponsesStreamEvent) {
		event.SequenceNumber = sequence
		sequence++
		output = append(output, event)
	}
	for _, event := range events {
		if event.Type == "response.created" {
			appendEvent(event)
			for index, item := range searchItems {
				inProgress := item
				inProgress.Status = "in_progress"
				appendEvent(apicompat.ResponsesStreamEvent{
					Type:        "response.output_item.added",
					OutputIndex: index,
					Item:        &inProgress,
				})
				completed := item
				appendEvent(apicompat.ResponsesStreamEvent{
					Type:        "response.output_item.done",
					OutputIndex: index,
					Item:        &completed,
				})
			}
			searchEventsAdded = true
			continue
		}
		if openAIResponsesEventHasOutputIndex(event.Type) {
			event.OutputIndex += len(searchItems)
		}
		if event.Type == "response.completed" {
			prependOpenAIResponsesWebRunSearchItems(event.Response, searchItems)
		}
		appendEvent(event)
	}
	if !searchEventsAdded {
		return events
	}
	return output
}

func openAIResponsesEventHasOutputIndex(eventType string) bool {
	switch eventType {
	case "response.output_item.added",
		"response.output_item.done",
		"response.content_part.added",
		"response.content_part.done",
		"response.output_text.delta",
		"response.output_text.done",
		"response.output_text.annotation.added",
		"response.reasoning_summary_part.added",
		"response.reasoning_summary_part.done",
		"response.reasoning_summary_text.delta",
		"response.reasoning_summary_text.done",
		"response.function_call_arguments.delta",
		"response.function_call_arguments.done",
		"response.custom_tool_call_input.delta",
		"response.custom_tool_call_input.done":
		return true
	default:
		return false
	}
}

func appendOpenAIResponsesWebSearchSources(
	response *apicompat.ResponsesResponse,
	sources []websearch.SearchResult,
) *openAIResponsesWebSearchSourceProjection {
	if response == nil {
		return nil
	}
	normalized := normalizeOpenAIResponsesWebSearchSources(sources)
	if len(normalized) == 0 {
		return nil
	}
	for outputIndex := len(response.Output) - 1; outputIndex >= 0; outputIndex-- {
		item := &response.Output[outputIndex]
		if item.Type != "message" || item.Role != "assistant" {
			continue
		}
		for contentIndex := len(item.Content) - 1; contentIndex >= 0; contentIndex-- {
			part := &item.Content[contentIndex]
			if part.Type != "output_text" || strings.TrimSpace(part.Text) == "" {
				continue
			}
			finalText, suffix, annotations := buildOpenAIResponsesWebSearchSourceAppendix(part.Text, normalized)
			if suffix == "" {
				return nil
			}
			part.Text = finalText
			part.Annotations = append(part.Annotations, annotations...)
			return &openAIResponsesWebSearchSourceProjection{
				OutputIndex:  outputIndex,
				ContentIndex: contentIndex,
				ItemID:       item.ID,
				Suffix:       suffix,
				FinalText:    finalText,
				Annotations:  annotations,
			}
		}
	}
	return nil
}

func normalizeOpenAIResponsesWebSearchSources(sources []websearch.SearchResult) []websearch.SearchResult {
	if len(sources) == 0 {
		return nil
	}
	// 来源只接受可直接展示的公开 HTTP(S) URL；去掉 fragment 后再去重，避免同一页面
	// 因页内锚点不同占用多个 citation 名额。
	seen := make(map[string]bool, min(len(sources), openAIResponsesWebSearchMaxSources))
	result := make([]websearch.SearchResult, 0, min(len(sources), openAIResponsesWebSearchMaxSources))
	for _, source := range sources {
		parsed, err := url.Parse(strings.TrimSpace(source.URL))
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
			continue
		}
		parsed.Scheme = strings.ToLower(parsed.Scheme)
		parsed.Host = strings.ToLower(parsed.Host)
		parsed.Fragment = ""
		normalizedURL := parsed.String()
		if len(normalizedURL) > openAIResponsesWebRunMaxURLBytes {
			continue
		}
		if seen[normalizedURL] {
			continue
		}
		seen[normalizedURL] = true
		title := strings.Join(strings.Fields(source.Title), " ")
		if title == "" {
			title = normalizedURL
		}
		result = append(result, websearch.SearchResult{
			URL:   normalizedURL,
			Title: truncateString(title, openAIResponsesWebRunMaxTitleBytes),
		})
		if len(result) >= openAIResponsesWebSearchMaxSources {
			break
		}
	}
	return result
}

func buildOpenAIResponsesWebSearchSourceAppendix(
	baseText string,
	sources []websearch.SearchResult,
) (string, string, []apicompat.ResponsesAnnotation) {
	if strings.TrimSpace(baseText) == "" || len(sources) == 0 {
		return baseText, "", nil
	}
	separator := "\n\n"
	if strings.HasSuffix(baseText, "\n") {
		separator = "\n"
	}
	var suffix strings.Builder
	_, _ = suffix.WriteString(separator)
	_, _ = suffix.WriteString("Sources:\n")
	// Responses annotation 使用字符位置而不是字节偏移；这里按 rune 计算，确保中文
	// 回答和标题不会让客户端截取到错误的 URL 范围。
	baseRunes := utf8.RuneCountInString(baseText)
	var annotations []apicompat.ResponsesAnnotation
	for _, source := range sources {
		_, _ = suffix.WriteString("- ")
		_, _ = suffix.WriteString(source.Title)
		_, _ = suffix.WriteString(": ")
		start := baseRunes + utf8.RuneCountInString(suffix.String())
		_, _ = suffix.WriteString(source.URL)
		end := baseRunes + utf8.RuneCountInString(suffix.String())
		_ = suffix.WriteByte('\n')
		annotations = append(annotations, apicompat.ResponsesAnnotation{
			Type:       "url_citation",
			URL:        source.URL,
			Title:      source.Title,
			StartIndex: start,
			EndIndex:   end,
		})
	}
	return baseText + suffix.String(), suffix.String(), annotations
}

func addOpenAIResponsesWebSearchSourceEvents(
	events []apicompat.ResponsesStreamEvent,
	sources []websearch.SearchResult,
) []apicompat.ResponsesStreamEvent {
	if len(events) == 0 || len(sources) == 0 {
		return events
	}
	var projection *openAIResponsesWebSearchSourceProjection
	// completed.response 是最终快照的唯一来源，先在快照上生成统一投影，再同步更新
	// output_text.done、content_part.done 和 output_item.done，避免四份文本或索引漂移。
	for i := range events {
		if events[i].Type == "response.completed" {
			projection = appendOpenAIResponsesWebSearchSources(events[i].Response, sources)
			break
		}
	}
	if projection == nil {
		return events
	}

	output := make([]apicompat.ResponsesStreamEvent, 0, len(events)+len(projection.Annotations)+1)
	for i := range events {
		event := events[i]
		matchesText := event.OutputIndex == projection.OutputIndex && event.ContentIndex == projection.ContentIndex && event.ItemID == projection.ItemID
		if event.Type == "response.output_text.done" && matchesText {
			output = append(output, apicompat.ResponsesStreamEvent{
				Type:         "response.output_text.delta",
				OutputIndex:  projection.OutputIndex,
				ContentIndex: projection.ContentIndex,
				ItemID:       projection.ItemID,
				Delta:        projection.Suffix,
			})
			for annotationIndex := range projection.Annotations {
				index := annotationIndex
				annotation := projection.Annotations[annotationIndex]
				output = append(output, apicompat.ResponsesStreamEvent{
					Type:            "response.output_text.annotation.added",
					OutputIndex:     projection.OutputIndex,
					ContentIndex:    projection.ContentIndex,
					ItemID:          projection.ItemID,
					Annotation:      &annotation,
					AnnotationIndex: &index,
				})
			}
			event.Text = projection.FinalText
		}
		if event.Type == "response.content_part.done" && matchesText && event.Part != nil {
			event.Part.Text = projection.FinalText
			event.Part.Annotations = append(event.Part.Annotations, projection.Annotations...)
		}
		if event.Type == "response.output_item.done" && event.OutputIndex == projection.OutputIndex && event.Item != nil && event.Item.ID == projection.ItemID {
			for contentIndex := range event.Item.Content {
				if contentIndex != projection.ContentIndex || event.Item.Content[contentIndex].Type != "output_text" {
					continue
				}
				event.Item.Content[contentIndex].Text = projection.FinalText
				event.Item.Content[contentIndex].Annotations = append(event.Item.Content[contentIndex].Annotations, projection.Annotations...)
			}
		}
		output = append(output, event)
	}
	for i := range output {
		output[i].SequenceNumber = i
	}
	return output
}
