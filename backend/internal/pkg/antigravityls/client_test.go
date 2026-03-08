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

func TestExtractClaudeUsage_FromModelUsageMetadata(t *testing.T) {
	steps := []TrajectoryStep{{
		Type:   "CORTEX_STEP_TYPE_CHECKPOINT",
		Status: "CORTEX_STEP_STATUS_DONE",
		Metadata: &StepMetadata{ModelUsage: &ModelUsage{
			InputTokens:             120,
			OutputTokens:            34,
			CachedContentTokenCount: 20,
		}},
	}}

	usage := ExtractClaudeUsage(steps)
	if usage.InputTokens != 120 || usage.OutputTokens != 34 || usage.CacheReadInputTokens != 20 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}

func TestExtractClaudeUsage_DerivesFromPromptAndThoughts(t *testing.T) {
	steps := []TrajectoryStep{{
		Type:   "CORTEX_STEP_TYPE_CHECKPOINT",
		Status: "CORTEX_STEP_STATUS_DONE",
		Metadata: &StepMetadata{ModelUsage: &ModelUsage{
			PromptTokenCount:        200,
			CandidatesTokenCount:    25,
			ThoughtsTokenCount:      5,
			CachedContentTokenCount: 40,
		}},
	}}

	usage := ExtractClaudeUsage(steps)
	if usage.InputTokens != 160 || usage.OutputTokens != 30 || usage.CacheReadInputTokens != 40 {
		t.Fatalf("unexpected derived usage: %+v", usage)
	}
}
