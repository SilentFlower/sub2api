/** Responses Lite 默认阻止模型列表。 */
export const DEFAULT_RESPONSES_LITE_BLOCKED_MODELS = ['gpt-5.4', 'gpt-5.4-mini', 'gpt-5.5'] as const

/** Responses Lite 模型规则校验错误。 */
export type ResponsesLiteModelRuleError = 'empty' | 'wildcard'

/** Responses Lite 模型规则归一化结果。 */
export interface ResponsesLiteModelRulesResult {
  models: string[]
  error?: ResponsesLiteModelRuleError
}

/**
 * 归一化 Responses Lite 阻止模型规则。
 *
 * @param rules 原始规则列表。
 * @return trim、稳定去重后的列表，或首个校验错误。
 */
export function normalizeResponsesLiteBlockedModelRules(
  rules: readonly string[]
): ResponsesLiteModelRulesResult {
  const models: string[] = []
  const seen = new Set<string>()
  for (const rawRule of rules) {
    const rule = rawRule.trim()
    if (!rule) return { models: [], error: 'empty' }

    const firstWildcard = rule.indexOf('*')
    if (
      firstWildcard >= 0 &&
      (firstWildcard !== rule.length - 1 || rule.lastIndexOf('*') !== firstWildcard)
    ) {
      return { models: [], error: 'wildcard' }
    }
    if (!seen.has(rule)) {
      seen.add(rule)
      models.push(rule)
    }
  }
  return { models }
}

/**
 * 返回可安全修改的默认 Responses Lite 阻止模型列表。
 *
 * @return 默认模型列表副本。
 */
export function defaultResponsesLiteBlockedModels(): string[] {
  return [...DEFAULT_RESPONSES_LITE_BLOCKED_MODELS]
}
