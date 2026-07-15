import { afterEach, describe, expect, it, vi } from 'vitest'
import type { GrokBillingQuota } from '@/types'
import {
  getCachedGrokBillingQuota,
  GROK_BILLING_QUOTA_CACHE_TTL_MS,
  setCachedGrokBillingQuota
} from '../grokBillingQuotaQueue'

describe('grokBillingQuotaQueue', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('expires cached quota after the real TTL boundary', () => {
    vi.useFakeTimers()
    const now = new Date('2026-07-15T00:00:00Z')
    vi.setSystemTime(now)
    const quota: GrokBillingQuota = {
      monthly_limit_cents: 15_000,
      updated_at: now.toISOString()
    }

    setCachedGrokBillingQuota(6201, quota)
    expect(getCachedGrokBillingQuota(6201)).toBe(quota)

    vi.setSystemTime(new Date(now.getTime() + GROK_BILLING_QUOTA_CACHE_TTL_MS))
    expect(getCachedGrokBillingQuota(6201)).toBeNull()
  })
})
