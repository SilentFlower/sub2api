/** Grok 套餐额度中的单产品用量。 */
export interface GrokBillingQuotaProductUsage {
  product: string
  usage_percent?: number | null
}

/** 独立 Grok 套餐额度快照。 */
export interface GrokBillingQuota {
  period_type?: string
  weekly_used_percent?: number | null
  weekly_reset_at?: string
  product_usage?: GrokBillingQuotaProductUsage[] | null
  monthly_limit_cents?: number | null
  monthly_used_cents?: number | null
  monthly_remaining_cents?: number | null
  monthly_used_percent?: number | null
  billing_period_start?: string
  billing_period_end?: string
  on_demand_cap_cents?: number | null
  on_demand_used_cents?: number | null
  on_demand_remaining_cents?: number | null
  on_demand_used_percent?: number | null
  plan_label?: string
  updated_at: string
  stale?: boolean
  partial?: boolean
  failed_windows?: string[]
}

/** 独立 Grok 套餐额度刷新结果。 */
export interface GrokBillingQuotaResult {
  source: 'grok_cli_billing_quota'
  snapshot?: GrokBillingQuota | null
  fetched_at: number
}
