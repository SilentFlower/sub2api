//go:build unit

package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequestPermissionsFunctionToolPreserved(t *testing.T) {
	strict := true
	parameters := json.RawMessage(`{"type":"object","properties":{"reason":{"type":"string"},"environment_id":{"type":"string"},"permissions":{"type":"object","properties":{"network":{"type":"object","properties":{"enabled":{"type":"boolean"}},"required":["enabled"]}},"required":["network"]}},"required":["permissions"]}`)
	req := &ResponsesRequest{
		Model: "deepseek-v4-pro",
		Input: json.RawMessage(`"search the web"`),
		Tools: []ResponsesTool{{
			Type:        "function",
			Name:        requestPermissionsToolName,
			Description: "Request additional permissions",
			Parameters:  parameters,
			Strict:      &strict,
		}},
		ToolChoice: json.RawMessage(`{"type":"function","name":"request_permissions"}`),
	}

	chatReq, err := ResponsesToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, chatReq.Tools, 1)
	require.Equal(t, requestPermissionsToolName, chatReq.Tools[0].Function.Name)
	require.Equal(t, parameters, chatReq.Tools[0].Function.Parameters)
	require.Equal(t, &strict, chatReq.Tools[0].Function.Strict)
	require.JSONEq(t, `{"type":"function","function":{"name":"request_permissions"}}`, string(chatReq.ToolChoice))
	require.True(t, HasFunctionTool(req.Tools, requestPermissionsToolName))
	require.False(t, HasFunctionTool([]ResponsesTool{{Type: "custom", Name: requestPermissionsToolName}}, requestPermissionsToolName))
}

func TestChatCompletionsResponseToResponsesRecoversRequestPermissions(t *testing.T) {
	content, err := json.Marshal("需要先获取网络权限。\n<request_permissions><requests><permission>network</permission></requests></request_permissions>\n")
	require.NoError(t, err)
	resp := &ChatCompletionsResponse{
		ID: "chatcmpl_permission",
		Choices: []ChatChoice{{
			Message:      ChatMessage{Role: "assistant", Content: content},
			FinishReason: "stop",
		}},
	}

	out := ChatCompletionsResponseToResponses(resp, "deepseek-v4-pro", nil, false, nil, true)
	require.Len(t, out.Output, 2)
	require.Equal(t, "message", out.Output[0].Type)
	require.Equal(t, "需要先获取网络权限。\n", out.Output[0].Content[0].Text)
	require.Equal(t, "function_call", out.Output[1].Type)
	require.Equal(t, requestPermissionsToolName, out.Output[1].Name)
	require.JSONEq(t, requestPermissionsNetworkArguments, out.Output[1].Arguments)
}

func TestChatCompletionsResponseToResponsesDoesNotGuessRequestPermissions(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		declared bool
	}{
		{"tool not declared", `<request_permissions><requests><permission>network</permission></requests></request_permissions>`, false},
		{"unknown permission", `<request_permissions><requests><permission>filesystem</permission></requests></request_permissions>`, true},
		{"duplicate permission", `<request_permissions><requests><permission>network</permission><permission>network</permission></requests></request_permissions>`, true},
		{"trailing text", `<request_permissions><requests><permission>network</permission></requests></request_permissions>done`, true},
		{"incomplete marker", `<request_permissions><requests><permission>network</permission></requests>`, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			content, err := json.Marshal(tc.content)
			require.NoError(t, err)
			resp := &ChatCompletionsResponse{Choices: []ChatChoice{{Message: ChatMessage{Role: "assistant", Content: content}}}}
			out := ChatCompletionsResponseToResponses(resp, "deepseek-v4-pro", nil, false, nil, tc.declared)
			require.Len(t, out.Output, 1)
			require.Equal(t, "message", out.Output[0].Type)
			require.Equal(t, tc.content, out.Output[0].Content[0].Text)
		})
	}
}

func TestChatCompletionsResponseToResponsesRealToolCallTakesPriority(t *testing.T) {
	marker := `<request_permissions><requests><permission>network</permission></requests></request_permissions>`
	content, err := json.Marshal(marker)
	require.NoError(t, err)
	resp := &ChatCompletionsResponse{Choices: []ChatChoice{{
		Message: ChatMessage{
			Role:    "assistant",
			Content: content,
			ToolCalls: []ChatToolCall{{
				ID:   "call_real",
				Type: "function",
				Function: ChatFunctionCall{
					Name:      "lookup",
					Arguments: `{"query":"typhoon"}`,
				},
			}},
		},
		FinishReason: "tool_calls",
	}}}

	out := ChatCompletionsResponseToResponses(resp, "deepseek-v4-pro", nil, false, nil, true)
	require.Len(t, out.Output, 2)
	require.Equal(t, marker, out.Output[0].Content[0].Text)
	require.Equal(t, "lookup", out.Output[1].Name)
	require.NotEqual(t, requestPermissionsToolName, out.Output[1].Name)
}

func TestParseRequestPermissionsMarkerStrictness(t *testing.T) {
	valid := `<request_permissions><requests><permission>network</permission></requests></request_permissions>   `
	arguments, ok := parseRequestPermissionsMarker(valid)
	require.True(t, ok)
	require.JSONEq(t, requestPermissionsNetworkArguments, arguments)

	invalid := []string{
		`<request_permissions reason="x"><requests><permission>network</permission></requests></request_permissions>`,
		`<request_permissions xmlns="urn:test"><requests><permission>network</permission></requests></request_permissions>`,
		`<request_permissions><permission>network</permission></request_permissions>`,
		`<request_permissions><requests><permission><name>network</name></permission></requests></request_permissions>`,
		`<!-- comment --><request_permissions><requests><permission>network</permission></requests></request_permissions>`,
	}
	for _, marker := range invalid {
		_, ok := parseRequestPermissionsMarker(marker)
		require.False(t, ok, marker)
	}
}

func TestChatCompletionsStreamRecoversRequestPermissionsAcrossChunks(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("deepseek-v4-pro")
	state.RequestPermissionsDeclared = true

	chunks := []string{
		"需要先获取网络权限。\n<request_per",
		"missions><requests><permission>net",
		"work</permission></requests></request_permissions>\n",
	}
	var events []ResponsesStreamEvent
	for _, content := range chunks {
		delta := content
		events = append(events, ChatCompletionsChunkToResponsesEvents(&ChatCompletionsChunk{
			Choices: []ChatChunkChoice{{Delta: ChatDelta{Content: &delta}}},
		}, state)...)
	}
	events = append(events, FinalizeChatCompletionsResponsesStream(state)...)

	var text strings.Builder
	var permissionDone *ResponsesOutput
	for _, event := range events {
		if event.Type == "response.output_text.delta" {
			_, _ = text.WriteString(event.Delta)
		}
		if event.Type == "response.output_item.done" && event.Item != nil && event.Item.Name == requestPermissionsToolName {
			permissionDone = event.Item
		}
	}
	require.Equal(t, "需要先获取网络权限。\n", text.String())
	require.NotNil(t, permissionDone)
	require.Equal(t, "function_call", permissionDone.Type)
	require.JSONEq(t, requestPermissionsNetworkArguments, permissionDone.Arguments)
}

func TestChatCompletionsStreamFlushesInvalidPermissionMarkerAsText(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("deepseek-v4-pro")
	state.RequestPermissionsDeclared = true
	content := `<request_permissions><requests><permission>filesystem</permission></requests></request_permissions>`
	events := ChatCompletionsChunkToResponsesEvents(&ChatCompletionsChunk{
		Choices: []ChatChunkChoice{{Delta: ChatDelta{Content: &content}}},
	}, state)
	require.NotEmpty(t, events)
	events = append(events, FinalizeChatCompletionsResponsesStream(state)...)

	var text strings.Builder
	for _, event := range events {
		if event.Type == "response.output_text.delta" {
			_, _ = text.WriteString(event.Delta)
		}
	}
	require.Equal(t, content, text.String())
	require.Empty(t, state.ToolCalls)
}

func TestChatCompletionsStreamRealToolCallTakesPriority(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("deepseek-v4-pro")
	state.RequestPermissionsDeclared = true
	marker := `<request_permissions><requests><permission>network</permission></requests></request_permissions>`
	events := ChatCompletionsChunkToResponsesEvents(&ChatCompletionsChunk{
		Choices: []ChatChunkChoice{{Delta: ChatDelta{Content: &marker}}},
	}, state)
	index := 0
	events = append(events, ChatCompletionsChunkToResponsesEvents(&ChatCompletionsChunk{
		Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{{
			Index: &index,
			ID:    "call_real",
			Function: ChatFunctionCall{
				Name:      "lookup",
				Arguments: `{"query":"typhoon"}`,
			},
		}}}}},
	}, state)...)
	events = append(events, FinalizeChatCompletionsResponsesStream(state)...)

	var text strings.Builder
	var toolNames []string
	for _, event := range events {
		if event.Type == "response.output_text.delta" {
			_, _ = text.WriteString(event.Delta)
		}
		if event.Type == "response.output_item.done" && event.Item != nil && event.Item.Type == "function_call" {
			toolNames = append(toolNames, event.Item.Name)
		}
	}
	require.Equal(t, marker, text.String())
	require.Equal(t, []string{"lookup"}, toolNames)
}

func TestChatCompletionsStreamNormalTextIsImmediate(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("deepseek-v4-pro")
	state.RequestPermissionsDeclared = true
	content := "ordinary response"
	events := ChatCompletionsChunkToResponsesEvents(&ChatCompletionsChunk{
		Choices: []ChatChunkChoice{{Delta: ChatDelta{Content: &content}}},
	}, state)

	var deltas []string
	for _, event := range events {
		if event.Type == "response.output_text.delta" {
			deltas = append(deltas, event.Delta)
		}
	}
	require.Equal(t, []string{content}, deltas)
}

func TestChatCompletionsStreamFlushesPartialPermissionPrefixAtFinalize(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("deepseek-v4-pro")
	state.RequestPermissionsDeclared = true
	content := "ordinary response <request_per"
	events := ChatCompletionsChunkToResponsesEvents(&ChatCompletionsChunk{
		Choices: []ChatChunkChoice{{Delta: ChatDelta{Content: &content}}},
	}, state)
	events = append(events, FinalizeChatCompletionsResponsesStream(state)...)

	var text strings.Builder
	for _, event := range events {
		if event.Type == "response.output_text.delta" {
			_, _ = text.WriteString(event.Delta)
		}
	}
	require.Equal(t, content, text.String())
	require.Empty(t, state.ToolCalls)
}
