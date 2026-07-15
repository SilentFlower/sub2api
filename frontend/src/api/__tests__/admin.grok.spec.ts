import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post },
}))

import { createFromSSO, getGrokSSOImportTimeout, queryBillingQuota } from '@/api/admin/grok'

describe('admin Grok SSO import API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    post.mockResolvedValue({ data: { created: [], failed: [] } })
  })

  it('queries the independent Grok billing quota endpoint', async () => {
    const result = {
      source: 'grok_cli_billing_quota',
      snapshot: { updated_at: '2026-07-15T00:00:00Z' },
      fetched_at: 1,
    }
    get.mockResolvedValue({ data: result })

    await expect(queryBillingQuota(77)).resolves.toEqual(result)
    expect(get).toHaveBeenCalledWith('/admin/grok/accounts/77/billing-quota')
  })

  it.each([
    [1, 180_000],
    [3, 180_000],
    [4, 270_000],
    [7, 360_000],
  ])('uses a timeout sized for %i keys', async (keyCount, expectedTimeout) => {
    expect(getGrokSSOImportTimeout(keyCount)).toBe(expectedTimeout)

    await createFromSSO({
      sso_tokens: Array.from({ length: keyCount }, (_, index) => `sso-${index + 1}`),
    })

    expect(post).toHaveBeenCalledWith(
      '/admin/grok/sso-to-oauth',
      expect.objectContaining({ sso_tokens: expect.any(Array) }),
      { timeout: expectedTimeout },
    )
  })
})
