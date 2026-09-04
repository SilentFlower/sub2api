import { describe, expect, it } from 'vitest'
import enAccounts from '../locales/en/admin/accounts'
import enAccountsCodexCustomClients from '../locales/en/admin/accountsCodexCustomClients'
import enAccountsOpenAICompatibility from '../locales/en/admin/accountsOpenAICompatibility'
import enAccountsOpenAIImageGeneration from '../locales/en/admin/accountsOpenAIImageGenerationOverrides'
import enAccountsWebSearch from '../locales/en/admin/accountsWebSearch'
import enChannels from '../locales/en/admin/channels'
import enChannelsOpenAIImageGeneration from '../locales/en/admin/channelsOpenAIImageGenerationOverrides'
import enChannelsWebSearch from '../locales/en/admin/channelsWebSearchOverrides'
import enSettings from '../locales/en/admin/settings'
import enSettingsOpenAIImageGeneration from '../locales/en/admin/settingsOpenAIImageGeneration'
import enSettingsResponsesLite from '../locales/en/admin/settingsResponsesLite'
import enSettingsDeepSeekReasoning from '../locales/en/admin/settingsDeepSeekReasoning'
import enSettingsWebSearchAnySearch from '../locales/en/admin/settingsWebSearchAnySearch'
import zhAccounts from '../locales/zh/admin/accounts'
import zhAccountsCodexCustomClients from '../locales/zh/admin/accountsCodexCustomClients'
import zhAccountsOpenAICompatibility from '../locales/zh/admin/accountsOpenAICompatibility'
import zhAccountsOpenAIImageGeneration from '../locales/zh/admin/accountsOpenAIImageGenerationOverrides'
import zhAccountsWebSearch from '../locales/zh/admin/accountsWebSearch'
import zhChannels from '../locales/zh/admin/channels'
import zhChannelsOpenAIImageGeneration from '../locales/zh/admin/channelsOpenAIImageGenerationOverrides'
import zhChannelsWebSearch from '../locales/zh/admin/channelsWebSearchOverrides'
import zhSettings from '../locales/zh/admin/settings'
import zhSettingsOpenAIImageGeneration from '../locales/zh/admin/settingsOpenAIImageGeneration'
import zhSettingsResponsesLite from '../locales/zh/admin/settingsResponsesLite'
import zhSettingsDeepSeekReasoning from '../locales/zh/admin/settingsDeepSeekReasoning'
import zhSettingsWebSearchAnySearch from '../locales/zh/admin/settingsWebSearchAnySearch'

function keyPaths(value: unknown, prefix = ''): string[] {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return [prefix]
  return Object.entries(value as Record<string, unknown>)
    .flatMap(([key, child]) => keyPaths(child, prefix ? `${prefix}.${key}` : key))
    .sort()
}

function getPath(root: Record<string, unknown>, path: string): unknown {
  return path.split('.').reduce<unknown>((current, part) => {
    if (!current || typeof current !== 'object') return undefined
    return (current as Record<string, unknown>)[part]
  }, root)
}

const localeExtensionPairs: Array<[string, unknown, unknown]> = [
  ['accounts Codex custom clients', enAccountsCodexCustomClients, zhAccountsCodexCustomClients],
  ['accounts OpenAI compatibility', enAccountsOpenAICompatibility, zhAccountsOpenAICompatibility],
  ['accounts OpenAI image generation', enAccountsOpenAIImageGeneration, zhAccountsOpenAIImageGeneration],
  ['accounts Web Search', enAccountsWebSearch, zhAccountsWebSearch],
  ['settings OpenAI image generation', enSettingsOpenAIImageGeneration, zhSettingsOpenAIImageGeneration],
  ['settings Responses Lite', enSettingsResponsesLite, zhSettingsResponsesLite],
  ['settings DeepSeek reasoning', enSettingsDeepSeekReasoning, zhSettingsDeepSeekReasoning],
  ['settings AnySearch', enSettingsWebSearchAnySearch, zhSettingsWebSearchAnySearch],
  ['channels Web Search', enChannelsWebSearch, zhChannelsWebSearch],
  ['channels OpenAI image generation', enChannelsOpenAIImageGeneration, zhChannelsOpenAIImageGeneration]
]

describe('build 功能 locale 扩展', () => {
  it.each(localeExtensionPairs)('%s 中英文 key 结构一致', (_name, en, zh) => {
    expect(keyPaths(en)).toEqual(keyPaths(zh))
  })

  it('主模块在原 key path 暴露扩展文案', () => {
    expect(getPath(enAccounts, 'accounts.openai.codexCLIOnlyCustomUA')).toBe('Custom allowed UA prefixes')
    expect(getPath(enAccounts, 'accounts.openai.codexImageToolDesc')).toContain('only to non-Responses Lite requests')
    expect(getPath(zhAccounts, 'accounts.openai.codexImageToolDesc')).toContain('仅适用于非 Responses Lite 请求')
    expect(getPath(enSettings, 'settings.gatewayForwarding.openaiResponsesLiteBlockedModels')).toBe(
      'Responses Lite Header blocked models'
    )
    expect(getPath(enSettings, 'settings.gatewayForwarding.deepSeekMissingReasoningAutoDowngradeHint')).toContain(
      'Anthropic-to-Chat'
    )
    expect(getPath(zhSettings, 'settings.webSearchEmulation.anySearchApiKeyHint')).toContain('AnySearch')
    expect(getPath(enChannels, 'channels.form.webSearchEmulationHint')).toContain('eligible API Key accounts')
    expect(getPath(enChannels, 'channels.form.codexImageGenerationBridgeHint')).toContain('does not inject tools for Responses Lite')
    expect(getPath(zhChannels, 'channels.form.codexImageGenerationBridgeHint')).toContain('桥接不会为 Responses Lite 注入工具')
  })
})
