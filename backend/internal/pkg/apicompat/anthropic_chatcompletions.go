package apicompat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// Anthropic Messages <-> OpenAI Chat Completions 直连桥接完全绕过
// Responses API 中转层：
//
//	request  : /v1/messages --AnthropicToChatCompletions--> /v1/chat/completions
//	response : chat SSE --ChatCompletionsChunkToAnthropicEvents--> Anthropic SSE
//
// 流式响应侧使用单个状态机，直接根据 chat delta 开关 Anthropic 内容块
// （reasoning_content -> thinking block，content -> text block，tool_calls ->
// tool_use block），避免经过 Responses item 生命周期时 reasoning 变成孤立
// delta。

// ===========================================================================
// 请求侧：Anthropic request -> Chat Completions request
// ===========================================================================

// AnthropicToChatCompletions 将 /v1/messages 请求体转换为
// /v1/chat/completions 请求，用于只支持 Chat Completions 的上游。
func AnthropicToChatCompletions(req *AnthropicRequest) (*ChatCompletionsRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}

	var messages []ChatMessage
	if sys := anthropicSystemParts(req.System); len(sys) > 0 {
		messages = append(messages, ChatMessage{Role: "system", Content: chatContentFromParts(sys)})
	}
	for _, m := range req.Messages {
		msgs, err := anthropicMessageToChat(m)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msgs...)
	}

	out := &ChatCompletionsRequest{
		Model:       req.Model,
		Messages:    messages,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
	}
	if req.MaxTokens > 0 {
		v := req.MaxTokens
		out.MaxCompletionTokens = &v
	}
	if len(req.StopSeqs) > 0 {
		if raw, err := json.Marshal(req.StopSeqs); err == nil {
			out.Stop = raw
		}
	}
	if len(req.Tools) > 0 {
		out.Tools = anthropicToolsToChat(req.Tools)
	}
	if len(req.ToolChoice) > 0 {
		if tc, err := anthropicToolChoiceToChat(req.ToolChoice); err == nil {
			out.ToolChoice = tc
		}
	}
	// 客户端显式关闭 thinking 时，把 {type:"disabled"} 透传给原生支持该形态的
	// 上游（GLM/DeepSeek/Qwen/...），同时去掉 reasoning_effort，避免把“关闭
	// thinking”和“指定推理强度”这两个互斥信号一起发给严格上游。
	// 其他场景沿用既有 reasoning_effort 映射（enabled / output_config.effort）。
	if req.Thinking != nil && req.Thinking.Type == "disabled" {
		out.Thinking = &ChatThinking{Type: "disabled"}
	} else if effort := anthropicReasoningEffort(req); effort != "" {
		out.ReasoningEffort = effort
	}
	return out, nil
}

const claudeCodeAttributionSystemPrefix = "x-anthropic-billing-header:"

// anthropicSystemParts 将 Anthropic system 字段转换成 Chat typed content parts。
func anthropicSystemParts(raw json.RawMessage) []ChatContentPart {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if part, ok := anthropicTextContentPart(s); ok {
			return []ChatContentPart{part}
		}
		return nil
	}
	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	var parts []ChatContentPart
	for _, b := range blocks {
		if b.Type != "text" {
			continue
		}
		if part, ok := anthropicTextContentPart(b.Text); ok {
			parts = append(parts, part)
		}
	}
	return parts
}

func anthropicTextContentPart(text string) (ChatContentPart, bool) {
	if strings.TrimSpace(text) == "" || isClaudeCodeAttributionSystemText(text) {
		return ChatContentPart{}, false
	}
	return ChatContentPart{Type: "text", Text: text}, true
}

func isClaudeCodeAttributionSystemText(text string) bool {
	text = strings.TrimLeftFunc(text, unicode.IsSpace)
	return strings.HasPrefix(text, claudeCodeAttributionSystemPrefix)
}

// anthropicMessageToChat 将一条 Anthropic message 转换为一条或多条 chat message。
// 带 tool_result block 的用户轮次会展开为独立 role:"tool" message，且必须紧跟
// assistant tool_calls message。
func anthropicMessageToChat(m AnthropicMessage) ([]ChatMessage, error) {
	// 普通字符串内容。
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		role := m.Role
		if role != "user" && role != "assistant" {
			role = "user"
		}
		part, ok := anthropicTextContentPart(s)
		if !ok {
			return nil, nil
		}
		return []ChatMessage{{Role: role, Content: chatContentFromParts([]ChatContentPart{part})}}, nil
	}

	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return nil, err
	}

	if m.Role == "assistant" {
		return anthropicAssistantBlocksToChat(blocks), nil
	}
	return anthropicUserBlocksToChat(blocks), nil
}

func anthropicUserBlocksToChat(blocks []AnthropicContentBlock) []ChatMessage {
	var out []ChatMessage
	var toolMsgs []ChatMessage
	var parts []ChatContentPart

	for _, b := range blocks {
		switch b.Type {
		case "text":
			if part, ok := anthropicTextContentPart(b.Text); ok {
				parts = append(parts, part)
			}
		case "image":
			if uri := anthropicImageDataURI(b.Source); uri != "" {
				parts = append(parts, ChatContentPart{Type: "image_url", ImageURL: &ChatImageURL{URL: uri}})
			}
		case "tool_result":
			toolMsgs = append(toolMsgs, ChatMessage{
				Role:       "tool",
				ToolCallID: b.ToolUseID,
				Content:    mustJSONString(anthropicToolResultText(b)),
			})
		}
	}

	// tool_result 回复必须排在前面：Chat Completions 要求 tool message 紧跟
	// assistant tool_calls message。同一 Anthropic 轮次中的新用户文本/图片属于
	// 新用户轮次，因此放在后面。
	out = append(out, toolMsgs...)
	if len(parts) > 0 {
		out = append(out, ChatMessage{Role: "user", Content: chatContentFromParts(parts)})
	}
	return out
}

func anthropicAssistantBlocksToChat(blocks []AnthropicContentBlock) []ChatMessage {
	msg := ChatMessage{Role: "assistant"}
	var parts []ChatContentPart
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if part, ok := anthropicTextContentPart(b.Text); ok {
				parts = append(parts, part)
			}
		case "image":
			if uri := anthropicImageDataURI(b.Source); uri != "" {
				parts = append(parts, ChatContentPart{Type: "image_url", ImageURL: &ChatImageURL{URL: uri}})
			}
		case "tool_use":
			args := "{}"
			if len(b.Input) > 0 {
				args = string(b.Input)
			}
			msg.ToolCalls = append(msg.ToolCalls, ChatToolCall{
				ID:       b.ID,
				Type:     "function",
				Function: ChatFunctionCall{Name: b.Name, Arguments: args},
			})
			// thinking block 不能作为 chat 输入，上游通常会拒绝，故丢弃。
		}
	}
	if len(parts) > 0 {
		msg.Content = chatContentFromParts(parts)
	} else if len(msg.ToolCalls) > 0 || msg.ReasoningContent != "" {
		msg.Content = mustJSONString("")
	} else {
		return nil
	}
	return []ChatMessage{msg}
}

func anthropicToolResultText(b AnthropicContentBlock) string {
	if len(b.Content) == 0 {
		return "(empty)"
	}
	var s string
	if err := json.Unmarshal(b.Content, &s); err == nil {
		if s == "" {
			return "(empty)"
		}
		return s
	}
	var inner []AnthropicContentBlock
	if err := json.Unmarshal(b.Content, &inner); err != nil {
		return "(empty)"
	}
	var parts []string
	for _, ib := range inner {
		if ib.Type == "text" && ib.Text != "" {
			parts = append(parts, ib.Text)
		}
	}
	if text := strings.Join(parts, "\n\n"); text != "" {
		return text
	}
	return "(empty)"
}

func anthropicImageDataURI(src *AnthropicImageSource) string {
	if src == nil || src.Data == "" {
		return ""
	}
	mediaType := src.MediaType
	if mediaType == "" {
		mediaType = "image/png"
	}
	return "data:" + mediaType + ";base64," + src.Data
}

// chatContentFromParts 始终保留 typed content part 数组，避免 string/array
// 形态随单 text、多 text 或多模态场景切换，影响上游 prefix cache 边界。
func chatContentFromParts(parts []ChatContentPart) json.RawMessage {
	raw, _ := json.Marshal(parts)
	return raw
}

func anthropicToolsToChat(tools []AnthropicTool) []ChatTool {
	var out []ChatTool
	for _, t := range tools {
		// 只保留自定义工具（type 为空）；有 type 的 Anthropic 内置工具无法被 chat 上游使用。
		if t.Type != "" {
			continue
		}
		strict := false
		out = append(out, ChatTool{
			Type: "function",
			Function: &ChatFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  normalizeToolParams(t.InputSchema),
				Strict:      &strict,
			},
		})
	}
	return out
}

func anthropicToolChoiceToChat(raw json.RawMessage) (json.RawMessage, error) {
	var tc struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &tc); err != nil {
		return nil, err
	}
	switch tc.Type {
	case "auto":
		return json.Marshal("auto")
	case "any":
		return json.Marshal("required")
	case "none":
		return json.Marshal("none")
	case "tool":
		return json.Marshal(map[string]any{
			"type":     "function",
			"function": map[string]any{"name": tc.Name},
		})
	default:
		return raw, nil
	}
}

// anthropicReasoningEffort 复用已验证的优先级：output_config.effort 优先；
// 否则 enabled thinking 默认映射为 "medium"；其他情况省略。
func anthropicReasoningEffort(req *AnthropicRequest) string {
	effort := ""
	if req.OutputConfig != nil && req.OutputConfig.Effort != "" {
		effort = req.OutputConfig.Effort
	} else if req.Thinking != nil && req.Thinking.Type == "enabled" {
		effort = "medium"
	}
	if effort == "max" {
		return "xhigh"
	}
	return effort
}

func normalizeToolParams(schema json.RawMessage) json.RawMessage {
	if len(schema) == 0 || string(schema) == "null" {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(schema, &m); err != nil {
		return schema
	}
	if string(m["type"]) != `"object"` {
		return schema
	}
	if _, ok := m["properties"]; ok {
		return schema
	}
	m["properties"] = json.RawMessage(`{}`)
	if out, err := json.Marshal(m); err == nil {
		return out
	}
	return schema
}

func mustJSONString(s string) json.RawMessage {
	raw, _ := json.Marshal(s)
	return raw
}

// ===========================================================================
// 响应侧：streaming Chat Completions -> Anthropic SSE
// ===========================================================================

// ChatCompletionsToAnthropicStreamState 保存跨 chunk 状态，用于从 Chat
// Completions SSE 流重建 Anthropic 内容块。
type ChatCompletionsToAnthropicStreamState struct {
	MessageID string
	Model     string

	started bool
	stopped bool

	blockOpen  bool
	blockType  string // "thinking" | "text" | "tool_use"
	blockIndex int
	nextIndex  int

	// 当前 tool_use block。block 按顺序串行打开，因此新的 tool_calls index 会先
	// 关闭前一个工具 block，再打开新的工具 block。
	curToolChatIdx  int // -1 when no tool block is open
	curToolName     string
	curToolArgs     strings.Builder
	curToolBuffered bool // "Read": buffer args, emit one sanitized delta at close
	curToolStreamed bool

	pendingToolCalls map[int]*pendingChatToolCall

	reasoning   strings.Builder // accumulated for the reasoning-only fallback
	textEmitted bool

	hasTool bool
	finish  string
	usage   *ChatUsage
}

type pendingChatToolCall struct {
	id   string
	name string
	args strings.Builder
}

// NewChatCompletionsToAnthropicStreamState 构造空的流式转换状态。
func NewChatCompletionsToAnthropicStreamState(model string) *ChatCompletionsToAnthropicStreamState {
	return &ChatCompletionsToAnthropicStreamState{Model: model, curToolChatIdx: -1}
}

// ChatCompletionsChunkToAnthropicEvents 将一个 Chat Completions 流式 chunk
// 转换为零个或多个 Anthropic SSE event，并更新状态。
func ChatCompletionsChunkToAnthropicEvents(chunk *ChatCompletionsChunk, s *ChatCompletionsToAnthropicStreamState) []AnthropicStreamEvent {
	if chunk == nil || s == nil {
		return nil
	}
	if chunk.ID != "" {
		s.MessageID = chunk.ID
	}
	if s.Model == "" && chunk.Model != "" {
		s.Model = chunk.Model
	}
	if chunk.Usage != nil {
		s.usage = chunk.Usage
	}

	var events []AnthropicStreamEvent
	events = append(events, s.ensureStart()...)

	for _, choice := range chunk.Choices {
		d := choice.Delta

		if d.ReasoningContent != nil && *d.ReasoningContent != "" {
			events = append(events, s.openBlock("thinking")...)
			_, _ = s.reasoning.WriteString(*d.ReasoningContent)
			events = append(events, contentBlockDelta(s.blockIndex, AnthropicDelta{
				Type: "thinking_delta", Thinking: *d.ReasoningContent,
			}))
		}

		if d.Content != nil && *d.Content != "" {
			events = append(events, s.openBlock("text")...)
			s.textEmitted = true
			events = append(events, contentBlockDelta(s.blockIndex, AnthropicDelta{
				Type: "text_delta", Text: *d.Content,
			}))
		}

		for _, tc := range d.ToolCalls {
			idx := 0
			if tc.Index != nil {
				idx = *tc.Index
			}
			events = append(events, s.handleToolCallDelta(idx, tc)...)
		}

		if choice.FinishReason != nil && *choice.FinishReason != "" {
			s.finish = *choice.FinishReason
		}
	}
	return events
}

// FinalizeChatCompletionsAnthropicStream 关闭尚未结束的 block，并发出终止
// message_delta + message_stop event。该函数只应调用一次。
func FinalizeChatCompletionsAnthropicStream(s *ChatCompletionsToAnthropicStreamState) []AnthropicStreamEvent {
	if s == nil || !s.started || s.stopped {
		return nil
	}
	var events []AnthropicStreamEvent
	events = append(events, s.closeBlock()...)
	events = append(events, s.flushPendingToolCalls()...)

	// 只有 reasoning、没有 text/tool 的完成结果会把 reasoning 作为回答回显，
	// 避免客户端收到没有正文的 thinking block。该逻辑对齐
	// synthesizeChatReasoningFallbackMessage。
	if !s.textEmitted && !s.hasTool {
		if r := s.reasoning.String(); strings.TrimSpace(r) != "" {
			events = append(events, s.openBlock("text")...)
			s.textEmitted = true
			events = append(events, contentBlockDelta(s.blockIndex, AnthropicDelta{
				Type: "text_delta", Text: r,
			}))
			events = append(events, s.closeBlock()...)
		}
	}

	usage := anthropicUsageFromChatUsage(s.usage)
	events = append(events,
		AnthropicStreamEvent{
			Type:  "message_delta",
			Delta: &AnthropicDelta{StopReason: chatFinishToAnthropicStopReason(s.finish, s.hasTool)},
			Usage: &usage,
		},
		AnthropicStreamEvent{Type: "message_stop"},
	)
	s.stopped = true
	return events
}

func (s *ChatCompletionsToAnthropicStreamState) handleToolCallDelta(idx int, tc ChatToolCall) []AnthropicStreamEvent {
	if s.blockOpen && s.blockType == "tool_use" && s.curToolChatIdx == idx {
		if tc.Function.Name != "" && s.curToolName == "" {
			s.curToolName = tc.Function.Name
			s.curToolBuffered = tc.Function.Name == "Read"
		}
		if tc.Function.Arguments == "" {
			return nil
		}
		_, _ = s.curToolArgs.WriteString(tc.Function.Arguments)
		if s.curToolBuffered {
			return nil
		}
		s.curToolStreamed = true
		return []AnthropicStreamEvent{contentBlockDelta(s.blockIndex, AnthropicDelta{
			Type: "input_json_delta", PartialJSON: tc.Function.Arguments,
		})}
	}

	pending := s.pendingToolCalls[idx]
	toolID := tc.ID
	toolName := tc.Function.Name
	prefixArgs := ""
	if pending != nil {
		if toolID == "" {
			toolID = pending.id
		}
		if toolName == "" {
			toolName = pending.name
		}
		prefixArgs = pending.args.String()
	}

	args := prefixArgs + tc.Function.Arguments
	if toolID == "" || toolName == "" {
		s.storePendingToolCall(idx, tc)
		return nil
	}
	delete(s.pendingToolCalls, idx)

	var events []AnthropicStreamEvent
	if !s.blockOpen || s.blockType != "tool_use" || s.curToolChatIdx != idx {
		events = append(events, s.openToolBlock(idx, toolID, toolName)...)
	} else if toolName != "" && s.curToolName == "" {
		s.curToolName = toolName
		s.curToolBuffered = toolName == "Read"
	}
	if args != "" {
		_, _ = s.curToolArgs.WriteString(args)
		if !s.curToolBuffered {
			s.curToolStreamed = true
			events = append(events, contentBlockDelta(s.blockIndex, AnthropicDelta{
				Type: "input_json_delta", PartialJSON: args,
			}))
		}
	}
	return events
}

func (s *ChatCompletionsToAnthropicStreamState) storePendingToolCall(idx int, tc ChatToolCall) {
	if s.pendingToolCalls == nil {
		s.pendingToolCalls = make(map[int]*pendingChatToolCall)
	}
	pending := s.pendingToolCalls[idx]
	if pending == nil {
		pending = &pendingChatToolCall{}
		s.pendingToolCalls[idx] = pending
	}
	if tc.ID != "" && pending.id == "" {
		pending.id = tc.ID
	}
	if tc.Function.Name != "" && pending.name == "" {
		pending.name = tc.Function.Name
	}
	if tc.Function.Arguments != "" {
		_, _ = pending.args.WriteString(tc.Function.Arguments)
	}
}

func (s *ChatCompletionsToAnthropicStreamState) flushPendingToolCalls() []AnthropicStreamEvent {
	if len(s.pendingToolCalls) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(s.pendingToolCalls))
	for idx := range s.pendingToolCalls {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)

	var events []AnthropicStreamEvent
	for _, idx := range indexes {
		pending := s.pendingToolCalls[idx]
		if pending == nil || pending.name == "" {
			continue
		}
		args := pending.args.String()
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		toolID := pending.id
		if toolID == "" {
			toolID = deterministicToolUseID(idx, pending.name, args)
		}
		events = append(events, s.openToolBlock(idx, toolID, pending.name)...)
		_, _ = s.curToolArgs.WriteString(args)
		events = append(events, s.closeBlock()...)
	}
	s.pendingToolCalls = nil
	return events
}

func (s *ChatCompletionsToAnthropicStreamState) ensureStart() []AnthropicStreamEvent {
	if s.started {
		return nil
	}
	s.started = true
	return []AnthropicStreamEvent{{
		Type: "message_start",
		Message: &AnthropicResponse{
			ID:      s.MessageID,
			Type:    "message",
			Role:    "assistant",
			Content: []AnthropicContentBlock{},
			Model:   s.Model,
			Usage:   AnthropicUsage{},
		},
	}}
}

// openBlock 确保指定类型的 thinking/text block 已打开；如有其他 block 打开则先关闭。
// 如果同类型 block 已经打开则不做任何处理。
func (s *ChatCompletionsToAnthropicStreamState) openBlock(blockType string) []AnthropicStreamEvent {
	if s.blockOpen && s.blockType == blockType {
		return nil
	}
	var events []AnthropicStreamEvent
	events = append(events, s.closeBlock()...)

	idx := s.nextIndex
	s.nextIndex++
	s.blockOpen = true
	s.blockType = blockType
	s.blockIndex = idx

	bi := idx
	events = append(events, AnthropicStreamEvent{
		Type:         "content_block_start",
		Index:        &bi,
		ContentBlock: &AnthropicContentBlock{Type: blockType},
	})
	return events
}

// openToolBlock 关闭当前 block，并为新的 chat-completions tool_calls index 打开
// tool_use block。
func (s *ChatCompletionsToAnthropicStreamState) openToolBlock(chatIdx int, id, name string) []AnthropicStreamEvent {
	var events []AnthropicStreamEvent
	events = append(events, s.closeBlock()...)

	idx := s.nextIndex
	s.nextIndex++
	s.blockOpen = true
	s.blockType = "tool_use"
	s.blockIndex = idx
	s.hasTool = true
	s.curToolChatIdx = chatIdx
	s.curToolName = name
	s.curToolBuffered = name == "Read"
	s.curToolStreamed = false
	s.curToolArgs.Reset()

	bi := idx
	events = append(events, AnthropicStreamEvent{
		Type:  "content_block_start",
		Index: &bi,
		ContentBlock: &AnthropicContentBlock{
			Type:  "tool_use",
			ID:    id,
			Name:  name,
			Input: json.RawMessage("{}"),
		},
	})
	return events
}

func (s *ChatCompletionsToAnthropicStreamState) closeBlock() []AnthropicStreamEvent {
	if !s.blockOpen {
		return nil
	}
	var events []AnthropicStreamEvent
	idx := s.blockIndex

	// 对从未流式输出参数的工具（缓冲的 "Read" 工具，或没有参数片段的调用），在
	// block 关闭前发出单个清洗后的 input_json_delta。该逻辑对齐
	// resToAnthHandleFuncArgsDone / closeChatToolItems。
	if s.blockType == "tool_use" && !s.curToolStreamed {
		args := s.curToolArgs.String()
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		events = append(events, contentBlockDelta(idx, AnthropicDelta{
			Type: "input_json_delta", PartialJSON: string(sanitizeAnthropicToolUseInput(s.curToolName, args)),
		}))
	}

	s.blockOpen = false
	s.blockType = ""
	s.curToolChatIdx = -1
	s.curToolName = ""
	s.curToolBuffered = false
	s.curToolStreamed = false
	s.curToolArgs.Reset()

	events = append(events, AnthropicStreamEvent{Type: "content_block_stop", Index: &idx})
	return events
}

func contentBlockDelta(index int, delta AnthropicDelta) AnthropicStreamEvent {
	i := index
	return AnthropicStreamEvent{Type: "content_block_delta", Index: &i, Delta: &delta}
}

func chatFinishToAnthropicStopReason(finish string, hasTool bool) string {
	switch finish {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "content_filter":
		return "end_turn"
	default:
		if hasTool {
			return "tool_use"
		}
		return "end_turn"
	}
}

// anthropicUsageFromChatUsage 将 Chat Completions usage 映射为 Anthropic usage。
// Anthropic 的 input_tokens 不包含 cached tokens（单独记录在
// cache_read_input_tokens），因此这里会扣除 cached 数量。该逻辑对齐
// anthropicUsageFromResponsesUsage。
func anthropicUsageFromChatUsage(u *ChatUsage) AnthropicUsage {
	if u == nil {
		return AnthropicUsage{}
	}
	cached := 0
	if u.PromptTokensDetails != nil {
		cached = u.PromptTokensDetails.CachedTokens
	}
	input := u.PromptTokens - cached
	if input < 0 {
		input = 0
	}
	return AnthropicUsage{
		InputTokens:          input,
		OutputTokens:         u.CompletionTokens,
		CacheReadInputTokens: cached,
	}
}

// ChatCompletionsStreamToAnthropicResponse 是非流式（同步）路径：它缓冲完整
// chat 流并组装成单个 Anthropic Messages JSON 响应，用于客户端请求 stream=false
// 的场景（上游始终流式，网关再折叠）。block 顺序对齐 ResponsesToAnthropic：
// thinking、text、tool_use。
func ChatCompletionsStreamToAnthropicResponse(chunks []*ChatCompletionsChunk, model string) *AnthropicResponse {
	id := ""
	var reasoning, text strings.Builder
	type toolAgg struct {
		id, name string
		args     strings.Builder
	}
	tools := map[int]*toolAgg{}
	maxToolIdx := -1
	finish := ""
	var usage *ChatUsage

	for _, chunk := range chunks {
		if chunk == nil {
			continue
		}
		if chunk.ID != "" {
			id = chunk.ID
		}
		if model == "" && chunk.Model != "" {
			model = chunk.Model
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		for _, choice := range chunk.Choices {
			d := choice.Delta
			if d.ReasoningContent != nil {
				_, _ = reasoning.WriteString(*d.ReasoningContent)
			}
			if d.Content != nil {
				_, _ = text.WriteString(*d.Content)
			}
			for _, tc := range d.ToolCalls {
				idx := 0
				if tc.Index != nil {
					idx = *tc.Index
				}
				agg, ok := tools[idx]
				if !ok {
					agg = &toolAgg{}
					tools[idx] = agg
					if idx > maxToolIdx {
						maxToolIdx = idx
					}
				}
				if tc.ID != "" {
					agg.id = tc.ID
				}
				if tc.Function.Name != "" {
					agg.name = tc.Function.Name
				}
				_, _ = agg.args.WriteString(tc.Function.Arguments)
			}
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				finish = *choice.FinishReason
			}
		}
	}

	hasTool := maxToolIdx >= 0
	var blocks []AnthropicContentBlock
	if reasoning.Len() > 0 {
		blocks = append(blocks, AnthropicContentBlock{Type: "thinking", Thinking: reasoning.String()})
	}
	finalText := text.String()
	if finalText == "" && !hasTool && strings.TrimSpace(reasoning.String()) != "" {
		finalText = reasoning.String() // reasoning-only fallback
	}
	if finalText != "" {
		blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: finalText})
	}
	for i := 0; i <= maxToolIdx; i++ {
		agg, ok := tools[i]
		if !ok {
			continue
		}
		args := agg.args.String()
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		toolID := agg.id
		if toolID == "" {
			toolID = deterministicToolUseID(i, agg.name, args)
		}
		blocks = append(blocks, AnthropicContentBlock{
			Type:  "tool_use",
			ID:    toolID,
			Name:  agg.name,
			Input: sanitizeAnthropicToolUseInput(agg.name, args),
		})
	}
	if len(blocks) == 0 {
		blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: ""})
	}

	return &AnthropicResponse{
		ID:         id,
		Type:       "message",
		Role:       "assistant",
		Model:      model,
		Content:    blocks,
		StopReason: chatFinishToAnthropicStopReason(finish, hasTool),
		Usage:      anthropicUsageFromChatUsage(usage),
	}
}

func deterministicToolUseID(index int, name, args string) string {
	if strings.TrimSpace(args) == "" {
		args = "{}"
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\n%s\n%s", index, name, args)))
	return "toolu_" + hex.EncodeToString(sum[:12])
}
