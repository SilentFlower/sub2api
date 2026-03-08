package antigravityls

import "testing"

func TestTrajectoryStep_IsTypeAndStatus_WithCortexPrefix(t *testing.T) {
	step := TrajectoryStep{
		Type:   "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
		Status: "CORTEX_STEP_STATUS_DONE",
	}

	if !step.IsType("PLANNER_RESPONSE") {
		t.Fatalf("expected planner response type to match")
	}
	if !step.IsStatus("DONE") {
		t.Fatalf("expected DONE status to match")
	}
}

func TestPlannerResponse_GetText_PrefersResponseFields(t *testing.T) {
	pr := &PlannerResponse{Response: "answer"}
	if got := pr.GetText(); got != "answer" {
		t.Fatalf("expected response text, got %q", got)
	}

	pr = &PlannerResponse{RawResponse: "raw-answer"}
	if got := pr.GetText(); got != "raw-answer" {
		t.Fatalf("expected raw response text, got %q", got)
	}
}

func TestIsTrajectoryDone_CheckpointIsTerminal(t *testing.T) {
	steps := []TrajectoryStep{{Type: "CORTEX_STEP_TYPE_CHECKPOINT", Status: "CORTEX_STEP_STATUS_DONE"}}
	if !isTrajectoryDone(steps) {
		t.Fatalf("expected checkpoint to be terminal")
	}
}

func TestBuildStepsFingerprint_UsesPlannerResponseTextFallback(t *testing.T) {
	steps := []TrajectoryStep{{
		Type:   "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
		Status: "CORTEX_STEP_STATUS_DONE",
		PlannerResponse: &PlannerResponse{
			Response: "hello",
		},
	}}

	fp := buildStepsFingerprint(steps)
	if fp == "CORTEX_STEP_TYPE_PLANNER_RESPONSE:CORTEX_STEP_STATUS_DONE:t0:k0:tc0|" {
		t.Fatalf("expected fingerprint to include response text length, got %q", fp)
	}
}
