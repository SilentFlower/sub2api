package service

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// normalizeOpenAIReasoningEffortForProvider 按最终上游模型归一 reasoning effort。
// GLM 直接复用上游 NormalizeGLMOpenAIReasoningEffort；Grok 4.5 使用 build 的原生档位映射。
//
// @param body 待发送的 OpenAI 请求体。
// @param mappedModel 最终映射后的上游模型。
// @return 改写后的请求体，以及是否发生了改写。
func normalizeOpenAIReasoningEffortForProvider(body []byte, mappedModel string) ([]byte, bool) {
	if normalized, changed := NormalizeGLMOpenAIReasoningEffort(body, mappedModel); changed {
		return normalized, true
	}
	return normalizeGrok45OpenAIReasoningEffortBody(body, mappedModel)
}

// normalizeGrok45OpenAIReasoningEffortBody 仅对 grok-4.5 把 effort 映射到其原生 low/medium/high 档位。
//
// @param body 待发送的 OpenAI 请求体。
// @param mappedModel 最终映射后的上游模型。
// @return 改写后的请求体，以及是否发生了改写；其它模型原样返回。
func normalizeGrok45OpenAIReasoningEffortBody(body []byte, mappedModel string) ([]byte, bool) {
	if !isGrok45OpenAIReasoningEffortModel(mappedModel) {
		return body, false
	}
	return normalizeOpenAIReasoningEffortBody(body, normalizeGrok45OpenAIReasoningEffort)
}

func normalizeOpenAIReasoningEffortBody(body []byte, mapper func(string) string) ([]byte, bool) {
	path, raw, exists := findOpenAIReasoningEffortField(body)
	if !exists || strings.TrimSpace(raw) == "" {
		return body, false
	}

	mapped := mapper(raw)
	if mapped == "" || mapped == raw {
		return body, false
	}

	modified, err := sjson.SetBytes(body, path, mapped)
	if err != nil {
		return body, false
	}
	return modified, true
}

func findOpenAIReasoningEffortField(body []byte) (path string, raw string, exists bool) {
	nested := gjson.GetBytes(body, "reasoning.effort")
	if nested.Exists() {
		return "reasoning.effort", nested.String(), true
	}

	flat := gjson.GetBytes(body, "reasoning_effort")
	if flat.Exists() {
		return "reasoning_effort", flat.String(), true
	}

	return "", "", false
}

func extractFinalOpenAIReasoningEffort(body []byte) *string {
	_, raw, exists := findOpenAIReasoningEffortField(body)
	if !exists {
		return nil
	}
	effort := strings.TrimSpace(raw)
	if effort == "" {
		return nil
	}
	return &effort
}

// extractOpenAIUpstreamReasoningEffort 从最终上游请求体提取 usage effort。
// Grok 4.5 必须记录实际发送值；其它模型（含 GLM）与上游一致，按上游、计费、原始模型
// 的顺序恢复被模型映射剥离的 effort 后缀，并对仅开启 thinking 的请求补默认档位。
func extractOpenAIUpstreamReasoningEffort(body []byte, requestedModel string, mappedModel string, additionalModelCandidates ...string) *string {
	if isGrok45OpenAIReasoningEffortModel(mappedModel) {
		return extractFinalOpenAIReasoningEffort(body)
	}
	modelCandidates := make([]string, 0, len(additionalModelCandidates)+2)
	modelCandidates = append(modelCandidates, mappedModel)
	modelCandidates = append(modelCandidates, additionalModelCandidates...)
	modelCandidates = append(modelCandidates, requestedModel)
	effort := extractOpenAIReasoningEffortFromBody(body, modelCandidates...)
	return ApplyThinkingEnabledFallback(effort, body, mappedModel)
}

func isGrok45OpenAIReasoningEffortModel(mappedModel string) bool {
	return strings.EqualFold(strings.TrimSpace(mappedModel), "grok-4.5")
}

func normalizeGrok45OpenAIReasoningEffort(raw string) string {
	value := compactOpenAIReasoningEffort(raw)

	switch value {
	case "none", "minimal", "low":
		return "low"
	case "medium":
		return "medium"
	case "high", "xhigh", "extrahigh", "max", "ultracode":
		return "high"
	default:
		return ""
	}
}

func compactOpenAIReasoningEffort(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	return strings.NewReplacer("-", "", "_", "", " ", "").Replace(value)
}
