import { apiClient } from '@/api/client'
import type { GrokBillingQuotaResult } from './types'

/**
 * 主动刷新独立 Grok 套餐额度。
 *
 * @param id Grok OAuth 账号 ID。
 * @return 独立套餐额度刷新结果。
 */
export async function queryBillingQuota(id: number): Promise<GrokBillingQuotaResult> {
  const { data } = await apiClient.get<GrokBillingQuotaResult>(
    `/admin/grok/accounts/${id}/billing-quota`
  )
  return data
}
