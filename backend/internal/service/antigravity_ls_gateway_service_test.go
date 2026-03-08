package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

func TestBuildUserText_RendersToolTranscriptForClaudeCode(t *testing.T) {
	svc := &AntigravityLSGatewayService{}
	req := &antigravity.ClaudeRequest{
		Tools: []antigravity.ClaudeTool{{Name: "bash", Description: "run shell command"}},
		Messages: []antigravity.ClaudeMessage{
			{
				Role: "assistant",
				Content: mustRawJSON(t, []map[string]any{{
					"type":  "tool_use",
					"id":    "toolu_1",
					"name":  "bash",
					"input": map[string]any{"command": "pwd"},
				}}),
			},
			{
				Role: "user",
				Content: mustRawJSON(t, []map[string]any{{
					"type":        "tool_result",
					"tool_use_id": "toolu_1",
					"content":     " /root/project/sub2api ",
				}}),
			},
		},
	}

	text := svc.buildUserText(req)
	if !strings.Contains(text, "run_command") {
		t.Fatalf("expected transcript to use AG tool name, got %q", text)
	}
	if !strings.Contains(text, "/root/project/sub2api") {
		t.Fatalf("expected tool result content in transcript, got %q", text)
	}
	if !strings.Contains(text, "[Available Tools]") || !strings.Contains(text, "ag=run_command") {
		t.Fatalf("expected transcript to include translated tool inventory, got %q", text)
	}
}

func mustRawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal raw json failed: %v", err)
	}
	return data
}
