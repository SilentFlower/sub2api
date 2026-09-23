//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type responsesLitePolicySettingRepoStub struct {
	mu       sync.Mutex
	values   map[string]string
	getCalls atomic.Int32
	delay    time.Duration
}

func (s *responsesLitePolicySettingRepoStub) Get(context.Context, string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *responsesLitePolicySettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	s.getCalls.Add(1)
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if value, ok := s.values[key]; ok {
		return value, nil
	}
	return "", ErrSettingNotFound
}

func (s *responsesLitePolicySettingRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (s *responsesLitePolicySettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	// 网关运行时设置（如 TTFT 口径）在响应处理阶段批量读取，缺失 key 按未配置处理。
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (s *responsesLitePolicySettingRepoStub) SetMultiple(_ context.Context, settings map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		s.values = make(map[string]string)
	}
	for key, value := range settings {
		s.values[key] = value
	}
	return nil
}

func (s *responsesLitePolicySettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]string, len(s.values))
	for key, value := range s.values {
		result[key] = value
	}
	return result, nil
}

func (s *responsesLitePolicySettingRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func TestNormalizeOpenAIResponsesLiteHeaderBlockedModels(t *testing.T) {
	t.Run("trim and stable deduplicate", func(t *testing.T) {
		got, err := NormalizeOpenAIResponsesLiteHeaderBlockedModels([]string{
			" gpt-5.4 ",
			"gpt-5.4*",
			"gpt-5.4",
		})

		require.NoError(t, err)
		require.Equal(t, []string{"gpt-5.4", "gpt-5.4*"}, got)
	})

	for _, rules := range [][]string{
		{""},
		{"   "},
		{"gpt-5.*-mini"},
		{"gpt-5.**"},
	} {
		rules := rules
		t.Run("reject invalid rule", func(t *testing.T) {
			_, err := NormalizeOpenAIResponsesLiteHeaderBlockedModels(rules)
			require.Error(t, err)
		})
	}
}

func TestOpenAIResponsesLitePolicy_GPT55CompatibilityAcrossTransports(t *testing.T) {
	for _, tt := range []struct {
		name        string
		accountType string
		model       string
		blocked     string
		wantLite    bool
	}{
		{"OAuth 空列表仍禁用 GPT-5.5", AccountTypeOAuth, "gpt-5.5", "[]", false},
		{"OAuth 移除规则仍禁用 GPT-5.5", AccountTypeOAuth, "gpt-5.5", `["gpt-5.4"]`, false},
		{"SetupToken 空列表仍禁用 GPT-5.5", AccountTypeSetupToken, "gpt-5.5", "[]", false},
		{"APIKey 空列表允许 GPT-5.5", AccountTypeAPIKey, "gpt-5.5", "[]", true},
		{"APIKey 默认阻止 GPT-5.5", AccountTypeAPIKey, "gpt-5.5", defaultOpenAIResponsesLiteHeaderBlockedModelsJSON, false},
		{"OAuth 空列表允许其它模型", AccountTypeOAuth, "gpt-5.4", "[]", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			account := &Account{Platform: PlatformOpenAI, Type: tt.accountType}
			svc := &OpenAIGatewayService{settingService: NewSettingService(&responsesLitePolicySettingRepoStub{
				values: map[string]string{SettingKeyOpenAIResponsesLiteHeaderBlockedModels: tt.blocked},
			}, &config.Config{})}
			// 客户端别名不同于最终模型，策略必须以账号映射后的模型为准。
			body := []byte(`{"type":"response.create","model":"client-alias","input":[{"type":"additional_tools","role":"developer","tools":[{"type":"namespace","name":"collaboration","tools":[]}]}],"reasoning":{"context":"current_turn"},"client_metadata":{"ws_request_header_x_openai_internal_codex_responses_lite":"true","keep":"yes"}}`)
			original := append([]byte(nil), body...)

			httpBody, httpAllowed, err := svc.applyOpenAIResponsesLiteHTTPBodyPolicy(ctx, account, body, tt.model, "true")
			require.NoError(t, err)
			require.Equal(t, tt.wantLite, httpAllowed)
			req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			req.Header.Set(responsesLiteHeader, "true")
			svc.enforceOpenAIResponsesLiteHTTPHeader(ctx, req, account, tt.model)
			require.Equal(t, tt.wantLite, isOpenAIResponsesLiteHeader(req.Header.Get(responsesLiteHeader)))

			wsBody, wsAllowed, err := svc.applyOpenAIResponsesLiteWebSocketPolicy(ctx, account, body, tt.model)
			require.NoError(t, err)
			require.Equal(t, tt.wantLite, wsAllowed)
			require.Equal(t, tt.wantLite, isOpenAIResponsesLiteWebSocketPayload(wsBody))
			require.Equal(t, "yes", gjson.GetBytes(wsBody, "client_metadata.keep").String())
			if !tt.wantLite {
				require.Equal(t, original, httpBody)
				require.Equal(t, gjson.GetBytes(original, "input").Raw, gjson.GetBytes(wsBody, "input").Raw)
				require.Equal(t, "current_turn", gjson.GetBytes(wsBody, "reasoning.context").String())
			}

			bridgeBody := []byte(`{"model":"` + tt.model + `","input":[]}`)
			svc.applyOpenAIResponsesLiteWSHTTPBridgePolicy(ctx, req, account, body, bridgeBody, "client-alias")
			require.Equal(t, tt.wantLite, isOpenAIResponsesLiteHeader(req.Header.Get(responsesLiteHeader)))
			require.Equal(t, original, body, "兼容处理不得修改入站数据，以便后续账号重试")
		})
	}
}

func TestSettingService_OpenAIResponsesLiteHeaderBlockedModels_DefaultAndExplicitEmpty(t *testing.T) {
	t.Run("missing key uses non-empty defaults", func(t *testing.T) {
		repo := &responsesLitePolicySettingRepoStub{values: map[string]string{}}
		svc := NewSettingService(repo, &config.Config{})

		settings, err := svc.GetAllSettings(context.Background())

		require.NoError(t, err)
		require.Equal(t, defaultOpenAIResponsesLiteHeaderBlockedModelsCopy(), settings.OpenAIResponsesLiteHeaderBlockedModels)
		require.True(t, svc.ShouldBlockOpenAIResponsesLite(context.Background(), "gpt-5.4-mini"))
	})

	t.Run("explicit empty array stays empty", func(t *testing.T) {
		repo := &responsesLitePolicySettingRepoStub{values: map[string]string{
			SettingKeyOpenAIResponsesLiteHeaderBlockedModels: "[]",
		}}
		svc := NewSettingService(repo, &config.Config{})

		settings, err := svc.GetAllSettings(context.Background())

		require.NoError(t, err)
		require.Empty(t, settings.OpenAIResponsesLiteHeaderBlockedModels)
		require.False(t, svc.ShouldBlockOpenAIResponsesLite(context.Background(), "gpt-5.4-mini"))
	})
}

func TestSettingService_OpenAIResponsesLiteHeaderBlockedModels_MatchingAndRefresh(t *testing.T) {
	repo := &responsesLitePolicySettingRepoStub{values: map[string]string{
		SettingKeyOpenAIResponsesLiteHeaderBlockedModels: `["gpt-5.4*","gpt-5.5"]`,
	}}
	svc := NewSettingService(repo, &config.Config{})

	require.True(t, svc.ShouldBlockOpenAIResponsesLite(context.Background(), "gpt-5.4-mini"))
	require.True(t, svc.ShouldBlockOpenAIResponsesLite(context.Background(), "gpt-5.5"))
	require.False(t, svc.ShouldBlockOpenAIResponsesLite(context.Background(), "gpt-5.6-terra"))
	require.Equal(t, int32(1), repo.getCalls.Load())

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		OpenAIResponsesLiteHeaderBlockedModels: []string{},
	})

	require.NoError(t, err)
	require.False(t, svc.ShouldBlockOpenAIResponsesLite(context.Background(), "gpt-5.4-mini"))
	require.Equal(t, int32(1), repo.getCalls.Load())
}

func TestSettingService_OpenAIResponsesLiteHeaderBlockedModels_Singleflight(t *testing.T) {
	repo := &responsesLitePolicySettingRepoStub{
		values: map[string]string{
			SettingKeyOpenAIResponsesLiteHeaderBlockedModels: `["gpt-5.5"]`,
		},
		delay: 20 * time.Millisecond,
	}
	svc := NewSettingService(repo, &config.Config{})

	const workers = 20
	var wg sync.WaitGroup
	results := make(chan bool, workers)
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			results <- svc.ShouldBlockOpenAIResponsesLite(context.Background(), "gpt-5.5")
		}()
	}
	wg.Wait()
	close(results)
	for blocked := range results {
		require.True(t, blocked)
	}

	require.Equal(t, int32(1), repo.getCalls.Load())
}
