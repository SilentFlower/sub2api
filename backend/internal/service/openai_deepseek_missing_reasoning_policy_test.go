//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type deepSeekMissingReasoningErrorSettingRepo struct {
	*responsesLitePolicySettingRepoStub
	err error
}

func (s *deepSeekMissingReasoningErrorSettingRepo) GetValue(context.Context, string) (string, error) {
	return "", s.err
}

type deepSeekMissingReasoningBlockingSettingRepo struct {
	*responsesLitePolicySettingRepoStub
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *deepSeekMissingReasoningBlockingSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	s.getCalls.Add(1)
	s.mu.Lock()
	value, ok := s.values[key]
	s.mu.Unlock()
	s.once.Do(func() { close(s.started) })
	<-s.release
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func TestApplyDeepSeekMissingReasoningPolicy_BehaviorMatrix(t *testing.T) {
	tests := []struct {
		name         string
		model        string
		enabled      bool
		body         string
		wantChanged  bool
		wantMissing  int
		wantDisabled bool
		wantEffort   string
	}{
		{
			name:         "缺失推理内容时关闭 thinking",
			model:        " DeepSeek-V4-Pro ",
			enabled:      true,
			body:         `{"messages":[{"role":"assistant","tool_calls":[{"id":"call_1"}]}],"thinking":{"type":"enabled"},"reasoning_effort":"high"}`,
			wantChanged:  true,
			wantMissing:  1,
			wantDisabled: true,
		},
		{
			name:       "reasoning_content 非空时保持请求",
			model:      "deepseek-v4-pro",
			enabled:    true,
			body:       `{"messages":[{"role":"assistant","reasoning_content":"需要调用工具","tool_calls":[{"id":"call_1"}]}],"reasoning_effort":"high"}`,
			wantEffort: "high",
		},
		{
			name:       "reasoning 兼容别名非空时保持请求",
			model:      "deepseek-v4-pro",
			enabled:    true,
			body:       `{"messages":[{"role":"assistant","reasoning":"兼容推理","tool_calls":[{"id":"call_1"}]}],"reasoning_effort":"medium"}`,
			wantEffort: "medium",
		},
		{
			name:         "空白推理内容不可阻止降级",
			model:        "deepseek-v4-pro",
			enabled:      true,
			body:         `{"messages":[{"role":"assistant","reasoning_content":"  ","reasoning":"\n","tool_calls":[{"id":"call_1"}]}]}`,
			wantChanged:  true,
			wantMissing:  1,
			wantDisabled: true,
		},
		{
			name:         "null 和非字符串推理内容不可阻止降级",
			model:        "deepseek-v4-pro",
			enabled:      true,
			body:         `{"messages":[{"role":"assistant","reasoning_content":null,"reasoning":{"text":"不可用"},"tool_calls":[{"id":"call_1"}]}]}`,
			wantChanged:  true,
			wantMissing:  1,
			wantDisabled: true,
		},
		{
			name:       "系统开关关闭时保持请求",
			model:      "deepseek-v4-pro",
			enabled:    false,
			body:       `{"messages":[{"role":"assistant","tool_calls":[{"id":"call_1"}]}],"reasoning_effort":"high"}`,
			wantEffort: "high",
		},
		{
			name:       "非 DeepSeek 模型保持请求",
			model:      "gpt-5.6-sol",
			enabled:    true,
			body:       `{"messages":[{"role":"assistant","tool_calls":[{"id":"call_1"}]}],"reasoning_effort":"high"}`,
			wantEffort: "high",
		},
		{
			name:    "没有非空工具调用历史时保持请求",
			model:   "deepseek-v4-pro",
			enabled: true,
			body:    `{"messages":[{"role":"assistant","tool_calls":[]},{"role":"user","tool_calls":[{"id":"call_1"}]}]}`,
		},
		{
			name:         "多条缺失历史统一降级并统计数量",
			model:        "deepseek-v4-pro",
			enabled:      true,
			body:         `{"messages":[{"role":"assistant","tool_calls":[{"id":"call_1"}]},{"role":"assistant","reasoning_content":"完整","tool_calls":[{"id":"call_2"}]},{"role":"assistant","tool_calls":[{"id":"call_3"}]}],"reasoning_effort":"high"}`,
			wantChanged:  true,
			wantMissing:  2,
			wantDisabled: true,
		},
		{
			name:         "已关闭 thinking 但仍有 effort 时只删除 effort",
			model:        "deepseek-v4-pro",
			enabled:      true,
			body:         `{"messages":[{"role":"assistant","tool_calls":[{"id":"call_1"}]}],"thinking":{"type":"disabled"},"reasoning_effort":"high"}`,
			wantChanged:  true,
			wantMissing:  1,
			wantDisabled: true,
		},
		{
			name:         "已关闭 thinking 且无 effort 时保持幂等",
			model:        "deepseek-v4-pro",
			enabled:      true,
			body:         `{"messages":[{"role":"assistant","tool_calls":[{"id":"call_1"}]}],"thinking":{"type":"disabled"}}`,
			wantMissing:  1,
			wantDisabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := []byte(tt.body)
			result, err := applyDeepSeekMissingReasoningPolicy(original, tt.model, tt.enabled)

			require.NoError(t, err)
			require.Equal(t, tt.wantChanged, result.changed)
			require.Equal(t, tt.wantMissing, result.missingCount)
			if !tt.wantChanged {
				require.Equal(t, original, result.body)
			}
			require.Equal(t, tt.wantDisabled, gjson.GetBytes(result.body, "thinking.type").String() == "disabled")
			require.Equal(t, tt.wantEffort, gjson.GetBytes(result.body, "reasoning_effort").String())
			if tt.wantChanged {
				require.False(t, gjson.GetBytes(result.body, "reasoning_effort").Exists())
			}
		})
	}
}

func TestApplyDeepSeekMissingReasoningPolicy_InvalidJSON(t *testing.T) {
	_, err := applyDeepSeekMissingReasoningPolicy([]byte(`{"messages":`), "deepseek-v4-pro", true)

	require.ErrorContains(t, err, "invalid chat completions request JSON")
}

func TestSettingService_DeepSeekMissingReasoningPolicy_DefaultCacheAndRefresh(t *testing.T) {
	t.Run("系统设置解析对缺失和显式关闭分别生效", func(t *testing.T) {
		svc := NewSettingService(nil, &config.Config{})

		require.True(t, svc.parseSettings(map[string]string{}).EnableDeepSeekMissingReasoningAutoDowngrade)
		require.False(t, svc.parseSettings(map[string]string{
			SettingKeyEnableDeepSeekMissingReasoningAutoDowngrade: "false",
		}).EnableDeepSeekMissingReasoningAutoDowngrade)
	})

	t.Run("缺失配置默认开启且复用缓存", func(t *testing.T) {
		repo := &responsesLitePolicySettingRepoStub{values: map[string]string{}}
		svc := NewSettingService(repo, &config.Config{})

		require.True(t, svc.IsDeepSeekMissingReasoningAutoDowngradeEnabled(context.Background()))
		require.True(t, svc.IsDeepSeekMissingReasoningAutoDowngradeEnabled(context.Background()))
		require.Equal(t, int32(1), repo.getCalls.Load())
	})

	t.Run("显式关闭后更新配置立即刷新", func(t *testing.T) {
		repo := &responsesLitePolicySettingRepoStub{values: map[string]string{
			SettingKeyEnableDeepSeekMissingReasoningAutoDowngrade: "false",
		}}
		svc := NewSettingService(repo, &config.Config{})

		require.False(t, svc.IsDeepSeekMissingReasoningAutoDowngradeEnabled(context.Background()))
		require.Equal(t, int32(1), repo.getCalls.Load())

		err := svc.UpdateSettings(context.Background(), &SystemSettings{
			EnableDeepSeekMissingReasoningAutoDowngrade: true,
			OpenAIResponsesLiteHeaderBlockedModels:      []string{},
		})

		require.NoError(t, err)
		require.True(t, svc.IsDeepSeekMissingReasoningAutoDowngradeEnabled(context.Background()))
		require.Equal(t, int32(1), repo.getCalls.Load())
	})

	t.Run("并发旧读取不会覆盖保存后的新缓存值", func(t *testing.T) {
		repo := &deepSeekMissingReasoningBlockingSettingRepo{
			responsesLitePolicySettingRepoStub: &responsesLitePolicySettingRepoStub{values: map[string]string{
				SettingKeyEnableDeepSeekMissingReasoningAutoDowngrade: "false",
			}},
			started: make(chan struct{}),
			release: make(chan struct{}),
		}
		svc := NewSettingService(repo, &config.Config{})
		readResult := make(chan bool, 1)
		go func() {
			readResult <- svc.IsDeepSeekMissingReasoningAutoDowngradeEnabled(context.Background())
		}()

		<-repo.started
		err := svc.UpdateSettings(context.Background(), &SystemSettings{
			EnableDeepSeekMissingReasoningAutoDowngrade: true,
			OpenAIResponsesLiteHeaderBlockedModels:      []string{},
		})
		require.NoError(t, err)
		close(repo.release)

		require.True(t, <-readResult)
		require.True(t, svc.IsDeepSeekMissingReasoningAutoDowngradeEnabled(context.Background()))
		require.Equal(t, int32(1), repo.getCalls.Load())
	})

	t.Run("读取异常短期回退默认开启", func(t *testing.T) {
		repo := &deepSeekMissingReasoningErrorSettingRepo{
			responsesLitePolicySettingRepoStub: &responsesLitePolicySettingRepoStub{values: map[string]string{}},
			err:                                errors.New("database unavailable"),
		}
		svc := NewSettingService(repo, &config.Config{})

		require.True(t, svc.IsDeepSeekMissingReasoningAutoDowngradeEnabled(context.Background()))
	})
}

func TestOpenAIGatewayService_DeepSeekMissingReasoningPolicy_LogsSafeFieldsOnRewrite(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	ctx := logger.IntoContext(context.Background(), zap.New(core))
	svc := &OpenAIGatewayService{}
	body := []byte(`{"messages":[{"role":"assistant","tool_calls":[{"id":"call_secret","function":{"arguments":"{\"secret\":\"value\"}"}}]}],"reasoning_effort":"high"}`)

	updated, err := svc.applyDeepSeekMissingReasoningAutoDowngrade(
		ctx,
		&Account{ID: 101},
		"deepseek-v4-pro",
		body,
		deepSeekMissingReasoningSourceChatCompletions,
	)

	require.NoError(t, err)
	require.Equal(t, "disabled", gjson.GetBytes(updated, "thinking.type").String())
	entries := observed.FilterMessage("DeepSeek 工具调用历史缺失推理内容，已自动关闭 thinking").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, "openai.deepseek_missing_reasoning_policy", fields["component"])
	require.EqualValues(t, 101, fields["account_id"])
	require.Equal(t, "deepseek-v4-pro", fields["upstream_model"])
	require.Equal(t, deepSeekMissingReasoningSourceChatCompletions, fields["source_path"])
	require.EqualValues(t, 1, fields["missing_assistant_tool_call_messages"])
	require.Equal(t, deepSeekMissingReasoningReasonAssistantToolCalls, fields["reason"])
	require.NotContains(t, fields, "body")
	require.NotContains(t, fields, "reasoning_content")
	require.NotContains(t, fields, "tool_arguments")
}
