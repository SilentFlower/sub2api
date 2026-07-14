//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/websearch"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newOpenAIResponsesWebSearchTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c, recorder
}

func enableOpenAIResponsesWebSearchTestManager(t *testing.T) {
	t.Helper()
	setGlobalWebSearchConfig(&WebSearchEmulationConfig{
		Enabled:   true,
		Providers: []WebSearchProviderConfig{{Type: websearch.ProviderTypeAnySearch}},
	})
	SetWebSearchManager(websearch.NewManager([]websearch.ProviderConfig{{Type: websearch.ProviderTypeAnySearch}}, nil))
	t.Cleanup(func() {
		SetWebSearchManager(nil)
		clearGlobalWebSearchConfig()
	})
}

func TestHandleOpenAIResponsesWebSearchRejectsChatFallback(t *testing.T) {
	c, recorder := newOpenAIResponsesWebSearchTestContext()
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
			featureKeyWebSearchEmulation:        WebSearchModeDisabled,
		},
	}
	body := []byte(`{"model":"deepseek-v4-pro","input":"latest","tools":[{"type":"web_search"},{"type":"function","name":"other","parameters":{"type":"object"}}]}`)

	result, handled, err := (&OpenAIGatewayService{}).handleOpenAIResponsesWebSearch(context.Background(), c, account, body)
	require.True(t, handled)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "cannot execute")
}

func TestOpenAIResponsesWebSearchToolChoiceDecision(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		emulate    bool
		rejectChat bool
		choice     openAIResponsesToolChoiceKind
	}{
		{name: "absent", body: `{"model":"m","input":"q","tools":[{"type":"web_search"}]}`, emulate: true, rejectChat: true, choice: openAIResponsesToolChoiceAbsent},
		{name: "auto", body: `{"model":"m","input":"q","tools":[{"type":"web_search"}],"tool_choice":"auto"}`, emulate: true, rejectChat: true, choice: openAIResponsesToolChoiceAuto},
		{name: "required", body: `{"model":"m","input":"q","tools":[{"type":"web_search"}],"tool_choice":"required"}`, emulate: true, rejectChat: true, choice: openAIResponsesToolChoiceRequired},
		{name: "none", body: `{"model":"m","input":"q","tools":[{"type":"web_search"}],"tool_choice":"none"}`, emulate: false, rejectChat: false, choice: openAIResponsesToolChoiceNone},
		{name: "forced web search in mixed tools", body: `{"model":"m","input":"q","tools":[{"type":"web_search"},{"type":"function","name":"other"}],"tool_choice":{"type":"web_search"}}`, emulate: true, rejectChat: true, choice: openAIResponsesToolChoiceForcedWebSearch},
		{name: "forced other in mixed tools", body: `{"model":"m","input":"q","tools":[{"type":"web_search"},{"type":"function","name":"other"}],"tool_choice":{"type":"function","name":"other"}}`, emulate: false, rejectChat: false, choice: openAIResponsesToolChoiceForcedOther},
		{name: "forced other without other tool", body: `{"model":"m","input":"q","tools":[{"type":"web_search"}],"tool_choice":{"type":"function","name":"other"}}`, emulate: false, rejectChat: true, choice: openAIResponsesToolChoiceForcedOther},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request, toolCount, webSearchCount, err := parseOpenAIResponsesWebSearchRequest([]byte(tc.body))
			require.NoError(t, err)
			require.Equal(t, tc.choice, request.ToolChoice.Kind)
			require.Equal(t, tc.emulate, shouldEmulateOpenAIResponsesWebSearch(toolCount, webSearchCount, request.ToolChoice.Kind))
			require.Equal(t, tc.rejectChat, shouldRejectChatFallbackWebSearch(toolCount, webSearchCount, request.ToolChoice.Kind))
		})
	}
}

func TestHandleOpenAIResponsesWebSearchHonorsToolChoiceNone(t *testing.T) {
	c, _ := newOpenAIResponsesWebSearchTestContext()
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
			featureKeyWebSearchEmulation:        WebSearchModeEnabled,
		},
	}
	body := []byte(`{"model":"deepseek-v4-pro","input":"latest","tools":[{"type":"web_search"}],"tool_choice":"none"}`)

	result, handled, err := (&OpenAIGatewayService{}).handleOpenAIResponsesWebSearch(context.Background(), c, account, body)
	require.False(t, handled)
	require.Nil(t, result)
	require.NoError(t, err)
}

func TestHandleOpenAIResponsesWebSearchReadsAdditionalTools(t *testing.T) {
	c, recorder := newOpenAIResponsesWebSearchTestContext()
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
			featureKeyWebSearchEmulation:        WebSearchModeDisabled,
		},
	}
	body := []byte(`{"model":"deepseek-v4-pro","input":[{"role":"user","content":"latest"},{"type":"additional_tools","tools":[{"type":"web_search"}]}]}`)

	result, handled, err := (&OpenAIGatewayService{}).handleOpenAIResponsesWebSearch(context.Background(), c, account, body)
	require.True(t, handled)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestHandleOpenAIResponsesWebSearchPassesMixedToolsToNativeResponses(t *testing.T) {
	c, _ := newOpenAIResponsesWebSearchTestContext()
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceResponses),
			featureKeyWebSearchEmulation:        WebSearchModeDisabled,
		},
	}
	body := []byte(`{"model":"deepseek-v4-pro","input":"latest","tools":[{"type":"web_search"},{"type":"function","name":"other","parameters":{"type":"object"}}]}`)

	result, handled, err := (&OpenAIGatewayService{}).handleOpenAIResponsesWebSearch(context.Background(), c, account, body)
	require.False(t, handled)
	require.Nil(t, result)
	require.NoError(t, err)
}

func TestOpenAIWebSearchEmulationDefaultFollowsChannel(t *testing.T) {
	groupID := int64(9)
	channel := &Channel{
		ID:     2,
		Status: StatusActive,
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: map[string]any{PlatformOpenAI: true},
		},
	}
	service := &OpenAIGatewayService{channelService: newChannelServiceWithCache(groupID, channel)}
	c, _ := newOpenAIResponsesWebSearchTestContext()
	c.Set("api_key", &APIKey{GroupID: &groupID})
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	require.True(t, service.isOpenAIWebSearchEmulationEnabled(context.Background(), c, account))
}

func TestHandleOpenAIResponsesWebSearchRejectsJSONSchemaConflict(t *testing.T) {
	enableOpenAIResponsesWebSearchTestManager(t)
	c, recorder := newOpenAIResponsesWebSearchTestContext()
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{featureKeyWebSearchEmulation: WebSearchModeEnabled}}
	body := []byte(`{"model":"deepseek-v4-pro","input":"latest","tools":[{"type":"web_search"}],"text":{"format":{"type":"json_schema","schema":{"type":"object"}}}}`)

	result, handled, err := (&OpenAIGatewayService{settingService: &SettingService{}}).handleOpenAIResponsesWebSearch(context.Background(), c, account, body)
	require.True(t, handled)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "incompatible")
}

func TestHandleOpenAIResponsesWebSearchWritesResponseAndBillsOnce(t *testing.T) {
	enableOpenAIResponsesWebSearchTestManager(t)
	c, recorder := newOpenAIResponsesWebSearchTestContext()
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{featureKeyWebSearchEmulation: WebSearchModeEnabled}}
	service := &OpenAIGatewayService{
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(_ context.Context, _ *Account, query string, maxResults int) (*websearch.SearchResponse, string, error) {
			require.Equal(t, "latest", query)
			require.Equal(t, 3, maxResults)
			return &websearch.SearchResponse{Query: query, Results: []websearch.SearchResult{
				{URL: "https://docs.example.com/a", Title: "Allowed", Snippet: "First"},
				{URL: "https://blocked.test/b", Title: "Blocked", Snippet: "Second"},
			}}, "anysearch", nil
		},
	}
	body := []byte(`{"model":"deepseek-v4-pro","input":[{"role":"user","content":[{"type":"input_text","text":"latest"}]}],"tools":[{"type":"web_search","search_context_size":"low","filters":{"allowed_domains":["example.com"]}}]}`)

	result, handled, err := service.handleOpenAIResponsesWebSearch(context.Background(), c, account, body)
	require.True(t, handled)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.WebSearchCalls)
	require.Equal(t, http.StatusOK, recorder.Code)

	var response apicompat.ResponsesResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Output, 2)
	require.Equal(t, "web_search_call", response.Output[0].Type)
	require.Equal(t, "latest", response.Output[0].Action.Query)
	part := response.Output[1].Content[0]
	require.Len(t, part.Annotations, 1)
	annotation := part.Annotations[0]
	require.Equal(t, annotation.URL, string([]rune(part.Text)[annotation.StartIndex:annotation.EndIndex]))
	require.NotContains(t, part.Text, "blocked.test")
}

func TestHandleOpenAIResponsesWebSearchStreamLifecycle(t *testing.T) {
	enableOpenAIResponsesWebSearchTestManager(t)
	c, recorder := newOpenAIResponsesWebSearchTestContext()
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{featureKeyWebSearchEmulation: WebSearchModeEnabled}}
	service := &OpenAIGatewayService{
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(_ context.Context, _ *Account, query string, _ int) (*websearch.SearchResponse, string, error) {
			return &websearch.SearchResponse{Query: query, Results: []websearch.SearchResult{{URL: "https://example.com", Title: "Example", Snippet: "Result"}}}, "anysearch", nil
		},
	}
	body := []byte(`{"model":"deepseek-v4-pro","input":"latest","stream":true,"tools":[{"type":"web_search"}]}`)

	result, handled, err := service.handleOpenAIResponsesWebSearch(context.Background(), c, account, body)
	require.True(t, handled)
	require.NoError(t, err)
	require.Equal(t, 1, result.WebSearchCalls)
	wire := recorder.Body.String()
	ordered := []string{
		"event: response.created",
		"event: response.output_item.added",
		"event: response.output_item.done",
		"event: response.content_part.added",
		"event: response.output_text.delta",
		"event: response.output_text.annotation.added",
		"event: response.output_text.done",
		"event: response.content_part.done",
		"event: response.completed",
		"data: [DONE]",
	}
	position := -1
	for _, marker := range ordered {
		next := strings.Index(wire[position+1:], marker)
		require.NotEqual(t, -1, next, marker)
		position += next + 1
	}
}

func TestHandleOpenAIResponsesWebSearchProviderFailureDoesNotReturnBillableResult(t *testing.T) {
	enableOpenAIResponsesWebSearchTestManager(t)
	c, recorder := newOpenAIResponsesWebSearchTestContext()
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{featureKeyWebSearchEmulation: WebSearchModeEnabled}}
	service := &OpenAIGatewayService{
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(context.Context, *Account, string, int) (*websearch.SearchResponse, string, error) {
			return nil, "", errors.New("provider failed")
		},
	}
	body := []byte(`{"model":"deepseek-v4-pro","input":"latest","tools":[{"type":"web_search"}]}`)

	result, handled, err := service.handleOpenAIResponsesWebSearch(context.Background(), c, account, body)
	require.True(t, handled)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, http.StatusBadGateway, recorder.Code)
}

func TestHandleOpenAIResponsesWebSearchEmptyProviderResponseDoesNotReturnBillableResult(t *testing.T) {
	enableOpenAIResponsesWebSearchTestManager(t)
	c, recorder := newOpenAIResponsesWebSearchTestContext()
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{featureKeyWebSearchEmulation: WebSearchModeEnabled}}
	service := &OpenAIGatewayService{
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(context.Context, *Account, string, int) (*websearch.SearchResponse, string, error) {
			return nil, "anysearch", nil
		},
	}
	body := []byte(`{"model":"deepseek-v4-pro","input":"latest","tools":[{"type":"web_search"}]}`)

	result, handled, err := service.handleOpenAIResponsesWebSearch(context.Background(), c, account, body)
	require.True(t, handled)
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, http.StatusBadGateway, recorder.Code)
}

func TestHandleOpenAIResponsesWebSearchReturnsFailoverForProxyFailure(t *testing.T) {
	enableOpenAIResponsesWebSearchTestManager(t)
	c, _ := newOpenAIResponsesWebSearchTestContext()
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{featureKeyWebSearchEmulation: WebSearchModeEnabled}}
	service := &OpenAIGatewayService{
		settingService: &SettingService{},
		openAIWebSearchExecutor: func(context.Context, *Account, string, int) (*websearch.SearchResponse, string, error) {
			return nil, "", websearch.ErrProxyUnavailable
		},
	}

	result, handled, err := service.handleOpenAIResponsesWebSearch(context.Background(), c, account, []byte(`{"model":"m","input":"latest","tools":[{"type":"web_search"}]}`))
	require.True(t, handled)
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
}

func TestHandleOpenAIResponsesWebSearchRejectsEmptyQueryAndUnsupportedFields(t *testing.T) {
	enableOpenAIResponsesWebSearchTestManager(t)
	tests := []struct {
		name  string
		body  string
		param string
	}{
		{name: "empty query", body: `{"model":"m","input":"  ","tools":[{"type":"web_search"}]}`, param: `"param":"input"`},
		{name: "unsupported user location", body: `{"model":"m","input":"latest","tools":[{"type":"web_search","user_location":{"type":"approximate","country":"CN"}}]}`, param: `"param":"tools"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, recorder := newOpenAIResponsesWebSearchTestContext()
			account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{featureKeyWebSearchEmulation: WebSearchModeEnabled}}
			service := &OpenAIGatewayService{settingService: &SettingService{}}

			result, handled, err := service.handleOpenAIResponsesWebSearch(context.Background(), c, account, []byte(tc.body))
			require.True(t, handled)
			require.Nil(t, result)
			require.Error(t, err)
			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Contains(t, recorder.Body.String(), tc.param)
		})
	}
}
