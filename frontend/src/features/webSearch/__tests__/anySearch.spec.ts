import { describe, expect, it } from 'vitest'
import {
  hasRequiredWebSearchProviderAPIKey,
  isAnySearchProviderType,
  webSearchProviderAPIKeyPlaceholderKey
} from '../anySearch'

describe('AnySearch provider 规则', () => {
  it('允许 AnySearch API Key 留空', () => {
    expect(hasRequiredWebSearchProviderAPIKey({
      type: 'anysearch',
      api_key: '',
      api_key_configured: false
    })).toBe(true)
  })

  it('其它 provider 需要新密钥或已配置标记', () => {
    expect(hasRequiredWebSearchProviderAPIKey({
      type: 'brave',
      api_key: '',
      api_key_configured: false
    })).toBe(false)
    expect(hasRequiredWebSearchProviderAPIKey({
      type: 'tavily',
      api_key: '',
      api_key_configured: true
    })).toBe(true)
  })

  it('为 AnySearch 选择专属占位文案', () => {
    expect(isAnySearchProviderType('anysearch')).toBe(true)
    expect(webSearchProviderAPIKeyPlaceholderKey('anysearch')).toContain('anySearch')
    expect(webSearchProviderAPIKeyPlaceholderKey('brave')).toBe(
      'admin.settings.webSearchEmulation.apiKeyPlaceholder'
    )
  })
})
