import { describe, expect, it } from 'vitest'
import {
  defaultResponsesLiteBlockedModels,
  normalizeResponsesLiteBlockedModelRules
} from '../modelRules'

describe('Responses Lite 模型规则', () => {
  it('默认列表不是空列表且每次返回独立副本', () => {
    const first = defaultResponsesLiteBlockedModels()
    const second = defaultResponsesLiteBlockedModels()

    expect(first).toEqual(['gpt-5.4', 'gpt-5.4-mini', 'gpt-5.5'])
    expect(first).not.toBe(second)
  })

  it('去除空白并稳定去重', () => {
    expect(
      normalizeResponsesLiteBlockedModelRules([' gpt-5.4* ', 'gpt-5.5', 'gpt-5.4*'])
    ).toEqual({ models: ['gpt-5.4*', 'gpt-5.5'] })
  })

  it('拒绝空规则和非末尾通配符', () => {
    expect(normalizeResponsesLiteBlockedModelRules([''])).toEqual({ models: [], error: 'empty' })
    expect(normalizeResponsesLiteBlockedModelRules(['gpt-*5'])).toEqual({
      models: [],
      error: 'wildcard'
    })
  })

  it('显式空列表表示允许全部透传', () => {
    expect(normalizeResponsesLiteBlockedModelRules([])).toEqual({ models: [] })
  })
})
