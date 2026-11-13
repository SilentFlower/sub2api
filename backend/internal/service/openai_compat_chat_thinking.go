package service

import (
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
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

// applyOpenAICompatChatThinkingBody 在分组策略完成后投影显式关闭推理的出站字段。
// 分组把 none 映射为有效档位时保留该决定，其余关闭请求沿用供应商兼容表达。
//
// @param req 原始 Anthropic 推理偏好。
// @param account 实际转发账号。
// @param body 已完成供应商归一化和分组策略的 Chat 请求体。
// @return 最终兼容请求体；JSON 字段改写失败时返回错误。
func applyOpenAICompatChatThinkingBody(req *apicompat.AnthropicRequest, account *Account, body []byte) ([]byte, error) {
	if req == nil || req.Thinking == nil || req.Thinking.Type != "disabled" {
		return body, nil
	}
	effort := strings.TrimSpace(gjson.GetBytes(body, "reasoning_effort").String())
	if effort != "" && effort != "none" {
		return body, nil
	}
	chatReq := &apicompat.ChatCompletionsRequest{Model: gjson.GetBytes(body, "model").String()}
	applyOpenAICompatChatThinking(req, chatReq, account)
	if chatReq.Thinking == nil {
		return body, nil
	}
	// 延后清理 none，让分组策略先看到真实关闭偏好，避免映射与拒绝规则被提前跳过。
	updated, err := sjson.SetBytes(body, "thinking.type", chatReq.Thinking.Type)
	if err != nil {
		return nil, err
	}
	return sjson.DeleteBytes(updated, "reasoning_effort")
}
