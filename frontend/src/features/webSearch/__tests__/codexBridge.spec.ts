import { describe, expect, it } from 'vitest'
import {
  applyAccountCodexWebSearchBridgeExtra,
  applyChannelCodexWebSearchBridgeConfig,
  normalizeCodexWebSearchBridgeMode,
  readChannelCodexWebSearchBridgeConfig
} from '../codexBridge'

describe('Codex Web Search 桥接配置', () => {
  it('账号三态只更新桥接字段并保留其它 extra', () => {
    const source = { compatibility_note: 'keep', codex_web_search_bridge: true }

    expect(applyAccountCodexWebSearchBridgeExtra(source, 'disabled')).toEqual({
      compatibility_note: 'keep',
      codex_web_search_bridge: false
    })
    expect(applyAccountCodexWebSearchBridgeExtra(source, 'inherit')).toEqual({
      compatibility_note: 'keep'
    })
    expect(source.codex_web_search_bridge).toBe(true)
  })

  it('只接受严格布尔账号值', () => {
    expect(normalizeCodexWebSearchBridgeMode(true)).toBe('enabled')
    expect(normalizeCodexWebSearchBridgeMode(false)).toBe('disabled')
    expect(normalizeCodexWebSearchBridgeMode('true')).toBe('inherit')
    expect(normalizeCodexWebSearchBridgeMode(undefined)).toBe('inherit')
  })

  it('渠道只序列化启用的 OpenAI 平台并保留其它字段', () => {
    const featuresConfig: Record<string, unknown> = {
      existing_feature: true,
      codex_web_search_bridge: { openai: true }
    }

    applyChannelCodexWebSearchBridgeConfig(featuresConfig, [
      { platform: 'openai', enabled: true, codex_web_search_bridge: false },
      { platform: 'anthropic', enabled: true, codex_web_search_bridge: true }
    ])

    expect(featuresConfig).toEqual({
      existing_feature: true,
      codex_web_search_bridge: { openai: false }
    })
  })

  it('渠道读取只接受平台严格布尔值', () => {
    expect(readChannelCodexWebSearchBridgeConfig({ codex_web_search_bridge: { openai: true } }, 'openai')).toBe(true)
    expect(readChannelCodexWebSearchBridgeConfig({ codex_web_search_bridge: { openai: 1 } }, 'openai')).toBe(false)
  })
})
