package service

import "strings"

// resolveOpenAIForwardModel 解析 OpenAI 兼容转发使用的模型。
// messagesDispatchMappedModel 是调用方已为 /v1/messages 解析的显式调度结果；
// 普通 OpenAI 请求必须传空，避免将分组配置作为通用模型兜底。
func resolveOpenAIForwardModel(account *Account, requestedModel, messagesDispatchMappedModel string) string {
	messagesDispatchMappedModel = strings.TrimSpace(messagesDispatchMappedModel)
	if account == nil {
		if messagesDispatchMappedModel != "" {
			return messagesDispatchMappedModel
		}
		return requestedModel
	}

	mappedModel, matched := account.ResolveMappedModel(requestedModel)
	if !matched && messagesDispatchMappedModel != "" {
		return messagesDispatchMappedModel
	}
	return mappedModel
}

// resolveOpenAIAccountUpstreamModelForRequest 解析 Responses 请求实际发送给账号上游的模型。
// 透传模式必须区分 OAuth、API Key、compact 与 Chat Completions fallback，避免调度、
// Lite 策略和请求改写分别维护不一致的映射顺序。
func resolveOpenAIAccountUpstreamModelForRequest(account *Account, requestedModel string, requireCompact bool) string {
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		return ""
	}

	// Forward 会先于 passthrough 判定 Responses -> Chat fallback；该路径只应用普通映射。
	if shouldForwardOpenAIResponsesViaRawChatCompletions(account) {
		upstreamModel := resolveOpenAIForwardModel(account, requestedModel, "")
		return normalizeOpenAIModelForUpstream(account, upstreamModel)
	}

	if account != nil && account.IsOpenAIPassthroughEnabled() {
		// compact 直接按入站模型查询 compact_model_mapping，不能先套普通映射。
		if requireCompact {
			return resolveOpenAICompactForwardModel(account, requestedModel)
		}
		// OAuth 仍需应用账号别名并收敛为 Codex 上游模型；API Key 原生透传保持入站模型。
		if account.Type == AccountTypeOAuth {
			upstreamModel := resolveOpenAIForwardModel(account, requestedModel, "")
			return normalizeOpenAIModelForUpstream(account, upstreamModel)
		}
		return requestedModel
	}

	upstreamModel := resolveOpenAIForwardModel(account, requestedModel, "")
	if requireCompact {
		compactModel := resolveOpenAICompactForwardModel(account, upstreamModel)
		if compactModel != upstreamModel {
			return compactModel
		}
	}
	return normalizeOpenAIModelForUpstream(account, upstreamModel)
}

// openAIOAuthForeignModelPrefixes 列出明确属于其他厂商家族的模型名前缀。
// Codex 上游不可能服务这些模型：转发阶段 normalizeOpenAIModelForUpstream
// 对未知模型原样透传，上游必然返回不可重试的 400。
//
// 采用保守黑名单而非 Codex 模型白名单：未知/自定义别名保持「允许」，
// 以兼容渠道级模型映射等「账号选定之后才改写模型名」的部署方式
// （调度过滤看到的是改写前的原始模型名）。前缀分类的先例见
// ResolveThinkingProtocol（thinking_protocol.go）。
var openAIOAuthForeignModelPrefixes = []string{
	"deepseek-",
	"glm-",
	"kimi-",
	"moonshot-",
	"qwen-",
	"qwen2-",
	"qwen3-",
	"qwen4-",
	"qwq-",
	"minimax-",
	"gemini-",
	"gemma-",
	"grok-",
	"doubao-",
	"hunyuan-",
	"llama-",
	"llama2-",
	"llama3-",
	"meta-llama",
	"mistral-",
	"mixtral-",
	"baichuan-",
	"ernie-",
	"step-",
	"seed-",
	"yi-",
}

// isOpenAIOAuthServableModel 判断「空 model_mapping 的 OpenAI OAuth 账号」能否
// 服务请求模型。空映射默认仍是「允许」，仅排除明确属于其他厂商家族的模型
// （deepseek-*/glm-*、以及 Kimi Code 官方 bare ID k3 / k3-256k 等）——这类
// 请求原样透传必然被 Codex 上游以不可重试的 400 拒绝，且不触发 failover，
// 应在调度阶段就跳过该账号，把请求让给显式声明支持该模型的账号（#3662）。
// bare k3 仅精确匹配（取 last segment 后），不使用宽泛 k3- 前缀，以免误伤
// 自定义别名；显式 model_mapping 命中路径不经过本函数，语义不变。
func isOpenAIOAuthServableModel(requestedModel string) bool {
	model := strings.ToLower(lastOpenAIModelSegment(requestedModel))
	if model == "" {
		return true // 空模型交由上层必填校验处理
	}
	// Kimi Code 官方 bare model ID：无厂商前缀，prefix 黑名单挡不住。
	if model == "k3" || model == "k3-256k" {
		return false
	}
	for _, prefix := range openAIOAuthForeignModelPrefixes {
		if strings.HasPrefix(model, prefix) {
			return false
		}
	}
	return true
}

// resolveOpenAICompactForwardModel determines the compact-only upstream model
// for /responses/compact requests. It never affects normal /responses traffic.
// When no compact-specific mapping matches, the input model is returned as-is.
func resolveOpenAICompactForwardModel(account *Account, model string) string {
	trimmedModel := strings.TrimSpace(model)
	if trimmedModel == "" || account == nil {
		return trimmedModel
	}

	mappedModel, matched := account.ResolveCompactMappedModel(trimmedModel)
	if !matched {
		return trimmedModel
	}
	if trimmedMapped := strings.TrimSpace(mappedModel); trimmedMapped != "" {
		return trimmedMapped
	}
	return trimmedModel
}
