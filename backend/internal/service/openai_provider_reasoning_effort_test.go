//go:build unit

package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeGLMOpenAIReasoningEffort(t *testing.T) {
	tests := []struct {
		name          string
		model         string
		input         string
		wantApplied   bool
		wantPath      string
		wantValue     string
		wantUnchanged bool
	}{
		{name: "flat xhigh maps to max", model: "glm-5.2", input: `{"model":"glm-5.2","reasoning_effort":"xhigh","messages":[]}`, wantApplied: true, wantPath: "reasoning_effort", wantValue: "max"},
		{name: "flat x-high maps to max", model: "GLM-5.2", input: `{"model":"glm-5.2","reasoning_effort":"x-high","messages":[]}`, wantApplied: true, wantPath: "reasoning_effort", wantValue: "max"},
		{name: "flat ultracode maps to max", model: "glm-5.2", input: `{"model":"glm-5.2","reasoning_effort":"ultracode","messages":[]}`, wantApplied: true, wantPath: "reasoning_effort", wantValue: "max"},
		{name: "flat medium maps to high", model: "glm-5.2", input: `{"model":"glm-5.2","reasoning_effort":"medium","messages":[]}`, wantApplied: true, wantPath: "reasoning_effort", wantValue: "high"},
		{name: "5.2 的 low 仍映射为 high", model: "glm-5.2", input: `{"reasoning_effort":"low"}`, wantApplied: true, wantPath: "reasoning_effort", wantValue: "high"},
		{name: "5.3 保留原生 low", model: "glm-5.3", input: `{"reasoning_effort":"low"}`, wantUnchanged: true},
		{name: "5.3 归一化 low 大小写", model: " GLM-5.3 ", input: `{"reasoning":{"effort":" LOW "}}`, wantApplied: true, wantPath: "reasoning.effort", wantValue: "low"},
		{name: "5.3 的 medium 映射为 high", model: "glm-5.3", input: `{"reasoning_effort":"medium"}`, wantApplied: true, wantPath: "reasoning_effort", wantValue: "high"},
		{name: "5.3 嵌套空值仍优先于平铺值", model: "glm-5.3", input: `{"reasoning":{"effort":""},"reasoning_effort":"xhigh"}`, wantUnchanged: true},
		{name: "nested high case-normalizes", model: "glm-5.2", input: `{"model":"glm-5.2","reasoning":{"effort":"HIGH"},"messages":[]}`, wantApplied: true, wantPath: "reasoning.effort", wantValue: "high"},
		{name: "flat none case-normalizes", model: "glm-5.2", input: `{"model":"glm-5.2","reasoning_effort":"NONE","messages":[]}`, wantApplied: true, wantPath: "reasoning_effort", wantValue: "none"},
		{name: "nested minimal trims and case-normalizes", model: "glm-5.2", input: `{"model":"glm-5.2","reasoning":{"effort":" Minimal "},"messages":[]}`, wantApplied: true, wantPath: "reasoning.effort", wantValue: "minimal"},
		{name: "native max unchanged", model: "glm-5.2", input: `{"model":"glm-5.2","reasoning_effort":"max","messages":[]}`, wantUnchanged: true},
		{name: "non glm unchanged", model: "deepseek-v4-pro", input: `{"model":"deepseek-v4-pro","reasoning_effort":"xhigh","messages":[]}`, wantUnchanged: true},
		{name: "missing effort unchanged", model: "glm-5.2", input: `{"model":"glm-5.2","messages":[]}`, wantUnchanged: true},
		{name: "unknown effort unchanged", model: "glm-5.2", input: `{"model":"glm-5.2","reasoning_effort":"banana","messages":[]}`, wantUnchanged: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, normalize := range []func([]byte, string) ([]byte, bool){NormalizeGLMOpenAIReasoningEffort, normalizeOpenAIReasoningEffortForProvider} {
				got, applied := normalize([]byte(tt.input), tt.model)
				require.Equal(t, tt.wantApplied, applied)
				if tt.wantUnchanged {
					require.Equal(t, tt.input, string(got))
					continue
				}
				require.Equal(t, tt.wantValue, gjson.GetBytes(got, tt.wantPath).String())
			}
		})
	}
}

// TestNormalizeGLM53AnthropicThinking 验证显式偏好优先级与其它模型的隔离。
// @param t 测试上下文。
// @return 无。
func TestNormalizeGLM53AnthropicThinking(t *testing.T) {
	for _, tt := range []struct {
		name  string
		model string
		body  string
		want  string
	}{
		{name: "关闭思考映射为 low", model: "glm-5.3", body: `{"thinking":{"type":"disabled"}}`, want: "low"},
		{name: "自适应思考映射为 high", model: " GLM-5.3 ", body: `{"thinking":{"type":"adaptive"}}`, want: "high"},
		{name: "effort 优先于 thinking", model: "glm-5.3", body: `{"output_config":{"effort":" X-HIGH "},"thinking":{"type":"disabled"}}`, want: "max"},
		{name: "空 effort 回退到 thinking", model: "glm-5.3", body: `{"output_config":{"effort":" "},"thinking":{"type":"enabled"}}`, want: "high"},
		{name: "未知显式 effort 保留请求", model: "glm-5.3", body: `{"output_config":{"effort":"custom"},"thinking":{"type":"enabled"}}`},
		{name: "缺少偏好使用上游默认", model: "glm-5.3", body: `{"messages":[]}`},
		{name: "其它 GLM 版本不受影响", model: "glm-5.2", body: `{"thinking":{"type":"disabled"}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, applied := NormalizeGLM53AnthropicThinking([]byte(tt.body), tt.model)
			require.Equal(t, tt.want != "", applied)
			if tt.want == "" {
				require.Equal(t, tt.body, string(got))
				return
			}
			require.Equal(t, tt.want, gjson.GetBytes(got, "output_config.effort").String())
			require.Equal(t, "enabled", gjson.GetBytes(got, "thinking.type").String())
		})
	}
}

func TestNormalizeOpenAIReasoningEffortForProvider_Grok45(t *testing.T) {
	tests := []struct {
		input       string
		want        string
		wantApplied bool
	}{
		{input: "none", want: "low", wantApplied: true},
		{input: "minimal", want: "low", wantApplied: true},
		{input: "low", want: "low", wantApplied: false},
		{input: "medium", want: "medium", wantApplied: false},
		{input: "high", want: "high", wantApplied: false},
		{input: "xhigh", want: "high", wantApplied: true},
		{input: "extra high", want: "high", wantApplied: true},
		{input: "max", want: "high", wantApplied: true},
		{input: "ultracode", want: "high", wantApplied: true},
		{input: " X-HIGH ", want: "high", wantApplied: true},
	}

	for _, tt := range tests {
		t.Run(strings.TrimSpace(tt.input), func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":"grok-4.5","reasoning":{"effort":%q}}`, tt.input))
			got, applied := normalizeOpenAIReasoningEffortForProvider(body, " Grok-4.5 ")
			require.Equal(t, tt.wantApplied, applied)
			require.Equal(t, tt.want, gjson.GetBytes(got, "reasoning.effort").String())
		})
	}
}

func TestNormalizeOpenAIReasoningEffortForProvider_PreservesUnknownAndOtherGrokModels(t *testing.T) {
	tests := []struct {
		name  string
		model string
		body  string
	}{
		{name: "unknown grok 4.5 effort", model: "grok-4.5", body: `{"reasoning":{"effort":"banana"}}`},
		{name: "grok multi agent keeps xhigh", model: "grok-4.20-multi-agent", body: `{"reasoning":{"effort":"xhigh"}}`},
		{name: "grok 4.3 keeps xhigh", model: "grok-4.3", body: `{"reasoning":{"effort":"xhigh"}}`},
		{name: "missing effort", model: "grok-4.5", body: `{"input":"hello"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, applied := normalizeOpenAIReasoningEffortForProvider([]byte(tt.body), tt.model)
			require.False(t, applied)
			require.Equal(t, tt.body, string(got))
		})
	}
}

func TestNormalizeOpenAIReasoningEffortForProvider_PrefersNestedField(t *testing.T) {
	body := []byte(`{"reasoning":{"effort":"xhigh"},"reasoning_effort":"none"}`)

	got, applied := normalizeOpenAIReasoningEffortForProvider(body, "grok-4.5")

	require.True(t, applied)
	require.Equal(t, "high", gjson.GetBytes(got, "reasoning.effort").String())
	require.Equal(t, "none", gjson.GetBytes(got, "reasoning_effort").String())
}

func TestExtractFinalOpenAIReasoningEffort(t *testing.T) {
	tests := []struct {
		name string
		body string
		want *string
	}{
		{name: "nested none", body: `{"reasoning":{"effort":"none"}}`, want: strPtr("none")},
		{name: "flat minimal", body: `{"reasoning_effort":"minimal"}`, want: strPtr("minimal")},
		{name: "unknown value", body: `{"reasoning":{"effort":"banana"}}`, want: strPtr("banana")},
		{name: "trim only", body: `{"reasoning":{"effort":"  X-Custom  "}}`, want: strPtr("X-Custom")},
		{name: "nested wins", body: `{"reasoning":{"effort":"high"},"reasoning_effort":"max"}`, want: strPtr("high")},
		{name: "missing", body: `{"model":"grok-4.5"}`, want: nil},
		{name: "empty", body: `{"reasoning":{"effort":"  "}}`, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFinalOpenAIReasoningEffort([]byte(tt.body))
			if tt.want == nil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, *tt.want, *got)
		})
	}
}

func TestExtractOpenAIUpstreamReasoningEffort_ProviderScaleUsesFinalBody(t *testing.T) {
	body := []byte(`{"thinking":{"type":"enabled"}}`)

	require.Nil(t, extractOpenAIUpstreamReasoningEffort(body, "glm-5.2", "glm-5.2"))
	require.Nil(t, extractOpenAIUpstreamReasoningEffort(body, "grok", "grok-4.5"))
	kimiEffort := extractOpenAIUpstreamReasoningEffort(body, "kimi-k2.6", "kimi-k2.6")
	require.NotNil(t, kimiEffort)
	require.Equal(t, "high", *kimiEffort)
}

func TestExtractOpenAIUpstreamReasoningEffort_UsesMappedBillingOriginalOrder(t *testing.T) {
	bodyWithoutEffort := []byte(`{"model":"alias","input":"hello"}`)
	bodyWithMax := []byte(`{"model":"alias","reasoning":{"effort":"max"},"input":"hello"}`)

	fromBillingSuffix := extractOpenAIUpstreamReasoningEffort(bodyWithoutEffort, "alias", "gpt-5.6-sol", "gpt-5.6-sol-max")
	require.NotNil(t, fromBillingSuffix)
	require.Equal(t, "max", *fromBillingSuffix)

	fromMappedModel := extractOpenAIUpstreamReasoningEffort(bodyWithMax, "gpt-5.6-sol", "gpt-5.4")
	require.NotNil(t, fromMappedModel)
	require.Equal(t, "xhigh", *fromMappedModel)
}
