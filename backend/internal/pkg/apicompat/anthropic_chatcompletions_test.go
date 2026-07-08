package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 测试辅助函数
// ---------------------------------------------------------------------------

func intPtr(i int) *int { return &i }

func anthChatTextChunk(s string) *ChatCompletionsChunk {
	return &ChatCompletionsChunk{Choices: []ChatChunkChoice{{Delta: ChatDelta{Content: stringPtr(s)}}}}
}

func anthChatReasoningChunk(s string) *ChatCompletionsChunk {
	return &ChatCompletionsChunk{Choices: []ChatChunkChoice{{Delta: ChatDelta{ReasoningContent: stringPtr(s)}}}}
}

func anthChatFinishChunk(reason string) *ChatCompletionsChunk {
	return &ChatCompletionsChunk{Choices: []ChatChunkChoice{{FinishReason: stringPtr(reason)}}}
}

func anthChatConcatDelta(events []AnthropicStreamEvent, deltaType string) string {
	var b strings.Builder
	for _, e := range events {
		if e.Type != "content_block_delta" || e.Delta == nil || e.Delta.Type != deltaType {
			continue
		}
		switch deltaType {
		case "text_delta":
			_, _ = b.WriteString(e.Delta.Text)
		case "thinking_delta":
			_, _ = b.WriteString(e.Delta.Thinking)
		case "input_json_delta":
			_, _ = b.WriteString(e.Delta.PartialJSON)
		}
	}
	return b.String()
}

// anthChatInputJSONByIndex 按 block index 拼接 input_json_delta 片段。
func anthChatInputJSONByIndex(events []AnthropicStreamEvent) map[int]string {
	m := map[int]string{}
	for _, e := range events {
		if e.Type == "content_block_delta" && e.Index != nil && e.Delta != nil && e.Delta.Type == "input_json_delta" {
			m[*e.Index] += e.Delta.PartialJSON
		}
	}
	return m
}

// anthChatStartedBlocks 将每个已启动 content block 的 index 映射到类型。
func anthChatStartedBlocks(events []AnthropicStreamEvent) map[int]string {
	m := map[int]string{}
	for _, e := range events {
		if e.Type == "content_block_start" && e.Index != nil && e.ContentBlock != nil {
			m[*e.Index] = e.ContentBlock.Type
		}
	}
	return m
}

func anthChatToolUseIDs(events []AnthropicStreamEvent) []string {
	var ids []string
	for _, e := range events {
		if e.Type == "content_block_start" && e.ContentBlock != nil && e.ContentBlock.Type == "tool_use" {
			ids = append(ids, e.ContentBlock.ID)
		}
	}
	return ids
}

func anthChatMessageDelta(events []AnthropicStreamEvent) *AnthropicStreamEvent {
	for i := range events {
		if events[i].Type == "message_delta" {
			return &events[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// AnthropicToChatCompletions 请求侧测试
// ---------------------------------------------------------------------------

func TestAnthropicToChatCompletions_SystemAndToolResultOrdering(t *testing.T) {
	req := &AnthropicRequest{
		Model:  "claude",
		System: json.RawMessage(`"You are helpful"`),
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"hi"`)},
			{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"call_1","name":"get_weather","input":{"location":"Paris"}}]`)},
			{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_1","content":"sunny"},{"type":"text","text":"thanks"}]`)},
		},
	}

	out, err := AnthropicToChatCompletions(req)
	require.NoError(t, err)
	require.Len(t, out.Messages, 5)

	assert.Equal(t, "system", out.Messages[0].Role)
	assert.JSONEq(t, `[{"type":"text","text":"You are helpful"}]`, string(out.Messages[0].Content))

	assert.Equal(t, "user", out.Messages[1].Role)
	assert.JSONEq(t, `[{"type":"text","text":"hi"}]`, string(out.Messages[1].Content))

	assert.Equal(t, "assistant", out.Messages[2].Role)
	require.Len(t, out.Messages[2].ToolCalls, 1)
	assert.Equal(t, "call_1", out.Messages[2].ToolCalls[0].ID)
	assert.Equal(t, "get_weather", out.Messages[2].ToolCalls[0].Function.Name)
	assert.JSONEq(t, `{"location":"Paris"}`, out.Messages[2].ToolCalls[0].Function.Arguments)

	// tool 回复必须排在同一轮次中的新用户文本之前。
	assert.Equal(t, "tool", out.Messages[3].Role)
	assert.Equal(t, "call_1", out.Messages[3].ToolCallID)
	assert.JSONEq(t, `"sunny"`, string(out.Messages[3].Content))

	assert.Equal(t, "user", out.Messages[4].Role)
	assert.JSONEq(t, `[{"type":"text","text":"thanks"}]`, string(out.Messages[4].Content))
}

func TestAnthropicToChatCompletions_ParallelToolResultsFollowToolCallOrder(t *testing.T) {
	req := &AnthropicRequest{
		Model: "claude",
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"inspect"`)},
			{Role: "assistant", Content: json.RawMessage(`[
				{"type":"tool_use","id":"call_a","name":"Read","input":{"file_path":"a.go"}},
				{"type":"tool_use","id":"call_b","name":"Read","input":{"file_path":"b.go"}}
			]`)},
			{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_b","content":"b result"}]`)},
			{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"call_a","content":"a result"}]`)},
			{Role: "user", Content: json.RawMessage(`"after tools"`)},
		},
	}

	out, err := AnthropicToChatCompletions(req)
	require.NoError(t, err)
	require.Len(t, out.Messages, 5)

	require.Equal(t, "assistant", out.Messages[1].Role)
	require.Len(t, out.Messages[1].ToolCalls, 2)
	assert.Equal(t, "call_a", out.Messages[1].ToolCalls[0].ID)
	assert.Equal(t, "call_b", out.Messages[1].ToolCalls[1].ID)

	assert.Equal(t, "tool", out.Messages[2].Role)
	assert.Equal(t, "call_a", out.Messages[2].ToolCallID)
	assert.JSONEq(t, `"a result"`, string(out.Messages[2].Content))
	assert.Equal(t, "tool", out.Messages[3].Role)
	assert.Equal(t, "call_b", out.Messages[3].ToolCallID)
	assert.JSONEq(t, `"b result"`, string(out.Messages[3].Content))

	assert.Equal(t, "user", out.Messages[4].Role)
	assert.JSONEq(t, `[{"type":"text","text":"after tools"}]`, string(out.Messages[4].Content))
}

func TestStabilizeChatToolResultOrder_UnknownToolResultsStayAfterKnown(t *testing.T) {
	messages := []ChatMessage{
		{Role: "assistant", ToolCalls: []ChatToolCall{{ID: "call_a"}, {ID: "call_b"}}},
		{Role: "tool", ToolCallID: "unknown_1"},
		{Role: "tool", ToolCallID: "call_b"},
		{Role: "tool", ToolCallID: "unknown_2"},
		{Role: "tool", ToolCallID: "call_a"},
	}

	out := stabilizeChatToolResultOrder(messages)

	assert.Equal(t, "call_a", out[1].ToolCallID)
	assert.Equal(t, "call_b", out[2].ToolCallID)
	assert.Equal(t, "unknown_1", out[3].ToolCallID)
	assert.Equal(t, "unknown_2", out[4].ToolCallID)
}

func TestStabilizeChatToolResultOrder_DoesNotCrossUserMessage(t *testing.T) {
	messages := []ChatMessage{
		{Role: "assistant", ToolCalls: []ChatToolCall{{ID: "call_a"}, {ID: "call_b"}}},
		{Role: "tool", ToolCallID: "call_b"},
		{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"continue"}]`)},
		{Role: "tool", ToolCallID: "call_a"},
	}

	out := stabilizeChatToolResultOrder(messages)

	assert.Equal(t, "call_b", out[1].ToolCallID)
	assert.Equal(t, "user", out[2].Role)
	assert.Equal(t, "call_a", out[3].ToolCallID)
}

func TestAnthropicToChatCompletions_StableContentPartsAndAttribution(t *testing.T) {
	req := &AnthropicRequest{
		Model:  "claude",
		System: json.RawMessage(`[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.63.abc; cc_entrypoint=cli; cch=12345;"},{"type":"text","text":"stable system"}]`),
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"hi"}]`)},
			{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"pre"},{"type":"tool_use","id":"call_1","name":"do_work","input":{"a":1}},{"type":"text","text":"post"}]`)},
		},
	}

	out, err := AnthropicToChatCompletions(req)
	require.NoError(t, err)
	require.Len(t, out.Messages, 3)

	require.Equal(t, "system", out.Messages[0].Role)
	assert.JSONEq(t, `[{"type":"text","text":"stable system"}]`, string(out.Messages[0].Content))
	assert.NotContains(t, string(out.Messages[0].Content), "x-anthropic-billing-header")

	require.Equal(t, "user", out.Messages[1].Role)
	assert.JSONEq(t, `[{"type":"text","text":"hi"}]`, string(out.Messages[1].Content))

	require.Equal(t, "assistant", out.Messages[2].Role)
	assert.JSONEq(t, `[{"type":"text","text":"pre"},{"type":"text","text":"post"}]`, string(out.Messages[2].Content))
	require.Len(t, out.Messages[2].ToolCalls, 1)
	assert.Equal(t, "call_1", out.Messages[2].ToolCalls[0].ID)
	assert.Equal(t, "do_work", out.Messages[2].ToolCalls[0].Function.Name)
	assert.JSONEq(t, `{"a":1}`, out.Messages[2].ToolCalls[0].Function.Arguments)
}

func TestAnthropicToChatCompletions_ToolsAndToolChoice(t *testing.T) {
	req := &AnthropicRequest{
		Model: "claude",
		Tools: []AnthropicTool{
			{Name: "get_weather", Description: "Get weather", InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}}}`)},
			{Type: "web_search_20250305", Name: "web_search", InputSchema: json.RawMessage(`{}`)},
			{Type: "bash_20250124", Name: "bash"},
		},
		ToolChoice: json.RawMessage(`{"type":"any"}`),
	}

	out, err := AnthropicToChatCompletions(req)
	require.NoError(t, err)

	require.Len(t, out.Tools, 1, "all typed built-in tools (web_search, bash) should be skipped; only the custom tool survives")
	assert.Equal(t, "function", out.Tools[0].Type)
	require.NotNil(t, out.Tools[0].Function)
	assert.Equal(t, "get_weather", out.Tools[0].Function.Name)
	require.NotNil(t, out.Tools[0].Function.Strict)
	assert.False(t, *out.Tools[0].Function.Strict)
	assert.Equal(t, `"required"`, string(out.ToolChoice))
}

func TestAnthropicToChatCompletions_ReasoningEffort(t *testing.T) {
	enabled, err := AnthropicToChatCompletions(&AnthropicRequest{Model: "c", Thinking: &AnthropicThinking{Type: "enabled"}})
	require.NoError(t, err)
	assert.Equal(t, "medium", enabled.ReasoningEffort)

	maxed, err := AnthropicToChatCompletions(&AnthropicRequest{Model: "c", OutputConfig: &AnthropicOutputConfig{Effort: "max"}})
	require.NoError(t, err)
	assert.Equal(t, "xhigh", maxed.ReasoningEffort)
}

func TestAnthropicToChatCompletions_ThinkingDisabled(t *testing.T) {
	// thinking:disabled 必须把 {type:"disabled"} 透传给上游，并去掉
	// reasoning_effort；即使 output_config.effort 原本会设置 effort 也一样。
	// 这样 reasoning 模型（GLM/...）才会停止 thinking，不会继续消耗 token 预算，
	// 严格上游也不会收到 disable+effort 的互斥参数组合。
	out, err := AnthropicToChatCompletions(&AnthropicRequest{
		Model:        "c",
		Thinking:     &AnthropicThinking{Type: "disabled"},
		OutputConfig: &AnthropicOutputConfig{Effort: "high"},
	})
	require.NoError(t, err)
	require.NotNil(t, out.Thinking)
	assert.Equal(t, "disabled", out.Thinking.Type)
	assert.Equal(t, "", out.ReasoningEffort, "reasoning_effort must be dropped when thinking is disabled")

	// Anthropic 专属的 budget_tokens 不能泄漏到 chat 请求。
	b, err := json.Marshal(out)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"thinking":{"type":"disabled"}`)
	assert.NotContains(t, string(b), "budget_tokens")

	// enabled 沿用既有 reasoning_effort 映射，不输出 thinking 字段。
	enabled, err := AnthropicToChatCompletions(&AnthropicRequest{Model: "c", Thinking: &AnthropicThinking{Type: "enabled"}})
	require.NoError(t, err)
	assert.Nil(t, enabled.Thinking)
	assert.Equal(t, "medium", enabled.ReasoningEffort)
}

// ---------------------------------------------------------------------------
// Streaming chat -> Anthropic SSE 测试
// ---------------------------------------------------------------------------

func TestChatCompletionsChunkToAnthropicEvents_TextAndUsage(t *testing.T) {
	st := NewChatCompletionsToAnthropicStreamState("deepseek")
	var all []AnthropicStreamEvent
	all = append(all, ChatCompletionsChunkToAnthropicEvents(&ChatCompletionsChunk{ID: "id1", Choices: []ChatChunkChoice{{Delta: ChatDelta{Content: stringPtr("Hello")}}}}, st)...)
	all = append(all, ChatCompletionsChunkToAnthropicEvents(anthChatTextChunk(" world"), st)...)
	all = append(all, ChatCompletionsChunkToAnthropicEvents(&ChatCompletionsChunk{
		Choices: []ChatChunkChoice{{FinishReason: stringPtr("stop")}},
		Usage:   &ChatUsage{PromptTokens: 100, CompletionTokens: 20, PromptTokensDetails: &ChatTokenDetails{CachedTokens: 30}},
	}, st)...)
	all = append(all, FinalizeChatCompletionsAnthropicStream(st)...)

	require.Equal(t, "message_start", all[0].Type)
	require.NotNil(t, all[0].Message)
	assert.Equal(t, "assistant", all[0].Message.Role)
	assert.Equal(t, "text", anthChatStartedBlocks(all)[0])
	assert.Equal(t, "Hello world", anthChatConcatDelta(all, "text_delta"))
	assert.Equal(t, "message_stop", all[len(all)-1].Type)

	md := anthChatMessageDelta(all)
	require.NotNil(t, md)
	assert.Equal(t, "end_turn", md.Delta.StopReason)
	require.NotNil(t, md.Usage)
	// Anthropic input_tokens 不包含 cached；cached 通过 cache_read 暴露。
	assert.Equal(t, 70, md.Usage.InputTokens)
	assert.Equal(t, 30, md.Usage.CacheReadInputTokens)
	assert.Equal(t, 20, md.Usage.OutputTokens)
}

func TestChatCompletionsChunkToAnthropicEvents_ReasoningThenText(t *testing.T) {
	st := NewChatCompletionsToAnthropicStreamState("m")
	var all []AnthropicStreamEvent
	all = append(all, ChatCompletionsChunkToAnthropicEvents(anthChatReasoningChunk("thinking "), st)...)
	all = append(all, ChatCompletionsChunkToAnthropicEvents(anthChatReasoningChunk("more"), st)...)
	all = append(all, ChatCompletionsChunkToAnthropicEvents(anthChatTextChunk("answer"), st)...)
	all = append(all, FinalizeChatCompletionsAnthropicStream(st)...)

	started := anthChatStartedBlocks(all)
	assert.Equal(t, "thinking", started[0])
	assert.Equal(t, "text", started[1])
	assert.Equal(t, "thinking more", anthChatConcatDelta(all, "thinking_delta"))
	assert.Equal(t, "answer", anthChatConcatDelta(all, "text_delta"))
}

// TestChatCompletionsChunkToAnthropicEvents_ParallelToolsNoOrphan 是关键回归保护：
// chat -> responses -> anthropic 路由曾在并行工具场景下产生指向虚假 block index 的
// 孤立 input_json_delta。直连路径必须先打开每个接收 delta 的 block，并且不能重复发送
// 参数片段。
func TestChatCompletionsChunkToAnthropicEvents_ParallelToolsNoOrphan(t *testing.T) {
	st := NewChatCompletionsToAnthropicStreamState("m")
	var all []AnthropicStreamEvent
	all = append(all, ChatCompletionsChunkToAnthropicEvents(&ChatCompletionsChunk{
		ID: "id",
		Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{
			{Index: intPtr(0), ID: "call_a", Type: "function", Function: ChatFunctionCall{Name: "get_weather", Arguments: `{"location":"Paris"}`}},
			{Index: intPtr(1), ID: "call_b", Type: "function", Function: ChatFunctionCall{Name: "get_time", Arguments: `{"tz":"UTC"}`}},
		}}}},
	}, st)...)
	all = append(all, ChatCompletionsChunkToAnthropicEvents(anthChatFinishChunk("tool_calls"), st)...)
	all = append(all, FinalizeChatCompletionsAnthropicStream(st)...)

	started := anthChatStartedBlocks(all)
	require.Len(t, started, 2)
	assert.Equal(t, "tool_use", started[0])
	assert.Equal(t, "tool_use", started[1])

	ij := anthChatInputJSONByIndex(all)
	require.Len(t, ij, 2)
	// 每个接收 input_json_delta 的 block 都必须已打开，不能出现孤立 delta。
	for idx := range ij {
		_, ok := started[idx]
		assert.Truef(t, ok, "input_json_delta at unopened block index %d (orphan)", idx)
	}
	// 每组参数只能出现一次，不能重复。
	assert.JSONEq(t, `{"location":"Paris"}`, ij[0])
	assert.JSONEq(t, `{"tz":"UTC"}`, ij[1])

	md := anthChatMessageDelta(all)
	require.NotNil(t, md)
	assert.Equal(t, "tool_use", md.Delta.StopReason)
}

func TestChatCompletionsChunkToAnthropicEvents_ReadToolBuffered(t *testing.T) {
	st := NewChatCompletionsToAnthropicStreamState("m")
	chunk1 := &ChatCompletionsChunk{ID: "id", Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{
		{Index: intPtr(0), ID: "call_r", Type: "function", Function: ChatFunctionCall{Name: "Read", Arguments: `{"file_path":"/x",`}},
	}}}}}
	chunk2 := &ChatCompletionsChunk{Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{
		{Index: intPtr(0), Function: ChatFunctionCall{Arguments: `"pages":""}`}},
	}}}}}

	e1 := ChatCompletionsChunkToAnthropicEvents(chunk1, st)
	e2 := ChatCompletionsChunkToAnthropicEvents(chunk2, st)
	streamPhase := append(append([]AnthropicStreamEvent{}, e1...), e2...)
	fin := FinalizeChatCompletionsAnthropicStream(st)
	all := append(append([]AnthropicStreamEvent{}, streamPhase...), fin...)

	// "Read" 参数会被缓冲，中途不流式输出。
	assert.Empty(t, anthChatInputJSONByIndex(streamPhase), "Read tool args must be buffered until close")

	// 关闭时只输出一个清洗后的 delta，并去掉 pages:""。
	ij := anthChatInputJSONByIndex(all)
	require.Len(t, ij, 1)
	assert.JSONEq(t, `{"file_path":"/x"}`, ij[0])
}

func TestChatCompletionsChunkToAnthropicEvents_MissingToolIDDeterministic(t *testing.T) {
	run := func() []AnthropicStreamEvent {
		st := NewChatCompletionsToAnthropicStreamState("m")
		var all []AnthropicStreamEvent
		first := ChatCompletionsChunkToAnthropicEvents(&ChatCompletionsChunk{ID: "id", Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{
			{Index: intPtr(0), Type: "function", Function: ChatFunctionCall{Name: "do_work", Arguments: `{"a":`}},
		}}}}}, st)
		assert.Empty(t, anthChatToolUseIDs(first), "缺少 tool_call.id 时应延迟发送 tool_use block")
		all = append(all, first...)
		all = append(all, ChatCompletionsChunkToAnthropicEvents(&ChatCompletionsChunk{Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{
			{Index: intPtr(0), Function: ChatFunctionCall{Arguments: `1}`}},
		}}}}}, st)...)
		all = append(all, ChatCompletionsChunkToAnthropicEvents(anthChatFinishChunk("tool_calls"), st)...)
		all = append(all, FinalizeChatCompletionsAnthropicStream(st)...)
		return all
	}

	firstRun := run()
	secondRun := run()
	firstIDs := anthChatToolUseIDs(firstRun)
	secondIDs := anthChatToolUseIDs(secondRun)
	require.Len(t, firstIDs, 1)
	require.Len(t, secondIDs, 1)
	assert.Equal(t, firstIDs[0], secondIDs[0])
	assert.True(t, strings.HasPrefix(firstIDs[0], "toolu_"))

	ij := anthChatInputJSONByIndex(firstRun)
	require.Len(t, ij, 1)
	assert.JSONEq(t, `{"a":1}`, ij[0])
	md := anthChatMessageDelta(firstRun)
	require.NotNil(t, md)
	assert.Equal(t, "tool_use", md.Delta.StopReason)
}

func TestChatCompletionsChunkToAnthropicEvents_LateToolIDUsesUpstreamID(t *testing.T) {
	st := NewChatCompletionsToAnthropicStreamState("m")
	var all []AnthropicStreamEvent
	first := ChatCompletionsChunkToAnthropicEvents(&ChatCompletionsChunk{ID: "id", Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{
		{Index: intPtr(0), Type: "function", Function: ChatFunctionCall{Name: "do_work", Arguments: `{"a":`}},
	}}}}}, st)
	assert.Empty(t, anthChatToolUseIDs(first), "上游 id 可能在后续 chunk 到达，不能提前生成 fallback id")
	all = append(all, first...)
	all = append(all, ChatCompletionsChunkToAnthropicEvents(&ChatCompletionsChunk{Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{
		{Index: intPtr(0), ID: "call_late", Function: ChatFunctionCall{Arguments: `1}`}},
	}}}}}, st)...)
	all = append(all, ChatCompletionsChunkToAnthropicEvents(anthChatFinishChunk("tool_calls"), st)...)
	all = append(all, FinalizeChatCompletionsAnthropicStream(st)...)

	ids := anthChatToolUseIDs(all)
	require.Len(t, ids, 1)
	assert.Equal(t, "call_late", ids[0])
	ij := anthChatInputJSONByIndex(all)
	require.Len(t, ij, 1)
	assert.JSONEq(t, `{"a":1}`, ij[0])
}

func TestFinalizeChatCompletionsAnthropicStream_ReasoningOnlyFallback(t *testing.T) {
	st := NewChatCompletionsToAnthropicStreamState("m")
	_ = ChatCompletionsChunkToAnthropicEvents(anthChatReasoningChunk("the reasoning"), st)
	_ = ChatCompletionsChunkToAnthropicEvents(anthChatFinishChunk("stop"), st)
	fin := FinalizeChatCompletionsAnthropicStream(st)

	// 只有 reasoning 的完成结果会把 reasoning 作为 text block 回显，避免客户端收到
	// 没有正文的 thinking block。
	assert.Equal(t, "text", anthChatStartedBlocks(fin)[1])
	assert.Equal(t, "the reasoning", anthChatConcatDelta(fin, "text_delta"))

	md := anthChatMessageDelta(fin)
	require.NotNil(t, md)
	assert.Equal(t, "end_turn", md.Delta.StopReason)
}

func TestFinalizeChatCompletionsAnthropicStream_EmptyStreamFramesMessage(t *testing.T) {
	st := NewChatCompletionsToAnthropicStreamState("m")
	fin := FinalizeChatCompletionsAnthropicStream(st)

	require.Len(t, fin, 3)
	assert.Equal(t, "message_start", fin[0].Type)
	assert.Equal(t, "message_delta", fin[1].Type)
	assert.Equal(t, "message_stop", fin[2].Type)
	require.NotNil(t, fin[1].Delta)
	assert.Equal(t, "end_turn", fin[1].Delta.StopReason)
}

// ---------------------------------------------------------------------------
// ChatCompletionsStreamToAnthropicResponse 同步折叠路径测试
// ---------------------------------------------------------------------------

func TestChatCompletionsStreamToAnthropicResponse_BlockOrderAndUsage(t *testing.T) {
	chunks := []*ChatCompletionsChunk{
		{ID: "id", Choices: []ChatChunkChoice{{Delta: ChatDelta{ReasoningContent: stringPtr("r")}}}},
		anthChatTextChunk("t"),
		{Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{
			{Index: intPtr(0), ID: "call_a", Type: "function", Function: ChatFunctionCall{Name: "foo", Arguments: `{"a":1}`}},
		}}}}},
		{Choices: []ChatChunkChoice{{FinishReason: stringPtr("tool_calls")}}, Usage: &ChatUsage{PromptTokens: 100, CompletionTokens: 20, PromptTokensDetails: &ChatTokenDetails{CachedTokens: 30}}},
	}

	resp := ChatCompletionsStreamToAnthropicResponse(chunks, "deepseek")
	require.NotNil(t, resp)
	assert.Equal(t, "id", resp.ID)
	assert.Equal(t, "message", resp.Type)
	assert.Equal(t, "assistant", resp.Role)

	// block 顺序对齐 ResponsesToAnthropic：thinking、text、tool_use。
	require.Len(t, resp.Content, 3)
	assert.Equal(t, "thinking", resp.Content[0].Type)
	assert.Equal(t, "r", resp.Content[0].Thinking)
	assert.Equal(t, "text", resp.Content[1].Type)
	assert.Equal(t, "t", resp.Content[1].Text)
	assert.Equal(t, "tool_use", resp.Content[2].Type)
	assert.Equal(t, "call_a", resp.Content[2].ID)
	assert.Equal(t, "foo", resp.Content[2].Name)
	assert.JSONEq(t, `{"a":1}`, string(resp.Content[2].Input))

	assert.Equal(t, "tool_use", resp.StopReason)
	assert.Equal(t, 70, resp.Usage.InputTokens)
	assert.Equal(t, 30, resp.Usage.CacheReadInputTokens)
	assert.Equal(t, 20, resp.Usage.OutputTokens)
}

func TestChatCompletionsStreamToAnthropicResponse_MissingToolIDDeterministic(t *testing.T) {
	build := func() *AnthropicResponse {
		chunks := []*ChatCompletionsChunk{
			{ID: "id", Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{
				{Index: intPtr(0), Type: "function", Function: ChatFunctionCall{Name: "foo", Arguments: `{"a":`}},
			}}}}},
			{Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{
				{Index: intPtr(0), Function: ChatFunctionCall{Arguments: `1}`}},
			}}}}},
			anthChatFinishChunk("tool_calls"),
		}
		return ChatCompletionsStreamToAnthropicResponse(chunks, "m")
	}

	first := build()
	second := build()
	require.Len(t, first.Content, 1)
	require.Len(t, second.Content, 1)
	assert.Equal(t, "tool_use", first.Content[0].Type)
	assert.Equal(t, first.Content[0].ID, second.Content[0].ID)
	assert.True(t, strings.HasPrefix(first.Content[0].ID, "toolu_"))
	assert.JSONEq(t, `{"a":1}`, string(first.Content[0].Input))
}

func TestChatCompletionsStreamToAnthropicResponse_ReasoningOnlyFallback(t *testing.T) {
	chunks := []*ChatCompletionsChunk{
		{ID: "id", Choices: []ChatChunkChoice{{Delta: ChatDelta{ReasoningContent: stringPtr("just thinking")}}}},
		anthChatFinishChunk("stop"),
	}

	resp := ChatCompletionsStreamToAnthropicResponse(chunks, "m")
	require.Len(t, resp.Content, 2)
	assert.Equal(t, "thinking", resp.Content[0].Type)
	assert.Equal(t, "text", resp.Content[1].Type)
	assert.Equal(t, "just thinking", resp.Content[1].Text)
	assert.Equal(t, "end_turn", resp.StopReason)
}
