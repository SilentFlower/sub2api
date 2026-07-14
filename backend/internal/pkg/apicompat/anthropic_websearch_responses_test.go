//go:build unit

package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponsesToAnthropicRequestMapsWebSearch(t *testing.T) {
	maxUses := 4
	req := &ResponsesRequest{
		Model: "deepseek-v4-pro",
		Input: json.RawMessage(`"latest news"`),
		Tools: []ResponsesTool{{
			Type:    "web_search_preview",
			MaxUses: &maxUses,
			Filters: &ResponsesWebSearchFilters{AllowedDomains: []string{"example.com"}},
			UserLocation: &ResponsesWebSearchUserLocation{
				Type: "approximate", Country: "CN", Timezone: "Asia/Shanghai",
			},
		}},
		ToolChoice: json.RawMessage(`{"type":"web_search"}`),
	}

	out, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)
	require.Len(t, out.Tools, 1)
	require.Equal(t, "web_search_20250305", out.Tools[0].Type)
	require.Equal(t, "web_search", out.Tools[0].Name)
	require.Equal(t, 4, *out.Tools[0].MaxUses)
	require.Equal(t, []string{"example.com"}, out.Tools[0].AllowedDomains)
	require.Equal(t, "CN", out.Tools[0].UserLocation.Country)
	require.JSONEq(t, `{"type":"tool","name":"web_search"}`, string(out.ToolChoice))
	wire, err := json.Marshal(out.Tools[0])
	require.NoError(t, err)
	require.NotContains(t, string(wire), "input_schema")
}

func TestResponsesToAnthropicRequestMapsAdditionalWebSearchTools(t *testing.T) {
	req := &ResponsesRequest{
		Model: "deepseek-v4-pro",
		Input: json.RawMessage(`[
			{"role":"user","content":"latest news"},
			{"type":"additional_tools","tools":[{"type":"web_search"}]}
		]`),
		ToolChoice: json.RawMessage(`{"type":"web_search"}`),
	}

	out, err := ResponsesToAnthropicRequest(req)
	require.NoError(t, err)
	require.Len(t, out.Tools, 1)
	require.Equal(t, "web_search_20250305", out.Tools[0].Type)
	require.Len(t, out.Messages, 1)
	require.Equal(t, "user", out.Messages[0].Role)
}

func TestResponsesToAnthropicRequestRejectsUnsupportedWebSearchFields(t *testing.T) {
	external := true
	budget := 100
	tests := []ResponsesTool{
		{Type: "web_search", SearchContextSize: "high"},
		{Type: "web_search", ExternalWebAccess: &external},
		{Type: "web_search", ReturnTokenBudget: &budget},
		{Type: "web_search", UserLocation: &ResponsesWebSearchUserLocation{Type: "precise"}},
	}
	for _, tool := range tests {
		req := &ResponsesRequest{Model: "deepseek-v4-pro", Input: json.RawMessage(`"query"`), Tools: []ResponsesTool{tool}}
		_, err := ResponsesToAnthropicRequest(req)
		require.Error(t, err)
	}
}

func TestAnthropicToResponsesResponseMapsWebSearchAndCitations(t *testing.T) {
	resp := &AnthropicResponse{
		ID:         "msg_1",
		Model:      "deepseek-v4-pro",
		StopReason: "end_turn",
		Content: []AnthropicContentBlock{
			{Type: "server_tool_use", ID: "srvtoolu_1", Name: "web_search", Input: json.RawMessage(`{"query":"latest"}`)},
			{Type: "web_search_tool_result", ToolUseID: "srvtoolu_1", Content: json.RawMessage(`[{"type":"web_search_result","url":"https://example.com"}]`)},
			{Type: "text", Text: "See Answer now", Citations: []AnthropicCitation{{Type: "web_search_result_location", URL: "https://example.com", Title: "Example", CitedText: "Answer"}}},
		},
	}

	out := AnthropicToResponsesResponse(resp)
	require.Equal(t, "completed", out.Status)
	require.Len(t, out.Output, 2)
	require.Equal(t, "web_search_call", out.Output[0].Type)
	require.Equal(t, "completed", out.Output[0].Status)
	require.Equal(t, "latest", out.Output[0].Action.Query)
	require.Equal(t, "message", out.Output[1].Type)
	require.Len(t, out.Output[1].Content[0].Annotations, 1)
	require.Equal(t, 4, out.Output[1].Content[0].Annotations[0].StartIndex)
	require.Equal(t, 10, out.Output[1].Content[0].Annotations[0].EndIndex)
}

func TestAnthropicToResponsesResponseMarksWebSearchFailure(t *testing.T) {
	resp := &AnthropicResponse{
		ID:         "msg_1",
		Model:      "deepseek-v4-pro",
		StopReason: "end_turn",
		Content: []AnthropicContentBlock{
			{Type: "server_tool_use", ID: "srvtoolu_1", Name: "web_search", Input: json.RawMessage(`{"query":"latest"}`)},
			{Type: "web_search_tool_result", ToolUseID: "srvtoolu_1", Content: json.RawMessage(`{"type":"web_search_tool_result_error","error_code":"max_uses_exceeded"}`)},
		},
	}

	out := AnthropicToResponsesResponse(resp)
	require.Equal(t, "failed", out.Status)
	require.Equal(t, "failed", out.Output[0].Status)
	require.Equal(t, "max_uses_exceeded", out.Error.Code)
}

func TestAnthropicToResponsesResponseMarksMissingWebSearchResultAsFailure(t *testing.T) {
	resp := &AnthropicResponse{
		ID:         "msg_1",
		Model:      "deepseek-v4-pro",
		StopReason: "end_turn",
		Content: []AnthropicContentBlock{
			{Type: "server_tool_use", ID: "srvtoolu_1", Name: "web_search", Input: json.RawMessage(`{"query":"latest"}`)},
		},
	}

	out := AnthropicToResponsesResponse(resp)
	require.Equal(t, "failed", out.Status)
	require.Equal(t, "failed", out.Output[0].Status)
	require.Equal(t, "web_search_result_missing", out.Error.Code)
}

func TestAnthropicStreamMapsWebSearchAndCitation(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "deepseek-v4-pro"
	AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1", Model: "deepseek-v4-pro"}}, state)

	serverIndex := 0
	added := AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type: "content_block_start", Index: &serverIndex,
		ContentBlock: &AnthropicContentBlock{Type: "server_tool_use", ID: "srvtoolu_1", Name: "web_search", Input: json.RawMessage(`{}`)},
	}, state)
	require.Len(t, added, 1)
	require.Equal(t, "web_search_call", added[0].Item.Type)

	AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "input_json_delta", PartialJSON: `{"query":"latest"}`}}, state)
	require.Empty(t, AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_stop"}, state))

	resultIndex := 1
	AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type: "content_block_start", Index: &resultIndex,
		ContentBlock: &AnthropicContentBlock{Type: "web_search_tool_result", ToolUseID: "srvtoolu_1", Content: json.RawMessage(`[]`)},
	}, state)
	require.Empty(t, AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type: "content_block_delta",
		Delta: &AnthropicDelta{Type: "citations_delta", Citation: &AnthropicCitation{
			URL: "https://example.com", Title: "Example", CitedText: "Answer",
		}},
	}, state))
	done := AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_stop"}, state)
	require.Len(t, done, 1)
	require.Equal(t, "response.output_item.done", done[0].Type)
	require.Equal(t, "latest", done[0].Item.Action.Query)

	textIndex := 2
	messageAdded := AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_start", Index: &textIndex, ContentBlock: &AnthropicContentBlock{Type: "text"}}, state)
	require.Len(t, messageAdded, 1)
	require.NotEmpty(t, messageAdded[0].Item.ID)
	textEvents := AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "text_delta", Text: "See Answer now"}}, state)
	require.Len(t, textEvents, 2)
	require.Equal(t, "response.output_text.delta", textEvents[0].Type)
	require.Equal(t, "response.output_text.annotation.added", textEvents[1].Type)
	require.Equal(t, messageAdded[0].Item.ID, textEvents[1].ItemID)
	require.Equal(t, 4, textEvents[1].Annotation.StartIndex)
	require.Equal(t, 10, textEvents[1].Annotation.EndIndex)
}

func TestAnthropicStreamMarksWebSearchFailure(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "deepseek-v4-pro"
	AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1", Model: "deepseek-v4-pro"}}, state)

	serverIndex := 0
	AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type: "content_block_start", Index: &serverIndex,
		ContentBlock: &AnthropicContentBlock{Type: "server_tool_use", ID: "srvtoolu_1", Name: "web_search", Input: json.RawMessage(`{"query":"latest"}`)},
	}, state)
	AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_stop"}, state)

	resultIndex := 1
	AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type: "content_block_start", Index: &resultIndex,
		ContentBlock: &AnthropicContentBlock{Type: "web_search_tool_result", ToolUseID: "srvtoolu_1", Content: json.RawMessage(`{"type":"web_search_tool_result_error","error_code":"max_uses_exceeded"}`)},
	}, state)
	done := AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_stop"}, state)
	require.Equal(t, "failed", done[0].Item.Status)

	terminal := AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "message_stop"}, state)
	require.Len(t, terminal, 1)
	require.Equal(t, "response.failed", terminal[0].Type)
	require.Equal(t, "failed", terminal[0].Response.Status)
	require.Equal(t, "max_uses_exceeded", terminal[0].Response.Error.Code)
}

func TestFinalizeAnthropicStreamMarksMissingWebSearchResultAsFailure(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "deepseek-v4-pro"
	AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{ID: "msg_1", Model: state.Model}}, state)

	serverIndex := 0
	AnthropicEventToResponsesEvents(&AnthropicStreamEvent{
		Type: "content_block_start", Index: &serverIndex,
		ContentBlock: &AnthropicContentBlock{Type: "server_tool_use", ID: "srvtoolu_1", Name: "web_search", Input: json.RawMessage(`{"query":"latest"}`)},
	}, state)

	events := FinalizeAnthropicResponsesStream(state)
	require.Len(t, events, 2)
	require.Equal(t, "response.output_item.done", events[0].Type)
	require.Equal(t, "failed", events[0].Item.Status)
	require.Equal(t, "response.failed", events[1].Type)
	require.Equal(t, "web_search_result_missing", events[1].Response.Error.Code)
}
