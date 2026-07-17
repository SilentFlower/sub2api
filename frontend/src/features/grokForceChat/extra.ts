import type { AccountPlatform, AccountType, OpenAIResponsesMode } from '@/types'
import {
  applyOpenAICompatibilityExtra,
  normalizeOpenAIResponsesMode
} from '@/features/openAICompatibility/extra'

/**
 * 判断账号是否支持 Grok Messages 强制 Chat 路由配置。
 *
 * @param platform 账号平台。
 * @param type 账号类型。
 * @return 仅 Grok OAuth/API Key 账号返回 true。
 */
export function supportsGrokForceChat(
  platform: AccountPlatform | undefined,
  type: AccountType | undefined
): boolean {
  return platform === 'grok' && (type === 'oauth' || type === 'apikey')
}

/**
 * 从 Grok 账号 extra 读取 Responses 路由模式。
 *
 * @param extra 账号已有 extra。
 * @return 合法路由模式，缺失或非法值回退 auto。
 */
export function readGrokForceChatMode(
  extra: Record<string, unknown> | undefined
): OpenAIResponsesMode {
  return normalizeOpenAIResponsesMode(extra?.openai_responses_mode)
}

/**
 * 复制 Grok 账号 extra 并只更新强制 Chat 路由键。
 *
 * @param source 账号已有 extra。
 * @param mode 本次选择的路由模式。
 * @return 保留额度快照等其它字段的新 extra 对象。
 */
export function applyGrokForceChatExtra(
  source: Record<string, unknown> | undefined,
  mode: OpenAIResponsesMode
): Record<string, unknown> {
  return applyOpenAICompatibilityExtra(source, { responsesMode: mode })
}
