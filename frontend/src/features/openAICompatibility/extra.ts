import type { OpenAIResponsesMode } from '@/types'
import type { WebSearchEmulationMode } from '@/features/webSearch/types'

/** OpenAI 兼容能力写入选项；undefined 表示保持原值，null 表示删除。 */
export interface OpenAICompatibilityExtraOptions {
  responsesMode?: OpenAIResponsesMode | null
  jsonSchemaToJSONObject?: boolean | null
  webSearchEmulation?: WebSearchEmulationMode | null
}

/** OpenAI 兼容能力从账号 extra 读取后的结构化状态。 */
export interface OpenAICompatibilityExtraState {
  responsesMode: OpenAIResponsesMode
  jsonSchemaToJSONObject: boolean
  webSearchEmulation: WebSearchEmulationMode
}

/**
 * 复制账号 extra 并只更新 OpenAI 兼容能力字段。
 *
 * @param source 账号已有 extra。
 * @param options 本次需要写入、删除或保持的兼容能力值。
 * @return 保留其它字段的新 extra 对象。
 */
export function applyOpenAICompatibilityExtra(
  source: Record<string, unknown> | undefined,
  options: OpenAICompatibilityExtraOptions
): Record<string, unknown> {
  const extra = { ...(source || {}) }

  if (options.responsesMode !== undefined) {
    if (options.responsesMode && options.responsesMode !== 'auto') {
      extra.openai_responses_mode = options.responsesMode
    } else {
      delete extra.openai_responses_mode
    }
  }

  if (options.jsonSchemaToJSONObject !== undefined) {
    if (options.jsonSchemaToJSONObject === true) {
      extra.openai_json_schema_to_json_object = true
    } else {
      delete extra.openai_json_schema_to_json_object
    }
  }

  if (options.webSearchEmulation !== undefined) {
    if (options.webSearchEmulation && options.webSearchEmulation !== 'default') {
      extra.web_search_emulation = options.webSearchEmulation
    } else {
      delete extra.web_search_emulation
    }
  }

  return extra
}

/**
 * 归一化账号 extra 中的 Responses 路由模式。
 *
 * @param value 未知来源的字段值。
 * @return 受支持的路由模式，非法值回退 auto。
 */
export function normalizeOpenAIResponsesMode(value: unknown): OpenAIResponsesMode {
  if (value === 'force_responses' || value === 'force_chat_completions') return value
  return 'auto'
}

/**
 * 归一化账号 extra 中的 Web Search 模拟模式。
 *
 * @param value 未知来源的字段值，兼容历史布尔 true。
 * @return 三态 Web Search 模式。
 */
export function normalizeWebSearchEmulationMode(value: unknown): WebSearchEmulationMode {
  if (value === 'enabled' || value === 'disabled') return value
  if (value === true) return 'enabled'
  return 'default'
}

/**
 * 从账号 extra 一次读取 OpenAI 兼容能力状态。
 *
 * @param source 账号已有 extra。
 * @return 已归一化的 Responses、JSON Schema 和 Web Search 状态。
 */
export function readOpenAICompatibilityExtra(
  source: Record<string, unknown> | undefined
): OpenAICompatibilityExtraState {
  return {
    responsesMode: normalizeOpenAIResponsesMode(source?.openai_responses_mode),
    jsonSchemaToJSONObject: source?.openai_json_schema_to_json_object === true,
    webSearchEmulation: normalizeWebSearchEmulationMode(source?.web_search_emulation)
  }
}
