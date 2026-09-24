package service

import "context"

type openAINonstreamTextRequestContextKey struct{}

// WithOpenAINonstreamTextRequest 标记客户端请求的是非流式文本响应。
//
// @param ctx 请求上下文；为 nil 时使用背景上下文。
// @return 包含非流式文本标记的上下文。
func WithOpenAINonstreamTextRequest(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, openAINonstreamTextRequestContextKey{}, true)
}

// OpenAINonstreamTextRequestFromContext 报告客户端是否请求非流式文本响应。
//
// @param ctx 请求上下文。
// @return 已标记非流式文本请求时返回 true。
func OpenAINonstreamTextRequestFromContext(ctx context.Context) bool {
	return ctx != nil && ctx.Value(openAINonstreamTextRequestContextKey{}) == true
}

func openAIResponseHeaderProfile(ctx context.Context) HTTPUpstreamProfile {
	if OpenAIImagesEndpointFromContext(ctx) {
		return HTTPUpstreamProfileOpenAIImages
	}
	if OpenAINonstreamTextRequestFromContext(ctx) {
		return HTTPUpstreamProfileOpenAINonstream
	}
	return HTTPUpstreamProfileOpenAI
}
