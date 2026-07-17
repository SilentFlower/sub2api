import { describe, expect, it } from 'vitest'

import {
  applyGrokForceChatExtra,
  readGrokForceChatMode,
  supportsGrokForceChat
} from '../extra'

describe('Grok Messages 强制 Chat extra', () => {
  it('只允许 Grok OAuth 和 API Key 账号配置', () => {
    expect(supportsGrokForceChat('grok', 'oauth')).toBe(true)
    expect(supportsGrokForceChat('grok', 'apikey')).toBe(true)
    expect(supportsGrokForceChat('grok', 'setup-token')).toBe(false)
    expect(supportsGrokForceChat('openai', 'apikey')).toBe(false)
  })

  it('读取非法值时回退 auto', () => {
    expect(readGrokForceChatMode({ openai_responses_mode: 'force_chat_completions' })).toBe('force_chat_completions')
    expect(readGrokForceChatMode({ openai_responses_mode: 'invalid' })).toBe('auto')
    expect(readGrokForceChatMode(undefined)).toBe('auto')
  })

  it('只更新路由键并保留独立套餐快照', () => {
    const source = {
      email: 'grok@example.com',
      grok_billing_quota_snapshot: { plan_label: 'supergrok' },
      openai_responses_mode: 'force_responses'
    }

    expect(applyGrokForceChatExtra(source, 'force_chat_completions')).toEqual({
      ...source,
      openai_responses_mode: 'force_chat_completions'
    })
    expect(applyGrokForceChatExtra(source, 'auto')).toEqual({
      email: 'grok@example.com',
      grok_billing_quota_snapshot: { plan_label: 'supergrok' }
    })
    expect(source.openai_responses_mode).toBe('force_responses')
  })
})
