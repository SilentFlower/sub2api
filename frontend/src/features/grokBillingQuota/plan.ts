import type { Account } from '@/types'

const readRecord = (value: unknown): Record<string, unknown> | undefined => {
  return value != null && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : undefined
}

const readTruthyString = (value: unknown): string | undefined => {
  return typeof value === 'string' && value !== '' ? value : undefined
}

/**
 * 按独立 Billing、被动 quota 和账号字段解析 Grok 套餐标签。
 *
 * @param account Grok 账号。
 * @return 首个可用套餐标签，全部缺失时返回 undefined。
 */
export function resolveGrokBillingQuotaPlanType(account: Account): string | undefined {
  const extra = account.extra as Record<string, unknown> | undefined
  const credentials = account.credentials
  const billing = readRecord(extra?.grok_billing_quota_snapshot)
  const quota = readRecord(extra?.grok_usage_snapshot)

  return readTruthyString(billing?.plan_label) ||
    readTruthyString(quota?.subscription_tier) ||
    readTruthyString(credentials?.subscription_tier) ||
    readTruthyString(extra?.subscription_tier) ||
    readTruthyString(credentials?.plan_type) ||
    account.parent_plan_type ||
    undefined
}
