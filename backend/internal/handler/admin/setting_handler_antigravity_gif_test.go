//go:build unit

package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type antigravityGIFSettingRepoStub struct {
	values map[string]string
}

func (s *antigravityGIFSettingRepoStub) Get(_ context.Context, key string) (*service.Setting, error) {
	value, ok := s.values[key]
	if !ok {
		return nil, service.ErrSettingNotFound
	}
	return &service.Setting{Key: key, Value: value}, nil
}

func (s *antigravityGIFSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}

func (s *antigravityGIFSettingRepoStub) Set(_ context.Context, key, value string) error {
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.values[key] = value
	return nil
}

func (s *antigravityGIFSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (s *antigravityGIFSettingRepoStub) SetMultiple(_ context.Context, settings map[string]string) error {
	for key, value := range settings {
		if err := s.Set(context.Background(), key, value); err != nil {
			return err
		}
	}
	return nil
}

func (s *antigravityGIFSettingRepoStub) GetAll(_ context.Context) (map[string]string, error) {
	result := make(map[string]string, len(s.values))
	for key, value := range s.values {
		result[key] = value
	}
	return result, nil
}

func (s *antigravityGIFSettingRepoStub) Delete(_ context.Context, key string) error {
	delete(s.values, key)
	return nil
}

func TestSettingHandler_GetAntigravityGIFCompatibilitySettings_Defaults(t *testing.T) {
	recorder, context := newAntigravityGIFSettingHandlerContext(http.MethodGet, nil)
	handler := newAntigravityGIFSettingHandler(&antigravityGIFSettingRepoStub{values: map[string]string{}})

	handler.GetAntigravityGIFCompatibilitySettings(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Code int `json:"code"`
		Data struct {
			Enabled         bool `json:"enabled"`
			MaxFramesPerGIF int  `json:"max_frames_per_gif"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Zero(t, response.Code)
	require.True(t, response.Data.Enabled)
	require.Equal(t, 8, response.Data.MaxFramesPerGIF)
}

func TestSettingHandler_UpdateAntigravityGIFCompatibilitySettings(t *testing.T) {
	repository := &antigravityGIFSettingRepoStub{values: map[string]string{}}
	recorder, context := newAntigravityGIFSettingHandlerContext(http.MethodPut, []byte(`{"enabled":false,"max_frames_per_gif":12}`))
	handler := newAntigravityGIFSettingHandler(repository)

	handler.UpdateAntigravityGIFCompatibilitySettings(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"enabled":false,"max_frames_per_gif":12}`, repository.values[service.SettingKeyAntigravityGIFCompatSettings])
}

func TestSettingHandler_UpdateAntigravityGIFCompatibilitySettings_RejectsInvalidLimit(t *testing.T) {
	recorder, context := newAntigravityGIFSettingHandlerContext(http.MethodPut, []byte(`{"enabled":true,"max_frames_per_gif":17}`))
	handler := newAntigravityGIFSettingHandler(&antigravityGIFSettingRepoStub{values: map[string]string{}})

	handler.UpdateAntigravityGIFCompatibilitySettings(context)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	var response struct {
		Reason string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "ANTIGRAVITY_GIF_MAX_FRAMES_INVALID", response.Reason)
}

func newAntigravityGIFSettingHandler(repository service.SettingRepository) *SettingHandler {
	return &SettingHandler{
		settingService: service.NewSettingService(repository, &config.Config{}),
	}
}

func newAntigravityGIFSettingHandlerContext(method string, body []byte) (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(method, "/api/v1/admin/settings/antigravity-gif", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	return recorder, context
}
