import { describe, expect, it } from 'vitest'

import {
  formatReasoningEffort,
  formatReasoningEffortMapping,
  reasoningEffortValuesEqual
} from '../format'

describe('formatReasoningEffort', () => {
  it('空值显示为占位符', () => {
    expect(formatReasoningEffort(null)).toBe('-')
    expect(formatReasoningEffort(undefined)).toBe('-')
    expect(formatReasoningEffort('  ')).toBe('-')
  })

  it('显示标准推理档位', () => {
    expect(formatReasoningEffort('none')).toBe('None')
    expect(formatReasoningEffort('minimal')).toBe('Minimal')
    expect(formatReasoningEffort('low')).toBe('Low')
    expect(formatReasoningEffort('medium')).toBe('Medium')
    expect(formatReasoningEffort('high')).toBe('High')
    expect(formatReasoningEffort('xhigh')).toBe('XHigh')
    expect(formatReasoningEffort('max')).toBe('Max')
  })

  it('兼容大小写和分隔符变体', () => {
    expect(formatReasoningEffort('x-high')).toBe('XHigh')
    expect(formatReasoningEffort(' Extra-High ')).toBe('XHigh')
    expect(formatReasoningEffort('MINIMAL')).toBe('Minimal')
  })

  it('未知值沿用展示兜底', () => {
    expect(formatReasoningEffort('banana')).toBe('Banana')
  })
})

describe('formatReasoningEffortMapping', () => {
  it('请求与转发档位相同时显示单个值', () => {
    expect(formatReasoningEffortMapping('max', 'max')).toBe('Max')
    expect(formatReasoningEffortMapping(null, 'high')).toBe('High')
  })

  it('映射改变档位时同时显示请求值和转发值', () => {
    expect(formatReasoningEffortMapping('max', 'xhigh')).toBe('Max → XHigh')
    expect(formatReasoningEffortMapping('high', 'medium')).toBe('High → Medium')
  })
})

describe('reasoningEffortValuesEqual', () => {
  it('将 x-high 别名识别为相同档位', () => {
    expect(reasoningEffortValuesEqual('x-high', 'xhigh')).toBe(true)
    expect(reasoningEffortValuesEqual('max', 'xhigh')).toBe(false)
  })
})
