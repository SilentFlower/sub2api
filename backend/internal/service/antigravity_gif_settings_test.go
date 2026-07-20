//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestGetAntigravityGIFCompatibilitySettings_Defaults(t *testing.T) {
	service := NewSettingService(newMockSettingRepo(), &config.Config{})

	settings, err := service.GetAntigravityGIFCompatibilitySettings(context.Background())

	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.Equal(t, 8, settings.MaxFramesPerGIF)
}

func TestGetAntigravityGIFCompatibilitySettings_MergesMissingFieldsWithDefaults(t *testing.T) {
	repository := newMockSettingRepo()
	repository.data[SettingKeyAntigravityGIFCompatSettings] = `{"max_frames_per_gif":4}`
	service := NewSettingService(repository, &config.Config{})

	settings, err := service.GetAntigravityGIFCompatibilitySettings(context.Background())

	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.Equal(t, 4, settings.MaxFramesPerGIF)
}

func TestGetAntigravityGIFCompatibilitySettings_InvalidValuesFallback(t *testing.T) {
	tests := []struct {
		name            string
		stored          string
		expectedEnabled bool
	}{
		{name: "损坏 JSON", stored: "not-json", expectedEnabled: true},
		{name: "帧数越界", stored: `{"enabled":false,"max_frames_per_gif":0}`, expectedEnabled: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := newMockSettingRepo()
			repository.data[SettingKeyAntigravityGIFCompatSettings] = test.stored
			service := NewSettingService(repository, &config.Config{})

			settings, err := service.GetAntigravityGIFCompatibilitySettings(context.Background())

			require.NoError(t, err)
			require.Equal(t, test.expectedEnabled, settings.Enabled)
			require.Equal(t, 8, settings.MaxFramesPerGIF)
		})
	}
}

func TestGetAntigravityGIFCompatibilitySettings_PropagatesRepositoryError(t *testing.T) {
	service := NewSettingService(&errSettingRepo{
		mockSettingRepo: *newMockSettingRepo(),
		readErr:         errors.New("read failed"),
	}, &config.Config{})

	settings, err := service.GetAntigravityGIFCompatibilitySettings(context.Background())

	require.Nil(t, settings)
	require.ErrorContains(t, err, "get antigravity GIF settings")
}

func TestSetAntigravityGIFCompatibilitySettings_RoundTrip(t *testing.T) {
	repository := newMockSettingRepo()
	service := NewSettingService(repository, &config.Config{})

	err := service.SetAntigravityGIFCompatibilitySettings(context.Background(), &AntigravityGIFCompatibilitySettings{
		Enabled:         false,
		MaxFramesPerGIF: 12,
	})

	require.NoError(t, err)
	var stored AntigravityGIFCompatibilitySettings
	require.NoError(t, json.Unmarshal([]byte(repository.data[SettingKeyAntigravityGIFCompatSettings]), &stored))
	require.Equal(t, AntigravityGIFCompatibilitySettings{Enabled: false, MaxFramesPerGIF: 12}, stored)
}

func TestSetAntigravityGIFCompatibilitySettings_RejectsInvalidFrameLimit(t *testing.T) {
	service := NewSettingService(newMockSettingRepo(), &config.Config{})

	for _, value := range []int{0, 17} {
		err := service.SetAntigravityGIFCompatibilitySettings(context.Background(), &AntigravityGIFCompatibilitySettings{
			Enabled:         true,
			MaxFramesPerGIF: value,
		})

		require.Error(t, err)
		require.Equal(t, "ANTIGRAVITY_GIF_MAX_FRAMES_INVALID", infraerrors.Reason(err))
	}
}
