//go:build unit

package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
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

func (s *responsesLitePolicySettingRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
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
