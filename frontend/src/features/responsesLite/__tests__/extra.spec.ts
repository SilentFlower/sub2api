import { describe, expect, it } from 'vitest'
import {
  RESPONSES_LITE_DOWNGRADE_EXTRA_KEY,
  applyResponsesLiteDowngradeExtra,
  readResponsesLiteDowngrade,
  supportsResponsesLiteDowngrade
} from '../extra'

describe('Responses Lite 降级开关 extra 转换', () => {
  it('写入开关时保留其它账号数据且不修改源对象', () => {
    const source = { email: 'user@example.com', quota_limit: 100 }
    const result = applyResponsesLiteDowngradeExtra(source, true)
    expect(result).toEqual({ ...source, [RESPONSES_LITE_DOWNGRADE_EXTRA_KEY]: true })
    expect(source).not.toHaveProperty(RESPONSES_LITE_DOWNGRADE_EXTRA_KEY)
  })

  it('关闭或置空时删除开关，undefined 保持原值', () => {
    const source = { email: 'user@example.com', [RESPONSES_LITE_DOWNGRADE_EXTRA_KEY]: true }
    expect(applyResponsesLiteDowngradeExtra(source, false)).toEqual({ email: 'user@example.com' })
    expect(applyResponsesLiteDowngradeExtra(source, null)).toEqual({ email: 'user@example.com' })
    expect(applyResponsesLiteDowngradeExtra(source, undefined)).toEqual(source)
    expect(applyResponsesLiteDowngradeExtra(undefined, true)).toEqual({ [RESPONSES_LITE_DOWNGRADE_EXTRA_KEY]: true })
  })

  it('只把布尔 true 读成开启', () => {
    expect(readResponsesLiteDowngrade({ [RESPONSES_LITE_DOWNGRADE_EXTRA_KEY]: true })).toBe(true)
    expect(readResponsesLiteDowngrade({ [RESPONSES_LITE_DOWNGRADE_EXTRA_KEY]: 'true' })).toBe(false)
    expect(readResponsesLiteDowngrade({ [RESPONSES_LITE_DOWNGRADE_EXTRA_KEY]: false })).toBe(false)
    expect(readResponsesLiteDowngrade(undefined)).toBe(false)
  })

  it('只对 API Key 的 openai 与国产供应商账号显示', () => {
    expect(supportsResponsesLiteDowngrade('openai', 'apikey')).toBe(true)
    expect(supportsResponsesLiteDowngrade('deepseek', 'apikey')).toBe(true)
    expect(supportsResponsesLiteDowngrade('kimi', 'apikey')).toBe(true)
    expect(supportsResponsesLiteDowngrade('openai', 'oauth')).toBe(false)
    expect(supportsResponsesLiteDowngrade('grok', 'apikey')).toBe(false)
    expect(supportsResponsesLiteDowngrade('anthropic', 'apikey')).toBe(false)
    expect(supportsResponsesLiteDowngrade(undefined, 'apikey')).toBe(false)
  })
})
