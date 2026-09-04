package apicompat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

// 本文件实现 Anthropic Messages 与 OpenAI Chat Completions 的直连桥接，
// 跳过 Responses API 中间结构。
//
// 既有 chat fallback 会在请求侧执行 Anthropic→Responses→ChatCompletions，
// 在响应侧执行 ChatCompletions→Responses→Anthropic，导致每个流式 token 经过
// 两套状态机。对于只实现 /v1/chat/completions 的第三方兼容上游，Responses
// 层不会承载额外语义，因此直连桥可避免重复转换。
//
// 直连桥把两个方向分别压缩为一次转换：
//
//	请求：Anthropic Messages → Chat Completions
//	响应：Chat Completions chunk/response → Anthropic event/response
//
// 复用 Responses 桥中的 helper（anthropicImageToDataURI、
// extractAnthropicTextFromBlocks, fromResponsesCallID, sanitizeAnthropicToolUseInput,
// parseAnthropicSystemContentParts, isReasoningModel, mapAnthropicEffortToResponses,
// normalizeToolParameters），确保两条转换路径的语义一致。

// ---------------------------------------------------------------------------
// 请求转换：AnthropicRequest → ChatCompletionsRequest
// ---------------------------------------------------------------------------

// AnthropicToChatCompletionsRequest 将 Anthropic Messages 请求直接转换为
// Chat Completions 请求，不经过 Responses 中间结构。
//
// @param req Anthropic Messages 请求。
// @return 转换后的 Chat Completions 请求；请求无效时返回错误。
func AnthropicToChatCompletionsRequest(req *AnthropicRequest) (*ChatCompletionsRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("anthropic request is nil")
	}

	messages, err := anthropicToChatMessages(req.System, req.Messages)
	if err != nil {
		return nil, err
	}

	out := &ChatCompletionsRequest{
		Model:    req.Model,
		Messages: messages,
		Stream:   req.Stream,
	}

	// 推理模型（如 gpt-5.x）会拒绝 temperature/top_p，因此只向普通模型透传。
	if !isReasoningModel(req.Model) {
		out.Temperature = req.Temperature
		out.TopP = req.TopP
	}

	if req.MaxTokens > 0 {
		v := req.MaxTokens
		if v < minMaxOutputTokens {
			v = minMaxOutputTokens
		}
		out.MaxCompletionTokens = &v
	}
	if len(req.StopSeqs) > 0 {
		stop, err := json.Marshal(req.StopSeqs)
		if err != nil {
			return nil, fmt.Errorf("marshal stop sequences: %w", err)
		}
		out.Stop = stop
	}

	// Anthropic input_schema 可直接作为 Chat function parameters；
	// web_search_* 等服务端工具没有 Chat Completions 等价形态，因此丢弃。
	if len(req.Tools) > 0 {
		tools := anthropicToolsToChatTools(req.Tools)
		if len(tools) > 0 {
			out.Tools = tools
		}
	}

	// 只有工具转换后仍存在时才透传 tool_choice；具名选择还必须指向已声明工具，
	// 避免严格上游因 tool_choice 引用未知工具而返回 400。
	if len(out.Tools) > 0 && len(req.ToolChoice) > 0 {
		declared := make(map[string]bool, len(out.Tools))
		for _, tool := range out.Tools {
			if tool.Function != nil {
				declared[tool.Function.Name] = true
			}
		}
		tc, err := convertAnthropicToolChoiceToChat(req.ToolChoice, declared)
		if err != nil {
			return nil, fmt.Errorf("convert tool_choice: %w", err)
		}
		if len(tc) > 0 {
			out.ToolChoice = tc
		}
	}

	// 显式关闭 thinking 时不能同时发送 reasoning_effort，否则严格上游会拒绝
	// 这组互斥参数；其他场景保留 main 的默认 medium 语义。
	if req.Thinking != nil && req.Thinking.Type == "disabled" {
		out.Thinking = &ChatThinking{Type: "disabled"}
	} else {
		effort := "medium"
		if req.OutputConfig != nil && req.OutputConfig.Effort != "" {
			effort = req.OutputConfig.Effort
		}
		out.ReasoningEffort = mapAnthropicEffortToResponses(effort)
	}

	parallelToolCalls := true
	out.ParallelToolCalls = &parallelToolCalls

	return out, nil
}

// anthropicToChatMessages 把 Anthropic system 和消息列表直接转换为 Chat 消息。
func anthropicToChatMessages(system json.RawMessage, msgs []AnthropicMessage) ([]ChatMessage, error) {
	var messages []ChatMessage

	// system 同时支持字符串和 block 数组，并过滤动态 billing attribution。
	if len(system) > 0 {
		sysParts, err := parseAnthropicSystemContentParts(system)
		if err != nil {
			return nil, err
		}
		if len(sysParts) > 0 {
			parts := make([]ChatContentPart, 0, len(sysParts))
			for _, part := range sysParts {
				if part.Type == "input_text" && part.Text != "" {
					parts = append(parts, ChatContentPart{Type: "text", Text: part.Text})
				}
			}
			if len(parts) > 0 {
				content, err := json.Marshal(parts)
				if err != nil {
					return nil, err
				}
				messages = append(messages, ChatMessage{Role: "system", Content: content})
			}
		}
	}

	for _, m := range msgs {
		converted, err := anthropicMsgToChatMessages(m)
		if err != nil {
			return nil, err
		}
		messages = append(messages, converted...)
	}

	return stabilizeChatToolResultOrder(messages), nil
}

const claudeCodeAttributionSystemPrefix = "x-anthropic-billing-header:"

func isClaudeCodeAttributionSystemText(text string) bool {
	text = strings.TrimLeftFunc(text, unicode.IsSpace)
	return strings.HasPrefix(text, claudeCodeAttributionSystemPrefix)
}

func stabilizeChatToolResultOrder(messages []ChatMessage) []ChatMessage {
	for i := 0; i < len(messages); i++ {
		message := messages[i]
		if message.Role != "assistant" || len(message.ToolCalls) < 2 {
			continue
		}
		toolOrder := make(map[string]int, len(message.ToolCalls))
		for index, toolCall := range message.ToolCalls {
			if toolCall.ID != "" {
				toolOrder[toolCall.ID] = index
			}
		}
		if len(toolOrder) == 0 {
			continue
		}

		start := i + 1
		end := start
		for end < len(messages) && messages[end].Role == "tool" {
			end++
		}
		if end-start < 2 {
			continue
		}

		// 并行工具结果按声明顺序恢复，避免完成时序改变历史请求的 prefix cache。
		sort.SliceStable(messages[start:end], func(a, b int) bool {
			aIndex, aKnown := toolOrder[messages[start+a].ToolCallID]
			bIndex, bKnown := toolOrder[messages[start+b].ToolCallID]
			switch {
			case aKnown && bKnown:
				return aIndex < bIndex
			case aKnown:
				return true
			case bKnown:
				return false
			default:
				return false
			}
		})
		i = end - 1
	}
	return messages
}

// anthropicMsgToChatMessages 把一条 Anthropic 消息转换为一条或多条 Chat 消息。
// tool_result 变为独立 tool 消息，text/image 保留在 user 消息中，assistant
// tool_use 则变为同一 assistant 消息上的 tool_calls。
func anthropicMsgToChatMessages(m AnthropicMessage) ([]ChatMessage, error) {
	switch m.Role {
	case "assistant":
		return anthropicAssistantToChatMessages(m.Content)
	default: // user 和未知角色都按用户消息处理。
		return anthropicUserToChatMessages(m.Content)
	}
}

// anthropicUserToChatMessages 转换 Anthropic user 消息。内容可以是字符串或 block
// 数组；tool_result 会拆成独立 tool 消息，其中的图片因 function_call_output
// 只能承载字符串，需要提升到后续 user 消息的 image_url part。
func anthropicUserToChatMessages(raw json.RawMessage) ([]ChatMessage, error) {
	// 文本也固定使用 typed content part，避免历史重放时 string/array 形态切换。
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		content, _ := json.Marshal([]ChatContentPart{{Type: "text", Text: s}})
		return []ChatMessage{{Role: "user", Content: content}}, nil
	}

	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, err
	}

	var out []ChatMessage
	var toolResultImageParts []ChatContentPart

	// tool_result 转为 tool 消息；文本直接提取，图片延后放入 user 消息。
	for _, b := range blocks {
		if b.Type != "tool_result" {
			continue
		}
		text, imageParts := convertToolResultOutput(b)
		content, _ := json.Marshal(text)
		out = append(out, ChatMessage{
			Role:       "tool",
			Content:    content,
			ToolCallID: b.ToolUseID,
		})
		for _, ip := range imageParts {
			toolResultImageParts = append(toolResultImageParts, ChatContentPart{
				Type:     "image_url",
				ImageURL: &ChatImageURL{URL: ip.ImageURL},
			})
		}
	}

	// 剩余文本和图片保留为 typed content part，稳定 Chat prefix cache 边界。
	var parts []ChatContentPart
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" && !isClaudeCodeAttributionSystemText(b.Text) {
				parts = append(parts, ChatContentPart{Type: "text", Text: b.Text})
			}
		case "image":
			if uri := anthropicImageToDataURI(b.Source); uri != "" {
				parts = append(parts, ChatContentPart{
					Type:     "image_url",
					ImageURL: &ChatImageURL{URL: uri},
				})
			}
		}
	}
	if len(toolResultImageParts) > 0 {
		parts = append(parts, toolResultImageParts...)
	}

	if len(parts) == 0 {
		return out, nil
	}

	content, err := json.Marshal(parts)
	if err != nil {
		return nil, err
	}
	out = append(out, ChatMessage{Role: "user", Content: content})

	return out, nil
}

// anthropicAssistantToChatMessages 转换 Anthropic assistant 消息，保持 typed content。
// 仅携带工具调用的消息回传 thinking 为 reasoning_content，供上游重放工具推理历史。
func anthropicAssistantToChatMessages(raw json.RawMessage) ([]ChatMessage, error) {
	// 助手文本同样固定使用 typed content part。
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		content, _ := json.Marshal([]ChatContentPart{{Type: "text", Text: s}})
		return []ChatMessage{{Role: "assistant", Content: content}}, nil
	}

	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, err
	}

	msg := ChatMessage{Role: "assistant"}
	var textParts []ChatContentPart
	for _, block := range blocks {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" && !isClaudeCodeAttributionSystemText(block.Text) {
			textParts = append(textParts, ChatContentPart{Type: "text", Text: block.Text})
		}
	}
	if len(textParts) > 0 {
		content, _ := json.Marshal(textParts)
		msg.Content = content
	}

	for _, b := range blocks {
		if b.Type != "tool_use" {
			continue
		}
		args := "{}"
		if len(b.Input) > 0 {
			args = string(b.Input)
		}
		msg.ToolCalls = append(msg.ToolCalls, ChatToolCall{
			ID:   b.ID,
			Type: "function",
			Function: ChatFunctionCall{
				Name:      b.Name,
				Arguments: args,
			},
		})
	}

	msg.ReasoningContent = anthropicThinkingToReasoningContent(blocks, len(msg.ToolCalls) > 0)

	return []ChatMessage{msg}, nil
}

// anthropicThinkingToReasoningContent 仅在工具调用轮次回传明文 thinking。
// DeepSeek 要求回传产生工具调用的推理内容；普通文本轮次和仅含签名的块不回传。
func anthropicThinkingToReasoningContent(blocks []AnthropicContentBlock, hasToolCalls bool) string {
	if !hasToolCalls {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "thinking" && b.Thinking != "" {
			parts = append(parts, b.Thinking)
		}
	}
	return strings.Join(parts, "\n")
}

// anthropicToolsToChatTools 把 Anthropic 工具定义转换为 Chat function 工具；
// web_search_* 等服务端工具没有等价形态，因此丢弃。
func anthropicToolsToChatTools(tools []AnthropicTool) []ChatTool {
	var out []ChatTool
	for _, t := range tools {
		if t.Type != "" {
			continue
		}
		out = append(out, ChatTool{
			Type: "function",
			Function: &ChatFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  normalizeToolParameters(t.InputSchema),
				Strict:      boolPtr(false),
			},
		})
	}
	return out
}

// convertAnthropicToolChoiceToChat 转换 Anthropic tool_choice。返回 nil 表示丢弃：
// 具名选择若指向未声明工具，或选择类型未知，都不能透传给严格 Chat 上游。
//
//	{"type":"auto"}            → "auto"
//	{"type":"any"}             → "required"
//	{"type":"none"}            → "none"
//	{"type":"tool","name":"X"} → {"type":"function","function":{"name":"X"}}（X 已声明）
func convertAnthropicToolChoiceToChat(raw json.RawMessage, declared map[string]bool) (json.RawMessage, error) {
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
		if tc.Name == "" || !declared[tc.Name] {
			return nil, nil
		}
		return json.Marshal(map[string]any{
			"type":     "function",
			"function": map[string]string{"name": tc.Name},
		})
	default:
		return nil, nil
	}
}

// ---------------------------------------------------------------------------
// 非流式响应：ChatCompletionsResponse → AnthropicResponse
// ---------------------------------------------------------------------------

// ChatCompletionsResponseToAnthropic 把 Chat Completions 响应直接转换为
// Anthropic Messages 响应，不创建 ResponsesResponse 中间对象。
//
// @param resp Chat Completions 响应。
// @param model 对客户端展示的模型名。
// @return 转换后的 Anthropic Messages 响应。
func ChatCompletionsResponseToAnthropic(resp *ChatCompletionsResponse, model string) *AnthropicResponse {
	out := &AnthropicResponse{
		Type:  "message",
		Role:  "assistant",
		Model: model,
	}

	if resp != nil {
		out.ID = resp.ID
		if out.Model == "" {
			out.Model = resp.Model
		}

		if len(resp.Choices) > 0 {
			choice := resp.Choices[0]
			out.Content = chatMessageToAnthropicBlocks(choice.Message)
			out.StopReason = AnthropicStopReasonPtr(chatFinishReasonToAnthropicStopReason(choice.FinishReason, out.Content))
			// Anthropic 只通过 stop_reason 表达 token 上限，不存在 incomplete_details。
		}
		if resp.Usage != nil {
			out.Usage = chatUsageToAnthropicUsage(resp.Usage)
		}
	}

	if len(out.Content) == 0 {
		out.Content = []AnthropicContentBlock{{Type: "text", Text: ""}}
	}
	// 空 choices 或 nil 响应也必须以 end_turn 完成，严格 Anthropic 客户端不接受
	// 空 stop_reason。
	if AnthropicStopReasonString(out.StopReason) == "" {
		out.StopReason = AnthropicStopReasonPtr(chatFinishReasonToAnthropicStopReason("", out.Content))
	}
	// 上游省略响应 ID 时生成兼容 ID，因为客户端把该字段视为必填。
	if out.ID == "" {
		out.ID = generateResponsesID()
	}

	return out
}

// chatMessageToAnthropicBlocks 把 Chat 消息转换为 Anthropic content block：
// reasoning 变为 thinking，正文变为 text，tool_calls 变为 tool_use。
func chatMessageToAnthropicBlocks(message ChatMessage) []AnthropicContentBlock {
	var blocks []AnthropicContentBlock
	reasoning := message.reasoningText()

	if reasoning != "" {
		blocks = append(blocks, AnthropicContentBlock{
			Type:     "thinking",
			Thinking: reasoning,
		})
	}

	text := chatMessageContentText(message.Content)
	// DeepSeek reasoning-only fallback: when there is no text and no tool calls,
	// surface the reasoning content as visible text so the turn isn't empty.
	if text == "" && strings.TrimSpace(reasoning) != "" && len(message.ToolCalls) == 0 {
		text = reasoning
	}
	if text != "" || len(message.ToolCalls) == 0 {
		blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: text})
	}

	for _, toolCall := range message.ToolCalls {
		if toolCall.Function.Name == "" {
			continue
		}
		arguments := toolCall.Function.Arguments
		if strings.TrimSpace(arguments) == "" {
			arguments = "{}"
		}
		blocks = append(blocks, AnthropicContentBlock{
			Type:  "tool_use",
			ID:    fromResponsesCallID(toolCall.ID),
			Name:  toolCall.Function.Name,
			Input: sanitizeAnthropicToolUseInput(toolCall.Function.Name, arguments),
		})
	}

	return blocks
}

// chatFinishReasonToAnthropicStopReason 把 Chat finish_reason 映射为 Anthropic
// stop_reason。
//
//	"length"     → "max_tokens"
//	"tool_calls" → "tool_use"
//	其它          → "end_turn"（存在 tool_use block 时为 "tool_use"）
//
// stop、content_filter 和未知原因都视为正常结束，再根据已有 block 判断是否为
// tool_use。
func chatFinishReasonToAnthropicStopReason(reason string, blocks []AnthropicContentBlock) string {
	switch reason {
	case "length":
		return "max_tokens"
	case "tool_calls":
		if containsAnthropicToolUseBlock(blocks) {
			return "tool_use"
		}
		return "end_turn"
	default:
		if containsAnthropicToolUseBlock(blocks) {
			return "tool_use"
		}
		return "end_turn"
	}
}

// chatUsageToAnthropicUsage 把 Chat token usage 转换为 Anthropic usage。
func chatUsageToAnthropicUsage(usage *ChatUsage) AnthropicUsage {
	if usage == nil {
		return AnthropicUsage{}
	}

	cachedTokens := 0
	cacheCreationTokens := 0
	if usage.PromptTokensDetails != nil {
		cachedTokens = usage.PromptTokensDetails.CachedTokens
		// cache_write_tokens 与 cache_creation_tokens 是同一数量的两种字段名，
		// 不能相加；优先使用 write，缺失时再使用 creation。
		if usage.PromptTokensDetails.CacheWriteTokens > 0 {
			cacheCreationTokens = usage.PromptTokensDetails.CacheWriteTokens
		} else {
			cacheCreationTokens = usage.PromptTokensDetails.CacheCreationTokens
		}
	}

	inputTokens := usage.PromptTokens - cachedTokens - cacheCreationTokens
	if inputTokens < 0 {
		inputTokens = 0
	}

	return AnthropicUsage{
		InputTokens:              inputTokens,
		OutputTokens:             usage.CompletionTokens,
		CacheReadInputTokens:     cachedTokens,
		CacheCreationInputTokens: cacheCreationTokens,
	}
}

// ---------------------------------------------------------------------------
// 流式响应：ChatCompletionsChunk → []AnthropicStreamEvent（有状态转换）
// ---------------------------------------------------------------------------

// ChatCompletionsToAnthropicStreamState 保存 Chat Completions SSE 到 Anthropic
// SSE 直连转换的跨 chunk 状态。
type ChatCompletionsToAnthropicStreamState struct {
	MessageStartSent bool
	MessageStopSent  bool

	// 当前 content block 生命周期。
	ContentBlockIndex   int
	ContentBlockOpen    bool
	CurrentBlockType    string // "text" | "thinking" | "tool_use"
	CurrentToolName     string
	CurrentToolIndex    int
	CurrentToolHadDelta bool
	HasToolCall         bool
	TextEmitted         bool
	ReasoningContent    strings.Builder

	// 工具调用按上游 tool_call index 聚合。function name 到达前缓存 ID 和参数，
	// name 到达后才分配 Anthropic block index；直到 finalize 仍缺 name 的调用
	// 无法组成合法 tool_use，因此跳过。
	toolBlockIndex    map[int]int
	toolAnnounced     map[int]bool
	toolName          map[int]string
	toolArgs          map[int]string
	pendingToolCallID map[int]string
	pendingToolArgs   map[int]string

	// DeepSeek 风格 reasoning_content 会先于正文流式到达。block 串行输出，
	// 因此 thinking 与其它 block 共用 ContentBlockIndex 计数器。

	FinishReason string

	InputTokens              int
	OutputTokens             int
	CacheReadInputTokens     int
	CacheCreationInputTokens int

	ResponseID string
	Model      string
	Created    int64
}

// NewChatCompletionsToAnthropicStreamState 创建初始化后的流式转换状态。
//
// @param model 对客户端展示的模型名。
// @return 初始化后的流式转换状态。
func NewChatCompletionsToAnthropicStreamState(model string) *ChatCompletionsToAnthropicStreamState {
	return &ChatCompletionsToAnthropicStreamState{
		ResponseID:        generateResponsesID(),
		Model:             model,
		Created:           time.Now().Unix(),
		CurrentToolIndex:  -1,
		toolBlockIndex:    make(map[int]int),
		toolAnnounced:     make(map[int]bool),
		toolName:          make(map[int]string),
		toolArgs:          make(map[int]string),
		pendingToolCallID: make(map[int]string),
		pendingToolArgs:   make(map[int]string),
	}
}

// ChatCompletionsChunkToAnthropicEvents 把一个 Chat Completions 流式 chunk
// 转换为零个或多个 Anthropic 流式事件，并同步更新转换状态。
//
// @param chunk 上游 Chat Completions 流式 chunk。
// @param state 当前流式转换状态。
// @return 本次 chunk 产生的 Anthropic 流式事件。
func ChatCompletionsChunkToAnthropicEvents(
	chunk *ChatCompletionsChunk,
	state *ChatCompletionsToAnthropicStreamState,
) []AnthropicStreamEvent {
	if chunk == nil || state == nil {
		return nil
	}
	if chunk.ID != "" {
		state.ResponseID = chunk.ID
	}
	if state.Model == "" && chunk.Model != "" {
		state.Model = chunk.Model
	}

	// include_usage 通常通过 choices 为空的独立 chunk 到达，需保存到 finalize
	// 阶段的 message_delta。
	if chunk.Usage != nil {
		u := chatUsageToAnthropicUsage(chunk.Usage)
		state.InputTokens = u.InputTokens
		state.OutputTokens = u.OutputTokens
		state.CacheReadInputTokens = u.CacheReadInputTokens
		state.CacheCreationInputTokens = u.CacheCreationInputTokens
	}

	var events []AnthropicStreamEvent
	events = append(events, ensureCCAnthropicMessageStart(state)...)

	for _, choice := range chunk.Choices {
		// Reasoning content → thinking block.
		reasoning := choice.Delta.reasoningText()
		if reasoning != nil && *reasoning != "" {
			_, _ = state.ReasoningContent.WriteString(*reasoning)
			events = append(events, ensureCCAnthropicThinkingBlock(state)...)
			events = append(events, ccAnthropicDelta(state, &AnthropicDelta{
				Type:     "thinking_delta",
				Thinking: *reasoning,
			})...)
		}

		// 正文转为 text block，并先关闭正在输出的 thinking block。
		if choice.Delta.Content != nil && *choice.Delta.Content != "" {
			state.TextEmitted = true
			events = append(events, closeCCAnthropicBlockIfOpen(state, "thinking")...)
			events = append(events, ensureCCAnthropicTextBlock(state)...)
			events = append(events, ccAnthropicDelta(state, &AnthropicDelta{
				Type: "text_delta",
				Text: *choice.Delta.Content,
			})...)
		}

		// 工具调用转为 tool_use block。
		for _, toolCall := range choice.Delta.ToolCalls {
			events = append(events, closeCCAnthropicBlockIfOpen(state, "thinking")...)
			events = append(events, handleCCAnthropicToolCall(state, &toolCall)...)
		}

		if choice.FinishReason != nil && *choice.FinishReason != "" {
			state.FinishReason = *choice.FinishReason
		}
	}

	return events
}

// FinalizeChatCompletionsAnthropicStream 在上游结束时关闭未完成的 block，并输出
// message_delta 和 message_stop 终止事件。
//
// @param state 当前流式转换状态。
// @return 流结束时需要补发的 Anthropic 事件。
func FinalizeChatCompletionsAnthropicStream(state *ChatCompletionsToAnthropicStreamState) []AnthropicStreamEvent {
	if state == nil || state.MessageStopSent {
		return nil
	}

	var events []AnthropicStreamEvent
	if !state.MessageStartSent {
		events = append(events, ensureCCAnthropicMessageStart(state)...)
	}

	// 缺少上游 ID 的合法工具在结束时使用稳定 ID；始终缺少 function name 的
	// pending 调用无法组成合法 Anthropic tool_use，因此跳过。
	if len(state.toolAnnounced) > 0 {
		idxs := make([]int, 0, len(state.toolAnnounced))
		for idx, announced := range state.toolAnnounced {
			if !announced && state.toolName[idx] != "" {
				idxs = append(idxs, idx)
			}
		}
		sort.Ints(idxs)
		for _, idx := range idxs {
			callID := state.pendingToolCallID[idx]
			if callID == "" {
				callID = deterministicToolUseID(idx, state.toolName[idx], state.pendingToolArgs[idx])
			}
			events = append(events, closeCCAnthropicBlock(state)...)
			events = append(events, announceCCAnthropicToolBlock(state, idx, callID, state.toolName[idx])...)
		}
	}

	events = append(events, closeCCAnthropicBlock(state)...)
	if !state.TextEmitted && !state.HasToolCall {
		if reasoning := state.ReasoningContent.String(); strings.TrimSpace(reasoning) != "" {
			events = append(events, ensureCCAnthropicTextBlock(state)...)
			state.TextEmitted = true
			events = append(events, ccAnthropicDelta(state, &AnthropicDelta{
				Type: "text_delta",
				Text: reasoning,
			})...)
			events = append(events, closeCCAnthropicBlock(state)...)
		}
	}

	stopReason := ccFinishReasonToAnthropicStopReason(state.FinishReason, state.HasToolCall)

	events = append(events,
		AnthropicStreamEvent{
			Type: "message_delta",
			Delta: &AnthropicDelta{
				StopReason: stopReason,
			},
			Usage: &AnthropicUsage{
				InputTokens:              state.InputTokens,
				OutputTokens:             state.OutputTokens,
				CacheReadInputTokens:     state.CacheReadInputTokens,
				CacheCreationInputTokens: state.CacheCreationInputTokens,
			},
		},
		AnthropicStreamEvent{Type: "message_stop"},
	)
	state.MessageStopSent = true
	return events
}

// ensureCCAnthropicMessageStart 在首次转换时输出 message_start。
func ensureCCAnthropicMessageStart(state *ChatCompletionsToAnthropicStreamState) []AnthropicStreamEvent {
	if state.MessageStartSent {
		return nil
	}
	state.MessageStartSent = true
	return []AnthropicStreamEvent{{
		Type: "message_start",
		Message: &AnthropicResponse{
			ID:         state.ResponseID,
			Type:       "message",
			Role:       "assistant",
			Content:    []AnthropicContentBlock{},
			Model:      state.Model,
			StopReason: nil, // JSON null; never ""
			Usage:      AnthropicUsage{InputTokens: 0, OutputTokens: 0},
		},
	}}
}

// ensureCCAnthropicThinkingBlock 在需要时打开 thinking block。
func ensureCCAnthropicThinkingBlock(state *ChatCompletionsToAnthropicStreamState) []AnthropicStreamEvent {
	if state.ContentBlockOpen && state.CurrentBlockType == "thinking" {
		return nil
	}
	events := closeCCAnthropicBlock(state)
	idx := state.ContentBlockIndex
	state.ContentBlockOpen = true
	state.CurrentBlockType = "thinking"
	events = append(events, AnthropicStreamEvent{
		Type:  "content_block_start",
		Index: &idx,
		ContentBlock: &AnthropicContentBlock{
			Type:     "thinking",
			Thinking: "",
		},
	})
	return events
}

// ensureCCAnthropicTextBlock 在需要时打开 text block。
func ensureCCAnthropicTextBlock(state *ChatCompletionsToAnthropicStreamState) []AnthropicStreamEvent {
	if state.ContentBlockOpen && state.CurrentBlockType == "text" {
		return nil
	}
	events := closeCCAnthropicBlock(state)
	idx := state.ContentBlockIndex
	state.ContentBlockOpen = true
	state.CurrentBlockType = "text"
	events = append(events, AnthropicStreamEvent{
		Type:  "content_block_start",
		Index: &idx,
		ContentBlock: &AnthropicContentBlock{
			Type: "text",
			Text: "",
		},
	})
	return events
}

// handleCCAnthropicToolCall 处理一个上游 tool_call delta。部分上游会先发送 ID
// 或参数再发送 name，因此 name 到达前不输出 content_block_start，而是缓存参数；
// block 创建时先刷新缓存参数，后续参数继续以 input_json_delta 流式输出。
func handleCCAnthropicToolCall(state *ChatCompletionsToAnthropicStreamState, toolCall *ChatToolCall) []AnthropicStreamEvent {
	idx := 0
	if toolCall.Index != nil {
		idx = *toolCall.Index
	}

	var events []AnthropicStreamEvent
	if _, seen := state.toolAnnounced[idx]; !seen {
		state.toolAnnounced[idx] = false
	}
	if toolCall.ID != "" {
		state.pendingToolCallID[idx] = toolCall.ID
	}
	if toolCall.Function.Name != "" {
		state.toolName[idx] = toolCall.Function.Name
	}
	if toolCall.Function.Arguments != "" {
		state.toolArgs[idx] += toolCall.Function.Arguments
		if !state.toolAnnounced[idx] {
			state.pendingToolArgs[idx] += toolCall.Function.Arguments
		}
	}

	if !state.toolAnnounced[idx] && state.pendingToolCallID[idx] != "" && state.toolName[idx] != "" {
		events = append(events, closeCCAnthropicBlock(state)...)
		events = append(events, announceCCAnthropicToolBlock(
			state,
			idx,
			state.pendingToolCallID[idx],
			state.toolName[idx],
		)...)
		return events
	}

	// Read 参数需要在完整 JSON 到齐后统一清洗；其他工具继续逐片流式输出。
	if state.toolAnnounced[idx] && toolCall.Function.Arguments != "" && state.toolName[idx] != "Read" {
		blockIdx := state.toolBlockIndex[idx]
		if state.ContentBlockOpen && blockIdx == state.ContentBlockIndex {
			state.CurrentToolHadDelta = true
		}
		events = append(events, AnthropicStreamEvent{
			Type:  "content_block_delta",
			Index: &blockIdx,
			Delta: &AnthropicDelta{
				Type:        "input_json_delta",
				PartialJSON: toolCall.Function.Arguments,
			},
		})
	}

	return events
}

// announceCCAnthropicToolBlock 为工具分配下一个 Anthropic block index，输出
// content_block_start，并刷新延迟创建期间缓存的参数。
func announceCCAnthropicToolBlock(state *ChatCompletionsToAnthropicStreamState, idx int, callID, name string) []AnthropicStreamEvent {
	blockIdx := state.ContentBlockIndex
	state.toolBlockIndex[idx] = blockIdx
	state.toolAnnounced[idx] = true
	state.toolName[idx] = name
	state.CurrentToolName = name
	state.CurrentToolIndex = idx
	state.CurrentToolHadDelta = false
	state.ContentBlockOpen = true
	state.CurrentBlockType = "tool_use"
	state.HasToolCall = true
	delete(state.pendingToolCallID, idx)

	events := []AnthropicStreamEvent{{
		Type:  "content_block_start",
		Index: &blockIdx,
		ContentBlock: &AnthropicContentBlock{
			Type:  "tool_use",
			ID:    fromResponsesCallID(callID),
			Name:  name,
			Input: json.RawMessage("{}"),
		},
	}}
	if pending := state.pendingToolArgs[idx]; pending != "" && name != "Read" {
		delete(state.pendingToolArgs, idx)
		state.CurrentToolHadDelta = true
		events = append(events, AnthropicStreamEvent{
			Type:  "content_block_delta",
			Index: &blockIdx,
			Delta: &AnthropicDelta{
				Type:        "input_json_delta",
				PartialJSON: pending,
			},
		})
	}
	return events
}

// ccAnthropicDelta 在当前 block 上输出 content_block_delta。
func ccAnthropicDelta(state *ChatCompletionsToAnthropicStreamState, delta *AnthropicDelta) []AnthropicStreamEvent {
	if !state.ContentBlockOpen {
		return nil
	}
	idx := state.ContentBlockIndex
	return []AnthropicStreamEvent{{
		Type:  "content_block_delta",
		Index: &idx,
		Delta: delta,
	}}
}

// closeCCAnthropicBlockIfOpen 只在当前 block 类型匹配时关闭它。
func closeCCAnthropicBlockIfOpen(state *ChatCompletionsToAnthropicStreamState, blockType string) []AnthropicStreamEvent {
	if !state.ContentBlockOpen || state.CurrentBlockType != blockType {
		return nil
	}
	return closeCCAnthropicBlock(state)
}

// closeCCAnthropicBlock 关闭当前 content block。若 tool_use 没有输出过参数 delta，
// 先补一个 input_json_delta "{}"；部分客户端只通过 delta 组装工具输入。
func closeCCAnthropicBlock(state *ChatCompletionsToAnthropicStreamState) []AnthropicStreamEvent {
	if !state.ContentBlockOpen {
		return nil
	}
	idx := state.ContentBlockIndex
	var events []AnthropicStreamEvent
	if state.CurrentBlockType == "tool_use" && (!state.CurrentToolHadDelta || state.CurrentToolName == "Read") {
		arguments := state.toolArgs[state.CurrentToolIndex]
		if strings.TrimSpace(arguments) == "" {
			arguments = "{}"
		}
		events = append(events, AnthropicStreamEvent{
			Type:  "content_block_delta",
			Index: &idx,
			Delta: &AnthropicDelta{
				Type:        "input_json_delta",
				PartialJSON: string(sanitizeAnthropicToolUseInput(state.CurrentToolName, arguments)),
			},
		})
	}
	state.ContentBlockOpen = false
	state.ContentBlockIndex++
	state.CurrentBlockType = ""
	state.CurrentToolName = ""
	state.CurrentToolIndex = -1
	state.CurrentToolHadDelta = false
	return append(events, AnthropicStreamEvent{
		Type:  "content_block_stop",
		Index: &idx,
	})
}

// ccFinishReasonToAnthropicStopReason 把流式 Chat finish_reason 映射为
// message_delta 使用的 Anthropic stop_reason。
func ccFinishReasonToAnthropicStopReason(reason string, hasToolCall bool) string {
	switch reason {
	case "length":
		return "max_tokens"
	case "tool_calls":
		if hasToolCall {
			return "tool_use"
		}
		return "end_turn"
	case "stop":
		if hasToolCall {
			return "tool_use"
		}
		return "end_turn"
	default:
		if hasToolCall {
			return "tool_use"
		}
		return "end_turn"
	}
}

// ChatCompletionsStreamToAnthropicResponse 将完整的 Chat Completions SSE
// chunk 折叠为单个 Anthropic Messages 响应。
//
// @param chunks 上游 SSE chunk 列表。
// @param model 对客户端展示的模型名。
// @return 折叠后的 Anthropic Messages 响应。
func ChatCompletionsStreamToAnthropicResponse(chunks []*ChatCompletionsChunk, model string) *AnthropicResponse {
	id := ""
	var reasoning strings.Builder
	var text strings.Builder
	type toolAggregate struct {
		id   string
		name string
		args strings.Builder
	}
	tools := make(map[int]*toolAggregate)
	maxToolIndex := -1
	finishReason := ""
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
			delta := choice.Delta
			if delta.ReasoningContent != nil {
				_, _ = reasoning.WriteString(*delta.ReasoningContent)
			}
			if delta.Content != nil {
				_, _ = text.WriteString(*delta.Content)
			}
			for _, toolCall := range delta.ToolCalls {
				index := 0
				if toolCall.Index != nil {
					index = *toolCall.Index
				}
				aggregate := tools[index]
				if aggregate == nil {
					aggregate = &toolAggregate{}
					tools[index] = aggregate
					if index > maxToolIndex {
						maxToolIndex = index
					}
				}
				if toolCall.ID != "" {
					aggregate.id = toolCall.ID
				}
				if toolCall.Function.Name != "" {
					aggregate.name = toolCall.Function.Name
				}
				_, _ = aggregate.args.WriteString(toolCall.Function.Arguments)
			}
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				finishReason = *choice.FinishReason
			}
		}
	}

	hasTool := false
	blocks := make([]AnthropicContentBlock, 0, maxToolIndex+3)
	if reasoning.Len() > 0 {
		blocks = append(blocks, AnthropicContentBlock{Type: "thinking", Thinking: reasoning.String()})
	}
	finalText := text.String()
	if finalText == "" && maxToolIndex < 0 && strings.TrimSpace(reasoning.String()) != "" {
		finalText = reasoning.String()
	}
	if finalText != "" {
		blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: finalText})
	}
	for index := 0; index <= maxToolIndex; index++ {
		aggregate := tools[index]
		if aggregate == nil || aggregate.name == "" {
			continue
		}
		hasTool = true
		arguments := aggregate.args.String()
		if strings.TrimSpace(arguments) == "" {
			arguments = "{}"
		}
		toolID := aggregate.id
		if toolID == "" {
			toolID = deterministicToolUseID(index, aggregate.name, arguments)
		}
		blocks = append(blocks, AnthropicContentBlock{
			Type:  "tool_use",
			ID:    toolID,
			Name:  aggregate.name,
			Input: sanitizeAnthropicToolUseInput(aggregate.name, arguments),
		})
	}
	if len(blocks) == 0 {
		blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: ""})
	}
	if id == "" {
		id = generateResponsesID()
	}

	return &AnthropicResponse{
		ID:         id,
		Type:       "message",
		Role:       "assistant",
		Model:      model,
		Content:    blocks,
		StopReason: AnthropicStopReasonPtr(ccFinishReasonToAnthropicStopReason(finishReason, hasTool)),
		Usage:      chatUsageToAnthropicUsage(usage),
	}
}

func deterministicToolUseID(index int, name, arguments string) string {
	if strings.TrimSpace(arguments) == "" {
		arguments = "{}"
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\n%s\n%s", index, name, arguments)))
	return "toolu_" + hex.EncodeToString(sum[:12])
}
