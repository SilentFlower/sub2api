//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/stretchr/testify/require"
)

type countingGIFSettingRepo struct {
	*mockSettingRepo
	getValueCalls int
	readErr       error
}

func (r *countingGIFSettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	r.getValueCalls++
	if r.readErr != nil {
		return "", r.readErr
	}
	return r.mockSettingRepo.GetValue(ctx, key)
}

func TestApplyAntigravityGIFCompatibility_FalsePositiveCandidateReadsSettingsAndKeepsBytes(t *testing.T) {
	repository := &countingGIFSettingRepo{mockSettingRepo: newMockSettingRepo()}
	service := &AntigravityGatewayService{
		settingService: NewSettingService(repository, nil),
	}
	body := []byte(`{"contents":[{"parts":[{"text":"image/gif-like but not exact"}]}]}`)

	transformed, err := service.applyAntigravityGIFCompatibility(context.Background(), body)

	require.NoError(t, err)
	require.Equal(t, body, transformed)
	require.Equal(t, 1, repository.getValueCalls)
}

func TestApplyAntigravityGIFCompatibility_WithoutMIMEKeepsBytesAndSkipsSettings(t *testing.T) {
	repository := &countingGIFSettingRepo{mockSettingRepo: newMockSettingRepo()}
	service := &AntigravityGatewayService{
		settingService: NewSettingService(repository, nil),
	}
	body := []byte(`{"contents":[{"parts":[{"text":"普通请求"}]}]}`)

	transformed, err := service.applyAntigravityGIFCompatibility(context.Background(), body)

	require.NoError(t, err)
	require.Equal(t, body, transformed)
	require.Zero(t, repository.getValueCalls)
}

func TestApplyAntigravityGIFCompatibility_DisabledKeepsOriginalBody(t *testing.T) {
	repository := &countingGIFSettingRepo{mockSettingRepo: newMockSettingRepo()}
	repository.data[SettingKeyAntigravityGIFCompatSettings] = `{"enabled":false,"max_frames_per_gif":8}`
	service := &AntigravityGatewayService{
		settingService: NewSettingService(repository, nil),
	}
	body := serviceGIFRequestBody(t, serviceTestGIFBase64(t))

	transformed, err := service.applyAntigravityGIFCompatibility(context.Background(), body)

	require.NoError(t, err)
	require.Equal(t, body, transformed)
	require.Equal(t, 1, repository.getValueCalls)
}

func TestApplyAntigravityGIFCompatibility_ReadFailureUsesDefaults(t *testing.T) {
	repository := &countingGIFSettingRepo{
		mockSettingRepo: newMockSettingRepo(),
		readErr:         errors.New("read failed"),
	}
	service := &AntigravityGatewayService{
		settingService: NewSettingService(repository, nil),
	}
	body := serviceGIFRequestBody(t, serviceTestGIFBase64(t))

	transformed, err := service.applyAntigravityGIFCompatibility(context.Background(), body)

	require.NoError(t, err)
	require.NotContains(t, string(transformed), "image/gif")
	require.Contains(t, string(transformed), "image/png")
}

func TestApplyAntigravityGIFCompatibility_UsesConfiguredFrameLimit(t *testing.T) {
	repository := &countingGIFSettingRepo{mockSettingRepo: newMockSettingRepo()}
	repository.data[SettingKeyAntigravityGIFCompatSettings] = `{"enabled":true,"max_frames_per_gif":2}`
	service := &AntigravityGatewayService{
		settingService: NewSettingService(repository, nil),
	}
	body := serviceGIFRequestBody(t, serviceTestAnimatedGIFBase64(t, 4))

	transformed, err := service.applyAntigravityGIFCompatibility(context.Background(), body)

	require.NoError(t, err)
	require.Equal(t, 2, serviceGIFPartCount(t, transformed))
}

func TestApplyAntigravityGIFCompatibility_InvalidGIFReturnsClientError(t *testing.T) {
	service := &AntigravityGatewayService{}
	body := serviceGIFRequestBody(t, "%%%")

	_, err := service.applyAntigravityGIFCompatibility(context.Background(), body)

	require.Error(t, err)
	require.True(t, antigravity.IsGIFCompatibilityError(err))
	require.Equal(t, "Invalid GIF base64 data", antigravityGIFClientErrorMessage(err, "Invalid request"))
}

func TestTransformClaudeRequestWithGIFCompatibility_ConvertsImagePart(t *testing.T) {
	service := &AntigravityGatewayService{}
	content, err := json.Marshal([]map[string]any{
		{
			"type": "image",
			"source": map[string]any{
				"type":       "base64",
				"media_type": "image/gif",
				"data":       serviceTestGIFBase64(t),
			},
		},
	})
	require.NoError(t, err)
	request := &antigravity.ClaudeRequest{
		Model:     "gemini-2.5-pro",
		MaxTokens: 128,
		Messages: []antigravity.ClaudeMessage{
			{Role: "user", Content: content},
		},
	}

	transformed, err := service.transformClaudeRequestWithGIFCompatibility(
		context.Background(),
		request,
		"project-id",
		"gemini-2.5-pro",
		antigravity.DefaultTransformOptions(),
	)

	require.NoError(t, err)
	require.NotContains(t, string(transformed), "image/gif")
	require.Contains(t, string(transformed), "image/png")
}

func serviceGIFRequestBody(t *testing.T, data string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"contents": []any{
			map[string]any{
				"parts": []any{
					map[string]any{
						"inlineData": map[string]any{
							"mimeType": "image/gif",
							"data":     data,
						},
					},
				},
			},
		},
	})
	require.NoError(t, err)
	return body
}

func serviceTestGIFBase64(t *testing.T) string {
	return serviceTestAnimatedGIFBase64(t, 1)
}

func serviceTestAnimatedGIFBase64(t *testing.T, frameCount int) string {
	t.Helper()
	palette := color.Palette{color.RGBA{}, color.RGBA{R: 255, A: 255}}
	frames := make([]*image.Paletted, frameCount)
	for index := range frames {
		frame := image.NewPaletted(image.Rect(0, 0, 1, 1), palette)
		frame.SetColorIndex(0, 0, 1)
		frames[index] = frame
	}
	var buffer bytes.Buffer
	err := gif.EncodeAll(&buffer, &gif.GIF{
		Image: frames,
		Delay: make([]int, len(frames)),
		Config: image.Config{
			ColorModel: palette,
			Width:      1,
			Height:     1,
		},
	})
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(buffer.Bytes())
}

func serviceGIFPartCount(t *testing.T, body []byte) int {
	t.Helper()
	var request struct {
		Contents []struct {
			Parts []json.RawMessage `json:"parts"`
		} `json:"contents"`
	}
	require.NoError(t, json.Unmarshal(body, &request))
	require.Len(t, request.Contents, 1)
	return len(request.Contents[0].Parts)
}
