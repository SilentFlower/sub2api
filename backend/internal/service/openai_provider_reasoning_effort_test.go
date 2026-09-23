//go:build unit

package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

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

	// GLM 与上游一致：仅开启 thinking 时按默认档位记录，便于按档位计费。
	glmEffort := extractOpenAIUpstreamReasoningEffort(body, "glm-5.2", "glm-5.2")
	require.NotNil(t, glmEffort)
	require.Equal(t, "high", *glmEffort)
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
