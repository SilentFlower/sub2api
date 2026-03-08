package antigravityls

import (
	"strings"
	"testing"
)

func TestProcessNewSteps_UsesCortexEnumAndResponseField(t *testing.T) {
	transformer := NewTrajectoryTransformer("claude-opus-4-6")
	steps := []TrajectoryStep{{
		Type:   "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
		Status: "CORTEX_STEP_STATUS_DONE",
		PlannerResponse: &PlannerResponse{
			Response: "hello from response",
		},
	}}

	events := string(transformer.ProcessNewSteps(steps))
	if !strings.Contains(events, "message_start") {
		t.Fatalf("expected message_start event, got %q", events)
	}
	if !strings.Contains(events, "hello from response") {
		t.Fatalf("expected response text in SSE events, got %q", events)
	}
}

func TestProcessNewSteps_TranslatesAGToolCallToClientTool(t *testing.T) {
	transformer := NewTrajectoryTransformer("claude-opus-4-6")
	steps := []TrajectoryStep{{
		Type:   "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
		Status: "CORTEX_STEP_STATUS_DONE",
		PlannerResponse: &PlannerResponse{
			ToolCalls: []ToolCall{{
				Name:          "run_command",
				ArgumentsJSON: `{"CommandLine":"ls -la","Cwd":"/tmp"}`,
			}},
		},
	}}

	events := string(transformer.ProcessNewSteps(steps))
	if !strings.Contains(events, `"name":"bash"`) {
		t.Fatalf("expected AG tool to translate to bash, got %q", events)
	}
	if !strings.Contains(events, `ls -la`) || !strings.Contains(events, `/tmp`) {
		t.Fatalf("expected translated command args, got %q", events)
	}
}

func TestBuildToolUseID_IsDeterministic(t *testing.T) {
	input := map[string]any{"command": "pwd"}
	id1 := buildToolUseID("", "bash", input, 0)
	id2 := buildToolUseID("", "bash", input, 0)
	if id1 == "" || id1 != id2 {
		t.Fatalf("expected deterministic tool id, got %q and %q", id1, id2)
	}
}
