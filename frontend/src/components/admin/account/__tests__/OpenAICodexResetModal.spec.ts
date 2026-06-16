import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import OpenAICodexResetModal from '../OpenAICodexResetModal.vue'
import type { Account } from '@/types'

const { getOpenAICodexResetStatus, consumeOpenAICodexResetCredit, sendOpenAICodexInvites } = vi.hoisted(() => ({
  getOpenAICodexResetStatus: vi.fn(),
  consumeOpenAICodexResetCredit: vi.fn(),
  sendOpenAICodexInvites: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getOpenAICodexResetStatus,
      consumeOpenAICodexResetCredit,
      sendOpenAICodexInvites
    }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) => {
        if (key.endsWith('inviteHint')) return `已识别 ${params?.count ?? 0} 个邮箱`
        if (key.endsWith('invalidEmail')) return `邮箱格式无效：${params?.email}`
        if (key.endsWith('inviteSuccess')) return `已发送 ${params?.count ?? 0} 个邀请`
        if (key.endsWith('consumeSuccess')) return `已使用重置次数：${params?.credit ?? ''}`
        if (key.endsWith('totalCredits')) return `共 ${params?.count ?? 0} 次`
        return key
      }
    })
  }
})

function testAccount(): Account {
  return {
    id: 42,
    name: 'OpenAI OAuth',
    platform: 'openai',
    type: 'oauth',
    credentials: {},
    extra: { email: 'extra@example.com' },
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: true,
    created_at: '2026-06-16T00:00:00Z',
    updated_at: '2026-06-16T00:00:00Z',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null
  }
}

function statusPayload(availableCount = 1) {
  return {
    account: { id: 42, name: 'OpenAI OAuth', email: 'status@example.com' },
    available_count: availableCount,
    credit_count: availableCount,
    available_credit_ids: availableCount > 0 ? ['credit-1'] : [],
    credit_statuses: availableCount > 0 ? [{ id: 'credit-1', status: 'available', title: 'Reset' }] : [],
    eligibility: { eligible: true },
    rules: { max: 5 }
  }
}

function mountModal() {
  return mount(OpenAICodexResetModal, {
    props: {
      show: true,
      account: testAccount()
    },
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /></div>' },
        Icon: true
      }
    }
  })
}

describe('OpenAICodexResetModal', () => {
  beforeEach(() => {
    getOpenAICodexResetStatus.mockResolvedValue(statusPayload())
    consumeOpenAICodexResetCredit.mockResolvedValue({
      account: { id: 42, name: 'OpenAI OAuth' },
      credit_id: 'credit-1'
    })
    sendOpenAICodexInvites.mockResolvedValue({
      account: { id: 42, name: 'OpenAI OAuth' },
      emails: ['a@example.com', 'b@example.com'],
      invited_count: 2
    })
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('打开弹窗后查询并展示 reset credit 状态', async () => {
    const wrapper = mountModal()
    await flushPromises()

    expect(getOpenAICodexResetStatus).toHaveBeenCalledWith(42)
    expect(wrapper.text()).toContain('status@example.com')
    expect(wrapper.text()).toContain('Reset')
  })

  it('无可用 credit 时禁用使用按钮', async () => {
    getOpenAICodexResetStatus.mockResolvedValueOnce(statusPayload(0))
    const wrapper = mountModal()
    await flushPromises()

    const consumeButton = wrapper.findAll('button').find((button) => button.text().includes('consume'))
    expect(consumeButton?.attributes('disabled')).toBeDefined()
  })

  it('未勾选确认时不能发送邀请，勾选后会去重并提交', async () => {
    const wrapper = mountModal()
    await flushPromises()

    const textarea = wrapper.find('textarea')
    await textarea.setValue('a@example.com, A@example.com\nb@example.com')

    const sendButton = wrapper.findAll('button').find((button) => button.text().includes('sendInvite'))
    expect(sendButton?.attributes('disabled')).toBeDefined()

    await wrapper.find('input[type="checkbox"]').setValue(true)
    await sendButton!.trigger('click')
    await flushPromises()

    expect(sendOpenAICodexInvites).toHaveBeenCalledWith(42, {
      emails: ['a@example.com', 'b@example.com'],
      consent_confirmed: true
    })
    expect(wrapper.text()).toContain('已发送 2 个邀请')
  })

  it('超过 5 个邮箱时展示错误并禁用发送', async () => {
    const wrapper = mountModal()
    await flushPromises()

    await wrapper.find('textarea').setValue('a1@example.com a2@example.com a3@example.com a4@example.com a5@example.com a6@example.com')
    await wrapper.find('input[type="checkbox"]').setValue(true)

    expect(wrapper.text()).toContain('tooManyEmails')
    const sendButton = wrapper.findAll('button').find((button) => button.text().includes('sendInvite'))
    expect(sendButton?.attributes('disabled')).toBeDefined()
  })

  it('展示 apiClient 扁平错误对象中的 message', async () => {
    getOpenAICodexResetStatus.mockRejectedValueOnce({
      status: 400,
      reason: 'OPENAI_CODEX_RESET_NO_AVAILABLE_CREDIT',
      message: '没有可用重置次数'
    })

    const wrapper = mountModal()
    await flushPromises()

    expect(wrapper.text()).toContain('没有可用重置次数')
  })
})
