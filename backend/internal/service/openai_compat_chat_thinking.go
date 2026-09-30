package service

import (
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// applyOpenAICompatChatThinking 将显式关闭推理投影为最终上游接受的字段。
//
// @param req 原始 Anthropic 偏好，显式 disabled 优先于模型后缀和输出档位。
// @param chatReq 模型映射完成后的 Chat 请求。
// @param account 实际转发账号，用于识别官方 OpenAI 端点。
func applyOpenAICompatChatThinking(req *apicompat.AnthropicRequest, chatReq *apicompat.ChatCompletionsRequest, account *Account) {
	if req == nil || chatReq == nil || req.Thinking == nil || req.Thinking.Type != "disabled" {
		return
	}
	model := openai.CanonicalizeOpenAIModelAliasSpelling(chatReq.Model)
	nativeOpenAI := strings.HasPrefix(model, "gpt-") || model == "o1" || strings.HasPrefix(model, "o1-") ||
		model == "o3" || strings.HasPrefix(model, "o3-") || model == "o4" || strings.HasPrefix(model, "o4-")
	if account != nil {
		if upstream, err := url.Parse(account.GetOpenAIBaseURL()); err == nil && strings.EqualFold(upstream.Hostname(), "api.openai.com") {
			nativeOpenAI = true
		}
	}
	if nativeOpenAI {
		chatReq.Thinking = nil
		chatReq.ReasoningEffort = "none"
		return
	}
	// 兼容上游沿用 build 的 thinking 开关，清理互斥 effort，避免 GLM 等将 none 映射为开启档位。
	chatReq.Thinking = &apicompat.ChatThinking{Type: "disabled"}
	chatReq.ReasoningEffort = ""
}
