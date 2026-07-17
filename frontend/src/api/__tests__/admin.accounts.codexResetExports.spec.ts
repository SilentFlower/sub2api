import { describe, expect, it } from 'vitest'

import {
  accountsAPI,
  consumeOpenAICodexResetCredit,
  getOpenAICodexResetStatus,
  sendOpenAICodexInvites
} from '@/api/admin/accounts'
import * as codexResetAPI from '@/features/openAICodexReset/api'

describe('admin account Codex reset 稳定导出', () => {
  it('中央模块和 accountsAPI 复用功能模块实现', () => {
    expect(getOpenAICodexResetStatus).toBe(codexResetAPI.getOpenAICodexResetStatus)
    expect(consumeOpenAICodexResetCredit).toBe(codexResetAPI.consumeOpenAICodexResetCredit)
    expect(sendOpenAICodexInvites).toBe(codexResetAPI.sendOpenAICodexInvites)
    expect(accountsAPI.getOpenAICodexResetStatus).toBe(codexResetAPI.getOpenAICodexResetStatus)
    expect(accountsAPI.consumeOpenAICodexResetCredit).toBe(codexResetAPI.consumeOpenAICodexResetCredit)
    expect(accountsAPI.sendOpenAICodexInvites).toBe(codexResetAPI.sendOpenAICodexInvites)
  })
})
