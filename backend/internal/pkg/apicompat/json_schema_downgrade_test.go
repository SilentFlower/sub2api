//go:build unit

package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDowngradeResponsesJSONSchemaToJSONObject(t *testing.T) {
	body := []byte(`{"model":"deepseek-v4-pro","instructions":"Keep this.","text":{"verbosity":"low","format":{"type":"json_schema","name":"answer","strict":true,"schema":{"type":"object","properties":{"answer":{"type":"string"}}}}},"tools":[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{"query":{"type":"string"}}}}]}`)

	out, changed, err := DowngradeResponsesJSONSchemaToJSONObject(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "json_object", gjson.GetBytes(out, "text.format.type").String())
	require.Equal(t, "low", gjson.GetBytes(out, "text.verbosity").String())
	require.Contains(t, gjson.GetBytes(out, "instructions").String(), "Keep this.")
	require.Contains(t, gjson.GetBytes(out, "instructions").String(), `"answer":{"type":"string"}`)
	require.Contains(t, gjson.GetBytes(out, "instructions").String(), "best-effort")
	require.Equal(t, gjson.GetBytes(body, "tools.0.parameters").Raw, gjson.GetBytes(out, "tools.0.parameters").Raw)

	outAgain, changedAgain, err := DowngradeResponsesJSONSchemaToJSONObject(out)
	require.NoError(t, err)
	require.False(t, changedAgain)
	require.Equal(t, out, outAgain)
}

func TestDowngradeResponsesJSONSchemaToJSONObjectLeavesUnsupportedShapesUnchanged(t *testing.T) {
	tests := []string{
		`{"text":{"format":{"type":"json_object"}}}`,
		`{"text":{"format":{"type":"text"}}}`,
		`{"text":{"format":{"type":"json_schema"}}}`,
		`{"text":{"format":{"type":"json_schema","schema":[]}}}`,
		`{"instructions":[],"text":{"format":{"type":"json_schema","schema":{}}}}`,
	}
	for _, input := range tests {
		out, changed, err := DowngradeResponsesJSONSchemaToJSONObject([]byte(input))
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, input, string(out))
	}
}

func TestDowngradeChatJSONSchemaToJSONObject(t *testing.T) {
	body := []byte(`{"model":"deepseek-v4-pro","messages":[{"role":"system","content":"first"},{"role":"developer","content":"second"},{"role":"user","content":"question"}],"response_format":{"type":"json_schema","json_schema":{"name":"answer","strict":true,"schema":{"type":"object","required":["answer"]}}},"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","required":["query"]}}}]}`)

	out, changed, err := DowngradeChatJSONSchemaToJSONObject(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "json_object", gjson.GetBytes(out, "response_format.type").String())
	require.Equal(t, "system", gjson.GetBytes(out, "messages.0.role").String())
	require.Equal(t, "developer", gjson.GetBytes(out, "messages.1.role").String())
	require.Equal(t, "system", gjson.GetBytes(out, "messages.2.role").String())
	require.Contains(t, gjson.GetBytes(out, "messages.2.content").String(), `"required":["answer"]`)
	require.Equal(t, "user", gjson.GetBytes(out, "messages.3.role").String())
	require.Equal(t, gjson.GetBytes(body, "tools.0.function.parameters").Raw, gjson.GetBytes(out, "tools.0.function.parameters").Raw)
}

func TestDowngradeChatJSONSchemaToJSONObjectLeavesUnsupportedShapesUnchanged(t *testing.T) {
	tests := []string{
		`{"messages":[],"response_format":{"type":"json_object"}}`,
		`{"messages":[],"response_format":{"type":"json_schema","json_schema":{}}}`,
		`{"messages":{},"response_format":{"type":"json_schema","json_schema":{"schema":{}}}}`,
		`{"messages":[],"response_format":{"type":"json_schema","json_schema":{"schema":"object"}}}`,
	}
	for _, input := range tests {
		out, changed, err := DowngradeChatJSONSchemaToJSONObject([]byte(input))
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, input, string(out))
	}
}

func TestJSONSchemaDowngradeRejectsInvalidJSON(t *testing.T) {
	for _, downgrade := range []func([]byte) ([]byte, bool, error){
		DowngradeResponsesJSONSchemaToJSONObject,
		DowngradeChatJSONSchemaToJSONObject,
	} {
		out, changed, err := downgrade([]byte(`{"broken"`))
		require.Error(t, err)
		require.False(t, changed)
		require.True(t, strings.HasPrefix(string(out), `{"broken"`))
	}
}

func TestJSONSchemaDowngradeProducesValidJSON(t *testing.T) {
	out, changed, err := DowngradeResponsesJSONSchemaToJSONObject([]byte(`{"text":{"format":{"type":"json_schema","schema":{}}}}`))
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, json.Valid(out))
}
