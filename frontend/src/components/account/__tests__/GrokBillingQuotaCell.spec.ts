import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import GrokBillingQuotaCell from '../GrokBillingQuotaCell.vue'
import type { Account, GrokBillingQuota } from '@/types'

const { queryBillingQuota } = vi.hoisted(() => ({
  queryBillingQuota: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    grok: { queryBillingQuota }
  }
}))

vi.mock('@/utils/format', () => ({
  formatRelativeTime: () => 'relative'
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (!params) return key
        return `${key}:${Object.values(params).join(',')}`
      }
    })
  }
})

const makeAccount = (id: number): Account => ({
  id,
  name: `grok-${id}`,
  platform: 'grok',
  type: 'oauth'
} as Account)

const makeQuota = (overrides: Partial<GrokBillingQuota> = {}): GrokBillingQuota => ({
  monthly_limit_cents: 15_000,
  monthly_used_cents: 5_000,
  monthly_remaining_cents: 10_000,
  monthly_used_percent: 33.333,
  weekly_used_percent: 25,
  weekly_reset_at: '2030-07-20T00:00:00Z',
  plan_label: 'supergrok',
  updated_at: new Date().toISOString(),
  ...overrides
})

const global = {
  stubs: {
    Icon: true,
    UsageProgressBar: {
      props: ['label', 'utilization', 'resetsAt', 'remainingCapacity'],
      template: '<div class="usage-bar">{{ label }}|{{ utilization }}|{{ resetsAt }}|{{ remainingCapacity }}</div>'
    }
  }
}

describe('GrokBillingQuotaCell', () => {
  beforeEach(() => {
    queryBillingQuota.mockReset()
  })

  it('uses a fresh snapshot without requesting the endpoint', async () => {
    const wrapper = mount(GrokBillingQuotaCell, {
      props: { account: makeAccount(6101), quota: makeQuota() },
      global
    })

    await flushPromises()

    expect(queryBillingQuota).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.accounts.usageWindow.grokBillingPlanSuperGrok')
    expect(wrapper.text()).toContain('admin.accounts.usageWindow.grokBillingMonthlyShort|66.666')
    expect(wrapper.text()).toContain('admin.accounts.usageWindow.grokBillingWeeklyShort|75')
  })

  it('refreshes stale data and emits the independent snapshot', async () => {
    const refreshed = makeQuota({ monthly_remaining_cents: 12_000, updated_at: new Date().toISOString() })
    queryBillingQuota.mockResolvedValue({
      source: 'grok_cli_billing_quota',
      snapshot: refreshed,
      fetched_at: 1
    })
    const wrapper = mount(GrokBillingQuotaCell, {
      props: { account: makeAccount(6102), quota: makeQuota({ stale: true }) },
      global
    })

    await flushPromises()

    expect(queryBillingQuota).toHaveBeenCalledWith(6102)
    expect(wrapper.emitted('updated')?.[0]?.[0]).toEqual(refreshed)
    expect(wrapper.text()).toContain('admin.accounts.usageWindow.grokBillingMonthlyShort|80')
  })

  it('shares a successful refresh cache across component instances', async () => {
    const refreshed = makeQuota({ monthly_remaining_cents: 11_000, updated_at: new Date().toISOString() })
    queryBillingQuota.mockResolvedValue({
      source: 'grok_cli_billing_quota',
      snapshot: refreshed,
      fetched_at: 1
    })
    const firstWrapper = mount(GrokBillingQuotaCell, {
      props: { account: makeAccount(6110), quota: makeQuota({ stale: true }) },
      global
    })
    await flushPromises()
    expect(queryBillingQuota).toHaveBeenCalledTimes(1)
    firstWrapper.unmount()

    queryBillingQuota.mockClear()
    const secondWrapper = mount(GrokBillingQuotaCell, {
      props: { account: makeAccount(6110), quota: makeQuota({ stale: true }) },
      global
    })
    await flushPromises()

    expect(queryBillingQuota).not.toHaveBeenCalled()
    expect(secondWrapper.emitted('updated')?.[0]?.[0]).toEqual(refreshed)
    expect(secondWrapper.text()).toContain('admin.accounts.usageWindow.grokBillingMonthlyShort|73.333')
    secondWrapper.unmount()
  })

  it('manual refresh bypasses a fresh snapshot', async () => {
    queryBillingQuota.mockResolvedValue({
      source: 'grok_cli_billing_quota',
      snapshot: makeQuota({ monthly_remaining_cents: 9_000 }),
      fetched_at: 1
    })
    const wrapper = mount(GrokBillingQuotaCell, {
      props: { account: makeAccount(6103), quota: makeQuota() },
      global
    })
    await flushPromises()
    expect(queryBillingQuota).not.toHaveBeenCalled()

    await wrapper.findAll('button')[0].trigger('click')
    await flushPromises()

    expect(queryBillingQuota).toHaveBeenCalledWith(6103)
  })

  it('keeps stale data visible when refresh fails', async () => {
    queryBillingQuota.mockRejectedValue({ message: 'billing unavailable' })
    const wrapper = mount(GrokBillingQuotaCell, {
      props: { account: makeAccount(6104), quota: makeQuota({ stale: true }) },
      global
    })

    await flushPromises()

    expect(wrapper.text()).toContain('admin.accounts.usageWindow.grokBillingMonthlyShort|66.666')
    expect(wrapper.text()).toContain('billing unavailable')
  })

  it('only shows pay-as-you-go disabled for an explicit zero cap', async () => {
    const unknownWrapper = mount(GrokBillingQuotaCell, {
      props: { account: makeAccount(6105), quota: makeQuota({ on_demand_cap_cents: undefined }) },
      global
    })
    await unknownWrapper.get('[data-testid="grok-billing-toggle"]').trigger('click')
    expect(unknownWrapper.text()).not.toContain('admin.accounts.usageWindow.grokBillingPayAsYouGoDisabled')

    const disabledWrapper = mount(GrokBillingQuotaCell, {
      props: { account: makeAccount(6106), quota: makeQuota({ on_demand_cap_cents: 0 }) },
      global
    })
    await disabledWrapper.get('[data-testid="grok-billing-toggle"]').trigger('click')
    expect(disabledWrapper.text()).toContain('admin.accounts.usageWindow.grokBillingPayAsYouGoDisabled')
  })

  it('limits automatic refreshes to two accounts', async () => {
    const resolvers: Array<(value: unknown) => void> = []
    queryBillingQuota.mockImplementation((id: number) => new Promise((resolve) => {
      resolvers.push(() => resolve({
        source: 'grok_cli_billing_quota',
        snapshot: makeQuota({ updated_at: new Date().toISOString() }),
        fetched_at: id
      }))
    }))

    const wrappers = [6107, 6108, 6109].map((id) => mount(GrokBillingQuotaCell, {
      props: { account: makeAccount(id), quota: makeQuota({ stale: true }) },
      global
    }))
    await flushPromises()
    expect(queryBillingQuota).toHaveBeenCalledTimes(2)

    resolvers[0](undefined)
    await flushPromises()
    expect(queryBillingQuota).toHaveBeenCalledTimes(3)

    resolvers.slice(1).forEach((resolve) => resolve(undefined))
    await flushPromises()
    wrappers.forEach((wrapper) => wrapper.unmount())
  })
})
