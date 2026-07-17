import type { Account, AccountUsageInfo } from '@/types'
import type { GrokBillingQuota } from './types'

const planLabelIsFree = (value: string): boolean => value.includes('free') || value.includes('basic')

const planLabelIsPaid = (value: string): boolean => {
  return value !== '' && !planLabelIsFree(value) && !value.includes('unknown')
}

const hasGrokBillingQuotaPaidSignal = (quota: GrokBillingQuota | null | undefined): boolean => {
  return quota?.weekly_used_percent != null ||
    quota?.monthly_used_percent != null ||
    (quota?.monthly_limit_cents != null && quota.monthly_limit_cents > 0)
}

/**
 * 判断独立 Grok 套餐快照是否提供官方 Billing 进度。
 *
 * @param quota 独立套餐额度快照。
 * @return 存在周、月进度或正数月额度时返回 true。
 */
export function hasGrokBillingQuotaProgress(quota: GrokBillingQuota | null | undefined): boolean {
  return quota?.weekly_used_percent != null ||
    quota?.monthly_used_percent != null ||
    (quota?.monthly_remaining_cents != null &&
      quota?.monthly_limit_cents != null &&
      quota.monthly_limit_cents > 0)
}

/**
 * 按独立套餐快照、被动 tier 和 entitlement 判断 Grok Free 账号。
 *
 * @param account 当前账号。
 * @param usage 当前账号用量投影。
 * @return 账号应使用 Free 24 小时额度展示时返回 true。
 */
export function isGrokBillingQuotaFreeAccount(
  account: Account,
  usage: AccountUsageInfo | null | undefined
): boolean {
  if (account.platform !== 'grok' || account.type !== 'oauth') return false

  const billing = usage?.grok_billing_quota
  if (hasGrokBillingQuotaPaidSignal(billing)) return false

  const plan = (billing?.plan_label || '').trim().toLowerCase()
  const tier = (usage?.subscription_tier || '').trim().toLowerCase()
  const entitlement = (usage?.grok_entitlement_status || '').toLowerCase()
  if (planLabelIsPaid(plan) || planLabelIsPaid(tier)) return false
  if (planLabelIsFree(plan) || planLabelIsFree(tier) || planLabelIsFree(entitlement)) return true
  return billing != null
}

/**
 * 合并独立 Grok 套餐额度刷新结果。
 *
 * @param usage 当前账号用量投影。
 * @param quota 最新独立套餐额度快照。
 * @return 只替换 grok_billing_quota 的新用量投影。
 */
export function mergeGrokBillingQuotaUsage(
  usage: AccountUsageInfo,
  quota: GrokBillingQuota
): AccountUsageInfo {
  return { ...usage, grok_billing_quota: quota }
}
