//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/websearch"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForwardResponses_ForceChatCompletionsRoutesNonStreamingToChatCompletions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_resp_chat_json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_json","object":"chat.completion","model":"gpt-5.4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5,"prompt_tokens_details":{"cached_tokens":1}}}`,
		)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "http://upstream.example/v1/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(upstream.lastReq.Context()))
	require.Equal(t, "hello", gjson.GetBytes(upstream.lastBody, "messages.0.content").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
	require.Equal(t, "response", gjson.Get(rec.Body.String(), "object").String())
	require.Equal(t, "ok", gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
	require.Equal(t, 3, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Equal(t, 1, result.Usage.CacheReadInputTokens)
	require.False(t, result.Stream)
}

func TestForwardResponses_ForceChatCompletionsNormalizesGLMReasoningEffort(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"glm-5.2","input":"hello","reasoning":{"effort":"extra high"},"stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_resp_glm_effort"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_glm","object":"chat.completion","model":"glm-5.2","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`,
		)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "max", gjson.GetBytes(upstream.lastBody, "reasoning_effort").String())
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "max", *result.ReasoningEffort)
}

func TestForwardResponses_ForceChatCompletionsRoutesStreamingToChatCompletions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"content":"he"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"content":"llo"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		"",
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[],"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_resp_chat_stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "http://upstream.example/v1/chat/completions", upstream.lastReq.URL.String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream_options.include_usage").Bool())
	require.Contains(t, rec.Body.String(), "event: response.output_text.delta")
	require.Contains(t, rec.Body.String(), `"delta":"he"`)
	require.Contains(t, rec.Body.String(), "event: response.completed")
	require.Contains(t, rec.Body.String(), `"input_tokens":4`)
	require.Contains(t, rec.Body.String(), "data: [DONE]")
	require.Equal(t, 4, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.True(t, result.Stream)
	require.NotNil(t, result.FirstTokenMs)
}

func TestForwardResponses_DeepSeekReasoningOnlyStreamProducesVisibleText(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"deepseek-reasoner","input":"hello","stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_reasoning","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[{"index":0,"delta":{"role":"assistant","content":null,"reasoning_content":""},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_reasoning","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[{"index":0,"delta":{"reasoning_content":"visible fallback"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_reasoning","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[{"index":0,"delta":{"content":""},"finish_reason":"length"}],"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_deepseek_reasoning_responses_stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Contains(t, rec.Body.String(), "event: response.output_text.delta")
	require.Contains(t, rec.Body.String(), `"delta":"visible fallback"`)
	require.Contains(t, rec.Body.String(), `"status":"incomplete"`)
	require.Contains(t, rec.Body.String(), "data: [DONE]")
}

func TestForwardResponses_AutoSupportedAccountStillUsesResponsesEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_resp_native"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"resp_native","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}],"status":"completed"}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}`,
		)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}
	account := rawChatCompletionsTestAccount()
	account.Extra = map[string]any{
		openai_compat.ExtraKeyResponsesMode:      string(openai_compat.ResponsesSupportModeAuto),
		openai_compat.ExtraKeyResponsesSupported: true,
	}

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "http://upstream.example/v1/responses", upstream.lastReq.URL.String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "messages").Exists())
	require.Equal(t, "ok", gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
}

func TestForwardResponses_WebRunSearchQueryExecutesAndContinuesModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)

	body := openAIResponsesWebRunTestBody(false)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAIResponsesWebRunTestResponse("rid_web_run_1", `{
			"id":"chatcmpl_web_run_1","object":"chat.completion","model":"deepseek-v4-pro",
			"choices":[{"index":0,"message":{"role":"assistant","content":"","reasoning_content":"需要搜索","tool_calls":[{"id":"call_web_1","type":"function","function":{"name":"web__run","arguments":"{\"search_query\":[{\"q\":\"杭州天气\",\"recency\":1}],\"response_length\":\"short\"}"}}]},"finish_reason":"tool_calls"}],
			"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"prompt_tokens_details":{"cached_tokens":2}}
		}`),
		openAIResponsesWebRunTestResponse("rid_web_run_2", `{
			"id":"chatcmpl_web_run_2","object":"chat.completion","model":"deepseek-v4-pro",
			"choices":[{"index":0,"message":{"role":"assistant","content":"杭州天气结果已找到"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":20,"completion_tokens":5,"total_tokens":25,"prompt_tokens_details":{"cached_tokens":3}}
		}`),
	}}
	searchCalls := 0
	svc := &OpenAIGatewayService{
		cfg:            rawChatCompletionsTestConfig(),
		httpUpstream:   upstream,
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(_ context.Context, _ *Account, query string, maxResults int) (*websearch.SearchResponse, string, error) {
			searchCalls++
			require.Equal(t, "杭州天气", query)
			require.Equal(t, 3, maxResults)
			return &websearch.SearchResponse{Query: query, Results: []websearch.SearchResult{{
				Title: "杭州天气", URL: "https://weather.example/hangzhou", Snippet: "晴，30℃",
			}}}, "anysearch", nil
		},
	}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, searchCalls)
	require.Equal(t, 1, result.WebSearchCalls)
	require.Equal(t, 30, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Equal(t, 5, result.Usage.CacheReadInputTokens)
	require.Len(t, upstream.bodies, 2)
	require.False(t, gjson.GetBytes(upstream.bodies[0], "stream").Bool())
	require.False(t, gjson.GetBytes(upstream.bodies[0], "parallel_tool_calls").Bool())
	require.True(t, gjson.GetBytes(upstream.bodies[0], "tools.0.function.parameters.properties.search_query").Exists())
	require.False(t, gjson.GetBytes(upstream.bodies[0], "tools.0.function.parameters.properties.weather").Exists())
	require.Equal(t, "assistant", gjson.GetBytes(upstream.bodies[1], "messages.1.role").String())
	require.Equal(t, "call_web_1", gjson.GetBytes(upstream.bodies[1], "messages.1.tool_calls.0.id").String())
	require.Equal(t, "tool", gjson.GetBytes(upstream.bodies[1], "messages.2.role").String())
	require.Equal(t, "call_web_1", gjson.GetBytes(upstream.bodies[1], "messages.2.tool_call_id").String())
	toolOutput := gjson.GetBytes(upstream.bodies[1], "messages.2.content").String()
	require.Contains(t, toolOutput, "https://weather.example/hangzhou")
	require.Contains(t, toolOutput, "recency_not_enforced")
	require.Equal(t, "杭州天气结果已找到", gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
	require.Equal(t, int64(30), gjson.Get(rec.Body.String(), "usage.input_tokens").Int())
}

func TestForwardResponses_WebRunWeatherRetriesAsSearchQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)

	body := openAIResponsesWebRunTestBody(false)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAIResponsesWebRunTestResponse("rid_weather_1", `{"id":"chatcmpl_weather_1","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_weather","type":"function","function":{"name":"web__run","arguments":"{\"weather\":[{\"location\":\"杭州\"}]}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`),
		openAIResponsesWebRunTestResponse("rid_weather_2", `{"id":"chatcmpl_weather_2","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_search","type":"function","function":{"name":"web__run","arguments":"{\"search_query\":[{\"q\":\"杭州天气\"}]}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":7,"completion_tokens":1,"total_tokens":8}}`),
		openAIResponsesWebRunTestResponse("rid_weather_3", `{"id":"chatcmpl_weather_3","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"天气搜索完成"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":2,"total_tokens":11}}`),
	}}
	searchCalls := 0
	svc := &OpenAIGatewayService{
		cfg:            rawChatCompletionsTestConfig(),
		httpUpstream:   upstream,
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(_ context.Context, _ *Account, query string, _ int) (*websearch.SearchResponse, string, error) {
			searchCalls++
			return &websearch.SearchResponse{Query: query, Results: []websearch.SearchResult{{Title: "天气", URL: "https://example.com/weather"}}}, "anysearch", nil
		},
	}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, searchCalls)
	require.Equal(t, 1, result.WebSearchCalls)
	require.Len(t, upstream.bodies, 3)
	require.Contains(t, gjson.GetBytes(upstream.bodies[1], "messages.2.content").String(), "unsupported_command")
	require.Equal(t, "call_search", gjson.GetBytes(upstream.bodies[2], "messages.3.tool_calls.0.id").String())
	require.Contains(t, gjson.GetBytes(upstream.bodies[2], "messages.4.content").String(), "https://example.com/weather")
	require.Equal(t, "天气搜索完成", gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
}

func TestForwardResponses_WebRunStreamingBuffersInternalRounds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)

	body := openAIResponsesWebRunTestBody(true)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAIResponsesWebRunTestResponse("rid_stream_1", `{"id":"chatcmpl_stream_1","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_stream","type":"function","function":{"name":"web__run","arguments":"{\"search_query\":[{\"q\":\"杭州天气\"}]}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`),
		openAIResponsesWebRunTestResponse("rid_stream_2", `{"id":"chatcmpl_stream_2","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"流式天气结果"},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":2,"total_tokens":10}}`),
	}}
	svc := &OpenAIGatewayService{
		cfg:            rawChatCompletionsTestConfig(),
		httpUpstream:   upstream,
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(_ context.Context, _ *Account, query string, _ int) (*websearch.SearchResponse, string, error) {
			return &websearch.SearchResponse{Query: query, Results: []websearch.SearchResult{{Title: "天气", URL: "https://example.com/weather"}}}, "anysearch", nil
		},
	}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Equal(t, 1, result.WebSearchCalls)
	require.Len(t, upstream.bodies, 2)
	require.False(t, gjson.GetBytes(upstream.bodies[0], "stream").Bool())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "stream").Bool())
	wire := rec.Body.String()
	require.Contains(t, wire, "event: response.created")
	require.Contains(t, wire, `"delta":"流式天气结果"`)
	require.Contains(t, wire, "event: response.completed")
	require.Contains(t, wire, `"input_tokens":13`)
	require.Contains(t, wire, "data: [DONE]")
	require.NotContains(t, wire, `"namespace":"web"`)
}

func TestForwardResponses_WebRunDisabledRemainsClientToolCall(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := openAIResponsesWebRunTestBody(false)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: openAIResponsesWebRunTestResponse("rid_disabled", `{"id":"chatcmpl_disabled","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_disabled","type":"function","function":{"name":"web__run","arguments":"{\"search_query\":[{\"q\":\"杭州天气\"}]}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`)}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeDisabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 0, result.WebSearchCalls)
	require.Len(t, upstream.bodies, 1)
	require.True(t, gjson.GetBytes(upstream.bodies[0], "tools.0.function.parameters.properties.weather").Exists())
	require.Equal(t, "function_call", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Equal(t, "web", gjson.Get(rec.Body.String(), "output.0.namespace").String())
	require.Equal(t, "run", gjson.Get(rec.Body.String(), "output.0.name").String())
}

func TestForwardResponses_WebRunEnabledPreservesOtherClientToolStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"deepseek-v4-pro","stream":true,
		"input":[
			{"role":"user","content":"delegate"},
			{"type":"additional_tools","tools":[
				{"type":"namespace","name":"web","tools":[{"type":"function","name":"run","parameters":{"type":"object","properties":{"search_query":{"type":"array"},"weather":{"type":"array"}}}}]},
				{"type":"namespace","name":"collaboration","tools":[{"type":"function","name":"spawn_agent","parameters":{"type":"object"}}]}
			]}
		]
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: openAIResponsesWebRunTestResponse("rid_other_tool", `{
		"id":"chatcmpl_other_tool","object":"chat.completion","model":"deepseek-v4-pro",
		"choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_spawn","type":"function","function":{"name":"collaboration__spawn_agent","arguments":"{\"prompt\":\"inspect\"}"}}]},"finish_reason":"tool_calls"}],
		"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}
	}`)}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Equal(t, 0, result.WebSearchCalls)
	require.Len(t, upstream.bodies, 1)
	wire := rec.Body.String()
	require.Contains(t, wire, `"namespace":"collaboration"`)
	require.Contains(t, wire, `"name":"spawn_agent"`)
	require.Contains(t, wire, `"call_id":"call_spawn"`)
	require.NotContains(t, wire, `"namespace":"web"`)
}

func TestForwardResponses_WebRunStopsAfterTwoToolRounds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)
	body := openAIResponsesWebRunTestBody(false)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	toolResponse := func(id, query string) *http.Response {
		return openAIResponsesWebRunTestResponse(id, `{"id":"`+id+`","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_`+id+`","type":"function","function":{"name":"web__run","arguments":"{\"search_query\":[{\"q\":\"`+query+`\"}]}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`)
	}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		toolResponse("round_1", "query one"),
		toolResponse("round_2", "query two"),
		toolResponse("round_3", "query three"),
	}}
	providerCalls := 0
	svc := &OpenAIGatewayService{
		cfg:            rawChatCompletionsTestConfig(),
		httpUpstream:   upstream,
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(_ context.Context, _ *Account, query string, _ int) (*websearch.SearchResponse, string, error) {
			providerCalls++
			return &websearch.SearchResponse{Query: query}, "anysearch", nil
		},
	}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "maximum of 2")
	require.Equal(t, 2, providerCalls)
	require.Len(t, upstream.bodies, 3)
	require.Equal(t, http.StatusBadGateway, rec.Code)
}

func TestForwardResponses_WebRunProviderFailureContinuesWithoutBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)
	body := openAIResponsesWebRunTestBody(false)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAIResponsesWebRunTestResponse("rid_provider_failure_1", `{"id":"chatcmpl_provider_failure_1","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_provider_failure","type":"function","function":{"name":"web__run","arguments":"{\"search_query\":[{\"q\":\"杭州天气\"}]}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`),
		openAIResponsesWebRunTestResponse("rid_provider_failure_2", `{"id":"chatcmpl_provider_failure_2","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"搜索服务暂时不可用"},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":2,"total_tokens":10}}`),
	}}
	searchCalls := 0
	svc := &OpenAIGatewayService{
		cfg:            rawChatCompletionsTestConfig(),
		httpUpstream:   upstream,
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(context.Context, *Account, string, int) (*websearch.SearchResponse, string, error) {
			searchCalls++
			return nil, "", errors.New("provider unavailable")
		},
	}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, searchCalls)
	require.Zero(t, result.WebSearchCalls)
	require.Len(t, upstream.bodies, 2)
	toolOutput := gjson.GetBytes(upstream.bodies[1], "messages.2.content").String()
	require.Contains(t, toolOutput, "web_search_failed")
	require.Equal(t, "call_provider_failure", gjson.GetBytes(upstream.bodies[1], "messages.2.tool_call_id").String())
	require.Equal(t, "搜索服务暂时不可用", gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
}

func TestForwardResponses_WebRunProxyUnavailableTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)
	body := openAIResponsesWebRunTestBody(false)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: openAIResponsesWebRunTestResponse("rid_proxy_unavailable", `{"id":"chatcmpl_proxy_unavailable","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_proxy_unavailable","type":"function","function":{"name":"web__run","arguments":"{\"search_query\":[{\"q\":\"杭州天气\"}]}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`)}
	svc := &OpenAIGatewayService{
		cfg:            rawChatCompletionsTestConfig(),
		httpUpstream:   upstream,
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(context.Context, *Account, string, int) (*websearch.SearchResponse, string, error) {
			return nil, "", websearch.ErrProxyUnavailable
		},
	}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Len(t, upstream.bodies, 1)
	require.Empty(t, rec.Body.String())
}

func TestForwardResponses_WebRunGeneratesMissingCallID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)
	body := openAIResponsesWebRunTestBody(false)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	arguments := `{"search_query":[{"q":"杭州天气"}]}`
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAIResponsesWebRunTestResponse("rid_missing_call_id_1", `{"id":"chatcmpl_missing_call_id_1","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"type":"function","function":{"name":"web__run","arguments":"{\"search_query\":[{\"q\":\"杭州天气\"}]}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`),
		openAIResponsesWebRunTestResponse("rid_missing_call_id_2", `{"id":"chatcmpl_missing_call_id_2","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"搜索完成"},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":2,"total_tokens":10}}`),
	}}
	svc := &OpenAIGatewayService{
		cfg:            rawChatCompletionsTestConfig(),
		httpUpstream:   upstream,
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(_ context.Context, _ *Account, query string, _ int) (*websearch.SearchResponse, string, error) {
			return &websearch.SearchResponse{Query: query}, "anysearch", nil
		},
	}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.WebSearchCalls)
	require.Len(t, upstream.bodies, 2)
	expectedCallID := deterministicOpenAIResponsesWebRunCallID(1, "web__run", arguments)
	require.Equal(t, expectedCallID, gjson.GetBytes(upstream.bodies[1], "messages.1.tool_calls.0.id").String())
	require.Equal(t, expectedCallID, gjson.GetBytes(upstream.bodies[1], "messages.2.tool_call_id").String())
}

func TestForwardResponses_WebRunEnforcesFourQueryLimitAcrossRounds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)
	body := openAIResponsesWebRunTestBody(false)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAIResponsesWebRunTestResponse("rid_query_limit_1", `{"id":"chatcmpl_query_limit_1","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_query_limit_1","type":"function","function":{"name":"web__run","arguments":"{\"search_query\":[{\"q\":\"one\"},{\"q\":\"two\"},{\"q\":\"three\"}]}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`),
		openAIResponsesWebRunTestResponse("rid_query_limit_2", `{"id":"chatcmpl_query_limit_2","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_query_limit_2","type":"function","function":{"name":"web__run","arguments":"{\"search_query\":[{\"q\":\"four\"},{\"q\":\"five\"}]}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":7,"completion_tokens":1,"total_tokens":8}}`),
		openAIResponsesWebRunTestResponse("rid_query_limit_3", `{"id":"chatcmpl_query_limit_3","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"已使用现有搜索结果"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":2,"total_tokens":11}}`),
	}}
	providerCalls := 0
	svc := &OpenAIGatewayService{
		cfg:            rawChatCompletionsTestConfig(),
		httpUpstream:   upstream,
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(_ context.Context, _ *Account, query string, _ int) (*websearch.SearchResponse, string, error) {
			providerCalls++
			return &websearch.SearchResponse{Query: query}, "anysearch", nil
		},
	}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 3, providerCalls)
	require.Equal(t, 3, result.WebSearchCalls)
	require.Len(t, upstream.bodies, 3)
	toolOutput := gjson.GetBytes(upstream.bodies[2], "messages.4.content").String()
	require.Contains(t, toolOutput, "search_limit_exceeded")
	require.Equal(t, "call_query_limit_2", gjson.GetBytes(upstream.bodies[2], "messages.4.tool_call_id").String())
}

func TestParseOpenAIResponsesWebRunArgumentsRejectsUnsupportedAndOverLimit(t *testing.T) {
	parsed, toolErr := parseOpenAIResponsesWebRunArguments(`{"search_query":[{"q":"one"}],"weather":[{"location":"杭州"}]}`, 4)
	require.Nil(t, parsed)
	require.NotNil(t, toolErr)
	require.Equal(t, "unsupported_command", toolErr.Code)

	parsed, toolErr = parseOpenAIResponsesWebRunArguments(`{"search_query":[{"q":"one"},{"q":"two"}]}`, 1)
	require.Nil(t, parsed)
	require.NotNil(t, toolErr)
	require.Equal(t, "search_limit_exceeded", toolErr.Code)
}

func TestFindOpenAIResponsesWebRunToolSupportsTopLevelAndAdditionalTools(t *testing.T) {
	declaration := `{"type":"namespace","name":"web","tools":[{"type":"function","name":"run","parameters":{"type":"object","properties":{"weather":{"type":"array"}}}}]}`
	tests := map[string]string{
		"top-level":        `{"model":"m","input":"q","tools":[` + declaration + `]}`,
		"additional_tools": `{"model":"m","input":[{"role":"user","content":"q"},{"type":"additional_tools","tools":[` + declaration + `]}]}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			var request apicompat.ResponsesRequest
			require.NoError(t, json.Unmarshal([]byte(body), &request))
			effectiveTools, err := apicompat.EffectiveResponsesTools(&request)
			require.NoError(t, err)
			chatRequest, err := apicompat.ResponsesToChatCompletionsRequest(&request)
			require.NoError(t, err)

			toolName, found := findOpenAIResponsesWebRunTool(chatRequest, apicompat.NamespaceToolNames(effectiveTools))
			require.True(t, found)
			require.Equal(t, "web__run", toolName)
			narrowOpenAIResponsesWebRunTool(chatRequest, toolName)
			require.True(t, gjson.GetBytes(chatRequest.Tools[0].Function.Parameters, "properties.search_query").Exists())
			require.False(t, gjson.GetBytes(chatRequest.Tools[0].Function.Parameters, "properties.weather").Exists())
		})
	}
}

func openAIResponsesWebRunTestBody(stream bool) []byte {
	streamValue := "false"
	if stream {
		streamValue = "true"
	}
	return []byte(`{
		"model":"deepseek-v4-pro",
		"stream":` + streamValue + `,
		"input":[
			{"role":"user","content":[{"type":"input_text","text":"搜索杭州天气"}]},
			{"type":"additional_tools","tools":[{"type":"namespace","name":"web","tools":[{"type":"function","name":"run","description":"Browse the web","parameters":{"type":"object","properties":{"search_query":{"type":"array"},"weather":{"type":"array"},"open":{"type":"array"}}}}]}]}
		]
	}`)
}

func openAIResponsesWebRunTestResponse(requestID, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{requestID}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func forceChatResponsesFallbackAccount() *Account {
	account := rawChatCompletionsTestAccount()
	account.Extra = map[string]any{
		openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
	}
	return account
}
