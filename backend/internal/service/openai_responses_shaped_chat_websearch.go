package service

import (
	"bytes"
	"errors"
)

// validateResponsesShapedChatWebSearch 防止误投到 Chat 端点的请求静默丢失服务端搜索。
//
// @param body 原始 Responses 形状请求体。
// @return 无法在 Chat 上游保持搜索语义时返回错误，否则返回 nil。
func validateResponsesShapedChatWebSearch(body []byte) error {
	if !bytes.Contains(body, []byte("web_search")) {
		return nil
	}
	request, toolCount, webSearchCount, err := parseOpenAIResponsesWebSearchRequest(body)
	if err != nil {
		return err
	}
	if shouldRejectChatFallbackWebSearch(toolCount, webSearchCount, request.ToolChoice.Kind) {
		return errors.New("responses-shaped request sent to Chat Completions cannot execute the requested web_search tool")
	}
	return nil
}
