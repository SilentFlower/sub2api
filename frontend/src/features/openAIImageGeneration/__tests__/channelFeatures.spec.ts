import { describe, expect, it } from 'vitest'
import {
  applyChannelCodexImageBridgeFeatureConfig,
  readChannelCodexImageBridgeFeatureConfig
} from '../channelFeatures'

describe('渠道 Codex 生图桥接 features_config', () => {
  it('只序列化启用的 OpenAI 平台并保留其它字段', () => {
    const featuresConfig: Record<string, unknown> = {
      existing_feature: true,
      codex_image_generation_bridge: { openai: true }
    }

    applyChannelCodexImageBridgeFeatureConfig(featuresConfig, [
      { platform: 'openai', enabled: true, codex_image_generation_bridge: false },
      { platform: 'anthropic', enabled: true, codex_image_generation_bridge: true }
    ])

    expect(featuresConfig).toEqual({
      existing_feature: true,
      codex_image_generation_bridge: { openai: false }
    })
  })

  it('没有启用的 OpenAI 平台时删除旧配置', () => {
    const featuresConfig: Record<string, unknown> = {
      codex_image_generation_bridge: { openai: true }
    }

    applyChannelCodexImageBridgeFeatureConfig(featuresConfig, [])

    expect(featuresConfig).not.toHaveProperty('codex_image_generation_bridge')
  })

  it('只接受严格布尔值', () => {
    expect(readChannelCodexImageBridgeFeatureConfig({ codex_image_generation_bridge: { openai: true } }, 'openai')).toBe(true)
    expect(readChannelCodexImageBridgeFeatureConfig({ codex_image_generation_bridge: { openai: 1 } }, 'openai')).toBe(false)
  })
})
