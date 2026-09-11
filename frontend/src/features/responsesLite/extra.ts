import { isCNProviderPlatform } from '@/components/account/credentialsBuilder'

/** 账号 extra 中 Responses Lite 降级开关的键名。 */
export const RESPONSES_LITE_DOWNGRADE_EXTRA_KEY = 'openai_responses_lite_downgrade'

/**
 * 判断账号是否可配置 Responses Lite 降级开关。
 *
 * 只有 API Key 且平台为 openai 或国产 OpenAI 兼容供应商的账号会把 Lite 请求转发到
 * 不认识 Lite 形态的原生 Responses 上游；OAuth、Grok、Anthropic 账号不显示该开关。
 *
 * @param platform 账号平台。
 * @param type 账号类型。
 * @return 可配置时返回 true。
 */
export function supportsResponsesLiteDowngrade(platform?: string, type?: string): boolean {
  if (type !== 'apikey' || !platform) return false
  return platform === 'openai' || isCNProviderPlatform(platform)
}

/**
 * 读取账号 extra 中的 Responses Lite 降级开关。
 *
 * @param extra 账号 extra。
 * @return 仅当值为布尔 true 时返回 true。
 */
export function readResponsesLiteDowngrade(extra?: Record<string, unknown>): boolean {
  return extra?.[RESPONSES_LITE_DOWNGRADE_EXTRA_KEY] === true
}

/**
 * 复制账号 extra 并只更新 Responses Lite 降级开关。
 *
 * @param source 账号已有 extra。
 * @param enabled true 写入开关；false/null 删除开关；undefined 保持原值。
 * @return 保留其它字段的新 extra 对象。
 */
export function applyResponsesLiteDowngradeExtra(
  source: Record<string, unknown> | undefined,
  enabled?: boolean | null
): Record<string, unknown> {
  const extra = { ...(source || {}) }
  if (enabled === undefined) return extra
  if (enabled === true) {
    extra[RESPONSES_LITE_DOWNGRADE_EXTRA_KEY] = true
  } else {
    delete extra[RESPONSES_LITE_DOWNGRADE_EXTRA_KEY]
  }
  return extra
}
