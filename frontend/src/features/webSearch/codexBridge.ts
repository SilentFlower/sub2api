import type { GroupPlatform } from '@/types'

const accountCodexWebSearchBridgeKey = 'codex_web_search_bridge'
const channelCodexWebSearchBridgeKey = 'codex_web_search_bridge'

/** Codex Web Search 账号级桥接模式。 */
export type CodexWebSearchBridgeMode = 'inherit' | 'enabled' | 'disabled'

/** 渠道 Codex Web Search 桥接序列化所需的最小平台字段。 */
export interface ChannelCodexWebSearchBridgeSection {
  platform: GroupPlatform
  enabled: boolean
  codex_web_search_bridge: boolean
}

/**
 * 归一化账号 extra 中的 Codex Web Search 桥接值。
 *
 * @param value 未知来源的账号字段值。
 * @return 账号桥接三态模式，非法或缺失值回退为跟随渠道。
 */
export function normalizeCodexWebSearchBridgeMode(value: unknown): CodexWebSearchBridgeMode {
  if (value === true) return 'enabled'
  if (value === false) return 'disabled'
  return 'inherit'
}

/**
 * 复制账号 extra 并更新 Codex Web Search 桥接覆盖值。
 *
 * @param source 账号已有 extra。
 * @param mode 本次账号桥接模式。
 * @return 保留其它字段的新 extra 对象。
 */
export function applyAccountCodexWebSearchBridgeExtra(
  source: Record<string, unknown> | undefined,
  mode: CodexWebSearchBridgeMode
): Record<string, unknown> {
  const extra = { ...(source || {}) }
  if (mode === 'enabled') {
    extra[accountCodexWebSearchBridgeKey] = true
  } else if (mode === 'disabled') {
    extra[accountCodexWebSearchBridgeKey] = false
  } else {
    delete extra[accountCodexWebSearchBridgeKey]
  }
  return extra
}

/**
 * 把渠道 Codex Web Search 桥接值写入 features_config。
 *
 * @param featuresConfig 待更新的渠道功能配置对象。
 * @param sections 渠道内各平台的桥接设置。
 * @return 无返回值。
 */
export function applyChannelCodexWebSearchBridgeConfig(
  featuresConfig: Record<string, unknown>,
  sections: ChannelCodexWebSearchBridgeSection[]
): void {
  const values: Record<string, boolean> = {}
  for (const section of sections) {
    if (!section.enabled || section.platform !== 'openai') continue
    values[section.platform] = section.codex_web_search_bridge === true
  }

  if (Object.keys(values).length > 0) {
    featuresConfig[channelCodexWebSearchBridgeKey] = values
    return
  }
  delete featuresConfig[channelCodexWebSearchBridgeKey]
}

/**
 * 从 features_config 读取指定平台的 Codex Web Search 桥接值。
 *
 * @param featuresConfig 渠道功能配置对象。
 * @param platform 待读取的平台。
 * @return 仅当该平台显式启用桥接时返回 true。
 */
export function readChannelCodexWebSearchBridgeConfig(
  featuresConfig: Record<string, unknown> | null | undefined,
  platform: GroupPlatform
): boolean {
  const value = featuresConfig?.[channelCodexWebSearchBridgeKey]
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  return (value as Record<string, unknown>)[platform] === true
}
