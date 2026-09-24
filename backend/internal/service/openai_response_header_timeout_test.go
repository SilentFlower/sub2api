package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIResponseHeaderProfileSelection(t *testing.T) {
	imageCtx := WithOpenAIImagesEndpoint(context.Background())
	nonstreamCtx := WithOpenAINonstreamTextRequest(context.Background())
	tests := []struct {
		name     string
		ctx      context.Context
		platform string
		want     HTTPUpstreamProfile
	}{
		{name: "streaming_text", ctx: context.Background(), platform: PlatformOpenAI, want: HTTPUpstreamProfileOpenAI},
		{name: "nonstream_text", ctx: nonstreamCtx, platform: PlatformOpenAI, want: HTTPUpstreamProfileOpenAINonstream},
		{name: "images", ctx: imageCtx, platform: PlatformOpenAI, want: HTTPUpstreamProfileOpenAIImages},
		{name: "images_take_priority", ctx: WithOpenAINonstreamTextRequest(imageCtx), platform: PlatformOpenAI, want: HTTPUpstreamProfileOpenAIImages},
		{name: "other_platform_keeps_profile", ctx: WithHTTPUpstreamProfile(nonstreamCtx, HTTPUpstreamProfileGrok), platform: PlatformGrok, want: HTTPUpstreamProfileGrok},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{err: errors.New("测试出站已捕获")}
			svc := &OpenAIGatewayService{httpUpstream: upstream}
			req, err := http.NewRequestWithContext(tc.ctx, http.MethodPost, "https://example.com/v1/responses", nil)
			require.NoError(t, err)
			_, err = svc.doOpenAIUpstream(req, "", &Account{ID: 1, Platform: tc.platform})
			require.Error(t, err)
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, tc.want, HTTPUpstreamProfileFromContext(upstream.lastReq.Context()))
		})
	}
}
