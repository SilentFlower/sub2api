import type { WebSearchProviderConfig } from '@/api/admin/settings'

/** AnySearch provider 的稳定类型值。 */
export const anySearchProviderType = 'anysearch' as const

/** SettingsView 中追加的 AnySearch provider 选项。 */
export const anySearchProviderOption = {
  value: anySearchProviderType,
  label: 'AnySearch'
} as const

/**
 * 判断 provider 类型是否为 AnySearch。
 *
 * @param providerType provider 类型值。
 * @return 类型为 AnySearch 时返回 true。
 */
export function isAnySearchProviderType(providerType: string): boolean {
  return providerType === anySearchProviderType
}

/**
 * 返回 provider 对应的 API Key 占位文案键。
 *
 * @param providerType provider 类型值。
 * @return 对应的 i18n 文案键。
 */
export function webSearchProviderAPIKeyPlaceholderKey(providerType: string): string {
  return isAnySearchProviderType(providerType)
    ? 'admin.settings.webSearchEmulation.anySearchApiKeyPlaceholder'
    : 'admin.settings.webSearchEmulation.apiKeyPlaceholder'
}

/**
 * 判断 provider 是否满足保存时的 API Key 要求。
 *
 * @param provider 待保存的 provider 配置。
 * @return AnySearch 或已提供有效 API Key 时返回 true。
 */
export function hasRequiredWebSearchProviderAPIKey(
  provider: Pick<WebSearchProviderConfig, 'type' | 'api_key' | 'api_key_configured'>
): boolean {
  if (isAnySearchProviderType(provider.type)) return true
  return provider.api_key.trim() !== '' || provider.api_key_configured === true
}
