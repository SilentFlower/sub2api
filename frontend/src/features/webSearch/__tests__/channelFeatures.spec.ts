import { describe, expect, it } from 'vitest'
import {
  applyChannelWebSearchFeatureConfig,
  readChannelWebSearchFeatureConfig,
  supportsChannelWebSearchEmulation
} from '../channelFeatures'

describe('渠道 Web Search features_config', () => {
  it('为支持的平台写入显式状态并保留其它字段', () => {
    const featuresConfig: Record<string, unknown> = {
      existing_feature: { enabled: true },
      web_search_emulation: { anthropic: true }
    }

    applyChannelWebSearchFeatureConfig(featuresConfig, [
      { platform: 'openai', enabled: true, web_search_emulation: false },
      { platform: 'anthropic', enabled: false, web_search_emulation: true },
      { platform: 'command_code', enabled: true, web_search_emulation: true },
      { platform: 'opencode_go', enabled: false, web_search_emulation: true },
      { platform: 'minimax', enabled: true, web_search_emulation: true },
      { platform: 'zhipu', enabled: true, web_search_emulation: false },
      { platform: 'grok', enabled: true, web_search_emulation: true }
    ])

    expect(featuresConfig).toEqual({
      existing_feature: { enabled: true },
      web_search_emulation: {
        openai: false,
        anthropic: false,
        command_code: true,
        opencode_go: false,
        minimax: true,
        zhipu: false
      }
    })
  })

  it('没有可用平台时删除旧配置', () => {
    const featuresConfig: Record<string, unknown> = {
      web_search_emulation: { openai: true }
    }

    applyChannelWebSearchFeatureConfig(featuresConfig, [])

    expect(featuresConfig).not.toHaveProperty('web_search_emulation')
  })

  it('安全读取合法布尔值并拒绝错误结构', () => {
    expect(readChannelWebSearchFeatureConfig({ web_search_emulation: { openai: true } }, 'openai')).toBe(true)
    expect(readChannelWebSearchFeatureConfig({ web_search_emulation: [] }, 'openai')).toBe(false)
    expect(readChannelWebSearchFeatureConfig({ web_search_emulation: { openai: 'true' } }, 'openai')).toBe(false)
    expect(supportsChannelWebSearchEmulation('anthropic')).toBe(true)
    expect(supportsChannelWebSearchEmulation('openai')).toBe(true)
    expect(supportsChannelWebSearchEmulation('command_code')).toBe(true)
    expect(supportsChannelWebSearchEmulation('opencode_go')).toBe(true)
    expect(supportsChannelWebSearchEmulation('minimax')).toBe(true)
    expect(supportsChannelWebSearchEmulation('zhipu')).toBe(true)
    expect(supportsChannelWebSearchEmulation('grok')).toBe(false)
  })
})
