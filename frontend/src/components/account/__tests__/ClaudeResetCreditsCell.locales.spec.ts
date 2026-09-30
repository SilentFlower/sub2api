import { runInNewContext } from 'node:vm'
import { baseCompile } from '@intlify/message-compiler'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n, type MessageFunction } from 'vue-i18n'
import type { Account } from '@/types'
import type { ClaudeResetCredits } from '@/api/admin/claudeResetCredits'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'
import ClaudeResetCreditsCell from '../ClaudeResetCreditsCell.vue'

const { getCredits, redeem } = vi.hoisted(() => ({
  getCredits: vi.fn(),
  redeem: vi.fn()
}))

vi.mock('@/api/admin/claudeResetCredits', () => ({
  getClaudeResetCredits: getCredits,
  redeemClaudeResetCredit: redeem
}))

const account = { id: 1, platform: 'anthropic', type: 'oauth' } as Account
const snapshot: ClaudeResetCredits = {
  eligible: true,
  available_count: 1,
  fetched_at: '2026-09-30T00:00:00Z',
  credits: [{
    label: 'test credit',
    resets_left: 1,
    clears: ['five_hour', 'seven_day'],
    percent_used: {},
    blocking: [],
    use_requires_limit: false,
    redeemable: true
  }]
}

describe('Claude 重置界面真实双语文案', () => {
  beforeEach(() => {
    getCredits.mockReset().mockResolvedValue(snapshot)
    redeem.mockReset()
  })

  it.each([
    { locale: 'en', count: 'Resets', reset: 'Reset', title: 'Confirm Claude Reset', notice: 'cannot be undone' },
    { locale: 'zh', count: '次数', reset: '重置', title: '确认使用 Claude 重置', notice: '不可撤销' }
  ])('$locale 查询和确认界面使用最终 locale，取消不兑换', async ({ locale, count, reset, title, notice }) => {
    const i18n = createI18n({
      legacy: false,
      locale,
      messages: { en, zh },
      // Vitest 沿用 runtime-only 别名；用官方编译器恢复真实插值，避免以 key 代替界面文案。
      messageCompiler: (message, { onError }) => {
        if (typeof message !== 'string') throw new Error('测试仅编译源码字符串消息')
        return runInNewContext(`(${baseCompile(message, { onError }).code})`) as MessageFunction
      }
    })
    const wrapper = mount(ClaudeResetCreditsCell, {
      props: { account },
      global: {
        plugins: [i18n],
        stubs: {
          BaseDialog: {
            props: ['show', 'title'],
            template: '<div v-if="show" data-testid="reset-dialog"><h2>{{ title }}</h2><slot /><slot name="footer" /></div>'
          }
        }
      }
    })
    expect(wrapper.get('[data-testid="claude-reset-count"]').text()).toBe(count)
    expect(wrapper.get('[data-testid="claude-reset-redeem"]').text()).toBe(reset)
    expect(getCredits).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="claude-reset-count"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="claude-reset-redeem"]').trigger('click')
    const dialog = wrapper.get('[data-testid="reset-dialog"]')
    expect(dialog.get('h2').text()).toBe(title)
    expect(dialog.text()).toContain(notice)
    expect(dialog.text()).toContain('5h, 7d')
    expect(dialog.text()).not.toContain('{windows}')
    expect(dialog.text()).not.toContain('{count}')
    expect(dialog.text()).not.toContain('admin.accounts.')
    expect(redeem).not.toHaveBeenCalled()
    await dialog.findAll('button')[0]!.trigger('click')
    expect(wrapper.find('[data-testid="reset-dialog"]').exists()).toBe(false)
    expect(redeem).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
