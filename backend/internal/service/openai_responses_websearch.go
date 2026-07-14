package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/websearch"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type openAIResponsesWebSearchAction string

const (
	openAIResponsesWebSearchPass    openAIResponsesWebSearchAction = "pass"
	openAIResponsesWebSearchEmulate openAIResponsesWebSearchAction = "emulation"
	openAIResponsesWebSearchReject  openAIResponsesWebSearchAction = "reject"
)

type openAIResponsesWebSearchRequest struct {
	Request    *apicompat.ResponsesRequest
	Tool       *apicompat.ResponsesTool
	ToolChoice openAIResponsesToolChoice
}

type openAIResponsesToolChoiceKind string

const (
	openAIResponsesToolChoiceAbsent          openAIResponsesToolChoiceKind = "absent"
	openAIResponsesToolChoiceAuto            openAIResponsesToolChoiceKind = "auto"
	openAIResponsesToolChoiceRequired        openAIResponsesToolChoiceKind = "required"
	openAIResponsesToolChoiceNone            openAIResponsesToolChoiceKind = "none"
	openAIResponsesToolChoiceForcedWebSearch openAIResponsesToolChoiceKind = "forced_web_search"
	openAIResponsesToolChoiceForcedOther     openAIResponsesToolChoiceKind = "forced_other"
)

type openAIResponsesToolChoice struct {
	Kind openAIResponsesToolChoiceKind
}

// handleOpenAIResponsesWebSearch 对 OpenAI APIKey 的 Responses Web Search 做能力决策。
//
// @param ctx 请求上下文。
// @param c Gin 请求上下文。
// @param account 已选中的上游账号。
// @param body 原始 Responses 请求体。
// @return 转发结果、是否已处理，以及处理错误。
func (s *OpenAIGatewayService) handleOpenAIResponsesWebSearch(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
) (*OpenAIForwardResult, bool, error) {
	if account == nil || !account.IsOpenAIApiKey() || !bytes.Contains(body, []byte("web_search")) {
		return nil, false, nil
	}

	request, toolCount, webSearchCount, err := parseOpenAIResponsesWebSearchRequest(body)
	if err != nil {
		writeOpenAIResponsesWebSearchError(c, http.StatusBadRequest, "invalid_request_error", err.Error(), "tool_choice")
		return nil, true, err
	}
	forcedWebSearch := request.ToolChoice.Kind == openAIResponsesToolChoiceForcedWebSearch
	if webSearchCount == 0 && !forcedWebSearch {
		return nil, false, nil
	}
	if forcedWebSearch && webSearchCount == 0 {
		err := errors.New("tool_choice requires web_search, but no web_search tool is declared")
		writeOpenAIResponsesWebSearchError(c, http.StatusBadRequest, "invalid_request_error", err.Error(), "tool_choice")
		return nil, true, err
	}

	localEnabled := s.isOpenAIWebSearchEmulationEnabled(ctx, c, account)
	chatFallback := !openai_compat.ShouldUseResponsesAPI(account.Extra)
	eligible := shouldEmulateOpenAIResponsesWebSearch(toolCount, webSearchCount, request.ToolChoice.Kind)
	action := openAIResponsesWebSearchPass
	if localEnabled && eligible {
		action = openAIResponsesWebSearchEmulate
	} else if chatFallback && shouldRejectChatFallbackWebSearch(toolCount, webSearchCount, request.ToolChoice.Kind) {
		action = openAIResponsesWebSearchReject
	}

	logger.L().Info("openai web search capability decision",
		zap.Int64("account_id", account.ID),
		zap.String("model", request.Request.Model),
		zap.String("decision", string(action)),
		zap.String("tool_choice", string(request.ToolChoice.Kind)),
		zap.Int("tool_count", toolCount),
	)

	switch action {
	case openAIResponsesWebSearchPass:
		return nil, false, nil
	case openAIResponsesWebSearchReject:
		err := errors.New("this account routes Responses through Chat Completions, which cannot execute the requested web_search tool")
		writeOpenAIResponsesWebSearchError(c, http.StatusBadRequest, "invalid_request_error", err.Error(), "tools")
		return nil, true, err
	case openAIResponsesWebSearchEmulate:
		if responsesRequestUsesJSONSchema(request.Request) {
			err := errors.New("local web_search emulation is incompatible with text.format=json_schema")
			writeOpenAIResponsesWebSearchError(c, http.StatusBadRequest, "invalid_request_error", err.Error(), "text.format")
			return nil, true, err
		}
		if s.settingService == nil || !s.settingService.IsWebSearchEmulationEnabled(ctx) || getWebSearchManager() == nil {
			err := errors.New("local web_search emulation is enabled for this account, but no global search provider is available")
			writeOpenAIResponsesWebSearchError(c, http.StatusServiceUnavailable, "web_search_unavailable", err.Error(), "tools")
			return nil, true, err
		}
		result, executeErr := s.executeOpenAIResponsesWebSearch(ctx, c, account, request)
		return result, true, executeErr
	default:
		return nil, false, nil
	}
}

func parseOpenAIResponsesWebSearchRequest(body []byte) (*openAIResponsesWebSearchRequest, int, int, error) {
	var req apicompat.ResponsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, 0, 0, fmt.Errorf("parse Responses request: %w", err)
	}
	effectiveTools, err := apicompat.EffectiveResponsesTools(&req)
	if err != nil {
		return nil, 0, 0, err
	}
	toolChoice, err := classifyOpenAIResponsesToolChoice(req.ToolChoice)
	if err != nil {
		return nil, 0, 0, err
	}
	webSearchCount := 0
	var selected *apicompat.ResponsesTool
	for i := range effectiveTools {
		if !isOpenAIResponsesWebSearchTool(effectiveTools[i].Type) {
			continue
		}
		webSearchCount++
		if selected == nil {
			selected = &effectiveTools[i]
		}
	}
	return &openAIResponsesWebSearchRequest{Request: &req, Tool: selected, ToolChoice: toolChoice}, len(effectiveTools), webSearchCount, nil
}

func isOpenAIResponsesWebSearchTool(toolType string) bool {
	switch toolType {
	case "web_search", "web_search_preview", "web_search_20250305":
		return true
	default:
		return false
	}
}

func classifyOpenAIResponsesToolChoice(raw json.RawMessage) (openAIResponsesToolChoice, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return openAIResponsesToolChoice{Kind: openAIResponsesToolChoiceAbsent}, nil
	}
	var scalar string
	if json.Unmarshal(trimmed, &scalar) == nil {
		switch scalar {
		case "auto":
			return openAIResponsesToolChoice{Kind: openAIResponsesToolChoiceAuto}, nil
		case "required":
			return openAIResponsesToolChoice{Kind: openAIResponsesToolChoiceRequired}, nil
		case "none":
			return openAIResponsesToolChoice{Kind: openAIResponsesToolChoiceNone}, nil
		default:
			return openAIResponsesToolChoice{}, fmt.Errorf("unsupported tool_choice %q", scalar)
		}
	}
	var choice struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(trimmed, &choice); err != nil || strings.TrimSpace(choice.Type) == "" {
		return openAIResponsesToolChoice{}, errors.New("tool_choice must be auto, required, none, or a typed tool object")
	}
	if isOpenAIResponsesWebSearchTool(choice.Type) {
		return openAIResponsesToolChoice{Kind: openAIResponsesToolChoiceForcedWebSearch}, nil
	}
	return openAIResponsesToolChoice{Kind: openAIResponsesToolChoiceForcedOther}, nil
}

func shouldEmulateOpenAIResponsesWebSearch(toolCount, webSearchCount int, choice openAIResponsesToolChoiceKind) bool {
	if webSearchCount == 0 || choice == openAIResponsesToolChoiceNone || choice == openAIResponsesToolChoiceForcedOther {
		return false
	}
	if choice == openAIResponsesToolChoiceForcedWebSearch {
		return true
	}
	return toolCount == 1 && webSearchCount == 1
}

func shouldRejectChatFallbackWebSearch(toolCount, webSearchCount int, choice openAIResponsesToolChoiceKind) bool {
	if webSearchCount == 0 || choice == openAIResponsesToolChoiceNone {
		return false
	}
	// 指定其它已声明工具时，Chat 桥仍可保留该工具和选择；纯 Web Search 请求则无法满足该选择。
	return choice != openAIResponsesToolChoiceForcedOther || toolCount == webSearchCount
}

func responsesRequestUsesJSONSchema(req *apicompat.ResponsesRequest) bool {
	if req == nil || req.Text == nil || len(req.Text.Format) == 0 {
		return false
	}
	var format struct {
		Type string `json:"type"`
	}
	return json.Unmarshal(req.Text.Format, &format) == nil && format.Type == "json_schema"
}

func (s *OpenAIGatewayService) isOpenAIWebSearchEmulationEnabled(ctx context.Context, c *gin.Context, account *Account) bool {
	switch account.GetWebSearchEmulationMode() {
	case WebSearchModeEnabled:
		return true
	case WebSearchModeDisabled:
		return false
	}
	if s == nil || s.channelService == nil {
		return false
	}
	apiKey := getAPIKeyFromContext(c)
	if apiKey == nil || apiKey.GroupID == nil {
		return false
	}
	channel, err := s.channelService.GetChannelForGroup(ctx, *apiKey.GroupID)
	if err != nil || channel == nil {
		return false
	}
	return channel.IsWebSearchEmulationEnabled(PlatformOpenAI)
}

func (s *OpenAIGatewayService) executeOpenAIResponsesWebSearch(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	request *openAIResponsesWebSearchRequest,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	query := extractOpenAIResponsesSearchQuery(request.Request.Input)
	if strings.TrimSpace(query) == "" {
		err := errors.New("web_search requires a non-empty final user input")
		writeOpenAIResponsesWebSearchError(c, http.StatusBadRequest, "invalid_request_error", err.Error(), "input")
		return nil, err
	}

	maxResults, err := openAIResponsesWebSearchMaxResults(request.Tool)
	if err != nil {
		writeOpenAIResponsesWebSearchError(c, http.StatusBadRequest, "invalid_request_error", err.Error(), "tools")
		return nil, err
	}
	allowedDomains, blockedDomains, err := normalizeOpenAIResponsesWebSearchDomains(request.Tool)
	if err != nil {
		writeOpenAIResponsesWebSearchError(c, http.StatusBadRequest, "invalid_request_error", err.Error(), "tools")
		return nil, err
	}
	if err := validateLocalOpenAIResponsesWebSearchTool(request.Tool); err != nil {
		writeOpenAIResponsesWebSearchError(c, http.StatusBadRequest, "invalid_request_error", err.Error(), "tools")
		return nil, err
	}

	searchExecutor := doWebSearchWithMaxResults
	if s.openAIWebSearchExecutor != nil {
		searchExecutor = s.openAIWebSearchExecutor
	}
	searchResponse, provider, err := searchExecutor(ctx, account, query, maxResults)
	if err != nil {
		if errors.Is(err, websearch.ErrProxyUnavailable) {
			return nil, &UpstreamFailoverError{StatusCode: http.StatusBadGateway, ResponseBody: []byte("web search account proxy unavailable")}
		}
		writeOpenAIResponsesWebSearchError(c, http.StatusBadGateway, "web_search_failed", "All configured web search providers failed", "tools")
		return nil, err
	}
	if searchResponse == nil {
		err := errors.New("web search provider returned an empty response")
		writeOpenAIResponsesWebSearchError(c, http.StatusBadGateway, "web_search_failed", "All configured web search providers failed", "tools")
		return nil, err
	}
	searchResponse.Results = filterOpenAIResponsesSearchResults(searchResponse.Results, allowedDomains, blockedDomains)

	response := buildOpenAIResponsesWebSearchResponse(request.Request.Model, query, searchResponse.Results)
	if request.Request.Stream {
		err = writeOpenAIResponsesWebSearchStream(c, response)
	} else {
		err = writeOpenAIResponsesWebSearchJSON(c, response)
	}
	if err != nil {
		return nil, err
	}

	logger.L().Info("openai web search emulation completed",
		zap.Int64("account_id", account.ID),
		zap.String("model", request.Request.Model),
		zap.String("provider", provider),
		zap.Int("results_count", len(searchResponse.Results)),
	)
	return &OpenAIForwardResult{
		ResponseID:       response.ID,
		Model:            request.Request.Model,
		UpstreamModel:    request.Request.Model,
		UpstreamEndpoint: openAIResponsesEndpoint,
		Stream:           request.Request.Stream,
		Duration:         time.Since(startTime),
		WebSearchCalls:   1,
	}, nil
}

func extractOpenAIResponsesSearchQuery(raw json.RawMessage) string {
	var direct string
	if json.Unmarshal(raw, &direct) == nil {
		return strings.TrimSpace(direct)
	}
	var items []apicompat.ResponsesInputItem
	if json.Unmarshal(raw, &items) != nil {
		return ""
	}
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].Role != "user" {
			continue
		}
		var text string
		if json.Unmarshal(items[i].Content, &text) == nil {
			return strings.TrimSpace(text)
		}
		var parts []apicompat.ResponsesContentPart
		if json.Unmarshal(items[i].Content, &parts) != nil {
			return ""
		}
		for j := len(parts) - 1; j >= 0; j-- {
			if parts[j].Type == "input_text" || parts[j].Type == "text" {
				if text := strings.TrimSpace(parts[j].Text); text != "" {
					return text
				}
			}
		}
		return ""
	}
	return ""
}

func openAIResponsesWebSearchMaxResults(tool *apicompat.ResponsesTool) (int, error) {
	if tool == nil {
		return webSearchDefaultMaxResults, nil
	}
	if tool.MaxUses != nil && *tool.MaxUses < 1 {
		return 0, errors.New("web_search max_uses must be greater than zero")
	}
	switch strings.TrimSpace(tool.SearchContextSize) {
	case "", "medium":
		return 5, nil
	case "low":
		return 3, nil
	case "high":
		return 10, nil
	default:
		return 0, fmt.Errorf("unsupported web_search search_context_size %q", tool.SearchContextSize)
	}
}

func validateLocalOpenAIResponsesWebSearchTool(tool *apicompat.ResponsesTool) error {
	if tool == nil {
		return nil
	}
	if tool.UserLocation != nil {
		return errors.New("local web_search emulation does not support user_location")
	}
	if tool.ExternalWebAccess != nil {
		return errors.New("local web_search emulation does not support external_web_access")
	}
	if tool.ReturnTokenBudget != nil {
		return errors.New("local web_search emulation does not support return_token_budget")
	}
	return nil
}

func normalizeOpenAIResponsesWebSearchDomains(tool *apicompat.ResponsesTool) ([]string, []string, error) {
	if tool == nil || tool.Filters == nil {
		return nil, nil, nil
	}
	allowed, err := normalizeOpenAIResponsesDomainList(tool.Filters.AllowedDomains)
	if err != nil {
		return nil, nil, err
	}
	blocked, err := normalizeOpenAIResponsesDomainList(tool.Filters.BlockedDomains)
	if err != nil {
		return nil, nil, err
	}
	return allowed, blocked, nil
}

func normalizeOpenAIResponsesDomainList(domains []string) ([]string, error) {
	result := make([]string, 0, len(domains))
	for _, domain := range domains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain == "" || strings.Contains(domain, "://") || strings.ContainsAny(domain, "/?#") {
			return nil, fmt.Errorf("invalid web_search domain %q", domain)
		}
		parsed, err := url.Parse("https://" + domain)
		if err != nil || parsed.Hostname() != domain {
			return nil, fmt.Errorf("invalid web_search domain %q", domain)
		}
		result = append(result, domain)
	}
	return result, nil
}

func filterOpenAIResponsesSearchResults(results []websearch.SearchResult, allowed, blocked []string) []websearch.SearchResult {
	if len(allowed) == 0 && len(blocked) == 0 {
		return results
	}
	filtered := make([]websearch.SearchResult, 0, len(results))
	for _, result := range results {
		host := ""
		if parsed, err := url.Parse(result.URL); err == nil {
			host = strings.ToLower(parsed.Hostname())
		}
		if host == "" || domainListMatchesHost(blocked, host) {
			continue
		}
		if len(allowed) > 0 && !domainListMatchesHost(allowed, host) {
			continue
		}
		filtered = append(filtered, result)
	}
	return filtered
}

func domainListMatchesHost(domains []string, host string) bool {
	for _, domain := range domains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func buildOpenAIResponsesWebSearchResponse(model, query string, results []websearch.SearchResult) *apicompat.ResponsesResponse {
	responseID := "resp_ws_" + uuid.NewString()
	searchID := "ws_" + uuid.NewString()
	messageID := "msg_ws_" + uuid.NewString()
	text, annotations := buildOpenAIResponsesSearchSummary(query, results)
	return &apicompat.ResponsesResponse{
		ID:     responseID,
		Object: "response",
		Model:  model,
		Status: "completed",
		Output: []apicompat.ResponsesOutput{
			{Type: "web_search_call", ID: searchID, Status: "completed", Action: &apicompat.WebSearchAction{Type: "search", Query: query}},
			{Type: "message", ID: messageID, Role: "assistant", Status: "completed", Content: []apicompat.ResponsesContentPart{{Type: "output_text", Text: text, Annotations: annotations}}},
		},
		Usage: &apicompat.ResponsesUsage{},
	}
}

func buildOpenAIResponsesSearchSummary(query string, results []websearch.SearchResult) (string, []apicompat.ResponsesAnnotation) {
	if len(results) == 0 {
		return "No search results found for: " + query, nil
	}
	var summary strings.Builder
	var annotations []apicompat.ResponsesAnnotation
	fmt.Fprintf(&summary, "Search results for %q:\n\n", query)
	for index, result := range results {
		fmt.Fprintf(&summary, "%d. %s\n", index+1, result.Title)
		start := utf8.RuneCountInString(summary.String())
		summary.WriteString(result.URL)
		end := utf8.RuneCountInString(summary.String())
		summary.WriteByte('\n')
		if result.Snippet != "" {
			summary.WriteString(result.Snippet)
			summary.WriteByte('\n')
		}
		summary.WriteByte('\n')
		if result.URL != "" {
			annotations = append(annotations, apicompat.ResponsesAnnotation{
				Type: "url_citation", URL: result.URL, Title: result.Title, StartIndex: start, EndIndex: end,
			})
		}
	}
	return summary.String(), annotations
}

func writeOpenAIResponsesWebSearchJSON(c *gin.Context, response *apicompat.ResponsesResponse) error {
	body, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal local web_search response: %w", err)
	}
	MarkResponseCommitted(c)
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	written, err := c.Writer.Write(body)
	if err != nil {
		return fmt.Errorf("write local web_search response: %w", err)
	}
	if written != len(body) {
		return io.ErrShortWrite
	}
	return nil
}

func writeOpenAIResponsesWebSearchStream(c *gin.Context, response *apicompat.ResponsesResponse) error {
	MarkResponseCommitted(c)
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	searchItem := response.Output[0]
	messageItem := response.Output[1]
	part := messageItem.Content[0]
	sequence := 0
	next := func(eventType string, event apicompat.ResponsesStreamEvent) apicompat.ResponsesStreamEvent {
		event.Type = eventType
		event.SequenceNumber = sequence
		sequence++
		return event
	}
	events := []apicompat.ResponsesStreamEvent{
		next("response.created", apicompat.ResponsesStreamEvent{Response: &apicompat.ResponsesResponse{ID: response.ID, Object: "response", Model: response.Model, Status: "in_progress", Output: []apicompat.ResponsesOutput{}}}),
		next("response.output_item.added", apicompat.ResponsesStreamEvent{OutputIndex: 0, Item: &apicompat.ResponsesOutput{Type: searchItem.Type, ID: searchItem.ID, Status: "in_progress", Action: searchItem.Action}}),
		next("response.output_item.done", apicompat.ResponsesStreamEvent{OutputIndex: 0, Item: &searchItem}),
		next("response.output_item.added", apicompat.ResponsesStreamEvent{OutputIndex: 1, Item: &apicompat.ResponsesOutput{Type: "message", ID: messageItem.ID, Role: "assistant", Status: "in_progress"}}),
		next("response.content_part.added", apicompat.ResponsesStreamEvent{OutputIndex: 1, ContentIndex: 0, ItemID: messageItem.ID, Part: &apicompat.ResponsesContentPart{Type: "output_text", Text: ""}}),
		next("response.output_text.delta", apicompat.ResponsesStreamEvent{OutputIndex: 1, ContentIndex: 0, ItemID: messageItem.ID, Delta: part.Text}),
	}
	for index := range part.Annotations {
		annotationIndex := index
		annotation := part.Annotations[index]
		events = append(events, next("response.output_text.annotation.added", apicompat.ResponsesStreamEvent{
			OutputIndex: 1, ContentIndex: 0, ItemID: messageItem.ID, Annotation: &annotation, AnnotationIndex: &annotationIndex,
		}))
	}
	events = append(events,
		next("response.output_text.done", apicompat.ResponsesStreamEvent{OutputIndex: 1, ContentIndex: 0, ItemID: messageItem.ID, Text: part.Text}),
		next("response.content_part.done", apicompat.ResponsesStreamEvent{OutputIndex: 1, ContentIndex: 0, ItemID: messageItem.ID, Part: &part}),
		next("response.output_item.done", apicompat.ResponsesStreamEvent{OutputIndex: 1, Item: &messageItem}),
		next("response.completed", apicompat.ResponsesStreamEvent{Response: response}),
	)

	for _, event := range events {
		frame, err := apicompat.ResponsesEventToSSE(event)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(c.Writer, frame); err != nil {
			return fmt.Errorf("write local web_search stream: %w", err)
		}
		c.Writer.Flush()
	}
	if _, err := io.WriteString(c.Writer, "data: [DONE]\n\n"); err != nil {
		return fmt.Errorf("write local web_search stream terminator: %w", err)
	}
	c.Writer.Flush()
	return nil
}

func writeOpenAIResponsesWebSearchError(c *gin.Context, status int, code, message, param string) {
	if c == nil {
		return
	}
	errType := "api_error"
	if status >= http.StatusBadRequest && status < http.StatusInternalServerError {
		errType = "invalid_request_error"
	}
	MarkResponseCommitted(c)
	c.JSON(status, gin.H{"error": gin.H{
		"code": code, "message": message, "param": param, "type": errType,
	}})
}
