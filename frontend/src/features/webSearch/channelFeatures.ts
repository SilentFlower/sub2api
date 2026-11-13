import type { GroupPlatform } from '@/types'

const channelWebSearchFeatureKey = 'web_search_emulation'

/** 渠道 Web Search 功能序列化所需的最小平台字段。 */
export interface ChannelWebSearchFeatureSection {
  platform: GroupPlatform
  enabled: boolean
  web_search_emulation: boolean
}

/**
 * 判断平台是否支持渠道级 Web Search 默认策略。
 *
 * @param platform 渠道平台。
 * @return 平台支持渠道级 Web Search 时返回 true。
 */
export function supportsChannelWebSearchEmulation(platform: GroupPlatform): boolean {
  return ['anthropic', 'openai', 'command_code', 'opencode_go', 'minimax', 'zhipu'].includes(platform)
}

/**
 * 把渠道 Web Search 开关写入 features_config，并保留其它功能字段。
 *
 * @param featuresConfig 待更新的渠道功能配置对象。
 * @param sections 渠道内各平台的 Web Search 设置。
 * @return 无返回值。
 */
export function applyChannelWebSearchFeatureConfig(
  featuresConfig: Record<string, unknown>,
  sections: ChannelWebSearchFeatureSection[]
): void {
  const values: Record<string, boolean> = {}
  for (const section of sections) {
    if (!supportsChannelWebSearchEmulation(section.platform)) continue
    // 显式写入关闭值，避免服务端合并配置时保留平台旧的启用状态。
    values[section.platform] = section.enabled && section.web_search_emulation === true
  }

  if (Object.keys(values).length > 0) {
    featuresConfig[channelWebSearchFeatureKey] = values
    return
  }
  delete featuresConfig[channelWebSearchFeatureKey]
}

/**
 * 从 features_config 读取指定平台的渠道 Web Search 默认值。
 *
 * @param featuresConfig 渠道功能配置对象。
 * @param platform 待读取的平台。
 * @return 仅当该平台显式启用 Web Search 时返回 true。
 */
export function readChannelWebSearchFeatureConfig(
  featuresConfig: Record<string, unknown> | null | undefined,
  platform: GroupPlatform
): boolean {
  const value = featuresConfig?.[channelWebSearchFeatureKey]
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  return (value as Record<string, unknown>)[platform] === true
}
