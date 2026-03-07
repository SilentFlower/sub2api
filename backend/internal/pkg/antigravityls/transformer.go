package antigravityls

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/google/uuid"
)

// TrajectoryTransformer 将 LS 的 Cascade Trajectory 步骤转换为 Claude SSE 事件
type TrajectoryTransformer struct {
	originalModel  string
	messageID      string // "msg_" 前缀的唯一 ID
	blockIndex     int    // 当前内容块索引
	processedSteps int    // 已处理的步骤数（增量检测）
	started        bool   // 是否已发送 message_start
}

// NewTrajectoryTransformer 创建转换器
func NewTrajectoryTransformer(originalModel string) *TrajectoryTransformer {
	return &TrajectoryTransformer{
		originalModel: originalModel,
		messageID:     "msg_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:24],
	}
}

// ProcessNewSteps 处理新增的轨迹步骤，返回 Claude SSE 事件字节
// 内部跟踪已处理的步骤数，只处理增量部分
func (t *TrajectoryTransformer) ProcessNewSteps(steps []TrajectoryStep) []byte {
	if len(steps) <= t.processedSteps {
		return nil
	}

	var buf bytes.Buffer

	// 首次调用时发送 message_start
	if !t.started {
		buf.Write(t.emitMessageStart())
		t.started = true
	}

	// 只处理新增的步骤
	newSteps := steps[t.processedSteps:]
	for _, step := range newSteps {
		if step.Type != "PLANNER_RESPONSE" || step.Status != "DONE" {
			continue
		}

		pr := step.GetPlannerResponse()
		if pr == nil {
			continue
		}

		// 处理 thinking 内容
		if pr.Thinking != "" {
			buf.Write(t.emitContentBlockStart("thinking"))
			buf.Write(t.emitThinkingDelta(pr.Thinking))
			buf.Write(t.emitContentBlockStop())
		}

		// 处理文本内容
		if pr.Text != "" {
			buf.Write(t.emitContentBlockStart("text"))
			buf.Write(t.emitTextDelta(pr.Text))
			buf.Write(t.emitContentBlockStop())
		}

		// 处理工具调用
		for _, tc := range pr.ToolCalls {
			buf.Write(t.emitToolUseBlock(tc))
		}
	}

	t.processedSteps = len(steps)
	return buf.Bytes()
}

// Finish 生成结束事件（message_delta + message_stop）
func (t *TrajectoryTransformer) Finish(stopReason string) []byte {
	if stopReason == "" {
		stopReason = "end_turn"
	}
	var buf bytes.Buffer
	buf.Write(t.emitMessageDelta(stopReason))
	buf.Write(t.emitMessageStop())
	return buf.Bytes()
}

// --- SSE 事件生成方法 ---

func (t *TrajectoryTransformer) emitMessageStart() []byte {
	event := map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":      t.messageID,
			"type":    "message",
			"role":    "assistant",
			"model":   t.originalModel,
			"content": []any{},
			"usage": map[string]any{
				"input_tokens":  0,
				"output_tokens": 0,
			},
		},
	}
	return formatSSE("message_start", event)
}

func (t *TrajectoryTransformer) emitContentBlockStart(blockType string) []byte {
	var contentBlock map[string]any
	switch blockType {
	case "thinking":
		contentBlock = map[string]any{"type": "thinking", "thinking": ""}
	case "text":
		contentBlock = map[string]any{"type": "text", "text": ""}
	default:
		contentBlock = map[string]any{"type": blockType}
	}

	event := map[string]any{
		"type":          "content_block_start",
		"index":         t.blockIndex,
		"content_block": contentBlock,
	}
	return formatSSE("content_block_start", event)
}

func (t *TrajectoryTransformer) emitThinkingDelta(thinking string) []byte {
	event := map[string]any{
		"type":  "content_block_delta",
		"index": t.blockIndex,
		"delta": map[string]any{
			"type":     "thinking_delta",
			"thinking": thinking,
		},
	}
	return formatSSE("content_block_delta", event)
}

func (t *TrajectoryTransformer) emitTextDelta(text string) []byte {
	event := map[string]any{
		"type":  "content_block_delta",
		"index": t.blockIndex,
		"delta": map[string]any{
			"type": "text_delta",
			"text": text,
		},
	}
	return formatSSE("content_block_delta", event)
}

func (t *TrajectoryTransformer) emitContentBlockStop() []byte {
	event := map[string]any{
		"type":  "content_block_stop",
		"index": t.blockIndex,
	}
	t.blockIndex++
	return formatSSE("content_block_stop", event)
}

func (t *TrajectoryTransformer) emitToolUseBlock(tc ToolCall) []byte {
	var buf bytes.Buffer

	// tool_use block start
	toolID := "toolu_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:24]
	startEvent := map[string]any{
		"type":  "content_block_start",
		"index": t.blockIndex,
		"content_block": map[string]any{
			"type":  "tool_use",
			"id":    toolID,
			"name":  tc.Name,
			"input": map[string]any{},
		},
	}
	buf.Write(formatSSE("content_block_start", startEvent))

	// tool_use input delta
	inputJSON := tc.ArgumentsJSON
	if inputJSON == "" {
		inputJSON = "{}"
	}
	deltaEvent := map[string]any{
		"type":  "content_block_delta",
		"index": t.blockIndex,
		"delta": map[string]any{
			"type":         "input_json_delta",
			"partial_json": inputJSON,
		},
	}
	buf.Write(formatSSE("content_block_delta", deltaEvent))

	// tool_use block stop
	stopEvent := map[string]any{
		"type":  "content_block_stop",
		"index": t.blockIndex,
	}
	buf.Write(formatSSE("content_block_stop", stopEvent))

	t.blockIndex++
	return buf.Bytes()
}

func (t *TrajectoryTransformer) emitMessageDelta(stopReason string) []byte {
	event := map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason": stopReason,
		},
		"usage": map[string]any{
			"output_tokens": 0,
		},
	}
	return formatSSE("message_delta", event)
}

func (t *TrajectoryTransformer) emitMessageStop() []byte {
	event := map[string]any{
		"type": "message_stop",
	}
	return formatSSE("message_stop", event)
}

// formatSSE 格式化 SSE 事件
func formatSSE(eventType string, data any) []byte {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, jsonData))
}

// --- 非流式转换 ---

// TransformTrajectoryToClaude 将完整的 Trajectory 步骤转为 Claude 非流式响应
func TransformTrajectoryToClaude(steps []TrajectoryStep, originalModel string) (*antigravity.ClaudeResponse, error) {
	msgID := "msg_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:24]
	content := make([]antigravity.ClaudeContentItem, 0)
	stopReason := "end_turn"

	for _, step := range steps {
		if step.Type != "PLANNER_RESPONSE" || step.Status != "DONE" {
			continue
		}

		pr := step.GetPlannerResponse()
		if pr == nil {
			continue
		}

		// 添加 thinking 块
		if pr.Thinking != "" {
			content = append(content, antigravity.ClaudeContentItem{
				Type:     "thinking",
				Thinking: pr.Thinking,
			})
		}

		// 添加文本块
		if pr.Text != "" {
			content = append(content, antigravity.ClaudeContentItem{
				Type: "text",
				Text: pr.Text,
			})
		}

		// 添加工具调用块
		for _, tc := range pr.ToolCalls {
			toolID := "toolu_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:24]
			var input any
			if tc.ArgumentsJSON != "" {
				_ = json.Unmarshal([]byte(tc.ArgumentsJSON), &input)
			}
			if input == nil {
				input = map[string]any{}
			}
			content = append(content, antigravity.ClaudeContentItem{
				Type:  "tool_use",
				ID:    toolID,
				Name:  tc.Name,
				Input: input,
			})
			stopReason = "tool_use"
		}
	}

	return &antigravity.ClaudeResponse{
		ID:         msgID,
		Type:       "message",
		Role:       "assistant",
		Model:      originalModel,
		Content:    content,
		StopReason: stopReason,
		Usage:      antigravity.ClaudeUsage{},
	}, nil
}
