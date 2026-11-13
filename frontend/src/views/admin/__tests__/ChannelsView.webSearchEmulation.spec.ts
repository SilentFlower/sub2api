import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import {
  applyChannelWebSearchFeatureConfig,
  supportsChannelWebSearchEmulation
} from '@/features/webSearch/channelFeatures'
import en from '@/i18n/locales/en/admin/channels'
import zh from '@/i18n/locales/zh/admin/channels'
import type { GroupPlatform } from '@/types'

/** 渠道保存须保留所有受支持的平台维度，防止覆盖 API 写入的其他平台配置。 */
describe('渠道 Web Search 平台维度', () => {
  const source = readFileSync(resolve('src/views/admin/ChannelsView.vue'), 'utf8')
  const platforms: GroupPlatform[] = [
    'anthropic', 'openai', 'command_code', 'opencode_go', 'minimax', 'zhipu'
  ]

  it('领域白名单覆盖后端支持的平台', () => {
    for (const platform of platforms) {
      expect(supportsChannelWebSearchEmulation(platform)).toBe(true)
    }
    expect(supportsChannelWebSearchEmulation('grok')).toBe(false)
  })

  it('页面用领域白名单决定开关可见性', () => {
    expect(source).toContain('<ChannelWebSearchEmulationToggle')
    expect(source).toContain('supportsChannelWebSearchEmulation(section.platform) && webSearchGlobalEnabled')
  })

  it('保存通过领域入口写入所有平台并显式关闭禁用平台', () => {
    expect(source).toContain('applyChannelWebSearchFeatureConfig(featuresConfig, form.platforms)')
    const config: Record<string, unknown> = { web_search_emulation: { minimax: true } }
    applyChannelWebSearchFeatureConfig(config, platforms.map((platform) => ({
      platform,
      enabled: platform !== 'minimax',
      web_search_emulation: true
    })))
    expect(config.web_search_emulation).toEqual({
      anthropic: true,
      openai: true,
      command_code: true,
      opencode_go: true,
      minimax: false,
      zhipu: true
    })
  })

  it('最终中英文提示描述当前平台的账号', () => {
    expect(zh.channels.form.webSearchEmulationHint).toContain('该平台')
    expect(en.channels.form.webSearchEmulationHint).toContain('this platform')
  })
})
