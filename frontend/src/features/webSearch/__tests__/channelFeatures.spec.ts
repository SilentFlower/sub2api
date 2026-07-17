import { describe, expect, it } from 'vitest'
import {
  applyChannelWebSearchFeatureConfig,
  readChannelWebSearchFeatureConfig,
  supportsChannelWebSearchEmulation
} from '../channelFeatures'

describe('渠道 Web Search features_config', () => {
  it('只序列化启用的 OpenAI 和 Anthropic 平台并保留其它字段', () => {
    const featuresConfig: Record<string, unknown> = {
      existing_feature: { enabled: true },
      web_search_emulation: { anthropic: true }
    }

    applyChannelWebSearchFeatureConfig(featuresConfig, [
      { platform: 'openai', enabled: true, web_search_emulation: false },
      { platform: 'anthropic', enabled: false, web_search_emulation: true },
      { platform: 'grok', enabled: true, web_search_emulation: true }
    ])

    expect(featuresConfig).toEqual({
      existing_feature: { enabled: true },
      web_search_emulation: { openai: false }
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
    expect(supportsChannelWebSearchEmulation('grok')).toBe(false)
  })
})
