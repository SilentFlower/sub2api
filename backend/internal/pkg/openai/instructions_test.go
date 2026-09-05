package openai

import (
	"strings"
	"testing"
)

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// CodexBaseInstructionsForModel 应按模型返回对应的真实 Codex base prompt。
func TestCodexBaseInstructionsForModel(t *testing.T) {
	cases := []struct {
		model    string
		wantHead string
	}{
		{"gpt-5-codex", "You are Codex, based on GPT-5"},
		{"gpt-5.3-codex", "You are Codex, based on GPT-5"},
		{"gpt-5.3-codex-spark", "You are Codex, based on GPT-5"},
		{"gpt-5.1-codex-max", "You are Codex, based on GPT-5"},
		{"gpt-5.2-codex", "You are Codex, based on GPT-5"},
		{"gpt-5.6-codex", "You are Codex, based on GPT-5"},
		{"gpt-6-codex", "You are Codex, based on GPT-5"},
		{"gpt-5.6", "You are Codex, an agent based on GPT-5."},
		{"gpt-5.6-sol", "You are Codex, an agent based on GPT-5."},
		{"gpt-5.6-terra", "You are Codex, an agent based on GPT-5."},
		{"gpt-5.6-luna", "You are Codex, an agent based on GPT-5."},
		{" GPT-5.6-SOL ", "You are Codex, an agent based on GPT-5."},
		{"gpt-6", "You are Codex, an agent based on GPT-6."},
		{"gpt-6-astra", "You are Codex, an agent based on GPT-6."},
		{" GPT-6-ASTRA ", "You are Codex, an agent based on GPT-6."},
		{"gpt-5.6-pro", "You are Codex, a coding agent based on GPT-5"},
		{"gpt-5.60", "You are Codex, a coding agent based on GPT-5"},
		{"gpt-6-pro", "You are Codex, a coding agent based on GPT-5"},
		{"gpt-6-astra-custom", "You are Codex, a coding agent based on GPT-5"},
		{"gpt-60", "You are Codex, a coding agent based on GPT-5"},
		{"gpt-5.5", "You are Codex, a coding agent based on GPT-5"},
		{" GPT-5.5 ", "You are Codex, a coding agent based on GPT-5"},
		{"gpt-5.2", "You are GPT-5.2 running in the Codex CLI"},
		{"gpt-5.1", "You are GPT-5.1 running in the Codex CLI"},
		{"gpt-5", "You are Codex, a coding agent based on GPT-5"},   // 回退到最新（GPT-5.5）
		{"gpt-5.4", "You are Codex, a coding agent based on GPT-5"}, // 未单独维护 → 最新
		{"gpt-5.3", "You are Codex, a coding agent based on GPT-5"}, // 未单独维护 → 最新
		{"some-unknown-model", "You are Codex, a coding agent based on GPT-5"},
		{"", "You are Codex, a coding agent based on GPT-5"}, // 回退到最新
	}
	for _, c := range cases {
		got := strings.TrimSpace(CodexBaseInstructionsForModel(c.model))
		if got == "" {
			t.Errorf("model %q: got empty instructions", c.model)
			continue
		}
		if !strings.HasPrefix(got, c.wantHead) {
			t.Errorf("model %q: got prefix %q, want %q", c.model, firstLine(got), c.wantHead)
		}
	}
}

// TestCodexBaseInstructionsNewModelsFallback 验证新模板为空时保持兼容回退且最终非空。
// @param t 测试上下文。
// @return 无。
func TestCodexBaseInstructionsNewModelsFallback(t *testing.T) {
	original55, original56, original6 := instructionsGPT55, instructionsGPT56, instructionsGPT6
	t.Cleanup(func() {
		instructionsGPT55, instructionsGPT56, instructionsGPT6 = original55, original56, original6
	})
	instructionsGPT56, instructionsGPT6 = "", " \n\t"
	for _, model := range []string{"gpt-5.6", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-6", "gpt-6-astra"} {
		if got := CodexBaseInstructionsForModel(model); got != original55 {
			t.Errorf("模型 %q 未回退到 GPT-5.5", model)
		}
	}
	instructionsGPT55 = " \n"
	for _, model := range []string{"gpt-5.6-sol", "gpt-6-astra", "unknown-model"} {
		if got := CodexBaseInstructionsForModel(model); got != DefaultInstructions || strings.TrimSpace(got) == "" {
			t.Errorf("模型 %q 未回退到非空通用提示词", model)
		}
	}
}
