package apicompat

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// Non-streaming: AnthropicResponse → ResponsesResponse
// ---------------------------------------------------------------------------

// AnthropicToResponsesResponse converts an Anthropic Messages response into a
// Responses API response. This is the reverse of ResponsesToAnthropic and
// enables Anthropic upstream responses to be returned in OpenAI Responses format.
func AnthropicToResponsesResponse(resp *AnthropicResponse) *ResponsesResponse {
	id := resp.ID
	if id == "" {
		id = generateResponsesID()
	}

	out := &ResponsesResponse{
		ID:     id,
		Object: "response",
		Model:  resp.Model,
	}

	var outputs []ResponsesOutput
	var msgParts []ResponsesContentPart
	searchFailed := false
	searchErrorCode := ""

	for _, block := range resp.Content {
		switch block.Type {
		case "thinking":
			if block.Thinking != "" {
				outputs = append(outputs, ResponsesOutput{
					Type: "reasoning",
					ID:   generateItemID(),
					Summary: []ResponsesSummary{{
						Type: "summary_text",
						Text: block.Thinking,
					}},
				})
			}
		case "text":
			if block.Text != "" {
				msgParts = append(msgParts, ResponsesContentPart{
					Type:        "output_text",
					Text:        block.Text,
					Annotations: anthropicCitationsToResponses(block.Citations, block.Text),
				})
			}
		case "server_tool_use":
			if block.Name != "web_search" {
				continue
			}
			itemID := block.ID
			if itemID == "" {
				itemID = generateItemID()
			}
			outputs = append(outputs, ResponsesOutput{
				Type:   "web_search_call",
				ID:     itemID,
				Status: "in_progress",
				Action: &WebSearchAction{Type: "search", Query: anthropicWebSearchQuery(block.Input)},
			})
		case "web_search_tool_result":
			code := anthropicWebSearchResultErrorCode(block)
			resultStatus := "completed"
			if code != "" {
				resultStatus = "failed"
				searchFailed = true
				searchErrorCode = code
			}
			for i := len(outputs) - 1; i >= 0; i-- {
				if outputs[i].Type == "web_search_call" && (block.ToolUseID == "" || outputs[i].ID == block.ToolUseID) {
					outputs[i].Status = resultStatus
					break
				}
			}
		case "tool_use":
			args := "{}"
			if len(block.Input) > 0 {
				args = string(block.Input)
			}
			outputs = append(outputs, ResponsesOutput{
				Type:      "function_call",
				ID:        generateItemID(),
				CallID:    toResponsesCallID(block.ID),
				Name:      block.Name,
				Arguments: args,
				Status:    "completed",
			})
		}
	}
	for i := range outputs {
		if outputs[i].Type != "web_search_call" || outputs[i].Status != "in_progress" {
			continue
		}
		outputs[i].Status = "failed"
		searchFailed = true
		if searchErrorCode == "" {
			searchErrorCode = "web_search_result_missing"
		}
	}

	// Assemble message output item from text parts
	if len(msgParts) > 0 {
		outputs = append(outputs, ResponsesOutput{
			Type:    "message",
			ID:      generateItemID(),
			Role:    "assistant",
			Content: msgParts,
			Status:  "completed",
		})
	}

	if len(outputs) == 0 {
		outputs = append(outputs, ResponsesOutput{
			Type:    "message",
			ID:      generateItemID(),
			Role:    "assistant",
			Content: []ResponsesContentPart{{Type: "output_text", Text: ""}},
			Status:  "completed",
		})
	}
	out.Output = outputs

	// Map stop_reason → status
	out.Status = anthropicStopReasonToResponsesStatus(resp.StopReason, resp.Content)
	if searchFailed {
		out.Status = "failed"
		out.Error = &ResponsesError{
			Code:    searchErrorCode,
			Message: "Upstream web search failed: " + searchErrorCode,
		}
	}
	if out.Status == "incomplete" {
		out.IncompleteDetails = &ResponsesIncompleteDetails{Reason: "max_output_tokens"}
	}

	// Usage
	// Anthropic's input_tokens excludes cache_read/cache_creation, while OpenAI
	// Responses' input_tokens is the total including cached tokens. Add them back
	// when converting so downstream consumers see OpenAI semantics.
	totalInputTokens := resp.Usage.InputTokens +
		resp.Usage.CacheReadInputTokens +
		resp.Usage.CacheCreationInputTokens
	out.Usage = &ResponsesUsage{
		InputTokens:              totalInputTokens,
		OutputTokens:             resp.Usage.OutputTokens,
		TotalTokens:              totalInputTokens + resp.Usage.OutputTokens,
		CacheCreationInputTokens: resp.Usage.CacheCreationInputTokens,
	}
	if resp.Usage.CacheReadInputTokens > 0 {
		out.Usage.InputTokensDetails = &ResponsesInputTokensDetails{
			CachedTokens: resp.Usage.CacheReadInputTokens,
		}
	}

	return out
}

// anthropicStopReasonToResponsesStatus maps Anthropic stop_reason to Responses status.
func anthropicStopReasonToResponsesStatus(stopReason string, blocks []AnthropicContentBlock) string {
	switch stopReason {
	case "max_tokens":
		return "incomplete"
	case "end_turn", "tool_use", "stop_sequence":
		return "completed"
	default:
		return "completed"
	}
}

// ---------------------------------------------------------------------------
// Streaming: AnthropicStreamEvent → []ResponsesStreamEvent (stateful converter)
// ---------------------------------------------------------------------------

// AnthropicEventToResponsesState tracks state for converting a sequence of
// Anthropic SSE events into Responses SSE events.
type AnthropicEventToResponsesState struct {
	ResponseID     string
	Model          string
	Created        int64
	SequenceNumber int

	// CreatedSent tracks whether response.created has been emitted.
	CreatedSent bool
	// CompletedSent tracks whether the terminal event has been emitted.
	CompletedSent bool

	// Current output tracking
	OutputIndex               int
	CurrentItemID             string
	CurrentItemType           string // "message" | "function_call" | "reasoning"
	CurrentAnthropicBlockType string

	// For message output: accumulate text parts
	ContentIndex int

	// For function_call: track per-output info
	CurrentCallID           string
	CurrentName             string
	CurrentSearchInputJSON  string
	CurrentSearchStatus     string
	CurrentSearchErrorCode  string
	CurrentSearchResultSeen bool
	CurrentText             string
	CurrentAnnotationIndex  int
	PendingCitations        []AnthropicCitation
	SearchFailed            bool
	SearchErrorCode         string

	// Usage from message_start / message_delta. InputTokens here follows
	// Anthropic semantics (excludes cached tokens); they are added back when
	// emitting the OpenAI Responses usage.
	InputTokens              int
	OutputTokens             int
	CacheReadInputTokens     int
	CacheCreationInputTokens int

	StopReason string
}

// NewAnthropicEventToResponsesState returns an initialised stream state.
func NewAnthropicEventToResponsesState() *AnthropicEventToResponsesState {
	return &AnthropicEventToResponsesState{
		Created: time.Now().Unix(),
	}
}

// AnthropicEventToResponsesEvents converts a single Anthropic SSE event into
// zero or more Responses SSE events, updating state as it goes.
func AnthropicEventToResponsesEvents(
	evt *AnthropicStreamEvent,
	state *AnthropicEventToResponsesState,
) []ResponsesStreamEvent {
	switch evt.Type {
	case "message_start":
		return anthToResHandleMessageStart(evt, state)
	case "content_block_start":
		return anthToResHandleContentBlockStart(evt, state)
	case "content_block_delta":
		return anthToResHandleContentBlockDelta(evt, state)
	case "content_block_stop":
		return anthToResHandleContentBlockStop(evt, state)
	case "message_delta":
		return anthToResHandleMessageDelta(evt, state)
	case "message_stop":
		return anthToResHandleMessageStop(state)
	default:
		return nil
	}
}

// FinalizeAnthropicResponsesStream emits synthetic termination events if the
// stream ended without a proper message_stop.
func FinalizeAnthropicResponsesStream(state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	if !state.CreatedSent || state.CompletedSent {
		return nil
	}

	var events []ResponsesStreamEvent

	// Close any open item
	events = append(events, closeCurrentResponsesItem(state)...)

	status := "completed"
	if state.SearchFailed {
		status = "failed"
	}
	events = append(events, makeResponsesCompletedEvent(state, status, nil))
	state.CompletedSent = true
	return events
}

// ResponsesEventToSSE formats a ResponsesStreamEvent as an SSE data line.
func ResponsesEventToSSE(evt ResponsesStreamEvent) (string, error) {
	data, err := json.Marshal(evt)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("event: %s\ndata: %s\n\n", evt.Type, data), nil
}

// --- internal handlers ---

func anthToResHandleMessageStart(evt *AnthropicStreamEvent, state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	if evt.Message != nil {
		state.ResponseID = evt.Message.ID
		if state.Model == "" {
			state.Model = evt.Message.Model
		}
		if evt.Message.Usage.InputTokens > 0 {
			state.InputTokens = evt.Message.Usage.InputTokens
		}
		if evt.Message.Usage.CacheReadInputTokens > 0 {
			state.CacheReadInputTokens = evt.Message.Usage.CacheReadInputTokens
		}
		if evt.Message.Usage.CacheCreationInputTokens > 0 {
			state.CacheCreationInputTokens = evt.Message.Usage.CacheCreationInputTokens
		}
	}

	if state.CreatedSent {
		return nil
	}
	state.CreatedSent = true

	// Emit response.created
	return []ResponsesStreamEvent{makeResponsesCreatedEvent(state)}
}

func anthToResHandleContentBlockStart(evt *AnthropicStreamEvent, state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	if evt.ContentBlock == nil {
		return nil
	}

	var events []ResponsesStreamEvent
	state.CurrentAnthropicBlockType = evt.ContentBlock.Type

	switch evt.ContentBlock.Type {
	case "thinking":
		state.CurrentItemID = generateItemID()
		state.CurrentItemType = "reasoning"
		state.ContentIndex = 0

		events = append(events, makeResponsesEvent(state, "response.output_item.added", &ResponsesStreamEvent{
			OutputIndex: state.OutputIndex,
			Item: &ResponsesOutput{
				Type: "reasoning",
				ID:   state.CurrentItemID,
			},
		}))

	case "text":
		if state.CurrentItemType == "web_search_call" {
			events = append(events, closeCurrentResponsesItem(state)...)
		}
		// If we don't have an open message item, open one
		if state.CurrentItemType != "message" {
			state.CurrentItemID = generateItemID()
			state.CurrentItemType = "message"
			state.ContentIndex = 0

			events = append(events, makeResponsesEvent(state, "response.output_item.added", &ResponsesStreamEvent{
				OutputIndex: state.OutputIndex,
				Item: &ResponsesOutput{
					Type:   "message",
					ID:     state.CurrentItemID,
					Role:   "assistant",
					Status: "in_progress",
				},
			}))
		}
		state.CurrentText = ""
		state.CurrentAnnotationIndex = 0

	case "server_tool_use":
		if evt.ContentBlock.Name != "web_search" {
			return nil
		}
		events = append(events, closeCurrentResponsesItem(state)...)
		state.CurrentItemID = evt.ContentBlock.ID
		if state.CurrentItemID == "" {
			state.CurrentItemID = generateItemID()
		}
		state.CurrentItemType = "web_search_call"
		state.CurrentSearchInputJSON = normalizedJSONFragment(evt.ContentBlock.Input)
		state.CurrentSearchStatus = "in_progress"
		state.CurrentSearchErrorCode = ""
		state.CurrentSearchResultSeen = false
		events = append(events, makeResponsesEvent(state, "response.output_item.added", &ResponsesStreamEvent{
			OutputIndex: state.OutputIndex,
			Item: &ResponsesOutput{
				Type:   "web_search_call",
				ID:     state.CurrentItemID,
				Status: "in_progress",
				Action: &WebSearchAction{Type: "search", Query: anthropicWebSearchQuery(json.RawMessage(state.CurrentSearchInputJSON))},
			},
		}))

	case "web_search_tool_result":
		if state.CurrentItemType != "web_search_call" {
			return nil
		}
		state.CurrentSearchResultSeen = true
		state.CurrentSearchStatus = "completed"
		if code := anthropicWebSearchResultErrorCode(*evt.ContentBlock); code != "" {
			state.CurrentSearchStatus = "failed"
			state.CurrentSearchErrorCode = code
			state.SearchFailed = true
			state.SearchErrorCode = code
		}

	case "tool_use":
		// Close previous item if any
		events = append(events, closeCurrentResponsesItem(state)...)

		state.CurrentItemID = generateItemID()
		state.CurrentItemType = "function_call"
		state.CurrentCallID = toResponsesCallID(evt.ContentBlock.ID)
		state.CurrentName = evt.ContentBlock.Name

		events = append(events, makeResponsesEvent(state, "response.output_item.added", &ResponsesStreamEvent{
			OutputIndex: state.OutputIndex,
			Item: &ResponsesOutput{
				Type:   "function_call",
				ID:     state.CurrentItemID,
				CallID: state.CurrentCallID,
				Name:   state.CurrentName,
				Status: "in_progress",
			},
		}))
	}

	return events
}

func anthToResHandleContentBlockDelta(evt *AnthropicStreamEvent, state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	if evt.Delta == nil {
		return nil
	}

	switch evt.Delta.Type {
	case "text_delta":
		if evt.Delta.Text == "" {
			return nil
		}
		state.CurrentText += evt.Delta.Text
		events := []ResponsesStreamEvent{makeResponsesEvent(state, "response.output_text.delta", &ResponsesStreamEvent{
			OutputIndex:  state.OutputIndex,
			ContentIndex: state.ContentIndex,
			Delta:        evt.Delta.Text,
			ItemID:       state.CurrentItemID,
		})}
		events = append(events, flushPendingResponsesCitations(state, false)...)
		return events

	case "thinking_delta":
		if evt.Delta.Thinking == "" {
			return nil
		}
		return []ResponsesStreamEvent{makeResponsesEvent(state, "response.reasoning_summary_text.delta", &ResponsesStreamEvent{
			OutputIndex:  state.OutputIndex,
			SummaryIndex: 0,
			Delta:        evt.Delta.Thinking,
			ItemID:       state.CurrentItemID,
		})}

	case "input_json_delta":
		if evt.Delta.PartialJSON == "" {
			return nil
		}
		if state.CurrentItemType == "web_search_call" {
			state.CurrentSearchInputJSON += evt.Delta.PartialJSON
			return nil
		}
		return []ResponsesStreamEvent{makeResponsesEvent(state, "response.function_call_arguments.delta", &ResponsesStreamEvent{
			OutputIndex: state.OutputIndex,
			Delta:       evt.Delta.PartialJSON,
			ItemID:      state.CurrentItemID,
			CallID:      state.CurrentCallID,
			Name:        state.CurrentName,
		})}

	case "signature_delta":
		// Anthropic signature deltas have no Responses equivalent; skip
		return nil

	case "citations_delta":
		if evt.Delta.Citation == nil || strings.TrimSpace(evt.Delta.Citation.URL) == "" {
			return nil
		}
		// DeepSeek 可能在搜索结果块中先发送引用，再开始最终文本块；先缓存，
		// 等 message item 与引用文本都存在后再生成合法的 item_id 和字符索引。
		state.PendingCitations = append(state.PendingCitations, *evt.Delta.Citation)
		return flushPendingResponsesCitations(state, false)
	}

	return nil
}

func anthToResHandleContentBlockStop(evt *AnthropicStreamEvent, state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	blockType := state.CurrentAnthropicBlockType
	state.CurrentAnthropicBlockType = ""
	if blockType == "server_tool_use" {
		// 搜索结果块可能携带失败状态，等待结果块后再关闭 search item。
		return nil
	}
	if blockType == "web_search_tool_result" {
		return closeCurrentResponsesItem(state)
	}
	switch state.CurrentItemType {
	case "reasoning":
		// Emit reasoning summary done + output item done
		events := []ResponsesStreamEvent{
			makeResponsesEvent(state, "response.reasoning_summary_text.done", &ResponsesStreamEvent{
				OutputIndex:  state.OutputIndex,
				SummaryIndex: 0,
				ItemID:       state.CurrentItemID,
			}),
		}
		events = append(events, closeCurrentResponsesItem(state)...)
		return events

	case "function_call":
		// Emit function_call_arguments.done + output item done
		events := []ResponsesStreamEvent{
			makeResponsesEvent(state, "response.function_call_arguments.done", &ResponsesStreamEvent{
				OutputIndex: state.OutputIndex,
				ItemID:      state.CurrentItemID,
				CallID:      state.CurrentCallID,
				Name:        state.CurrentName,
			}),
		}
		events = append(events, closeCurrentResponsesItem(state)...)
		return events

	case "message":
		// 文本块结束时再兜底发送没有 cited_text 或无法精确匹配的引用。
		events := flushPendingResponsesCitations(state, true)
		events = append(events,
			makeResponsesEvent(state, "response.output_text.done", &ResponsesStreamEvent{
				OutputIndex:  state.OutputIndex,
				ContentIndex: state.ContentIndex,
				ItemID:       state.CurrentItemID,
			}),
		)
		return events
	}

	return nil
}

func anthToResHandleMessageDelta(evt *AnthropicStreamEvent, state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	if evt.Usage != nil {
		state.OutputTokens = evt.Usage.OutputTokens
		if evt.Usage.InputTokens > 0 {
			state.InputTokens = evt.Usage.InputTokens
		}
		if evt.Usage.CacheReadInputTokens > 0 {
			state.CacheReadInputTokens = evt.Usage.CacheReadInputTokens
		}
		if evt.Usage.CacheCreationInputTokens > 0 {
			state.CacheCreationInputTokens = evt.Usage.CacheCreationInputTokens
		}
	}
	if evt.Delta != nil && evt.Delta.StopReason != "" {
		state.StopReason = evt.Delta.StopReason
	}

	return nil
}

func anthToResHandleMessageStop(state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	if state.CompletedSent {
		return nil
	}

	var events []ResponsesStreamEvent
	events = append(events, closeCurrentResponsesItem(state)...)

	status := "completed"
	var incompleteDetails *ResponsesIncompleteDetails
	if state.SearchFailed {
		status = "failed"
	} else if state.StopReason == "max_tokens" {
		status = "incomplete"
		incompleteDetails = &ResponsesIncompleteDetails{Reason: "max_output_tokens"}
	}

	events = append(events, makeResponsesCompletedEvent(state, status, incompleteDetails))
	state.CompletedSent = true
	return events
}

// --- helper functions ---

func closeCurrentResponsesItem(state *AnthropicEventToResponsesState) []ResponsesStreamEvent {
	if state.CurrentItemType == "" {
		return nil
	}

	itemType := state.CurrentItemType
	itemID := state.CurrentItemID
	events := make([]ResponsesStreamEvent, 0, 2)
	if itemType == "message" {
		events = append(events, flushPendingResponsesCitations(state, true)...)
	}
	status := "completed"
	var action *WebSearchAction
	if itemType == "web_search_call" {
		if !state.CurrentSearchResultSeen {
			status = "failed"
			state.SearchFailed = true
			state.SearchErrorCode = "web_search_result_missing"
		} else if state.CurrentSearchStatus != "" {
			status = state.CurrentSearchStatus
		}
		action = &WebSearchAction{
			Type:  "search",
			Query: anthropicWebSearchQuery(json.RawMessage(state.CurrentSearchInputJSON)),
		}
	}

	// Reset
	state.CurrentItemType = ""
	state.CurrentItemID = ""
	state.CurrentCallID = ""
	state.CurrentName = ""
	state.CurrentSearchInputJSON = ""
	state.CurrentSearchStatus = ""
	state.CurrentSearchErrorCode = ""
	state.CurrentSearchResultSeen = false
	state.CurrentText = ""
	state.CurrentAnnotationIndex = 0
	if itemType == "message" {
		state.PendingCitations = nil
	}
	state.OutputIndex++
	state.ContentIndex = 0

	events = append(events, makeResponsesEvent(state, "response.output_item.done", &ResponsesStreamEvent{
		OutputIndex: state.OutputIndex - 1, // Use the index before increment
		Item: &ResponsesOutput{
			Type:   itemType,
			ID:     itemID,
			Status: status,
			Action: action,
		},
	}))
	return events
}

func normalizedJSONFragment(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" || trimmed == "{}" {
		return ""
	}
	return trimmed
}

func anthropicWebSearchQuery(raw json.RawMessage) string {
	var input struct {
		Query string `json:"query"`
	}
	if json.Unmarshal(raw, &input) != nil {
		return ""
	}
	return input.Query
}

func anthropicWebSearchResultErrorCode(block AnthropicContentBlock) string {
	if block.IsError {
		return "web_search_tool_error"
	}
	type resultError struct {
		Type      string `json:"type"`
		ErrorCode string `json:"error_code"`
	}
	var direct resultError
	if json.Unmarshal(block.Content, &direct) == nil && (direct.Type == "web_search_tool_result_error" || direct.ErrorCode != "") {
		if direct.ErrorCode != "" {
			return direct.ErrorCode
		}
		return "web_search_tool_error"
	}
	var list []resultError
	if json.Unmarshal(block.Content, &list) == nil {
		for _, item := range list {
			if item.Type == "web_search_tool_result_error" || item.ErrorCode != "" {
				if item.ErrorCode != "" {
					return item.ErrorCode
				}
				return "web_search_tool_error"
			}
		}
	}
	return ""
}

func anthropicCitationsToResponses(citations []AnthropicCitation, text string) []ResponsesAnnotation {
	annotations := make([]ResponsesAnnotation, 0, len(citations))
	for _, citation := range citations {
		if strings.TrimSpace(citation.URL) == "" {
			continue
		}
		annotations = append(annotations, anthropicCitationToResponses(citation, text))
	}
	return annotations
}

func anthropicCitationToResponses(citation AnthropicCitation, text string) ResponsesAnnotation {
	startIndex, endIndex, found := anthropicCitationTextRange(citation, text)
	if !found {
		startIndex = 0
		endIndex = utf8.RuneCountInString(text)
	}
	return ResponsesAnnotation{
		Type:       "url_citation",
		URL:        citation.URL,
		Title:      citation.Title,
		StartIndex: startIndex,
		EndIndex:   endIndex,
	}
}

func anthropicCitationTextRange(citation AnthropicCitation, text string) (int, int, bool) {
	if citation.CitedText == "" {
		return 0, 0, false
	}
	byteIndex := strings.Index(text, citation.CitedText)
	if byteIndex < 0 {
		return 0, 0, false
	}
	startIndex := utf8.RuneCountInString(text[:byteIndex])
	return startIndex, startIndex + utf8.RuneCountInString(citation.CitedText), true
}

func flushPendingResponsesCitations(state *AnthropicEventToResponsesState, fallback bool) []ResponsesStreamEvent {
	if state.CurrentItemType != "message" || state.CurrentItemID == "" || len(state.PendingCitations) == 0 {
		return nil
	}
	events := make([]ResponsesStreamEvent, 0, len(state.PendingCitations))
	pending := state.PendingCitations[:0]
	for _, citation := range state.PendingCitations {
		_, _, found := anthropicCitationTextRange(citation, state.CurrentText)
		if !found && !fallback {
			pending = append(pending, citation)
			continue
		}
		annotationIndex := state.CurrentAnnotationIndex
		state.CurrentAnnotationIndex++
		annotation := anthropicCitationToResponses(citation, state.CurrentText)
		events = append(events, makeResponsesEvent(state, "response.output_text.annotation.added", &ResponsesStreamEvent{
			OutputIndex:     state.OutputIndex,
			ContentIndex:    state.ContentIndex,
			ItemID:          state.CurrentItemID,
			Annotation:      &annotation,
			AnnotationIndex: &annotationIndex,
		}))
	}
	state.PendingCitations = pending
	return events
}

func makeResponsesCreatedEvent(state *AnthropicEventToResponsesState) ResponsesStreamEvent {
	seq := state.SequenceNumber
	state.SequenceNumber++
	return ResponsesStreamEvent{
		Type:           "response.created",
		SequenceNumber: seq,
		Response: &ResponsesResponse{
			ID:     state.ResponseID,
			Object: "response",
			Model:  state.Model,
			Status: "in_progress",
			Output: []ResponsesOutput{},
		},
	}
}

func makeResponsesCompletedEvent(
	state *AnthropicEventToResponsesState,
	status string,
	incompleteDetails *ResponsesIncompleteDetails,
) ResponsesStreamEvent {
	seq := state.SequenceNumber
	state.SequenceNumber++

	// Anthropic's input_tokens excludes cache_read/cache_creation; add them
	// back to match OpenAI Responses semantics where input_tokens is the total.
	totalInputTokens := state.InputTokens + state.CacheReadInputTokens + state.CacheCreationInputTokens
	usage := &ResponsesUsage{
		InputTokens:              totalInputTokens,
		OutputTokens:             state.OutputTokens,
		TotalTokens:              totalInputTokens + state.OutputTokens,
		CacheCreationInputTokens: state.CacheCreationInputTokens,
	}
	if state.CacheReadInputTokens > 0 {
		usage.InputTokensDetails = &ResponsesInputTokensDetails{
			CachedTokens: state.CacheReadInputTokens,
		}
	}

	eventType := "response.completed"
	if status == "incomplete" {
		eventType = "response.incomplete"
	} else if status == "failed" {
		eventType = "response.failed"
	}
	var responseError *ResponsesError
	if status == "failed" {
		code := state.SearchErrorCode
		if code == "" {
			code = "web_search_tool_error"
		}
		responseError = &ResponsesError{Code: code, Message: "Upstream web search failed: " + code}
	}

	return ResponsesStreamEvent{
		Type:           eventType,
		SequenceNumber: seq,
		Response: &ResponsesResponse{
			ID:                state.ResponseID,
			Object:            "response",
			Model:             state.Model,
			Status:            status,
			Output:            []ResponsesOutput{},
			Usage:             usage,
			IncompleteDetails: incompleteDetails,
			Error:             responseError,
		},
	}
}

func makeResponsesEvent(state *AnthropicEventToResponsesState, eventType string, template *ResponsesStreamEvent) ResponsesStreamEvent {
	seq := state.SequenceNumber
	state.SequenceNumber++

	evt := *template
	evt.Type = eventType
	evt.SequenceNumber = seq
	return evt
}

func generateResponsesID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "resp_" + hex.EncodeToString(b)
}

func generateItemID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "item_" + hex.EncodeToString(b)
}
