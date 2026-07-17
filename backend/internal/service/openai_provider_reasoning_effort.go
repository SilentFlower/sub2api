package service

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// NormalizeGLMOpenAIReasoningEffort 将 OpenAI reasoning effort 改写为 GLM 原生档位。
// 仅对最终映射为 glm-* 的模型生效，未知值和其它模型保持不变。
//
// @param body 待发送的 OpenAI 请求体。
// @param mappedModel 最终映射后的上游模型。
// @return 改写后的请求体，以及是否发生了改写。
func NormalizeGLMOpenAIReasoningEffort(body []byte, mappedModel string) ([]byte, bool) {
	if !isGLMOpenAIReasoningEffortModel(mappedModel) {
		return body, false
	}
	return normalizeOpenAIReasoningEffortBody(body, normalizeGLMOpenAIReasoningEffort)
}

func normalizeOpenAIReasoningEffortForProvider(body []byte, mappedModel string) ([]byte, bool) {
	switch {
	case isGLMOpenAIReasoningEffortModel(mappedModel):
		return normalizeOpenAIReasoningEffortBody(body, normalizeGLMOpenAIReasoningEffort)
	case isGrok45OpenAIReasoningEffortModel(mappedModel):
		return normalizeOpenAIReasoningEffortBody(body, normalizeGrok45OpenAIReasoningEffort)
	default:
		return body, false
	}
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
// provider-specific 模型必须记录实际发送值；其它模型按上游、计费、原始模型
// 的顺序恢复被模型映射剥离的 effort 后缀。
func extractOpenAIUpstreamReasoningEffort(body []byte, requestedModel string, mappedModel string, additionalModelCandidates ...string) *string {
	if isGLMOpenAIReasoningEffortModel(mappedModel) || isGrok45OpenAIReasoningEffortModel(mappedModel) {
		return extractFinalOpenAIReasoningEffort(body)
	}
	modelCandidates := make([]string, 0, len(additionalModelCandidates)+2)
	modelCandidates = append(modelCandidates, mappedModel)
	modelCandidates = append(modelCandidates, additionalModelCandidates...)
	modelCandidates = append(modelCandidates, requestedModel)
	effort := extractOpenAIReasoningEffortFromBody(body, modelCandidates...)
	return ApplyThinkingEnabledFallback(effort, body, mappedModel)
}

func isGLMOpenAIReasoningEffortModel(mappedModel string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(mappedModel)), "glm-")
}

func isGrok45OpenAIReasoningEffortModel(mappedModel string) bool {
	return strings.EqualFold(strings.TrimSpace(mappedModel), "grok-4.5")
}

func normalizeGLMOpenAIReasoningEffort(raw string) string {
	value := compactOpenAIReasoningEffort(raw)

	switch value {
	case "none":
		return "none"
	case "minimal":
		return "minimal"
	case "low", "medium", "high":
		return "high"
	case "xhigh", "extrahigh", "max", "ultracode":
		return "max"
	default:
		return ""
	}
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
