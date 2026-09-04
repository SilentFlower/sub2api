//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeOpenAIResponsesLiteTools_MovesNamespacesAndKeepsSupportedTools(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.6-terra",
		"tools": []any{
			map[string]any{"type": "function", "name": "shell"},
			map[string]any{"type": "custom", "name": "exec"},
			map[string]any{"type": "tool_search"},
			map[string]any{"type": "namespace", "name": "collaboration", "tools": []any{
				map[string]any{"type": "function", "name": "spawn_agent"},
			}},
		},
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": "hello"},
			map[string]any{"type": "additional_tools", "role": "developer", "tools": []any{
				map[string]any{"type": "namespace", "name": "image_gen"},
				map[string]any{"type": "namespace", "name": "collaboration", "tools": []any{
					map[string]any{"type": "function", "name": "spawn_agent"},
				}},
			}},
		},
		"tool_choice": map[string]any{"type": "namespace", "name": "collaboration"},
	}

	changed, err := normalizeOpenAIResponsesLiteTools(reqBody)

	require.NoError(t, err)
	require.True(t, changed)
	tools := requireResponsesLiteSlice(t, reqBody["tools"])
	require.Len(t, tools, 3)
	require.Equal(t, "function", requireResponsesLiteMap(t, tools[0])["type"])
	require.Equal(t, "custom", requireResponsesLiteMap(t, tools[1])["type"])
	require.Equal(t, "tool_search", requireResponsesLiteMap(t, tools[2])["type"])
	input := requireResponsesLiteSlice(t, reqBody["input"])
	require.Len(t, input, 2)
	additional := requireResponsesLiteSlice(t, requireResponsesLiteMap(t, input[1])["tools"])
	require.Len(t, additional, 2)
	require.Equal(t, "image_gen", requireResponsesLiteMap(t, additional[0])["name"])
	require.Equal(t, "collaboration", requireResponsesLiteMap(t, additional[1])["name"], "existing namespace must not be duplicated")
	require.Equal(t, map[string]any{"type": "namespace", "name": "collaboration"}, reqBody["tool_choice"])
}

func TestNormalizeOpenAIResponsesLiteTools_PreservesDeferredFlagsWithToolSearch(t *testing.T) {
	reqBody := map[string]any{
		"tools": []any{
			map[string]any{"type": "tool_search"},
			map[string]any{"type": "function", "name": "shell", "defer_loading": true},
		},
	}

	_, err := normalizeOpenAIResponsesLiteTools(reqBody)
	require.NoError(t, err)
	tools := reqBody["tools"].([]any)
	require.Equal(t, "tool_search", tools[0].(map[string]any)["type"])
	require.Equal(t, true, tools[1].(map[string]any)["defer_loading"])
}

func TestNormalizeOpenAIResponsesLiteTools_RejectsConflictingAdditionalTool(t *testing.T) {
	reqBody := map[string]any{
		"tools": []any{map[string]any{
			"type":  "namespace",
			"name":  "collaboration",
			"tools": []any{map[string]any{"type": "function", "name": "spawn_agent"}},
		}},
		"input": []any{map[string]any{
			"type": "additional_tools",
			"tools": []any{map[string]any{
				"type":  "namespace",
				"name":  "collaboration",
				"tools": []any{map[string]any{"type": "function", "name": "send_message"}},
			}},
		}},
	}

	changed, err := normalizeOpenAIResponsesLiteTools(reqBody)

	require.ErrorContains(t, err, `conflicts with migrated tool type "namespace" name "collaboration"`)
	require.False(t, changed)
	require.Len(t, reqBody["tools"], 1, "conflicts must not partially remove top-level tools")
}

func TestNormalizeOpenAIResponsesLiteTools_DeduplicatesAcrossAdditionalToolItems(t *testing.T) {
	namespace := map[string]any{
		"type":  "namespace",
		"name":  "collaboration",
		"tools": []any{map[string]any{"type": "function", "name": "spawn_agent"}},
	}
	reqBody := map[string]any{
		"tools": []any{namespace},
		"input": []any{
			map[string]any{
				"type":  "additional_tools",
				"tools": []any{map[string]any{"type": "custom", "name": "exec"}},
			},
			map[string]any{
				"type":  "additional_tools",
				"tools": []any{namespace},
			},
		},
	}

	changed, err := normalizeOpenAIResponsesLiteTools(reqBody)

	require.NoError(t, err)
	require.True(t, changed)
	require.NotContains(t, reqBody, "tools")
	input := requireResponsesLiteSlice(t, reqBody["input"])
	require.Len(t, requireResponsesLiteMap(t, input[0])["tools"], 1)
	require.Len(t, requireResponsesLiteMap(t, input[1])["tools"], 1)
}

func TestNormalizeOpenAIResponsesLiteTools_ConvertsStringInput(t *testing.T) {
	reqBody := map[string]any{
		"input": "hello",
		"tools": []any{map[string]any{
			"type": "namespace",
			"name": "collaboration",
		}},
	}

	changed, err := normalizeOpenAIResponsesLiteTools(reqBody)

	require.NoError(t, err)
	require.True(t, changed)
	require.NotContains(t, reqBody, "tools")
	input := requireResponsesLiteSlice(t, reqBody["input"])
	require.Len(t, input, 2)
	require.Equal(t, "message", requireResponsesLiteMap(t, input[0])["type"])
	require.Equal(t, "hello", requireResponsesLiteMap(t, input[0])["content"])
	require.Equal(t, "additional_tools", requireResponsesLiteMap(t, input[1])["type"])
}

func TestNormalizeOpenAIResponsesLiteTools_KeepsSupportedTopLevelTools(t *testing.T) {
	reqBody := map[string]any{
		"reasoning": map[string]any{"context": "all_turns"},
		"tools": []any{
			map[string]any{"type": "function", "name": "shell"},
			map[string]any{"type": "custom", "name": "exec"},
			map[string]any{"type": "tool_search"},
			"custom shorthand",
		},
	}

	changed, err := normalizeOpenAIResponsesLiteTools(reqBody)

	require.NoError(t, err)
	require.True(t, changed)
	require.Len(t, reqBody["tools"], 4)
	require.Equal(t, false, reqBody["parallel_tool_calls"])
}

func TestNormalizeOpenAIResponsesLiteTools_ForcesParallelToolCallsFalse(t *testing.T) {
	tests := []struct {
		name string
		body map[string]any
	}{
		{
			name: "top-level tools",
			body: map[string]any{
				"tools":               []any{map[string]any{"type": "function", "name": "shell"}},
				"parallel_tool_calls": true,
			},
		},
		{
			name: "input additional tools",
			body: map[string]any{
				"input": []any{map[string]any{
					"type":  "additional_tools",
					"tools": []any{map[string]any{"type": "namespace", "name": "collaboration"}},
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed, err := normalizeOpenAIResponsesLiteTools(tt.body)

			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, false, tt.body["parallel_tool_calls"])
		})
	}
}

func TestNormalizeOpenAIResponsesLiteTools_PinsParallelToolCallsWithoutTools(t *testing.T) {
	tests := []struct {
		name        string
		parallel    any
		include     bool
		wantChanged bool
	}{
		{name: "字段缺失", wantChanged: true},
		{name: "值为 true", parallel: true, include: true, wantChanged: true},
		{name: "值为 false", parallel: false, include: true, wantChanged: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := map[string]any{"reasoning": map[string]any{"context": "all_turns"}}
			if tt.include {
				reqBody["parallel_tool_calls"] = tt.parallel
			}

			changed, err := normalizeOpenAIResponsesLiteTools(reqBody)

			require.NoError(t, err)
			require.Equal(t, tt.wantChanged, changed)
			require.Contains(t, reqBody, "parallel_tool_calls")
			require.Equal(t, false, reqBody["parallel_tool_calls"])
		})
	}
}

func TestNormalizeOpenAIResponsesLiteTools_RejectsNonBooleanParallelToolCalls(t *testing.T) {
	for _, value := range []any{"false", float64(0), nil, map[string]any{}} {
		reqBody := map[string]any{
			"tools":               []any{map[string]any{"type": "function", "name": "shell"}},
			"parallel_tool_calls": value,
		}

		changed, err := normalizeOpenAIResponsesLiteTools(reqBody)

		require.ErrorContains(t, err, "parallel_tool_calls to be a boolean")
		require.False(t, changed)
		require.Equal(t, value, reqBody["parallel_tool_calls"])
	}

	reqBody := map[string]any{"parallel_tool_calls": []any{}}
	changed, err := normalizeOpenAIResponsesLiteTools(reqBody)
	require.ErrorContains(t, err, "parallel_tool_calls to be a boolean")
	require.False(t, changed)
}

func TestNormalizeOpenAIResponsesLiteTools_ParallelToolCallsIsIdempotent(t *testing.T) {
	reqBody := map[string]any{
		"reasoning":           map[string]any{"context": "all_turns"},
		"tools":               []any{map[string]any{"type": "function", "name": "shell"}},
		"parallel_tool_calls": true,
	}

	changed, err := normalizeOpenAIResponsesLiteTools(reqBody)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, false, reqBody["parallel_tool_calls"])

	changed, err = normalizeOpenAIResponsesLiteTools(reqBody)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, false, reqBody["parallel_tool_calls"])
}

func TestNormalizeOpenAIResponsesLiteTools_EnsuresReasoningContext(t *testing.T) {
	tests := []struct {
		name      string
		reasoning any
	}{
		{name: "missing"},
		{name: "missing context", reasoning: map[string]any{"effort": "high"}},
		{name: "wrong context", reasoning: map[string]any{"effort": "medium", "context": "current_turn"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := map[string]any{"input": "hello"}
			if tt.reasoning != nil {
				reqBody["reasoning"] = tt.reasoning
			}

			changed, err := normalizeOpenAIResponsesLiteTools(reqBody)

			require.NoError(t, err)
			require.True(t, changed)
			reasoning := reqBody["reasoning"].(map[string]any)
			require.Equal(t, "all_turns", reasoning["context"])
			if tt.name != "missing" {
				require.Equal(t, tt.reasoning.(map[string]any)["effort"], reasoning["effort"])
			}
		})
	}
}

func TestNormalizeOpenAIResponsesLiteTools_RejectsNonObjectReasoning(t *testing.T) {
	reqBody := map[string]any{"reasoning": "high"}

	changed, err := normalizeOpenAIResponsesLiteTools(reqBody)

	require.ErrorContains(t, err, "reasoning to be an object")
	require.False(t, changed)
	require.Equal(t, "high", reqBody["reasoning"])
}

func TestNormalizeOpenAIResponsesLiteTools_RejectsUnsupportedTools(t *testing.T) {
	tests := []struct {
		name string
		tool map[string]any
		want string
	}{
		{name: "hosted web search", tool: map[string]any{"type": "web_search"}, want: `top-level tool type "web_search"`},
		{name: "hosted image generation", tool: map[string]any{"type": "image_generation"}, want: `top-level tool type "image_generation"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := map[string]any{"tools": []any{tt.tool}}
			changed, err := normalizeOpenAIResponsesLiteTools(reqBody)
			require.ErrorContains(t, err, tt.want)
			require.False(t, changed)
			require.Len(t, reqBody["tools"], 1, "validation errors must not partially mutate tools")
		})
	}
}

func TestNormalizeOpenAIResponsesLiteToolsPayload_PreservesResponseCreateShape(t *testing.T) {
	body := []byte(`{
		"type":"response.create",
		"model":"gpt-5.6-terra",
		"client_metadata":{"ws_request_header_x_openai_internal_codex_responses_lite":"true"},
		"input":[{"type":"message","role":"user","content":"hello"}],
		"tools":[{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"spawn_agent"}]}],
		"tool_choice":{"type":"namespace","name":"collaboration"}
	}`)

	updated, changed, err := normalizeOpenAIResponsesLiteToolsPayload(body)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "response.create", gjson.GetBytes(updated, "type").String())
	require.False(t, gjson.GetBytes(updated, "tools").Exists())
	require.Equal(t, "collaboration", gjson.GetBytes(updated, `input.#(type=="additional_tools").tools.0.name`).String())
	require.Equal(t, "namespace", gjson.GetBytes(updated, "tool_choice.type").String())
	require.True(t, gjson.GetBytes(updated, "parallel_tool_calls").Exists())
	require.False(t, gjson.GetBytes(updated, "parallel_tool_calls").Bool())
}

func TestNormalizeOpenAIResponsesLitePayloads_PreserveLargeSequence(t *testing.T) {
	body := []byte(`{
		"type":"response.create",
		"sequence":900719925474099312345,
		"tools":[{"type":"function","name":"lookup"}],
		"parallel_tool_calls":true
	}`)
	tests := []struct {
		name      string
		normalize func([]byte) ([]byte, bool, error)
	}{
		{name: "OAuth-like tools normalization", normalize: normalizeOpenAIResponsesLiteToolsPayload},
		{name: "API key parallel normalization", normalize: normalizeOpenAIResponsesLiteParallelToolCallsPayload},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated, changed, err := tt.normalize(body)

			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, "900719925474099312345", gjson.GetBytes(updated, "sequence").Raw)
			require.True(t, gjson.GetBytes(updated, "parallel_tool_calls").Exists())
			require.False(t, gjson.GetBytes(updated, "parallel_tool_calls").Bool())
		})
	}
}

func TestApplyCodexOAuthTransform_PreservesLiteNamespaceToolChoice(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.6-terra",
		"input": []any{map[string]any{
			"type": "additional_tools",
			"tools": []any{map[string]any{
				"type": "namespace",
				"name": "collaboration",
			}},
		}},
		"tool_choice": map[string]any{"type": "namespace", "name": "collaboration"},
	}

	applyCodexOAuthTransform(reqBody, true, false)

	require.Equal(t, map[string]any{"type": "namespace", "name": "collaboration"}, reqBody["tool_choice"])
}

func TestOpenAIGatewayServiceForward_NormalizesResponsesLiteToolsForOAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, passthrough := range []bool{false, true} {
		name := "managed"
		if passthrough {
			name = "passthrough"
		}
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
			c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")
			c.Request.Header.Set(responsesLiteHeader, "true")
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader(
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_lite\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n" +
						"data: [DONE]\n\n",
				)),
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := &Account{
				ID: 501, Name: "responses-lite", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Concurrency: 1, Status: StatusActive, Schedulable: true, RateMultiplier: f64p(1),
				Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"},
				Extra:       map[string]any{"openai_passthrough": passthrough},
			}
			body := []byte(`{
				"model":"gpt-5.6-terra","stream":true,"instructions":"test",
				"reasoning":{"effort":"high","context":"current_turn"},
				"parallel_tool_calls":true,
				"tools":[
					{"type":"function","name":"shell","parameters":{"type":"object"}},
					{"type":"custom","name":"exec"},
					{"type":"tool_search"},
					{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"spawn_agent","parameters":{"type":"object"}}]}
				],
				"input":[{"type":"message","role":"user","content":"hello"}],
				"tool_choice":{"type":"namespace","name":"collaboration"}
			}`)

			result, err := svc.Forward(context.Background(), c, account, body)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "true", upstream.lastReq.Header.Get(responsesLiteHeader))
			require.Equal(t, "high", gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
			require.Equal(t, "all_turns", gjson.GetBytes(upstream.lastBody, "reasoning.context").String())
			require.False(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="namespace")`).Exists())
			require.Equal(t, "shell", gjson.GetBytes(upstream.lastBody, `tools.#(type=="function").name`).String())
			require.Equal(t, "exec", gjson.GetBytes(upstream.lastBody, `tools.#(type=="custom").name`).String())
			require.True(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="tool_search")`).Exists())
			require.Equal(t, "collaboration", gjson.GetBytes(upstream.lastBody, `input.#(type=="additional_tools").tools.0.name`).String())
			require.Equal(t, "namespace", gjson.GetBytes(upstream.lastBody, "tool_choice.type").String())
			require.Equal(t, "collaboration", gjson.GetBytes(upstream.lastBody, "tool_choice.name").String())
			require.True(t, gjson.GetBytes(upstream.lastBody, "parallel_tool_calls").Exists())
			require.False(t, gjson.GetBytes(upstream.lastBody, "parallel_tool_calls").Bool())

			badRec := httptest.NewRecorder()
			badCtx, _ := gin.CreateTestContext(badRec)
			badCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
			badCtx.Request.Header.Set(responsesLiteHeader, "true")
			badUpstream := &httpUpstreamRecorder{}
			svc.httpUpstream = badUpstream

			result, err = svc.Forward(context.Background(), badCtx, account, []byte(`{"model":"gpt-5.6-terra","tools":[{"type":"function","name":"shell"}],"parallel_tool_calls":"false"}`))

			require.ErrorContains(t, err, "parallel_tool_calls to be a boolean")
			require.Nil(t, result)
			require.Equal(t, http.StatusBadRequest, badRec.Code)
			require.Equal(t, "invalid_request_error", gjson.Get(badRec.Body.String(), "error.type").String())
			require.Equal(t, "parallel_tool_calls", gjson.Get(badRec.Body.String(), "error.param").String())
			require.Contains(t, gjson.Get(badRec.Body.String(), "error.message").String(), "parallel_tool_calls to be a boolean")
			require.Nil(t, badUpstream.lastReq)

			for _, malformed := range []struct {
				body      string
				wantParam string
			}{
				{body: `{"model":"gpt-5.6-terra","tools":{}}`, wantParam: "tools"},
				{body: `{"model":"gpt-5.6-terra","reasoning":[]}`, wantParam: "reasoning"},
			} {
				rec := httptest.NewRecorder()
				requestCtx, _ := gin.CreateTestContext(rec)
				requestCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
				requestCtx.Request.Header.Set(responsesLiteHeader, "true")

				result, err = svc.Forward(context.Background(), requestCtx, account, []byte(malformed.body))

				require.Error(t, err)
				require.Nil(t, result)
				require.Equal(t, http.StatusBadRequest, rec.Code)
				require.Equal(t, malformed.wantParam, gjson.Get(rec.Body.String(), "error.param").String())
			}
		})
	}
}

func TestOpenAIGatewayServiceForward_PinsParallelToolCallsForToollessResponsesLite(t *testing.T) {
	gin.SetMode(gin.TestMode)

	accountCases := []struct {
		name        string
		accountType string
		credentials map[string]any
	}{
		{name: "oauth", accountType: AccountTypeOAuth, credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"}},
		{name: "apikey", accountType: AccountTypeAPIKey, credentials: map[string]any{"api_key": "sk-test"}},
	}
	parallelCases := []struct {
		name  string
		field string
	}{
		{name: "字段缺失"},
		{name: "值为 true", field: `,"parallel_tool_calls":true`},
		{name: "值为 false", field: `,"parallel_tool_calls":false`},
	}

	for _, accountCase := range accountCases {
		for _, passthrough := range []bool{false, true} {
			mode := "managed"
			if passthrough {
				mode = "passthrough"
			}
			for _, parallelCase := range parallelCases {
				name := accountCase.name + "/" + mode + "/" + parallelCase.name
				t.Run(name, func(t *testing.T) {
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
					c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")
					c.Request.Header.Set(responsesLiteHeader, "true")
					upstream := &httpUpstreamRecorder{resp: &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
						Body: io.NopCloser(strings.NewReader(
							"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_lite\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n" +
								"data: [DONE]\n\n",
						)),
					}}
					svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
					account := &Account{
						ID: 502, Name: "responses-lite-no-tools", Platform: PlatformOpenAI, Type: accountCase.accountType,
						Concurrency: 1, Status: StatusActive, Schedulable: true, RateMultiplier: f64p(1),
						Credentials: accountCase.credentials,
						Extra:       map[string]any{"openai_passthrough": passthrough},
					}
					body := []byte(`{
						"model":"gpt-5.6-terra","stream":true,"instructions":"test",
						"reasoning":{"effort":"high","context":"current_turn"},
						"input":[{"type":"message","role":"user","content":"hello"}]` + parallelCase.field + `
					}`)

					result, err := svc.Forward(context.Background(), c, account, body)

					require.NoError(t, err)
					require.NotNil(t, result)
					require.Equal(t, "true", upstream.lastReq.Header.Get(responsesLiteHeader))
					require.Equal(t, gjson.False, gjson.GetBytes(upstream.lastBody, "parallel_tool_calls").Type, string(upstream.lastBody))
				})
			}
		}
	}
}

func TestOpenAIGatewayServiceForward_DisablesParallelToolCallsForResponsesLiteAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, passthrough := range []bool{false, true} {
		name := "managed"
		if passthrough {
			name = "passthrough"
		}
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
			c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")
			c.Request.Header.Set(responsesLiteHeader, "true")
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader(
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_lite\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n" +
						"data: [DONE]\n\n",
				)),
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := &Account{
				ID: 503, Name: "responses-lite-api-key", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Concurrency: 1, Status: StatusActive, Schedulable: true, RateMultiplier: f64p(1),
				Credentials: map[string]any{"api_key": "sk-test"},
				Extra:       map[string]any{"openai_passthrough": passthrough},
			}
			body := []byte(`{
				"model":"gpt-5.6-terra","stream":true,"instructions":"test",
				"tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}],
				"parallel_tool_calls":true,
				"input":[{"type":"message","role":"user","content":"hello"}]
			}`)

			result, err := svc.Forward(context.Background(), c, account, body)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "true", upstream.lastReq.Header.Get(responsesLiteHeader))
			require.True(t, gjson.GetBytes(upstream.lastBody, "tools").IsArray())
			require.True(t, gjson.GetBytes(upstream.lastBody, "parallel_tool_calls").Exists())
			require.False(t, gjson.GetBytes(upstream.lastBody, "parallel_tool_calls").Bool())
		})
	}
}

func TestOpenAIGatewayServiceForward_HeaderOverrideCannotCreateResponsesLiteRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1,"output_tokens":1}}`)),
	}}
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	account := &Account{
		ID: 504, Name: "responses-lite-header-override", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Concurrency: 1, Status: StatusActive, Schedulable: true, RateMultiplier: f64p(1),
		Credentials: map[string]any{
			"api_key":                    "sk-test",
			"base_url":                   "https://example.com",
			credKeyHeaderOverrideEnabled: true,
			credKeyHeaderOverrides: map[string]any{
				responsesLiteHeader: "true",
				"x-custom":          "override-applied",
			},
		},
		Extra: map[string]any{"use_responses_api": true},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
	body := []byte(`{
		"model":"gpt-5.6-terra","stream":false,
		"reasoning":{"context":"current_turn"},
		"input":"hello"
	}`)

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Empty(t, upstream.lastReq.Header.Get(responsesLiteHeader))
	require.Equal(t, "override-applied", getHeaderRaw(upstream.lastReq.Header, "x-custom"))
	require.Equal(t, "current_turn", gjson.GetBytes(upstream.lastBody, "reasoning.context").String())
}

func TestOpenAIGatewayServiceForward_AppliesResponsesLitePolicyToFinalModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name              string
		requestedModel    string
		mappedModel       string
		settingValue      string
		wantHeader        string
		wantContext       string
		wantUpstreamModel string
	}{
		{
			name:              "default blocked exact model",
			requestedModel:    "gpt-5.5",
			wantContext:       "current_turn",
			wantUpstreamModel: "gpt-5.5",
		},
		{
			name:              "mapped model is blocked",
			requestedModel:    "client-alias",
			mappedModel:       "gpt-5.5",
			wantContext:       "current_turn",
			wantUpstreamModel: "gpt-5.5",
		},
		{
			name:              "explicit empty list allows model",
			requestedModel:    "gpt-5.5",
			settingValue:      "[]",
			wantHeader:        "true",
			wantContext:       "all_turns",
			wantUpstreamModel: "gpt-5.5",
		},
		{
			name:              "custom wildcard blocks model",
			requestedModel:    "gpt-5.6-terra",
			settingValue:      `["gpt-5.6*"]`,
			wantContext:       "current_turn",
			wantUpstreamModel: "gpt-5.6-terra",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, passthrough := range []bool{false, true} {
				mode := "managed"
				if passthrough {
					mode = "passthrough"
				}
				t.Run(mode, func(t *testing.T) {
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(nil))
					c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")
					c.Request.Header.Set(responsesLiteHeader, "true")
					upstream := &httpUpstreamRecorder{resp: &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
						Body: io.NopCloser(strings.NewReader(
							"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_lite_policy\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n" +
								"data: [DONE]\n\n",
						)),
					}}
					settingValues := map[string]string{}
					if tt.settingValue != "" {
						settingValues[SettingKeyOpenAIResponsesLiteHeaderBlockedModels] = tt.settingValue
					}
					svc := &OpenAIGatewayService{
						cfg:            &config.Config{},
						httpUpstream:   upstream,
						settingService: NewSettingService(&settingValuesRepoStub{values: settingValues}, &config.Config{}),
					}
					credentials := map[string]any{
						"access_token":       "oauth-token",
						"chatgpt_account_id": "chatgpt-account",
					}
					if tt.mappedModel != "" {
						credentials["model_mapping"] = map[string]any{tt.requestedModel: tt.mappedModel}
					}
					account := &Account{
						ID: 502, Name: "responses-lite-policy", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
						Concurrency: 1, Status: StatusActive, Schedulable: true, RateMultiplier: f64p(1),
						Credentials: credentials,
						Extra:       map[string]any{"openai_passthrough": passthrough},
					}
					body := []byte(`{
						"model":"` + tt.requestedModel + `","stream":true,"instructions":"test",
						"reasoning":{"effort":"high","context":"current_turn"},
						"input":[
							{"type":"message","role":"user","content":"hello"},
							{"type":"additional_tools","role":"developer","tools":[{"type":"namespace","name":"collaboration"}]}
						]
					}`)

					result, err := svc.Forward(context.Background(), c, account, body)

					require.NoError(t, err)
					require.NotNil(t, result)
					require.Equal(t, tt.wantHeader, upstream.lastReq.Header.Get(responsesLiteHeader))
					require.Equal(t, tt.wantContext, gjson.GetBytes(upstream.lastBody, "reasoning.context").String())
					require.Equal(t, tt.wantUpstreamModel, gjson.GetBytes(upstream.lastBody, "model").String())
					require.Equal(t, "collaboration", gjson.GetBytes(upstream.lastBody, `input.#(type=="additional_tools").tools.0.name`).String())
				})
			}
		})
	}
}

func TestApplyOpenAIResponsesLiteHTTPIngressPolicy_UsesPassthroughFinalModel(t *testing.T) {
	settingService := NewSettingService(&settingValuesRepoStub{values: map[string]string{
		SettingKeyOpenAIResponsesLiteHeaderBlockedModels: `["gpt-5.5"]`,
	}}, &config.Config{})
	svc := &OpenAIGatewayService{settingService: settingService}
	tests := []struct {
		name    string
		account *Account
		compact bool
		want    string
	}{
		{
			name: "API Key 透传按保持不变的入站模型放行 Lite",
			account: &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"client-alias": "gpt-5.5"},
				},
				Extra: map[string]any{"openai_passthrough": true},
			},
			want: "current_turn",
		},
		{
			name: "OAuth 透传普通请求按账号映射阻止 Lite",
			account: &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"client-alias": "gpt-5.5"},
				},
				Extra: map[string]any{"openai_passthrough": true},
			},
			want: "current_turn",
		},
		{
			name: "透传 compact 按原始模型的 compact 映射放行 Lite",
			account: &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Credentials: map[string]any{
					"model_mapping":         map[string]any{"client-alias": "gpt-5.5"},
					"compact_model_mapping": map[string]any{"client-alias": "gpt-5.6-sol-openai-compact"},
				},
				Extra: map[string]any{"openai_passthrough": true},
			},
			compact: true,
			want:    "all_turns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(`{
				"model":"client-alias",
				"reasoning":{"context":"current_turn"},
				"input":"hello"
			}`)

			updated, err := svc.applyOpenAIResponsesLiteHTTPIngressPolicy(
				context.Background(), tt.account, body, "true", tt.compact,
			)

			require.NoError(t, err)
			require.Equal(t, tt.want, gjson.GetBytes(updated, "reasoning.context").String())
		})
	}
}

func requireResponsesLiteSlice(t *testing.T, value any) []any {
	t.Helper()
	result, ok := value.([]any)
	require.True(t, ok)
	return result
}

func requireResponsesLiteMap(t *testing.T, value any) map[string]any {
	t.Helper()
	result, ok := value.(map[string]any)
	require.True(t, ok)
	return result
}
