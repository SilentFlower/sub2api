import type { GroupPlatform } from '@/types'

const channelCodexImageBridgeFeatureKey = 'codex_image_generation_bridge'

/** 渠道 Codex 生图桥接序列化所需的最小平台字段。 */
export interface ChannelCodexImageBridgeFeatureSection {
  platform: GroupPlatform
  enabled: boolean
  codex_image_generation_bridge: boolean
}

/**
 * 把渠道 Codex 生图桥接开关写入 features_config，并保留其它功能字段。
 *
 * @param featuresConfig 待更新的渠道功能配置对象。
 * @param sections 渠道内各平台的生图桥接设置。
 * @return 无返回值。
 */
export function applyChannelCodexImageBridgeFeatureConfig(
  featuresConfig: Record<string, unknown>,
  sections: ChannelCodexImageBridgeFeatureSection[]
): void {
  const values: Record<string, boolean> = {}
  for (const section of sections) {
    if (!section.enabled || section.platform !== 'openai') continue
    values[section.platform] = section.codex_image_generation_bridge === true
  }

  if (Object.keys(values).length > 0) {
    featuresConfig[channelCodexImageBridgeFeatureKey] = values
    return
  }
  delete featuresConfig[channelCodexImageBridgeFeatureKey]
}

/**
 * 从 features_config 读取指定平台的渠道 Codex 生图桥接值。
 *
 * @param featuresConfig 渠道功能配置对象。
 * @param platform 待读取的平台。
 * @return 仅当该平台显式启用生图桥接时返回 true。
 */
export function readChannelCodexImageBridgeFeatureConfig(
  featuresConfig: Record<string, unknown> | null | undefined,
  platform: GroupPlatform
): boolean {
  const value = featuresConfig?.[channelCodexImageBridgeFeatureKey]
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  return (value as Record<string, unknown>)[platform] === true
}
