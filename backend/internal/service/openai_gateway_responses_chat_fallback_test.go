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

type openAIResponsesWebRunFailingWriter struct {
	gin.ResponseWriter
}

func (w *openAIResponsesWebRunFailingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func (w *openAIResponsesWebRunFailingWriter) WriteString(string) (int, error) {
	return 0, errors.New("write failed")
}

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

func TestForwardResponses_ForceChatCompletionsDeepSeekMissingReasoningAutoDowngrade(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{
		"model":"deepseek-v4-pro",
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"weather"}]},
			{"type":"function_call","call_id":"call_1","name":"get_weather","arguments":"{}"},
			{"type":"function_call_output","call_id":"call_1","output":"cloudy"}
		],
		"reasoning":{"effort":"high"},
		"stream":false
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: openAIResponsesWebRunTestResponse(
		"rid_resp_deepseek_missing_reasoning",
		`{"id":"chatcmpl_json","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
	)}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "assistant", gjson.GetBytes(upstream.lastBody, "messages.1.role").String())
	require.Equal(t, "call_1", gjson.GetBytes(upstream.lastBody, "messages.1.tool_calls.0.id").String())
	require.Equal(t, "disabled", gjson.GetBytes(upstream.lastBody, "thinking.type").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "reasoning_effort").Exists())
	require.Nil(t, result.ReasoningEffort)
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
	require.Equal(t, "web_search_call", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Equal(t, deterministicOpenAIResponsesWebRunSearchID(1, 0, "杭州天气"), gjson.Get(rec.Body.String(), "output.0.id").String())
	require.Equal(t, "completed", gjson.Get(rec.Body.String(), "output.0.status").String())
	require.Equal(t, "search", gjson.Get(rec.Body.String(), "output.0.action.type").String())
	require.Equal(t, "杭州天气", gjson.Get(rec.Body.String(), "output.0.action.query").String())
	require.Equal(t, "message", gjson.Get(rec.Body.String(), "output.1.type").String())
	require.Equal(t, "杭州天气结果已找到", gjson.Get(rec.Body.String(), "output.1.content.0.text").String())
	require.Equal(t, int64(30), gjson.Get(rec.Body.String(), "usage.input_tokens").Int())
}

func TestForwardResponses_WebRunDeepSeekMissingReasoningDowngradesContinuation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)

	body := bytes.Replace(
		openAIResponsesWebRunTestBody(false),
		[]byte(`"stream":false,`),
		[]byte(`"stream":false,"reasoning":{"effort":"high"},`),
		1,
	)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAIResponsesWebRunTestResponse("rid_web_run_missing_1", `{
			"id":"chatcmpl_web_run_missing_1","object":"chat.completion","model":"deepseek-v4-pro",
			"choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_web_missing","type":"function","function":{"name":"web__run","arguments":"{\"search_query\":[{\"q\":\"杭州天气\"}]}"}}]},"finish_reason":"tool_calls"}],
			"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}
		}`),
		openAIResponsesWebRunTestResponse("rid_web_run_missing_2", `{
			"id":"chatcmpl_web_run_missing_2","object":"chat.completion","model":"deepseek-v4-pro",
			"choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":20,"completion_tokens":5,"total_tokens":25}
		}`),
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
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "high", gjson.GetBytes(upstream.bodies[0], "reasoning_effort").String())
	require.False(t, gjson.GetBytes(upstream.bodies[0], "thinking").Exists())
	require.Equal(t, "disabled", gjson.GetBytes(upstream.bodies[1], "thinking.type").String())
	require.False(t, gjson.GetBytes(upstream.bodies[1], "reasoning_effort").Exists())
	require.Nil(t, result.ReasoningEffort)
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
	require.Equal(t, "web_search_call", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Equal(t, "杭州天气", gjson.Get(rec.Body.String(), "output.0.action.query").String())
	require.Equal(t, "天气搜索完成", gjson.Get(rec.Body.String(), "output.1.content.0.text").String())
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
	require.Contains(t, wire, `"type":"web_search_call"`)
	require.Contains(t, wire, `"action":{"type":"search","query":"杭州天气"}`)
	require.Contains(t, wire, `"delta":"流式天气结果"`)
	require.Contains(t, wire, "event: response.completed")
	require.Contains(t, wire, `"input_tokens":13`)
	require.Contains(t, wire, "data: [DONE]")
	require.NotContains(t, wire, `"namespace":"web"`)
	require.Less(t, strings.Index(wire, `"type":"web_search_call"`), strings.Index(wire, `"delta":"流式天气结果"`))
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

func TestForwardResponses_WebRunCompletesAfterSearchRoundLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)
	body := openAIResponsesWebRunTestBody(true)
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
		toolResponse("round_4", "query four"),
		toolResponse("round_5", "query five"),
		toolResponse("round_6", "query six"),
		openAIResponsesWebRunTestResponse("round_final", `{"id":"round_final","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"已根据现有搜索结果完成回答"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15}}`),
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
	require.True(t, result.Stream)
	require.Equal(t, 5, result.WebSearchCalls)
	require.Equal(t, 5, providerCalls)
	require.Len(t, upstream.bodies, 7)
	finalRequest := upstream.bodies[6]
	require.False(t, gjson.GetBytes(finalRequest, "tools").Exists())
	require.False(t, gjson.GetBytes(finalRequest, "tool_choice").Exists())
	require.Contains(t, gjson.GetBytes(finalRequest, "messages.12.content").String(), "search_limit_reached")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "已根据现有搜索结果完成回答")
	require.NotContains(t, rec.Body.String(), "response.failed")
	require.NotContains(t, rec.Body.String(), "Upstream request failed")
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
	require.Equal(t, "web_search_call", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Equal(t, "failed", gjson.Get(rec.Body.String(), "output.0.status").String())
	require.Equal(t, "杭州天气", gjson.Get(rec.Body.String(), "output.0.action.query").String())
	require.Equal(t, "搜索服务暂时不可用", gjson.Get(rec.Body.String(), "output.1.content.0.text").String())
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

func TestForwardResponses_WebRunEnforcesFiveQueryLimitAcrossRounds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)
	body := openAIResponsesWebRunTestBody(false)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAIResponsesWebRunTestResponse("rid_query_limit_1", `{"id":"chatcmpl_query_limit_1","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_query_limit_1","type":"function","function":{"name":"web__run","arguments":"{\"search_query\":[{\"q\":\"one\"},{\"q\":\"two\"},{\"q\":\"three\"}]}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`),
		openAIResponsesWebRunTestResponse("rid_query_limit_2", `{"id":"chatcmpl_query_limit_2","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_query_limit_2","type":"function","function":{"name":"web__run","arguments":"{\"search_query\":[{\"q\":\"four\"},{\"q\":\"five\"},{\"q\":\"six\"}]}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":7,"completion_tokens":1,"total_tokens":8}}`),
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
	require.Contains(t, toolOutput, "at most 5 web search queries")
	require.Equal(t, "call_query_limit_2", gjson.GetBytes(upstream.bodies[2], "messages.4.tool_call_id").String())
	require.Equal(t, "one", gjson.Get(rec.Body.String(), "output.0.action.query").String())
	require.Equal(t, "two", gjson.Get(rec.Body.String(), "output.1.action.query").String())
	require.Equal(t, "three", gjson.Get(rec.Body.String(), "output.2.action.query").String())
	require.Equal(t, "message", gjson.Get(rec.Body.String(), "output.3.type").String())
	require.Equal(t, "已使用现有搜索结果", gjson.Get(rec.Body.String(), "output.3.content.0.text").String())
}

func TestForwardResponses_TypedWebSearchMixedAutoExecutesAndAppendsCitations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)

	body := openAIResponsesTypedWebSearchTestBody(false, "")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.147.0")
	c.Request.Header.Set(responsesLiteHeader, "true")
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAIResponsesWebRunTestResponse("rid_typed_1", `{"id":"chatcmpl_typed_1","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_typed","type":"function","function":{"name":"sub2api_web_search","arguments":"{\"search_query\":[{\"q\":\"杭州天气\"}]}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`),
		openAIResponsesWebRunTestResponse("rid_typed_2", `{"id":"chatcmpl_typed_2","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"已找到杭州天气"},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":5,"total_tokens":25}}`),
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
			return &websearch.SearchResponse{Query: query, Results: []websearch.SearchResult{
				{Title: "杭州天气", URL: "https://Docs.Example.com/weather#today", Snippet: "晴"},
				{Title: "重复来源", URL: "https://docs.example.com/weather#details", Snippet: "晴"},
				{Title: "被过滤", URL: "https://blocked.test/weather", Snippet: "未知"},
			}}, "anysearch", nil
		},
	}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyCodexWebSearchBridge] = true
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, searchCalls)
	require.Equal(t, 1, result.WebSearchCalls)
	require.Equal(t, 30, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, 1, strings.Count(string(upstream.bodies[0]), `"name":"sub2api_web_search"`))
	require.Equal(t, "auto", gjson.GetBytes(upstream.bodies[0], "tool_choice").String())
	require.False(t, gjson.GetBytes(upstream.bodies[0], "parallel_tool_calls").Bool())
	require.Equal(t, "call_typed", gjson.GetBytes(upstream.bodies[1], "messages.1.tool_calls.0.id").String())
	require.Equal(t, "call_typed", gjson.GetBytes(upstream.bodies[1], "messages.2.tool_call_id").String())
	require.NotContains(t, rec.Body.String(), openAIResponsesTypedWebSearchToolName)

	var response apicompat.ResponsesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Len(t, response.Output, 2)
	require.Equal(t, "web_search_call", response.Output[0].Type)
	require.Equal(t, "杭州天气", response.Output[0].Action.Query)
	part := response.Output[1].Content[0]
	require.Contains(t, part.Text, "已找到杭州天气\n\nSources:")
	require.Equal(t, 1, strings.Count(part.Text, "https://docs.example.com/weather"))
	require.NotContains(t, part.Text, "blocked.test")
	require.Len(t, part.Annotations, 1)
	annotation := part.Annotations[0]
	require.Equal(t, "https://docs.example.com/weather", annotation.URL)
	require.Equal(t, annotation.URL, string([]rune(part.Text)[annotation.StartIndex:annotation.EndIndex]))
}

func TestForwardResponses_CodexLiteWebSearchBridgeDoesNotSearchUnlessSelected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)

	body := openAIResponsesCodexLiteBridgeTestBody(false, `"tool_choice":"auto",`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.147.0")
	c.Request.Header.Set(responsesLiteHeader, "true")
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	upstream := &httpUpstreamRecorder{resp: openAIResponsesWebRunTestResponse("rid_lite_bridge_no_search", `{"id":"chatcmpl_lite_bridge_no_search","object":"chat.completion","model":"gpt-5.6-sol","choices":[{"index":0,"message":{"role":"assistant","content":"无需搜索"},"finish_reason":"stop"}],"usage":{"prompt_tokens":6,"completion_tokens":2,"total_tokens":8}}`)}
	searchCalls := 0
	svc := &OpenAIGatewayService{
		cfg:            rawChatCompletionsTestConfig(),
		httpUpstream:   upstream,
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(context.Context, *Account, string, int) (*websearch.SearchResponse, string, error) {
			searchCalls++
			return nil, "", errors.New("unexpected search")
		},
	}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyCodexWebSearchBridge] = true
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 0, searchCalls)
	require.Equal(t, 0, result.WebSearchCalls)
	require.Len(t, upstream.bodies, 1)
	require.Equal(t, 1, strings.Count(string(upstream.bodies[0]), `"name":"sub2api_web_search"`))
	require.False(t, gjson.GetBytes(upstream.bodies[0], "parallel_tool_calls").Bool())
	require.Equal(t, "无需搜索", gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
	require.NotContains(t, rec.Body.String(), "web_search_call")
}

func TestForwardResponses_CodexLiteWebSearchBridgeExecutesExistingLoop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)

	body := openAIResponsesCodexLiteBridgeTestBody(false, `"tool_choice":"auto",`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.147.0")
	c.Request.Header.Set(responsesLiteHeader, "true")
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAIResponsesWebRunTestResponse("rid_lite_bridge_1", `{"id":"chatcmpl_lite_bridge_1","object":"chat.completion","model":"gpt-5.6-sol","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_lite_search","type":"function","function":{"name":"sub2api_web_search","arguments":"{\"search_query\":[{\"q\":\"最新消息\"}]}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`),
		openAIResponsesWebRunTestResponse("rid_lite_bridge_2", `{"id":"chatcmpl_lite_bridge_2","object":"chat.completion","model":"gpt-5.6-sol","choices":[{"index":0,"message":{"role":"assistant","content":"搜索完成"},"finish_reason":"stop"}],"usage":{"prompt_tokens":15,"completion_tokens":3,"total_tokens":18}}`),
	}}
	searchCalls := 0
	svc := &OpenAIGatewayService{
		cfg:            rawChatCompletionsTestConfig(),
		httpUpstream:   upstream,
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(_ context.Context, _ *Account, query string, maxResults int) (*websearch.SearchResponse, string, error) {
			searchCalls++
			require.Equal(t, "最新消息", query)
			require.Equal(t, webSearchDefaultMaxResults, maxResults)
			return &websearch.SearchResponse{Query: query, Results: []websearch.SearchResult{
				{Title: "最新消息", URL: "https://example.com/latest", Snippet: "内容"},
			}}, "anysearch", nil
		},
	}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyCodexWebSearchBridge] = true
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, searchCalls)
	require.Equal(t, 1, result.WebSearchCalls)
	require.Equal(t, 25, result.Usage.InputTokens)
	require.Equal(t, 5, result.Usage.OutputTokens)
	require.Len(t, upstream.bodies, 2)
	require.Equal(t, "call_lite_search", gjson.GetBytes(upstream.bodies[1], "messages.1.tool_calls.0.id").String())
	require.Equal(t, "call_lite_search", gjson.GetBytes(upstream.bodies[1], "messages.2.tool_call_id").String())
	require.Equal(t, "web_search_call", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Equal(t, "最新消息", gjson.Get(rec.Body.String(), "output.0.action.query").String())
	require.Contains(t, gjson.Get(rec.Body.String(), "output.1.content.0.text").String(), "https://example.com/latest")
	require.NotContains(t, rec.Body.String(), openAIResponsesTypedWebSearchToolName)
}

func TestForwardResponses_CodexLiteWebSearchBridgeStreamingEmitsCitationLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)

	body := openAIResponsesCodexLiteBridgeTestBody(true, `"tool_choice":"auto",`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.147.0")
	c.Request.Header.Set(responsesLiteHeader, "true")
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAIResponsesWebRunTestResponse("rid_lite_bridge_stream_1", `{"id":"chatcmpl_lite_bridge_stream_1","object":"chat.completion","model":"gpt-5.6-sol","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_lite_stream","type":"function","function":{"name":"sub2api_web_search","arguments":"{\"search_query\":[{\"q\":\"流式最新消息\"}]}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":8,"completion_tokens":1,"total_tokens":9}}`),
		openAIResponsesWebRunTestResponse("rid_lite_bridge_stream_2", `{"id":"chatcmpl_lite_bridge_stream_2","object":"chat.completion","model":"gpt-5.6-sol","choices":[{"index":0,"message":{"role":"assistant","content":"流式搜索完成"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15}}`),
	}}
	svc := &OpenAIGatewayService{
		cfg:            rawChatCompletionsTestConfig(),
		httpUpstream:   upstream,
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(_ context.Context, _ *Account, query string, _ int) (*websearch.SearchResponse, string, error) {
			return &websearch.SearchResponse{Query: query, Results: []websearch.SearchResult{{Title: "流式来源", URL: "https://example.com/stream"}}}, "anysearch", nil
		},
	}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyCodexWebSearchBridge] = true
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Equal(t, 1, result.WebSearchCalls)
	wire := rec.Body.String()
	require.Contains(t, wire, `"type":"web_search_call"`)
	require.Contains(t, wire, `"delta":"\n\nSources:\n- 流式来源: https://example.com/stream\n"`)
	require.Contains(t, wire, "event: response.output_text.annotation.added")
	require.Contains(t, wire, `"annotations":[{"type":"url_citation","url":"https://example.com/stream"`)
	require.NotContains(t, wire, openAIResponsesTypedWebSearchToolName)
}

func TestForwardResponses_CodexLiteWebSearchBridgeRejectsReservedNameConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)

	body := openAIResponsesCodexLiteBridgeTestBody(false, `"tool_choice":"auto",`)
	body = bytes.Replace(body, []byte(`"name":"wait"`), []byte(`"name":"sub2api_web_search"`), 1)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.147.0")
	c.Request.Header.Set(responsesLiteHeader, "true")
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), settingService: &SettingService{}}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyCodexWebSearchBridge] = true
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "tools", gjson.Get(rec.Body.String(), "error.param").String())
	require.Contains(t, rec.Body.String(), "conflicts with a declared client tool")
}

func TestForwardResponses_CodexLiteWebSearchBridgeSkipsUnavailableProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setGlobalWebSearchConfig(&WebSearchEmulationConfig{
		Enabled:   true,
		Providers: []WebSearchProviderConfig{{Type: websearch.ProviderTypeAnySearch}},
	})
	SetWebSearchManager(websearch.NewManager(nil, nil))
	t.Cleanup(func() {
		SetWebSearchManager(nil)
		clearGlobalWebSearchConfig()
	})

	body := openAIResponsesCodexLiteBridgeTestBody(false, `"tool_choice":"auto",`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.147.0")
	c.Request.Header.Set(responsesLiteHeader, "true")
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	upstream := &httpUpstreamRecorder{resp: openAIResponsesWebRunTestResponse("rid_lite_bridge_unavailable", `{"id":"chatcmpl_lite_bridge_unavailable","object":"chat.completion","model":"gpt-5.6-sol","choices":[{"index":0,"message":{"role":"assistant","content":"普通回答"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}}`)}
	svc := &OpenAIGatewayService{
		cfg:            rawChatCompletionsTestConfig(),
		httpUpstream:   upstream,
		settingService: &SettingService{},
	}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyCodexWebSearchBridge] = true
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 0, result.WebSearchCalls)
	require.NotContains(t, string(upstream.lastBody), openAIResponsesTypedWebSearchToolName)
	require.Equal(t, "普通回答", gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
}

func TestForwardResponses_TypedWebSearchMixedAutoPreservesOtherClientTool(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := openAIResponsesTypedWebSearchTestBody(false, "")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: openAIResponsesWebRunTestResponse("rid_typed_other", `{"id":"chatcmpl_typed_other","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_wait","type":"function","function":{"name":"wait","arguments":"{\"cell_id\":\"abc\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`)}
	searchCalls := 0
	svc := &OpenAIGatewayService{
		cfg:            rawChatCompletionsTestConfig(),
		httpUpstream:   upstream,
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(context.Context, *Account, string, int) (*websearch.SearchResponse, string, error) {
			searchCalls++
			return nil, "", errors.New("unexpected search")
		},
	}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 0, searchCalls)
	require.Equal(t, 0, result.WebSearchCalls)
	require.Equal(t, "function_call", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Equal(t, "wait", gjson.Get(rec.Body.String(), "output.0.name").String())
	require.NotContains(t, rec.Body.String(), "web_search_call")
	require.NotContains(t, rec.Body.String(), "Sources:")
}

func TestForwardResponses_TypedWebSearchRequiredPreservesToolChoice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := openAIResponsesTypedWebSearchTestBody(false, "")
	body = bytes.Replace(body, []byte(`"tool_choice":"auto"`), []byte(`"tool_choice":"required"`), 1)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: openAIResponsesWebRunTestResponse("rid_typed_required", `{"id":"chatcmpl_typed_required","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_wait_required","type":"function","function":{"name":"wait","arguments":"{\"cell_id\":\"abc\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`)}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream, settingService: &SettingService{}}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "required", gjson.GetBytes(upstream.lastBody, "tool_choice").String())
	require.Equal(t, "wait", gjson.Get(rec.Body.String(), "output.0.name").String())
}

func TestForwardResponses_TypedWebSearchBypassPreservesClientToolChoice(t *testing.T) {
	tests := []struct {
		name               string
		toolChoice         string
		expectedUpstream   string
		expectedOutputName string
	}{
		{name: "none", toolChoice: `"none"`, expectedUpstream: "none"},
		{
			name:               "forced other",
			toolChoice:         `{"type":"function","name":"wait"}`,
			expectedUpstream:   "wait",
			expectedOutputName: "wait",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			body := openAIResponsesTypedWebSearchTestBody(false, "")
			body = bytes.Replace(body, []byte(`"tool_choice":"auto"`), []byte(`"tool_choice":`+tc.toolChoice), 1)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			responseBody := `{"id":"chatcmpl_typed_bypass","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"未执行搜索"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`
			if tc.expectedOutputName != "" {
				responseBody = `{"id":"chatcmpl_typed_bypass","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_wait_bypass","type":"function","function":{"name":"wait","arguments":"{\"cell_id\":\"abc\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`
			}
			upstream := &httpUpstreamRecorder{resp: openAIResponsesWebRunTestResponse("rid_typed_bypass", responseBody)}
			searchCalls := 0
			svc := &OpenAIGatewayService{
				cfg:            rawChatCompletionsTestConfig(),
				httpUpstream:   upstream,
				settingService: &SettingService{},
				openAIWebSearchExecutor: func(context.Context, *Account, string, int) (*websearch.SearchResponse, string, error) {
					searchCalls++
					return nil, "", errors.New("unexpected search")
				},
			}
			account := forceChatResponsesFallbackAccount()
			account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

			result, err := svc.Forward(context.Background(), c, account, body)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 0, searchCalls)
			require.Equal(t, 0, result.WebSearchCalls)
			require.NotContains(t, string(upstream.lastBody), openAIResponsesTypedWebSearchToolName)
			if tc.name == "none" {
				require.Equal(t, tc.expectedUpstream, gjson.GetBytes(upstream.lastBody, "tool_choice").String())
			} else {
				require.Equal(t, tc.expectedUpstream, gjson.GetBytes(upstream.lastBody, "tool_choice.function.name").String())
				require.Equal(t, tc.expectedOutputName, gjson.Get(rec.Body.String(), "output.0.name").String())
			}
			require.NotContains(t, rec.Body.String(), "web_search_call")
		})
	}
}

func TestForwardResponses_TypedWebSearchAbsentChoiceKeepsModelSelection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := openAIResponsesTypedWebSearchTestBody(false, "")
	body = bytes.Replace(body, []byte(`"tool_choice":"auto",`), nil, 1)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: openAIResponsesWebRunTestResponse("rid_typed_absent", `{"id":"chatcmpl_typed_absent","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_wait_absent","type":"function","function":{"name":"wait","arguments":"{\"cell_id\":\"abc\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`)}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream, settingService: &SettingService{}}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, string(upstream.lastBody), `"name":"sub2api_web_search"`)
	require.False(t, gjson.GetBytes(upstream.lastBody, "tool_choice").Exists())
	require.Equal(t, "wait", gjson.Get(rec.Body.String(), "output.0.name").String())
}

func TestForwardResponses_TypedWebSearchRejectsParallelClientToolCall(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := openAIResponsesTypedWebSearchTestBody(false, "")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: openAIResponsesWebRunTestResponse("rid_typed_parallel", `{"id":"chatcmpl_typed_parallel","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_typed_parallel","type":"function","function":{"name":"sub2api_web_search","arguments":"{\"search_query\":[{\"q\":\"latest\"}]}"}},{"id":"call_wait_parallel","type":"function","function":{"name":"wait","arguments":"{\"cell_id\":\"abc\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`)}
	searchCalls := 0
	svc := &OpenAIGatewayService{
		cfg:            rawChatCompletionsTestConfig(),
		httpUpstream:   upstream,
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(context.Context, *Account, string, int) (*websearch.SearchResponse, string, error) {
			searchCalls++
			return nil, "", errors.New("unexpected search")
		},
	}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.ErrorContains(t, err, "cannot be combined with parallel client tool calls")
	require.Nil(t, result)
	require.Equal(t, 0, searchCalls)
	require.Equal(t, http.StatusBadGateway, rec.Code)
}

func TestForwardResponses_TypedWebSearchHonorsMaxUses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)
	body := openAIResponsesTypedWebSearchTestBody(false, "")
	body = bytes.Replace(body, []byte(`"type":"web_search",`), []byte(`"type":"web_search","max_uses":1,`), 1)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAIResponsesWebRunTestResponse("rid_typed_max_1", `{"id":"chatcmpl_typed_max_1","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_typed_max_1","type":"function","function":{"name":"sub2api_web_search","arguments":"{\"search_query\":[{\"q\":\"one\"}]}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`),
		openAIResponsesWebRunTestResponse("rid_typed_max_2", `{"id":"chatcmpl_typed_max_2","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_typed_max_2","type":"function","function":{"name":"sub2api_web_search","arguments":"{\"search_query\":[{\"q\":\"two\"}]}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":7,"completion_tokens":1,"total_tokens":8}}`),
		openAIResponsesWebRunTestResponse("rid_typed_max_final", `{"id":"chatcmpl_typed_max_final","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"已使用首次搜索结果完成回答"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":3,"total_tokens":12}}`),
	}}
	searchCalls := 0
	svc := &OpenAIGatewayService{
		cfg:            rawChatCompletionsTestConfig(),
		httpUpstream:   upstream,
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(_ context.Context, _ *Account, query string, _ int) (*websearch.SearchResponse, string, error) {
			searchCalls++
			return &websearch.SearchResponse{Query: query}, "anysearch", nil
		},
	}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, searchCalls)
	require.Len(t, upstream.bodies, 3)
	finalRequest := upstream.bodies[2]
	require.Equal(t, "auto", gjson.GetBytes(finalRequest, "tool_choice").String())
	require.Empty(t, gjson.GetBytes(finalRequest, `tools.#(function.name=="sub2api_web_search")`).String())
	require.Contains(t, gjson.GetBytes(finalRequest, "messages.4.content").String(), "search_limit_reached")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "已使用首次搜索结果完成回答", gjson.Get(rec.Body.String(), "output.1.content.0.text").String())
}

func TestForwardResponses_TypedWebSearchStreamingEmitsCitationLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)
	body := openAIResponsesTypedWebSearchTestBody(true, "")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAIResponsesWebRunTestResponse("rid_typed_stream_1", `{"id":"chatcmpl_typed_stream_1","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_typed_stream","type":"function","function":{"name":"sub2api_web_search","arguments":"{\"search_query\":[{\"q\":\"中文查询\"}]}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":4,"completion_tokens":1,"total_tokens":5}}`),
		openAIResponsesWebRunTestResponse("rid_typed_stream_2", `{"id":"chatcmpl_typed_stream_2","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"中文回答"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9}}`),
	}}
	svc := &OpenAIGatewayService{
		cfg:            rawChatCompletionsTestConfig(),
		httpUpstream:   upstream,
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(_ context.Context, _ *Account, query string, _ int) (*websearch.SearchResponse, string, error) {
			return &websearch.SearchResponse{Query: query, Results: []websearch.SearchResult{{Title: "中文来源", URL: "https://example.com/source"}}}, "anysearch", nil
		},
	}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Equal(t, 1, result.WebSearchCalls)
	wire := rec.Body.String()
	require.Contains(t, wire, `"type":"web_search_call"`)
	require.Contains(t, wire, `"delta":"\n\nSources:\n- 中文来源: https://example.com/source\n"`)
	require.Contains(t, wire, "event: response.output_text.annotation.added")
	require.Contains(t, wire, `"annotations":[{"type":"url_citation","url":"https://example.com/source"`)
	require.NotContains(t, wire, openAIResponsesTypedWebSearchToolName)
	require.Less(t, strings.Index(wire, `"delta":"\n\nSources:`), strings.Index(wire, "event: response.output_text.annotation.added"))
	require.Less(t, strings.Index(wire, "event: response.output_text.annotation.added"), strings.Index(wire, "event: response.output_text.done"))
}

func TestForwardResponses_TypedWebSearchStreamingMarksClientDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)
	body := openAIResponsesTypedWebSearchTestBody(true, "")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Writer = &openAIResponsesWebRunFailingWriter{ResponseWriter: c.Writer}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAIResponsesWebRunTestResponse("rid_typed_disconnect_1", `{"id":"chatcmpl_typed_disconnect_1","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_typed_disconnect","type":"function","function":{"name":"sub2api_web_search","arguments":"{\"search_query\":[{\"q\":\"latest\"}]}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":4,"completion_tokens":1,"total_tokens":5}}`),
		openAIResponsesWebRunTestResponse("rid_typed_disconnect_2", `{"id":"chatcmpl_typed_disconnect_2","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9}}`),
	}}
	svc := &OpenAIGatewayService{
		cfg:            rawChatCompletionsTestConfig(),
		httpUpstream:   upstream,
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(_ context.Context, _ *Account, query string, _ int) (*websearch.SearchResponse, string, error) {
			return &websearch.SearchResponse{Query: query, Results: []websearch.SearchResult{{Title: "Source", URL: "https://example.com/source"}}}, "anysearch", nil
		},
	}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.True(t, result.ClientDisconnect)
	require.Equal(t, 1, result.WebSearchCalls)
	require.Equal(t, 11, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
}

func TestForwardResponses_TypedWebSearchProxyNameConflictReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := openAIResponsesTypedWebSearchTestBody(false, `,{"type":"function","name":"sub2api_web_search","parameters":{"type":"object"}}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), settingService: &SettingService{}}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "conflicts with a declared client tool")
	require.Equal(t, "tools", gjson.Get(rec.Body.String(), "error.param").String())
}

func TestForwardResponses_TypedWebSearchValidationErrorIncludesToolsParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := openAIResponsesTypedWebSearchTestBody(false, "")
	body = bytes.Replace(
		body,
		[]byte(`"search_context_size":"low"`),
		[]byte(`"search_context_size":"low","user_location":{"type":"approximate","country":"CN"}`),
		1,
	)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), settingService: &SettingService{}}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.ErrorContains(t, err, "user_location")
	require.Nil(t, result)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "tools", gjson.Get(rec.Body.String(), "error.param").String())
}

func TestForwardResponses_TypedWebSearchStructuredOutputDoesNotAppendSources(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableOpenAIResponsesWebSearchTestManager(t)
	body := openAIResponsesTypedWebSearchTestBody(false, "")
	body = bytes.Replace(body, []byte(`"tool_choice":"auto",`), []byte(`"tool_choice":"auto","text":{"format":{"type":"json_object"}},`), 1)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAIResponsesWebRunTestResponse("rid_typed_json_1", `{"id":"chatcmpl_typed_json_1","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_typed_json","type":"function","function":{"name":"sub2api_web_search","arguments":"{\"search_query\":[{\"q\":\"latest\"}]}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":4,"completion_tokens":1,"total_tokens":5}}`),
		openAIResponsesWebRunTestResponse("rid_typed_json_2", `{"id":"chatcmpl_typed_json_2","object":"chat.completion","model":"deepseek-v4-pro","choices":[{"index":0,"message":{"role":"assistant","content":"{\"answer\":\"ok\"}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9}}`),
	}}
	svc := &OpenAIGatewayService{
		cfg:            rawChatCompletionsTestConfig(),
		httpUpstream:   upstream,
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(_ context.Context, _ *Account, query string, _ int) (*websearch.SearchResponse, string, error) {
			return &websearch.SearchResponse{Query: query, Results: []websearch.SearchResult{{Title: "Source", URL: "https://example.com/source"}}}, "anysearch", nil
		},
	}
	account := forceChatResponsesFallbackAccount()
	account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled

	result, err := svc.Forward(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.WebSearchCalls)
	require.Equal(t, "web_search_call", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Equal(t, `{"answer":"ok"}`, gjson.Get(rec.Body.String(), "output.1.content.0.text").String())
	require.False(t, gjson.Get(rec.Body.String(), "output.1.content.0.annotations").Exists())
	require.NotContains(t, rec.Body.String(), "Sources:")
}

func TestAppendOpenAIResponsesWebSearchSourcesCapsAndUsesRuneIndexes(t *testing.T) {
	response := &apicompat.ResponsesResponse{Output: []apicompat.ResponsesOutput{{
		Type: "message",
		ID:   "msg_sources",
		Role: "assistant",
		Content: []apicompat.ResponsesContentPart{{
			Type: "output_text",
			Text: "中文回答",
		}},
	}}}
	sources := []websearch.SearchResult{
		{Title: "一", URL: "https://example.com/1"},
		{Title: "二", URL: "https://example.com/2"},
		{Title: "三", URL: "https://example.com/3"},
		{Title: "四", URL: "https://example.com/4"},
		{Title: "五", URL: "https://example.com/5"},
		{Title: "六", URL: "https://example.com/6"},
		{Title: "无效", URL: "javascript:alert(1)"},
	}

	projection := appendOpenAIResponsesWebSearchSources(response, sources)

	require.NotNil(t, projection)
	part := response.Output[0].Content[0]
	require.Len(t, part.Annotations, openAIResponsesWebSearchMaxSources)
	require.NotContains(t, part.Text, "https://example.com/6")
	require.NotContains(t, part.Text, "javascript:")
	for _, annotation := range part.Annotations {
		require.Equal(t, annotation.URL, string([]rune(part.Text)[annotation.StartIndex:annotation.EndIndex]))
	}
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

func TestParseOpenAIResponsesInternalWebToolArgumentsEnforcesPerCallQueryLimit(t *testing.T) {
	require.Equal(t, int64(openAIResponsesWebRunMaxQueriesPerCall), gjson.Get(openAIResponsesWebRunToolSchema, "properties.search_query.maxItems").Int())
	require.Equal(t, int64(openAIResponsesWebRunMaxQueriesPerCall), gjson.Get(openAIResponsesTypedWebSearchToolSchema, "properties.search_query.maxItems").Int())

	arguments := `{"search_query":[{"q":"one"},{"q":"two"},{"q":"three"},{"q":"four"},{"q":"five"}]}`
	tests := []struct {
		name   string
		config openAIResponsesInternalWebToolConfig
	}{
		{
			name: "web.run",
			config: openAIResponsesInternalWebToolConfig{
				Name: "web__run",
				Kind: openAIResponsesInternalWebToolWebRun,
			},
		},
		{
			name: "typed web_search",
			config: openAIResponsesInternalWebToolConfig{
				Name: openAIResponsesTypedWebSearchToolName,
				Kind: openAIResponsesInternalWebToolTypedSearch,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, toolErr := parseOpenAIResponsesInternalWebToolArguments(
				tt.config,
				arguments,
				openAIResponsesWebRunMaxQueries,
			)

			require.Nil(t, parsed)
			require.NotNil(t, toolErr)
			require.Equal(t, "search_limit_exceeded", toolErr.Code)
			require.Contains(t, toolErr.Message, "at most 4 web search queries")
		})
	}
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

func openAIResponsesTypedWebSearchTestBody(stream bool, extraTool string) []byte {
	streamValue := "false"
	if stream {
		streamValue = "true"
	}
	return []byte(`{
		"model":"deepseek-v4-pro",
		"stream":` + streamValue + `,
		"tool_choice":"auto",
		"input":[
			{"role":"user","content":[{"type":"input_text","text":"搜索杭州天气"}]},
			{"type":"additional_tools","tools":[
				{"type":"web_search","search_context_size":"low","filters":{"allowed_domains":["example.com"]}},
				{"type":"function","name":"wait","parameters":{"type":"object","properties":{"cell_id":{"type":"string"}}}},
				{"type":"custom","name":"exec","description":"Execute a command"},
				{"type":"function","name":"tool_1","parameters":{"type":"object"}},
				{"type":"function","name":"tool_2","parameters":{"type":"object"}},
				{"type":"function","name":"tool_3","parameters":{"type":"object"}},
				{"type":"function","name":"tool_4","parameters":{"type":"object"}},
				{"type":"function","name":"tool_5","parameters":{"type":"object"}},
				{"type":"function","name":"tool_6","parameters":{"type":"object"}},
				{"type":"function","name":"tool_7","parameters":{"type":"object"}},
				{"type":"function","name":"tool_8","parameters":{"type":"object"}}` + extraTool + `
			]}
		]
	}`)
}

func openAIResponsesCodexLiteBridgeTestBody(stream bool, choice string) []byte {
	streamValue := "false"
	if stream {
		streamValue = "true"
	}
	return []byte(`{
		"model":"gpt-5.6-sol",
		"stream":` + streamValue + `,
		` + choice + `
		"input":[
			{"role":"user","content":[{"type":"input_text","text":"需要时搜索最新信息"}]},
			{"type":"additional_tools","tools":[
				{"type":"function","name":"wait","parameters":{"type":"object","properties":{"cell_id":{"type":"string"}}}},
				{"type":"custom","name":"exec","description":"Execute a command"}
			]}
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
