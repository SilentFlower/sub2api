import { describe, expect, it } from 'vitest'

import type { Account, AccountUsageInfo } from '@/types'
import { resolveGrokBillingQuotaPlanType } from '../plan'
import {
  hasGrokBillingQuotaProgress,
  isGrokBillingQuotaFreeAccount,
  mergeGrokBillingQuotaUsage
} from '../usage'

const grokOAuthAccount = {
  platform: 'grok',
  type: 'oauth'
} as Account

describe('Grok 独立套餐额度投影', () => {
  it('把周/月进度和正数月额度视为官方 Billing 进度', () => {
    expect(hasGrokBillingQuotaProgress({ weekly_used_percent: 0, updated_at: '2030-01-01T00:00:00Z' })).toBe(true)
    expect(hasGrokBillingQuotaProgress({ monthly_used_percent: 20, updated_at: '2030-01-01T00:00:00Z' })).toBe(true)
    expect(hasGrokBillingQuotaProgress({
      monthly_limit_cents: 1,
      monthly_remaining_cents: 1,
      updated_at: '2030-01-01T00:00:00Z'
    })).toBe(true)
    expect(hasGrokBillingQuotaProgress({ monthly_limit_cents: 1, updated_at: '2030-01-01T00:00:00Z' })).toBe(false)
    expect(hasGrokBillingQuotaProgress({ updated_at: '2030-01-01T00:00:00Z' })).toBe(false)
  })

  it('付费套餐和自定义月额度优先于过期 Free 信号', () => {
    expect(isGrokBillingQuotaFreeAccount(grokOAuthAccount, {
      grok_billing_quota: {
        plan_label: 'supergrok_heavy',
        updated_at: '2030-01-01T00:00:00Z'
      },
      grok_entitlement_status: 'free'
    } as AccountUsageInfo)).toBe(false)

    expect(isGrokBillingQuotaFreeAccount(grokOAuthAccount, {
      grok_billing_quota: {
        monthly_limit_cents: 25_000,
        updated_at: '2030-01-01T00:00:00Z'
      },
      subscription_tier: 'free'
    } as AccountUsageInfo)).toBe(false)
  })

  it('空套餐快照或 Free tier 使用 Free 展示', () => {
    expect(isGrokBillingQuotaFreeAccount(grokOAuthAccount, {
      grok_billing_quota: { updated_at: '2030-01-01T00:00:00Z' }
    } as AccountUsageInfo)).toBe(true)
    expect(isGrokBillingQuotaFreeAccount(grokOAuthAccount, {
      subscription_tier: 'FREE'
    } as AccountUsageInfo)).toBe(true)
  })

  it('套餐标签只读取独立 Billing，不读取 main 手动 Billing', () => {
    const account = {
      platform: 'grok',
      extra: {
        grok_billing_quota_snapshot: { plan_label: 'SuperGrok' },
        grok_billing_snapshot: { plan: 'SuperGrok Heavy' },
        grok_usage_snapshot: { subscription_tier: 'FREE' }
      },
      credentials: { subscription_tier: 'Basic' },
      parent_plan_type: 'legacy'
    } as Account

    expect(resolveGrokBillingQuotaPlanType(account)).toBe('SuperGrok')
  })

  it('刷新时只替换独立套餐额度字段', () => {
    const usage = {
      source: 'passive',
      updated_at: null,
      five_hour: null,
      seven_day: null,
      seven_day_sonnet: null,
      grok_billing: { plan: 'legacy-main' }
    } as AccountUsageInfo
    const quota = { plan_label: 'supergrok', updated_at: '2030-01-01T00:00:00Z' }

    expect(mergeGrokBillingQuotaUsage(usage, quota)).toEqual({
      ...usage,
      grok_billing_quota: quota
    })
    expect(usage).not.toHaveProperty('grok_billing_quota')
  })
})
