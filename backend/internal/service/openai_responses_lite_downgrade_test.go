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

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// responsesLiteDowngradeTestBody 按 Codex Responses Lite 的真实形态构造请求：
// 顶层无 tools/instructions，所有 function/custom 工具收进 additional_tools 的
// functions 命名空间，另有一个 MCP 命名空间。
func responsesLiteDowngradeTestBody() []byte {
	return []byte(`{
		"model":"gpt-6-astra","stream":true,"store":false,
		"input":[
			{"type":"additional_tools","role":"developer","tools":[
				{"type":"namespace","name":"functions","description":"Tools","tools":[
					{"type":"custom","name":"exec","description":"Run","format":{"type":"text"}},
					{"type":"function","name":"wait","parameters":{"type":"object"}}
				]},
				{"type":"namespace","name":"mcp_demo","tools":[
					{"type":"function","name":"lookup","parameters":{"type":"object"}}
				]}
			]},
			{"type":"message","role":"developer","content":[{"type":"input_text","text":"BASE"}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}
		],
		"tool_choice":"auto","parallel_tool_calls":false,
		"reasoning":{"effort":"medium","context":"all_turns"},
		"include":["reasoning.encrypted_content"]
	}`)
}

func responsesLiteDowngradeTestContext(body []byte, lite bool) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.153.0")
	if lite {
		c.Request.Header.Set(responsesLiteHeader, "true")
	}
	return c, rec
}

// responsesLiteDowngradeNativeUpstream 模拟原生 Responses 上游返回一次对摊平名
// functions__exec 的 function_call。
func responsesLiteDowngradeNativeUpstream() *httpUpstreamRecorder {
	return &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.created\",\"sequence_number\":0,\"response\":{\"id\":\"resp_1\",\"status\":\"in_progress\",\"output\":[]}}\n\n" +
				"data: {\"type\":\"response.output_item.added\",\"sequence_number\":1,\"output_index\":0,\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"functions__exec\",\"arguments\":\"\",\"status\":\"in_progress\"}}\n\n" +
				"data: {\"type\":\"response.function_call_arguments.delta\",\"sequence_number\":2,\"output_index\":0,\"item_id\":\"fc_1\",\"delta\":\"{\\\"input\\\":\\\"pwd\\\"}\"}\n\n" +
				"data: {\"type\":\"response.function_call_arguments.done\",\"sequence_number\":3,\"output_index\":0,\"item_id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"functions__exec\",\"arguments\":\"{\\\"input\\\":\\\"pwd\\\"}\"}\n\n" +
				"data: {\"type\":\"response.output_item.done\",\"sequence_number\":4,\"output_index\":0,\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"functions__exec\",\"arguments\":\"{\\\"input\\\":\\\"pwd\\\"}\",\"status\":\"completed\"}}\n\n" +
				"data: {\"type\":\"response.completed\",\"sequence_number\":5,\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"output\":[{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"functions__exec\",\"arguments\":\"{\\\"input\\\":\\\"pwd\\\"}\",\"status\":\"completed\"}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n" +
				"data: [DONE]\n\n",
		)),
	}}
}

func responsesLiteDowngradeDeepSeekAccount(enabled bool) *Account {
	account := &Account{
		ID: 901, Name: "ds-native", Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{
			"api_key":       "sk-ds",
			"api_protocol":  APIProtocolResponses,
			"model_mapping": map[string]any{"gpt-6-astra": "deepseek-reasoner"},
		},
		Extra: map[string]any{},
	}
	if enabled {
		account.Extra[accountExtraKeyOpenAIResponsesLiteDowngrade] = true
	}
	return account
}

func responsesLiteDowngradeOpenAIAccount(enabled, passthrough bool) *Account {
	account := rawChatCompletionsTestAccount()
	account.Credentials["model_mapping"] = map[string]any{"gpt-6-astra": "deepseek-reasoner"}
	account.Extra = map[string]any{
		openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceResponses),
		"openai_passthrough":                passthrough,
	}
	if enabled {
		account.Extra[accountExtraKeyOpenAIResponsesLiteDowngrade] = true
	}
	return account
}

func requireResponsesLiteDowngradedUpstream(t *testing.T, upstream *httpUpstreamRecorder) {
	t.Helper()
	require.NotNil(t, upstream.lastReq)
	require.True(t, strings.HasSuffix(upstream.lastReq.URL.Path, "/responses"), upstream.lastReq.URL.String())
	require.Empty(t, upstream.lastReq.Header.Get(responsesLiteHeader), "降级后不得再向上游传播 Lite 头")
	body := upstream.lastBody
	require.False(t, gjson.GetBytes(body, `input.#(type=="additional_tools")`).Exists(), "additional_tools 必须被提升到顶层 tools")
	require.False(t, gjson.GetBytes(body, "reasoning.context").Exists(), "Lite 专属 reasoning.context 必须删除")
	require.Equal(t, "medium", gjson.GetBytes(body, "reasoning.effort").String())
	require.Equal(t, "function", gjson.GetBytes(body, `tools.#(name=="functions__exec").type`).String())
	require.True(t, gjson.GetBytes(body, `tools.#(name=="functions__exec").parameters.properties.input`).Exists(), "custom 子工具降级为 input 参数 function")
	require.True(t, gjson.GetBytes(body, `tools.#(name=="functions__wait")`).Exists())
	require.True(t, gjson.GetBytes(body, `tools.#(name=="mcp_demo__lookup")`).Exists())
	require.False(t, gjson.GetBytes(body, `tools.#(type=="namespace")`).Exists(), "namespace 声明必须全部摊平")
	require.False(t, gjson.GetBytes(body, `tools.#(type=="custom")`).Exists())
}

func requireResponsesLiteRestoredCustomCall(t *testing.T, wire string) {
	t.Helper()
	require.Contains(t, wire, `"type":"custom_tool_call"`)
	require.Contains(t, wire, `"namespace":"functions"`)
	require.Contains(t, wire, `"name":"exec"`)
	require.Contains(t, wire, `"input":"pwd"`)
	require.NotContains(t, wire, `functions__exec`, "客户端不得看到摊平名")
}

// Scenario: 开关关闭时 DeepSeek 原生路径保持现状，Lite 形态原样透传。
func TestForwardResponses_LiteDowngradeDisabledKeepsNativeBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := responsesLiteDowngradeTestBody()
	c, _ := responsesLiteDowngradeTestContext(body, true)
	upstream := responsesLiteDowngradeNativeUpstream()
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	_, err := svc.Forward(context.Background(), c, responsesLiteDowngradeDeepSeekAccount(false), body)
	require.NoError(t, err)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://api.deepseek.com/responses", upstream.lastReq.URL.String())
	require.Equal(t, "additional_tools", gjson.GetBytes(upstream.lastBody, "input.0.type").String())
	require.Equal(t, "all_turns", gjson.GetBytes(upstream.lastBody, "reasoning.context").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "tools").Exists())
}

// Scenario: 开关开启时 DeepSeek 原生路径把 Lite 降级为标准 Responses，并把上游的
// functions__exec 调用还原为带 namespace 的 custom_tool_call。
func TestForwardResponses_LiteDowngradeEnabledDeepSeekNative(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := responsesLiteDowngradeTestBody()
	c, rec := responsesLiteDowngradeTestContext(body, true)
	upstream := responsesLiteDowngradeNativeUpstream()
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := svc.Forward(context.Background(), c, responsesLiteDowngradeDeepSeekAccount(true), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "https://api.deepseek.com/responses", upstream.lastReq.URL.String())
	requireResponsesLiteDowngradedUpstream(t, upstream)
	require.Equal(t, "deepseek-reasoner", gjson.GetBytes(upstream.lastBody, "model").String())
	requireResponsesLiteRestoredCustomCall(t, rec.Body.String())
}

// Scenario: 非 Lite 请求即使开关开启也不做任何降级。
func TestForwardResponses_LiteDowngradeIgnoresNonLiteRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := responsesLiteDowngradeTestBody()
	c, _ := responsesLiteDowngradeTestContext(body, false)
	upstream := responsesLiteDowngradeNativeUpstream()
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	_, err := svc.Forward(context.Background(), c, responsesLiteDowngradeDeepSeekAccount(true), body)
	require.NoError(t, err)
	require.Equal(t, "additional_tools", gjson.GetBytes(upstream.lastBody, "input.0.type").String())
	require.Equal(t, "all_turns", gjson.GetBytes(upstream.lastBody, "reasoning.context").String())
}

// Scenario: OpenAI API-key 托管路径与 passthrough 路径开关开启时同样降级；关闭时保持
// Lite 形态并继续透传 Lite 头。
func TestForwardResponses_LiteDowngradeOpenAIAPIKeyPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name        string
		passthrough bool
	}{
		{name: "managed", passthrough: false},
		{name: "passthrough", passthrough: true},
	} {
		t.Run(tc.name+"/enabled", func(t *testing.T) {
			body := responsesLiteDowngradeTestBody()
			c, rec := responsesLiteDowngradeTestContext(body, true)
			upstream := responsesLiteDowngradeNativeUpstream()
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

			_, err := svc.Forward(context.Background(), c, responsesLiteDowngradeOpenAIAccount(true, tc.passthrough), body)
			require.NoError(t, err)
			requireResponsesLiteDowngradedUpstream(t, upstream)
			requireResponsesLiteRestoredCustomCall(t, rec.Body.String())
		})
		t.Run(tc.name+"/disabled", func(t *testing.T) {
			body := responsesLiteDowngradeTestBody()
			c, _ := responsesLiteDowngradeTestContext(body, true)
			upstream := responsesLiteDowngradeNativeUpstream()
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

			_, err := svc.Forward(context.Background(), c, responsesLiteDowngradeOpenAIAccount(false, tc.passthrough), body)
			require.NoError(t, err)
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, "true", upstream.lastReq.Header.Get(responsesLiteHeader), "开关关闭时 OpenAI API-key 保持 Lite 头透传")
			require.Equal(t, "additional_tools", gjson.GetBytes(upstream.lastBody, "input.0.type").String())
		})
	}
}

// Scenario: chat 回退桥接对 Lite 形态无条件生效（不依赖开关）：functions 命名空间内的
// custom exec 降级为 function 工具，回程还原为带 namespace 的 custom_tool_call。
func TestForwardResponses_ChatFallbackHandlesLiteFunctionsNamespaceWithoutDowngrade(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := responsesLiteDowngradeTestBody()
	c, rec := responsesLiteDowngradeTestContext(body, true)
	chunk := func(delta string) string {
		return "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"model\":\"deepseek-reasoner\",\"choices\":[{\"index\":0,\"delta\":" + delta + ",\"finish_reason\":null}]}\n\n"
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			chunk(`{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"functions__exec","arguments":"{\"input\":\"pwd\"}"}}]}`) +
				chunk(`{"tool_calls":[{"index":1,"id":"call_2","type":"function","function":{"name":"functions__wait","arguments":"{\"cell_id\":\"1\"}"}}]}`) +
				chunk(`{"tool_calls":[{"index":2,"id":"call_3","type":"function","function":{"name":"mcp_demo__lookup","arguments":"{}"}}]}`) +
				"data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"model\":\"deepseek-reasoner\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\n" +
				"data: [DONE]\n\n",
		)),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := forceChatResponsesFallbackAccount()
	account.Credentials["model_mapping"] = map[string]any{"gpt-6-astra": "deepseek-reasoner"}

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "http://upstream.example/v1/chat/completions", upstream.lastReq.URL.String())
	chatBody := upstream.lastBody
	require.True(t, gjson.GetBytes(chatBody, `tools.#(function.name=="functions__exec").function.parameters.properties.input`).Exists())
	require.True(t, gjson.GetBytes(chatBody, `tools.#(function.name=="functions__wait")`).Exists())
	require.True(t, gjson.GetBytes(chatBody, `tools.#(function.name=="mcp_demo__lookup")`).Exists())
	require.Equal(t, "system", gjson.GetBytes(chatBody, "messages.0.role").String())
	require.Equal(t, "BASE", gjson.GetBytes(chatBody, "messages.0.content").String())

	var execItem, waitItem, lookupItem gjson.Result
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") || !strings.Contains(line, "response.output_item.done") {
			continue
		}
		item := gjson.Get(strings.TrimPrefix(line, "data: "), "item")
		switch item.Get("call_id").String() {
		case "call_1":
			execItem = item
		case "call_2":
			waitItem = item
		case "call_3":
			lookupItem = item
		}
	}
	require.True(t, execItem.Exists())
	require.Equal(t, "custom_tool_call", execItem.Get("type").String())
	require.Equal(t, "exec", execItem.Get("name").String())
	require.Equal(t, "functions", execItem.Get("namespace").String())
	require.Equal(t, "pwd", execItem.Get("input").String())
	require.True(t, waitItem.Exists())
	require.Equal(t, "function_call", waitItem.Get("type").String())
	require.Equal(t, "wait", waitItem.Get("name").String())
	require.Equal(t, "functions", waitItem.Get("namespace").String())
	require.True(t, lookupItem.Exists())
	require.Equal(t, "function_call", lookupItem.Get("type").String())
	require.Equal(t, "lookup", lookupItem.Get("name").String())
	require.Equal(t, "mcp_demo", lookupItem.Get("namespace").String())
}

// Scenario: 开关访问器只对 API key 且 openai/CN 供应商账号生效。
func TestAccount_IsOpenAIResponsesLiteDowngradeEnabled(t *testing.T) {
	enabled := map[string]any{accountExtraKeyOpenAIResponsesLiteDowngrade: true}
	cases := []struct {
		name    string
		account *Account
		want    bool
	}{
		{name: "nil", account: nil, want: false},
		{name: "openai apikey enabled", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: enabled}, want: true},
		{name: "deepseek apikey enabled", account: &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Extra: enabled}, want: true},
		{name: "kimi apikey enabled", account: &Account{Platform: PlatformKimi, Type: AccountTypeAPIKey, Extra: enabled}, want: true},
		{name: "openai oauth", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: enabled}, want: false},
		{name: "grok apikey", account: &Account{Platform: PlatformGrok, Type: AccountTypeAPIKey, Extra: enabled}, want: false},
		{name: "anthropic apikey", account: &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Extra: enabled}, want: false},
		{name: "missing", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{}}, want: false},
		{name: "nil extra", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, want: false},
		{name: "non-bool", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{accountExtraKeyOpenAIResponsesLiteDowngrade: "true"}}, want: false},
		{name: "false", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{accountExtraKeyOpenAIResponsesLiteDowngrade: false}}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.account.IsOpenAIResponsesLiteDowngradeEnabled())
		})
	}
}

// Scenario: 降级辅助函数只在 Lite 头存在且开关开启时改写 body 并删除入站头；
// reasoning 只剩 context 时整体删除。
func TestApplyOpenAIResponsesLiteDowngrade_Body(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"m","input":[{"type":"additional_tools","role":"developer","tools":[{"type":"function","name":"f"}]},{"role":"user","content":"hi"}],"reasoning":{"context":"all_turns"}}`)
	c, _ := responsesLiteDowngradeTestContext(body, true)
	account := responsesLiteDowngradeDeepSeekAccount(true)

	out, changed, err := applyOpenAIResponsesLiteDowngrade(c, account, body)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, openAIResponsesLiteDowngraded(c))
	require.Empty(t, c.GetHeader(responsesLiteHeader))
	require.Equal(t, "f", gjson.GetBytes(out, "tools.0.name").String())
	require.Len(t, gjson.GetBytes(out, "input").Array(), 1)
	require.False(t, gjson.GetBytes(out, "reasoning").Exists(), "只剩 context 的 reasoning 整体删除")

	// 未开启：原样返回，头保留。
	c2, _ := responsesLiteDowngradeTestContext(body, true)
	out2, changed2, err := applyOpenAIResponsesLiteDowngrade(c2, responsesLiteDowngradeDeepSeekAccount(false), body)
	require.NoError(t, err)
	require.False(t, changed2)
	require.Equal(t, body, out2)
	require.Equal(t, "true", c2.GetHeader(responsesLiteHeader))
	require.False(t, openAIResponsesLiteDowngraded(c2))
}
