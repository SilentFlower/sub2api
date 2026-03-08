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
