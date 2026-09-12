//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/websearch"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// alphaSearchViaResponsesTestAccount 构造开启"Alpha Search 经上游 Responses 执行"的
// openai API Key 账号，可选开启账号级 Web Search Emulation。
func alphaSearchViaResponsesTestAccount(emulation bool) *Account {
	account := &Account{
		ID:          9,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://relay.example",
		},
		Extra: map[string]any{accountExtraKeyOpenAIAlphaSearchViaResponses: true},
	}
	if emulation {
		account.Extra[featureKeyWebSearchEmulation] = WebSearchModeEnabled
	}
	return account
}

func alphaSearchViaResponsesTestContext(t *testing.T, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/alpha/search", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", codexCLIUserAgent)
	c.Request.Header.Set(responsesLiteHeaderKey, "true")
	return c, recorder
}

func alphaSearchViaResponsesTestService(upstream *httpUpstreamRecorder) *OpenAIGatewayService {
	repo := &alphaSearchAccountStateRepo{}
	cfg := &config.Config{}
	return &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		accountRepo:      repo,
		rateLimitService: NewRateLimitService(repo, nil, cfg, nil, nil),
		settingService:   &SettingService{},
	}
}

// disableOpenAIAlphaSearchTestEmulation 关闭系统级 Web Search Emulation 并清空供应商管理器，
// 模拟"本地模拟不可用"场景。
func disableOpenAIAlphaSearchTestEmulation(t *testing.T) {
	t.Helper()
	setGlobalWebSearchConfig(&WebSearchEmulationConfig{Enabled: false})
	SetWebSearchManager(nil)
	t.Cleanup(func() {
		SetWebSearchManager(nil)
		clearGlobalWebSearchConfig()
	})
}

func alphaSearchUpstreamRecorder(status int, contentType, body string) *httpUpstreamRecorder {
	return &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}, "X-Request-Id": []string{"req-bridge"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}}
}

// alphaSearchNoSearchSSE 是上游忽略 web_search 工具时的典型 SSE：只有模型文本，无引用、无 web_search_call。
func alphaSearchNoSearchSSE() string {
	return "event: response.output_text.delta\n" +
		`data: {"type":"response.output_text.delta","delta":"memory answer"}` + "\n\n" +
		"event: response.completed\n" +
		`data: {"type":"response.completed","response":{"output":[{"type":"message","content":[{"type":"output_text","text":"memory answer"}]}]}}` + "\n\n"
}

func alphaSearchWebSearchCallOnlySSE() string {
	return "event: response.output_item.done\n" +
		`data: {"type":"response.output_item.done","item":{"type":"web_search_call","status":"completed","action":{"type":"search","query":"news"}}}` + "\n\n" +
		"event: response.output_text.delta\n" +
		`data: {"type":"response.output_text.delta","delta":"searched answer"}` + "\n\n" +
		"event: response.completed\n" +
		`data: {"type":"response.completed","response":{"output":[{"type":"web_search_call","status":"completed"},{"type":"message","content":[{"type":"output_text","text":"searched answer"}]}]}}` + "\n\n"
}

func alphaSearchStubExecutor(t *testing.T, calls *[]string, maxResultsSeen *[]int, byQuery map[string]*websearch.SearchResponse, errs map[string]error) func(context.Context, *Account, string, int) (*websearch.SearchResponse, string, error) {
	t.Helper()
	return func(_ context.Context, _ *Account, query string, maxResults int) (*websearch.SearchResponse, string, error) {
		if calls != nil {
			*calls = append(*calls, query)
		}
		if maxResultsSeen != nil {
			*maxResultsSeen = append(*maxResultsSeen, maxResults)
		}
		if err, ok := errs[query]; ok {
			return nil, "", err
		}
		return byQuery[query], "anysearch", nil
	}
}

// Scenario: AC1 访问器只对 openai API Key 且 extra 为布尔 true 的账号返回真。
func TestAccount_IsOpenAIAlphaSearchViaResponsesEnabled(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{name: "nil", account: nil, want: false},
		{name: "openai apikey enabled", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{accountExtraKeyOpenAIAlphaSearchViaResponses: true}}, want: true},
		{name: "openai apikey false", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{accountExtraKeyOpenAIAlphaSearchViaResponses: false}}, want: false},
		{name: "openai apikey string", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{accountExtraKeyOpenAIAlphaSearchViaResponses: "true"}}, want: false},
		{name: "openai apikey missing", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{}}, want: false},
		{name: "openai apikey nil extra", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, want: false},
		{name: "openai oauth", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{accountExtraKeyOpenAIAlphaSearchViaResponses: true}}, want: false},
		{name: "openai pat", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"auth_mode": OpenAIAuthModePersonalAccessToken}, Extra: map[string]any{accountExtraKeyOpenAIAlphaSearchViaResponses: true}}, want: false},
		{name: "deepseek apikey", account: &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Extra: map[string]any{accountExtraKeyOpenAIAlphaSearchViaResponses: true}}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.account.IsOpenAIAlphaSearchViaResponsesEnabled())
		})
	}
}

// Scenario: AC2 开关开启时 alpha 请求翻译成上游 Responses web_search，含 url_citation 即写回并计费。
func TestForwardAlphaSearchViaResponsesUsesUpstreamWebSearch(t *testing.T) {
	body := []byte(`{"id":"search-session","model":"gpt-5.6-sol","commands":{"search_query":[{"q":"OpenAI news"}]},"settings":{"search_context_size":"high"}}`)
	c, recorder := alphaSearchViaResponsesTestContext(t, body)
	upstream := alphaSearchUpstreamRecorder(http.StatusOK, "text/event-stream", alphaSearchResponsesSSE("search result"))
	service := alphaSearchViaResponsesTestService(upstream)
	account := alphaSearchViaResponsesTestAccount(false)

	result, err := service.ForwardAlphaSearch(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.WebSearchCalls)
	require.Equal(t, "/v1/responses", result.UpstreamEndpoint)
	require.Equal(t, "req-bridge", result.RequestID)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"output":"search result","results":[{"type":"text_result","ref_id":"turn0search0","url":"https://example.com/news","title":"Example News"}]}`, recorder.Body.String())
	require.Equal(t, "https://relay.example/v1/responses", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-test", upstream.lastReq.Header.Get("Authorization"))
	require.Empty(t, upstream.lastReq.Header.Get("ChatGPT-Account-ID"))
	require.Empty(t, upstream.lastReq.Header.Get(responsesLiteHeaderKey))
	require.Empty(t, upstream.lastReq.Header.Get("OpenAI-Beta"))
	require.Equal(t, "application/json", upstream.lastReq.Header.Get("Content-Type"))
	require.Equal(t, "text/event-stream", upstream.lastReq.Header.Get("Accept"))
	require.Equal(t, codexCLIUserAgent, upstream.lastReq.Header.Get("User-Agent"))
	require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(upstream.lastBody, "model").String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.False(t, gjson.GetBytes(upstream.lastBody, "store").Bool())
	require.Equal(t, "web_search", gjson.GetBytes(upstream.lastBody, "tools.0.type").String())
	require.Equal(t, "high", gjson.GetBytes(upstream.lastBody, "tools.0.search_context_size").String())
	require.Contains(t, gjson.GetBytes(upstream.lastBody, "input.0.content.0.text").String(), `"search_query"`)
}

// Scenario: AC3 只有 web_search_call 输出项、无引用时同样视为上游已搜索。
func TestForwardAlphaSearchViaResponsesAcceptsWebSearchCallEvidence(t *testing.T) {
	body := []byte(`{"id":"s","model":"gpt-5.6-sol","commands":{"search_query":[{"q":"news"}]}}`)
	c, recorder := alphaSearchViaResponsesTestContext(t, body)
	upstream := alphaSearchUpstreamRecorder(http.StatusOK, "text/event-stream", alphaSearchWebSearchCallOnlySSE())
	service := alphaSearchViaResponsesTestService(upstream)

	result, err := service.ForwardAlphaSearch(context.Background(), c, alphaSearchViaResponsesTestAccount(false), body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.WebSearchCalls)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"output":"searched answer"}`, recorder.Body.String())
}

// Scenario: AC4 上游 2xx 但无搜索证据且模拟资格满足时改走本地供应商并计费。
func TestForwardAlphaSearchViaResponsesFallsBackToEmulationWhenUpstreamDidNotSearch(t *testing.T) {
	enableOpenAIResponsesWebSearchTestManager(t)
	body := []byte(`{"id":"s","model":"gpt-5.6-sol","commands":{"search_query":[{"q":"台风 最新"}]}}`)
	c, recorder := alphaSearchViaResponsesTestContext(t, body)
	upstream := alphaSearchUpstreamRecorder(http.StatusOK, "text/event-stream", alphaSearchNoSearchSSE())
	service := alphaSearchViaResponsesTestService(upstream)
	var calls []string
	service.openAIWebSearchExecutor = alphaSearchStubExecutor(t, &calls, nil, map[string]*websearch.SearchResponse{
		"台风 最新": {Results: []websearch.SearchResult{
			{URL: "https://typhoon.example/a", Title: "台风路径", Snippet: "最新路径", PageAge: "2026-09-12"},
			{URL: "https://typhoon.example/b", Title: "预警"},
		}},
	}, nil)

	result, err := service.ForwardAlphaSearch(context.Background(), c, alphaSearchViaResponsesTestAccount(true), body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.WebSearchCalls)
	require.Equal(t, "/v1/alpha/search", result.UpstreamEndpoint)
	require.Equal(t, []string{"台风 最新"}, calls)
	require.Equal(t, http.StatusOK, recorder.Code)
	output := gjson.Get(recorder.Body.String(), "output").String()
	require.Contains(t, output, `Search results for "台风 最新"`)
	require.Contains(t, output, "[turn0search0] 台风路径")
	require.Contains(t, output, "https://typhoon.example/a")
	require.Contains(t, output, "最新路径")
	require.Contains(t, output, "Published: 2026-09-12")
	require.Contains(t, output, "[turn0search1] 预警")
	require.NotContains(t, output, "memory answer")
	require.Equal(t, "turn0search0", gjson.Get(recorder.Body.String(), "results.0.ref_id").String())
	require.Equal(t, "https://typhoon.example/a", gjson.Get(recorder.Body.String(), "results.0.url").String())
	require.Equal(t, "text_result", gjson.Get(recorder.Body.String(), "results.1.type").String())
}

// Scenario: AC5 上游 404 时，资格满足走模拟；不可用则保持既有 UpstreamFailoverError 且未写下游。
func TestForwardAlphaSearchViaResponsesUpstreamNotFound(t *testing.T) {
	body := []byte(`{"id":"s","model":"gpt-5.6-sol","commands":{"search_query":[{"q":"news"}]}}`)

	t.Run("emulation eligible", func(t *testing.T) {
		enableOpenAIResponsesWebSearchTestManager(t)
		c, recorder := alphaSearchViaResponsesTestContext(t, body)
		upstream := alphaSearchUpstreamRecorder(http.StatusNotFound, "application/json", `{"error":{"message":"Not Found"}}`)
		service := alphaSearchViaResponsesTestService(upstream)
		service.openAIWebSearchExecutor = alphaSearchStubExecutor(t, nil, nil, map[string]*websearch.SearchResponse{
			"news": {Results: []websearch.SearchResult{{URL: "https://example.com/n", Title: "N"}}},
		}, nil)

		result, err := service.ForwardAlphaSearch(context.Background(), c, alphaSearchViaResponsesTestAccount(true), body)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, 1, result.WebSearchCalls)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "https://example.com/n", gjson.Get(recorder.Body.String(), "results.0.url").String())
	})

	t.Run("emulation unavailable", func(t *testing.T) {
		disableOpenAIAlphaSearchTestEmulation(t)
		c, recorder := alphaSearchViaResponsesTestContext(t, body)
		upstream := alphaSearchUpstreamRecorder(http.StatusNotFound, "application/json", `{"error":{"message":"Not Found"}}`)
		service := alphaSearchViaResponsesTestService(upstream)

		result, err := service.ForwardAlphaSearch(context.Background(), c, alphaSearchViaResponsesTestAccount(true), body)

		require.Nil(t, result)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.Equal(t, http.StatusNotFound, failoverErr.StatusCode)
		require.False(t, c.Writer.Written())
		require.Empty(t, recorder.Body.String())
	})
}

// Scenario: AC6 上游 2xx 无搜索证据且模拟不可用时返回 502 web_search_failed，不计费。
func TestForwardAlphaSearchViaResponsesNoSearchAndEmulationUnavailable(t *testing.T) {
	disableOpenAIAlphaSearchTestEmulation(t)
	body := []byte(`{"id":"s","model":"gpt-5.6-sol","commands":{"search_query":[{"q":"news"}]}}`)
	c, recorder := alphaSearchViaResponsesTestContext(t, body)
	upstream := alphaSearchUpstreamRecorder(http.StatusOK, "text/event-stream", alphaSearchNoSearchSSE())
	service := alphaSearchViaResponsesTestService(upstream)

	result, err := service.ForwardAlphaSearch(context.Background(), c, alphaSearchViaResponsesTestAccount(false), body)

	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.Equal(t, "web_search_failed", gjson.Get(recorder.Body.String(), "error.code").String())
}

// Scenario: AC7 多查询去重、blocked_domains 过滤、low 结果数映射与 ref_id 连续编号。
func TestEmulateOpenAIAlphaSearchFiltersAndDedupes(t *testing.T) {
	enableOpenAIResponsesWebSearchTestManager(t)
	body := []byte(`{
		"id":"s","model":"gpt-5.6-sol",
		"commands":{"search_query":[{"q":"alpha"},{"q":"beta","domains":["allowed.example"]}]},
		"settings":{"search_context_size":"low","filters":{"blocked_domains":["blocked.example"]}}
	}`)
	c, recorder := alphaSearchViaResponsesTestContext(t, body)
	service := alphaSearchViaResponsesTestService(nil)
	var calls []string
	var maxResults []int
	service.openAIWebSearchExecutor = alphaSearchStubExecutor(t, &calls, &maxResults, map[string]*websearch.SearchResponse{
		"alpha": {Results: []websearch.SearchResult{
			{URL: "https://shared.example/x", Title: "Shared"},
			{URL: "https://blocked.example/y", Title: "Blocked"},
		}},
		"beta": {Results: []websearch.SearchResult{
			{URL: "https://shared.example/x", Title: "Shared again"},
			{URL: "https://allowed.example/z", Title: "Allowed"},
			{URL: "https://other.example/w", Title: "Other"},
		}},
	}, nil)

	result, err := service.emulateOpenAIAlphaSearch(context.Background(), c, alphaSearchViaResponsesTestAccount(true), body, "gpt-5.6-sol", "gpt-5.6-sol")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, []string{"alpha", "beta"}, calls)
	require.Equal(t, []int{3, 3}, maxResults)
	require.Equal(t, http.StatusOK, recorder.Code)
	results := gjson.Get(recorder.Body.String(), "results").Array()
	require.Len(t, results, 2)
	require.Equal(t, "turn0search0", results[0].Get("ref_id").String())
	require.Equal(t, "https://shared.example/x", results[0].Get("url").String())
	require.Equal(t, "turn0search1", results[1].Get("ref_id").String())
	require.Equal(t, "https://allowed.example/z", results[1].Get("url").String())
	output := gjson.Get(recorder.Body.String(), "output").String()
	require.NotContains(t, output, "blocked.example")
	require.NotContains(t, output, "other.example")
}

// Scenario: AC8 只含 open 时返回说明且不计费；search_query 与 open 并存时说明附在末尾。
func TestEmulateOpenAIAlphaSearchUnsupportedCommands(t *testing.T) {
	enableOpenAIResponsesWebSearchTestManager(t)

	t.Run("open only", func(t *testing.T) {
		body := []byte(`{"id":"s","model":"gpt-5.6-sol","commands":{"open":[{"ref_id":"https://www.nmc.cn/"}]}}`)
		c, recorder := alphaSearchViaResponsesTestContext(t, body)
		service := alphaSearchViaResponsesTestService(nil)
		service.openAIWebSearchExecutor = alphaSearchStubExecutor(t, nil, nil, nil, nil)

		result, err := service.emulateOpenAIAlphaSearch(context.Background(), c, alphaSearchViaResponsesTestAccount(true), body, "m", "m")

		require.NoError(t, err)
		require.Nil(t, result)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Contains(t, gjson.Get(recorder.Body.String(), "output").String(), "Unsupported commands in this gateway: open.")
		require.False(t, gjson.Get(recorder.Body.String(), "results").Exists())
	})

	t.Run("search with open", func(t *testing.T) {
		body := []byte(`{"id":"s","model":"gpt-5.6-sol","commands":{"search_query":[{"q":"news"}],"open":[{"ref_id":"turn0search0"}],"find":[{"ref_id":"turn0search0","pattern":"x"}]}}`)
		c, recorder := alphaSearchViaResponsesTestContext(t, body)
		service := alphaSearchViaResponsesTestService(nil)
		service.openAIWebSearchExecutor = alphaSearchStubExecutor(t, nil, nil, map[string]*websearch.SearchResponse{
			"news": {Results: []websearch.SearchResult{{URL: "https://example.com/n", Title: "N"}}},
		}, nil)

		result, err := service.emulateOpenAIAlphaSearch(context.Background(), c, alphaSearchViaResponsesTestAccount(true), body, "m", "m")

		require.NoError(t, err)
		require.NotNil(t, result)
		output := gjson.Get(recorder.Body.String(), "output").String()
		require.True(t, strings.HasSuffix(output, "Unsupported commands in this gateway: open, find. Use the search results above, or fetch the page yourself."))
		require.Contains(t, output, "[turn0search0] N")
	})
}

// Scenario: AC9 供应商全部失败返回 502 不计费；部分失败按成功结果返回并计费。
func TestEmulateOpenAIAlphaSearchProviderFailures(t *testing.T) {
	enableOpenAIResponsesWebSearchTestManager(t)
	body := []byte(`{"id":"s","model":"gpt-5.6-sol","commands":{"search_query":[{"q":"one"},{"q":"two"}]},"max_output_tokens":20}`)

	t.Run("all fail", func(t *testing.T) {
		c, recorder := alphaSearchViaResponsesTestContext(t, body)
		service := alphaSearchViaResponsesTestService(nil)
		service.openAIWebSearchExecutor = alphaSearchStubExecutor(t, nil, nil, nil, map[string]error{"one": errors.New("boom"), "two": errors.New("boom")})

		result, err := service.emulateOpenAIAlphaSearch(context.Background(), c, alphaSearchViaResponsesTestAccount(true), body, "m", "m")

		require.Error(t, err)
		require.Nil(t, result)
		require.Equal(t, http.StatusBadGateway, recorder.Code)
		require.Equal(t, "web_search_failed", gjson.Get(recorder.Body.String(), "error.code").String())
	})

	t.Run("partial fail with truncation", func(t *testing.T) {
		c, recorder := alphaSearchViaResponsesTestContext(t, body)
		service := alphaSearchViaResponsesTestService(nil)
		service.openAIWebSearchExecutor = alphaSearchStubExecutor(t, nil, nil, map[string]*websearch.SearchResponse{
			"two": {Results: []websearch.SearchResult{{URL: "https://example.com/two", Title: "Two", Snippet: strings.Repeat("s", 200)}}},
		}, map[string]error{"one": errors.New("boom")})

		result, err := service.emulateOpenAIAlphaSearch(context.Background(), c, alphaSearchViaResponsesTestAccount(true), body, "m", "m")

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, 1, result.WebSearchCalls)
		require.Equal(t, http.StatusOK, recorder.Code)
		output := gjson.Get(recorder.Body.String(), "output").String()
		require.True(t, strings.HasSuffix(output, "\n...<truncated>"))
		require.LessOrEqual(t, len([]rune(strings.TrimSuffix(output, "\n...<truncated>"))), 80)
		require.Equal(t, "https://example.com/two", gjson.Get(recorder.Body.String(), "results.0.url").String())
	})
}

// Scenario: AC10 开关关闭时 API Key 账号仍按既有 alpha/search 透传路径处理。
func TestForwardAlphaSearchViaResponsesDisabledKeepsLegacyPath(t *testing.T) {
	body := []byte(`{"id":"s","model":"gpt-5.6-sol","commands":{"search_query":[{"q":"news"}]}}`)
	c, recorder := alphaSearchViaResponsesTestContext(t, body)
	upstream := alphaSearchUpstreamRecorder(http.StatusOK, "application/json", `{"output":"legacy"}`)
	service := alphaSearchViaResponsesTestService(upstream)
	account := alphaSearchViaResponsesTestAccount(false)
	account.Extra[accountExtraKeyOpenAIAlphaSearchViaResponses] = false

	result, err := service.ForwardAlphaSearch(context.Background(), c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "https://relay.example/v1/alpha/search", upstream.lastReq.URL.String())
	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"output":"legacy"}`, recorder.Body.String())
}

// Scenario: AC14 无 URL 的文本结果保留且 results 省略 url；有 URL 的重复项仍去重。
func TestEmulateOpenAIAlphaSearchKeepsResultsWithoutURL(t *testing.T) {
	enableOpenAIResponsesWebSearchTestManager(t)
	body := []byte(`{"id":"s","model":"gpt-5.6-sol","commands":{"search_query":[{"q":"one"},{"q":"two"}]}}`)
	c, recorder := alphaSearchViaResponsesTestContext(t, body)
	service := alphaSearchViaResponsesTestService(nil)
	service.openAIWebSearchExecutor = alphaSearchStubExecutor(t, nil, nil, map[string]*websearch.SearchResponse{
		"one": {Results: []websearch.SearchResult{
			{Title: "AnySearch", Snippet: "整段文本结果"},
			{URL: "https://example.com/a", Title: "A"},
		}},
		"two": {Results: []websearch.SearchResult{
			{URL: "https://example.com/a", Title: "A again"},
			{Title: "AnySearch", Snippet: "另一段文本"},
		}},
	}, nil)

	result, err := service.emulateOpenAIAlphaSearch(context.Background(), c, alphaSearchViaResponsesTestAccount(true), body, "m", "m")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.WebSearchCalls)
	results := gjson.Get(recorder.Body.String(), "results").Array()
	require.Len(t, results, 3)
	require.False(t, results[0].Get("url").Exists())
	require.Equal(t, "turn0search0", results[0].Get("ref_id").String())
	require.Equal(t, "https://example.com/a", results[1].Get("url").String())
	require.False(t, results[2].Get("url").Exists())
	output := gjson.Get(recorder.Body.String(), "output").String()
	require.Contains(t, output, "[turn0search0] AnySearch\n整段文本结果")
	require.Contains(t, output, "[turn0search1] A\nhttps://example.com/a")
	require.Contains(t, output, "[turn0search2] AnySearch\n另一段文本")
	require.NotContains(t, output, "A again")
}
