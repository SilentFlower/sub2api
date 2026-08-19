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
	require.False(t, changed)
	require.Len(t, reqBody["tools"], 4)
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
			want: "all_turns",
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
