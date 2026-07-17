import { describe, expect, it } from 'vitest'
import {
  applyOpenAICompatibilityExtra,
  normalizeOpenAIResponsesMode,
  normalizeWebSearchEmulationMode,
  readOpenAICompatibilityExtra
} from '../extra'

describe('OpenAI 兼容字段 extra 转换', () => {
  it('只更新功能键并保留其它账号数据', () => {
    const source = {
      email: 'user@example.com',
      grok_usage_snapshot: { status: 'observed' },
      quota_limit: 100
    }

    const result = applyOpenAICompatibilityExtra(source, {
      responsesMode: 'force_chat_completions',
      jsonSchemaToJSONObject: true,
      webSearchEmulation: 'enabled'
    })

    expect(result).toEqual({
      ...source,
      openai_responses_mode: 'force_chat_completions',
      openai_json_schema_to_json_object: true,
      web_search_emulation: 'enabled'
    })
    expect(source).not.toHaveProperty('openai_responses_mode')
  })

  it('使用默认值时只删除对应功能键', () => {
    const result = applyOpenAICompatibilityExtra(
      {
        email: 'user@example.com',
        openai_responses_mode: 'force_responses',
        openai_json_schema_to_json_object: true,
        web_search_emulation: 'disabled'
      },
      {
        responsesMode: 'auto',
        jsonSchemaToJSONObject: false,
        webSearchEmulation: 'default'
      }
    )

    expect(result).toEqual({ email: 'user@example.com' })
  })

  it('归一化非法值和历史 Web Search 布尔值', () => {
    expect(normalizeOpenAIResponsesMode('force_responses')).toBe('force_responses')
    expect(normalizeOpenAIResponsesMode('invalid')).toBe('auto')
    expect(normalizeWebSearchEmulationMode(true)).toBe('enabled')
    expect(normalizeWebSearchEmulationMode(false)).toBe('default')
  })

  it('一次投影全部兼容字段', () => {
    expect(readOpenAICompatibilityExtra({
      openai_responses_mode: 'force_responses',
      openai_json_schema_to_json_object: true,
      web_search_emulation: 'disabled'
    })).toEqual({
      responsesMode: 'force_responses',
      jsonSchemaToJSONObject: true,
      webSearchEmulation: 'disabled'
    })
    expect(readOpenAICompatibilityExtra(undefined)).toEqual({
      responsesMode: 'auto',
      jsonSchemaToJSONObject: false,
      webSearchEmulation: 'default'
    })
  })
})
