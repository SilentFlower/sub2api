//go:build unit

package openai_compat

import "testing"

func TestJSONSchemaToJSONObjectEnabled(t *testing.T) {
	tests := []struct {
		name  string
		extra map[string]any
		want  bool
	}{
		{"nil extra", nil, false},
		{"key missing", map[string]any{"other": true}, false},
		{"enabled", map[string]any{ExtraKeyJSONSchemaToJSONObject: true}, true},
		{"disabled", map[string]any{ExtraKeyJSONSchemaToJSONObject: false}, false},
		{"string is invalid", map[string]any{ExtraKeyJSONSchemaToJSONObject: "true"}, false},
		{"number is invalid", map[string]any{ExtraKeyJSONSchemaToJSONObject: 1}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := JSONSchemaToJSONObjectEnabled(tc.extra); got != tc.want {
				t.Fatalf("JSONSchemaToJSONObjectEnabled(%v) = %v, want %v", tc.extra, got, tc.want)
			}
		})
	}
}
