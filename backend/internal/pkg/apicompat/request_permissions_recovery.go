package apicompat

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"strings"
)

const (
	requestPermissionsToolName         = "request_permissions"
	requestPermissionsMarkerPrefix     = "<request_permissions"
	requestPermissionsCandidateMaxSize = 64 * 1024
	requestPermissionsNetworkArguments = `{"permissions":{"network":{"enabled":true}}}`
)

func recoverRequestPermissionsMessage(message ChatMessage) (ChatMessage, bool) {
	if len(message.ToolCalls) > 0 {
		return message, false
	}
	var content string
	if err := json.Unmarshal(message.Content, &content); err != nil {
		return message, false
	}
	prefix, arguments, ok := splitRequestPermissionsMarker(content)
	if !ok {
		return message, false
	}
	if strings.TrimSpace(prefix) == "" {
		message.Content = nil
	} else {
		message.Content, _ = json.Marshal(prefix)
	}
	message.ToolCalls = []ChatToolCall{{
		ID:   generateItemID(),
		Type: "function",
		Function: ChatFunctionCall{
			Name:      requestPermissionsToolName,
			Arguments: arguments,
		},
	}}
	return message, true
}

func splitRequestPermissionsMarker(content string) (string, string, bool) {
	markerIndex := strings.Index(content, requestPermissionsMarkerPrefix)
	if markerIndex < 0 {
		return "", "", false
	}
	arguments, ok := parseRequestPermissionsMarker(content[markerIndex:])
	if !ok {
		return "", "", false
	}
	return content[:markerIndex], arguments, true
}

func parseRequestPermissionsMarker(marker string) (string, bool) {
	decoder := xml.NewDecoder(strings.NewReader(marker))
	stack := make([]string, 0, 3)
	rootSeen := false
	rootClosed := false
	requestsSeen := false
	permissionCount := 0
	var permissionText strings.Builder

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", false
		}
		switch typed := token.(type) {
		case xml.StartElement:
			if rootClosed || typed.Name.Space != "" || len(typed.Attr) != 0 {
				return "", false
			}
			name := typed.Name.Local
			switch len(stack) {
			case 0:
				if rootSeen || name != requestPermissionsToolName {
					return "", false
				}
				rootSeen = true
			case 1:
				if requestsSeen || name != "requests" {
					return "", false
				}
				requestsSeen = true
			case 2:
				if permissionCount != 0 || name != "permission" {
					return "", false
				}
			default:
				return "", false
			}
			stack = append(stack, name)
		case xml.EndElement:
			if typed.Name.Space != "" || len(stack) == 0 || stack[len(stack)-1] != typed.Name.Local {
				return "", false
			}
			if typed.Name.Local == "permission" {
				permissionCount++
				if strings.TrimSpace(permissionText.String()) != "network" {
					return "", false
				}
			}
			stack = stack[:len(stack)-1]
			if typed.Name.Local == requestPermissionsToolName {
				rootClosed = true
			}
		case xml.CharData:
			if len(stack) == 3 && stack[2] == "permission" {
				_, _ = permissionText.Write(typed)
			} else if strings.TrimSpace(string(typed)) != "" {
				return "", false
			}
		case xml.Comment, xml.Directive, xml.ProcInst:
			return "", false
		}
	}

	if !rootSeen || !rootClosed || !requestsSeen || permissionCount != 1 || len(stack) != 0 {
		return "", false
	}
	return requestPermissionsNetworkArguments, true
}

func routeChatContentDelta(state *ChatCompletionsToResponsesStreamState, content string) []ResponsesStreamEvent {
	if !state.RequestPermissionsDeclared || state.requestPermissionsRecoveryDisabled || len(state.ToolCalls) > 0 {
		return emitChatTextDelta(state, content)
	}

	if state.requestPermissionsCandidateActive {
		_, _ = state.requestPermissionsCandidate.WriteString(content)
		if state.requestPermissionsCandidate.Len() <= requestPermissionsCandidateMaxSize {
			return nil
		}
		buffered := state.requestPermissionsCandidate.String()
		state.requestPermissionsCandidate.Reset()
		state.requestPermissionsCandidateActive = false
		state.requestPermissionsRecoveryDisabled = true
		return emitChatTextDelta(state, buffered)
	}

	combined := state.requestPermissionsPending.String() + content
	state.requestPermissionsPending.Reset()
	if markerIndex := strings.Index(combined, requestPermissionsMarkerPrefix); markerIndex >= 0 {
		state.requestPermissionsCandidateActive = true
		_, _ = state.requestPermissionsCandidate.WriteString(combined[markerIndex:])
		return emitChatTextDelta(state, combined[:markerIndex])
	}

	keep := longestRequestPermissionsMarkerPrefixSuffix(combined)
	emitUntil := len(combined) - keep
	if keep > 0 {
		_, _ = state.requestPermissionsPending.WriteString(combined[emitUntil:])
	}
	return emitChatTextDelta(state, combined[:emitUntil])
}

func longestRequestPermissionsMarkerPrefixSuffix(content string) int {
	maxKeep := len(requestPermissionsMarkerPrefix) - 1
	if len(content) < maxKeep {
		maxKeep = len(content)
	}
	for keep := maxKeep; keep > 0; keep-- {
		if strings.HasSuffix(content, requestPermissionsMarkerPrefix[:keep]) {
			return keep
		}
	}
	return 0
}

func emitChatTextDelta(state *ChatCompletionsToResponsesStreamState, content string) []ResponsesStreamEvent {
	if content == "" {
		return nil
	}
	var events []ResponsesStreamEvent
	events = append(events, closeChatReasoningItem(state)...)
	events = append(events, ensureChatToResponsesMessageItem(state)...)
	events = append(events, ensureChatToResponsesTextPart(state)...)
	_, _ = state.Text.WriteString(content)
	events = append(events, chatToResponsesEvent(state, "response.output_text.delta", &ResponsesStreamEvent{
		OutputIndex:  state.MessageIndex,
		ContentIndex: 0,
		Delta:        content,
		ItemID:       state.MessageItemID,
	}))
	return events
}

func disableRequestPermissionsRecovery(state *ChatCompletionsToResponsesStreamState) []ResponsesStreamEvent {
	if state == nil || !state.RequestPermissionsDeclared || state.requestPermissionsRecoveryDisabled {
		return nil
	}
	buffered := state.requestPermissionsPending.String() + state.requestPermissionsCandidate.String()
	state.requestPermissionsPending.Reset()
	state.requestPermissionsCandidate.Reset()
	state.requestPermissionsCandidateActive = false
	state.requestPermissionsRecoveryDisabled = true
	return emitChatTextDelta(state, buffered)
}

func finalizeRequestPermissionsRecovery(state *ChatCompletionsToResponsesStreamState) []ResponsesStreamEvent {
	if state == nil || !state.RequestPermissionsDeclared || state.requestPermissionsRecoveryDisabled {
		return nil
	}
	if len(state.ToolCalls) > 0 {
		return disableRequestPermissionsRecovery(state)
	}
	if !state.requestPermissionsCandidateActive {
		pending := state.requestPermissionsPending.String()
		state.requestPermissionsPending.Reset()
		return emitChatTextDelta(state, pending)
	}

	candidate := state.requestPermissionsCandidate.String()
	state.requestPermissionsCandidate.Reset()
	state.requestPermissionsCandidateActive = false
	arguments, ok := parseRequestPermissionsMarker(candidate)
	if !ok {
		return emitChatTextDelta(state, candidate)
	}

	var events []ResponsesStreamEvent
	events = append(events, closeChatReasoningItem(state)...)
	index := 0
	for {
		if _, exists := state.ToolCalls[index]; !exists {
			break
		}
		index++
	}
	toolCall := &ChatToolCall{
		ID:   generateItemID(),
		Type: "function",
		Function: ChatFunctionCall{
			Name:      requestPermissionsToolName,
			Arguments: arguments,
		},
	}
	state.ToolCalls[index] = toolCall
	state.ToolItemIDs[index] = generateItemID()
	state.ToolOutputIndex[index] = state.allocOutputIndex()
	state.FinishReason = "tool_calls"
	events = append(events, announceChatToolItem(state, index, toolCall, false)...)
	return events
}
