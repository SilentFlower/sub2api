import { describe, expect, it } from 'vitest'
import {
  ALPHA_SEARCH_VIA_RESPONSES_EXTRA_KEY,
  applyAlphaSearchViaResponsesExtra,
  readAlphaSearchViaResponses,
  supportsAlphaSearchViaResponses
} from '../extra'

describe('Alpha Search 经上游 Responses 开关 extra 转换', () => {
  it('写入开关时保留其它账号数据且不修改源对象', () => {
    const source = { email: 'user@example.com', quota_limit: 100 }
    const result = applyAlphaSearchViaResponsesExtra(source, true)
    expect(result).toEqual({ ...source, [ALPHA_SEARCH_VIA_RESPONSES_EXTRA_KEY]: true })
    expect(source).not.toHaveProperty(ALPHA_SEARCH_VIA_RESPONSES_EXTRA_KEY)
  })

  it('关闭或置空时删除开关，undefined 保持原值', () => {
    const source = { email: 'user@example.com', [ALPHA_SEARCH_VIA_RESPONSES_EXTRA_KEY]: true }
    expect(applyAlphaSearchViaResponsesExtra(source, false)).toEqual({ email: 'user@example.com' })
    expect(applyAlphaSearchViaResponsesExtra(source, null)).toEqual({ email: 'user@example.com' })
    expect(applyAlphaSearchViaResponsesExtra(source, undefined)).toEqual(source)
    expect(applyAlphaSearchViaResponsesExtra(undefined, true)).toEqual({ [ALPHA_SEARCH_VIA_RESPONSES_EXTRA_KEY]: true })
  })

  it('只把布尔 true 读成开启', () => {
    expect(readAlphaSearchViaResponses({ [ALPHA_SEARCH_VIA_RESPONSES_EXTRA_KEY]: true })).toBe(true)
    expect(readAlphaSearchViaResponses({ [ALPHA_SEARCH_VIA_RESPONSES_EXTRA_KEY]: 'true' })).toBe(false)
    expect(readAlphaSearchViaResponses({ [ALPHA_SEARCH_VIA_RESPONSES_EXTRA_KEY]: false })).toBe(false)
    expect(readAlphaSearchViaResponses(undefined)).toBe(false)
  })

  it('只对 openai 平台的 API Key 账号显示', () => {
    expect(supportsAlphaSearchViaResponses('openai', 'apikey')).toBe(true)
    expect(supportsAlphaSearchViaResponses('deepseek', 'apikey')).toBe(false)
    expect(supportsAlphaSearchViaResponses('openai', 'oauth')).toBe(false)
    expect(supportsAlphaSearchViaResponses('grok', 'apikey')).toBe(false)
    expect(supportsAlphaSearchViaResponses(undefined, 'apikey')).toBe(false)
  })
})
