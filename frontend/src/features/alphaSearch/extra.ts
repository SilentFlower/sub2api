/** 账号 extra 中"Alpha Search 经上游 Responses 执行"开关的键名。 */
export const ALPHA_SEARCH_VIA_RESPONSES_EXTRA_KEY = 'openai_alpha_search_via_responses'

/**
 * 判断账号是否可配置"Alpha Search 经上游 Responses 执行"开关。
 *
 * Codex 独立搜索端点只在 OpenAI 分组内调度，只有 openai 平台的 API Key 账号会收到
 * /v1/alpha/search 请求；OAuth/PAT 账号沿用 chatgpt.com 路径，国产供应商账号进不了该入口。
 *
 * @param platform 账号平台。
 * @param type 账号类型。
 * @return 可配置时返回 true。
 */
export function supportsAlphaSearchViaResponses(platform?: string, type?: string): boolean {
  return platform === 'openai' && type === 'apikey'
}

/**
 * 读取账号 extra 中的"Alpha Search 经上游 Responses 执行"开关。
 *
 * @param extra 账号 extra。
 * @return 仅当值为布尔 true 时返回 true。
 */
export function readAlphaSearchViaResponses(extra?: Record<string, unknown>): boolean {
  return extra?.[ALPHA_SEARCH_VIA_RESPONSES_EXTRA_KEY] === true
}

/**
 * 复制账号 extra 并只更新"Alpha Search 经上游 Responses 执行"开关。
 *
 * @param source 账号已有 extra。
 * @param enabled true 写入开关；false/null 删除开关；undefined 保持原值。
 * @return 保留其它字段的新 extra 对象。
 */
export function applyAlphaSearchViaResponsesExtra(
  source: Record<string, unknown> | undefined,
  enabled?: boolean | null
): Record<string, unknown> {
  const extra = { ...(source || {}) }
  if (enabled === undefined) return extra
  if (enabled === true) {
    extra[ALPHA_SEARCH_VIA_RESPONSES_EXTRA_KEY] = true
  } else {
    delete extra[ALPHA_SEARCH_VIA_RESPONSES_EXTRA_KEY]
  }
  return extra
}
