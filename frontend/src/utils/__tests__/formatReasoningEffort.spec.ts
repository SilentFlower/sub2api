import { describe, expect, it } from 'vitest'

import { formatReasoningEffort } from '../format'

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
    expect(formatReasoningEffort(' Extra-High ')).toBe('XHigh')
    expect(formatReasoningEffort('MINIMAL')).toBe('Minimal')
  })

  it('未知值沿用展示兜底', () => {
    expect(formatReasoningEffort('banana')).toBe('Banana')
  })
})
