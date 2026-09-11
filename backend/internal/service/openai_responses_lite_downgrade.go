package service

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
)

// accountExtraKeyOpenAIResponsesLiteDowngrade 是账号级"Responses Lite 降级"开关在
// accounts.extra 中的键名。
const accountExtraKeyOpenAIResponsesLiteDowngrade = "openai_responses_lite_downgrade"

// openAIResponsesLiteDowngradedContextKey 记录本次请求已执行 Lite 降级，供原生
// Responses 路径决定是否执行客户端工具适配。
const openAIResponsesLiteDowngradedContextKey = "openai_responses_lite_downgraded"

// IsOpenAIResponsesLiteDowngradeEnabled 报告账号是否开启 Responses Lite → 标准
// Responses 降级。
//
// 仅 API Key 类型且平台为 openai 或国产 OpenAI 兼容供应商的账号可开启；OAuth、
// Grok、Anthropic 等账号恒为 false。开关只影响原生 Responses 上游路径，chat 回退
// 桥接对 Lite 形态的处理不受它控制。
//
// @return extra 中该键为布尔 true 时返回 true。
func (a *Account) IsOpenAIResponsesLiteDowngradeEnabled() bool {
	if a == nil || a.Type != AccountTypeAPIKey || (!a.IsOpenAI() && !a.IsCNProvider()) || a.Extra == nil {
		return false
	}
	enabled, ok := a.Extra[accountExtraKeyOpenAIResponsesLiteDowngrade].(bool)
	return ok && enabled
}

// applyOpenAIResponsesLiteDowngrade 在账号开启降级且入站为 HTTP Lite 请求时，把
// Codex Lite 形态还原为标准 Responses：input.additional_tools 提升到顶层 tools，
// 删除 Lite 专属的 reasoning.context，并移除入站 Lite 头。
//
// 为什么在这里删入站头而不是出站时再删：Forward 内多处（responsesLite 判定、
// ingress policy、图片桥接、passthrough enforce）都直接读取入站头，逐处加分支容易
// 漂移；降级后请求体已不是 Lite 形态，头应一起消失，后续统一按非 Lite 处理。
//
// @param c gin 上下文，入站头从 c.Request 读取并原地删除。
// @param account 当前账号。
// @param body 入站 Responses 请求体。
// @return 处理后的请求体、是否执行了降级、错误。
func applyOpenAIResponsesLiteDowngrade(c *gin.Context, account *Account, body []byte) ([]byte, bool, error) {
	if c == nil || c.Request == nil || !account.IsOpenAIResponsesLiteDowngradeEnabled() {
		return body, false, nil
	}
	if !isOpenAIResponsesLiteHeader(c.GetHeader(responsesLiteHeader)) {
		return body, false, nil
	}
	var requestBody map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &requestBody); err != nil {
		return body, false, fmt.Errorf("decode responses Lite downgrade body: %w", err)
	}
	lifted, err := liftResponsesAdditionalTools(requestBody)
	if err != nil {
		return body, false, fmt.Errorf("lift responses Lite additional_tools: %w", err)
	}
	contextRemoved := removeOpenAIResponsesLiteReasoningContext(requestBody)
	c.Request.Header.Del(responsesLiteHeader)
	c.Set(openAIResponsesLiteDowngradedContextKey, true)
	if !lifted && !contextRemoved {
		return body, true, nil
	}
	rebuilt, err := marshalOpenAIUpstreamJSON(requestBody)
	if err != nil {
		return body, false, fmt.Errorf("encode responses Lite downgrade body: %w", err)
	}
	return rebuilt, true, nil
}

// removeOpenAIResponsesLiteReasoningContext 删除 Lite 专属的 reasoning.context；
// reasoning 因此变为空对象时整体删除，避免上游收到空的 reasoning。
//
// @param requestBody 已解码的请求体，原地修改。
// @return 是否发生了删除。
func removeOpenAIResponsesLiteReasoningContext(requestBody map[string]any) bool {
	reasoning, ok := requestBody["reasoning"].(map[string]any)
	if !ok {
		return false
	}
	if _, exists := reasoning["context"]; !exists {
		return false
	}
	delete(reasoning, "context")
	if len(reasoning) == 0 {
		delete(requestBody, "reasoning")
	}
	return true
}

// openAIResponsesLiteDowngraded 报告本次请求是否已执行 Lite 降级。
//
// @param c gin 上下文。
// @return 已降级返回 true。
func openAIResponsesLiteDowngraded(c *gin.Context) bool {
	if c == nil {
		return false
	}
	value, ok := c.Get(openAIResponsesLiteDowngradedContextKey)
	if !ok {
		return false
	}
	downgraded, _ := value.(bool)
	return downgraded
}

// adaptOpenAIResponsesLiteDowngradedClientTools 对降级后的标准 Responses 请求执行
// 客户端工具适配。与 adaptOpenAIResponsesClientTools 不同，这里不以 custom/tool_search
// 是否出现为前置条件：Lite 提升出来的顶层 namespace 声明即使只含 function 子工具，
// function-only 上游也无法识别，必须摊平。
//
// @param body 降级后的请求体。
// @return 适配后的请求体、客户端工具映射、是否发生改写、错误。
func adaptOpenAIResponsesLiteDowngradedClientTools(body []byte) ([]byte, apicompat.ResponsesClientToolMapping, bool, error) {
	var requestBody map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &requestBody); err != nil {
		return body, apicompat.ResponsesClientToolMapping{}, false, fmt.Errorf("decode responses Lite downgraded client tools: %w", err)
	}
	mapping, changed, err := apicompat.AdaptResponsesClientTools(requestBody)
	if err != nil || !changed {
		return body, mapping, false, err
	}
	rebuilt, err := marshalOpenAIUpstreamJSON(requestBody)
	if err != nil {
		return body, apicompat.ResponsesClientToolMapping{}, false, fmt.Errorf("encode responses Lite downgraded client tools: %w", err)
	}
	return rebuilt, mapping, true, nil
}
