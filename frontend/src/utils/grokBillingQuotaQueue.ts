import type { GrokBillingQuota } from '@/types'

type BillingQueueTask = () => void

/** Grok 套餐额度前端缓存有效期。 */
export const GROK_BILLING_QUOTA_CACHE_TTL_MS = 30 * 60 * 1000

const GROK_BILLING_QUOTA_MAX_CONCURRENT = 2
const billingRefreshCache = new Map<number, { data: GrokBillingQuota; ts: number }>()
const billingQueue: BillingQueueTask[] = []
let billingActiveCount = 0

const runNextBillingTask = () => {
  if (billingActiveCount >= GROK_BILLING_QUOTA_MAX_CONCURRENT) return
  const task = billingQueue.shift()
  if (!task) return
  billingActiveCount += 1
  task()
}

/**
 * 读取仍在 TTL 内的 Grok 套餐额度缓存。
 *
 * @param accountId Grok 账号 ID。
 * @returns 命中时返回缓存快照，否则返回 null。
 */
export function getCachedGrokBillingQuota(accountId: number): GrokBillingQuota | null {
  const cached = billingRefreshCache.get(accountId)
  if (!cached || Date.now() - cached.ts >= GROK_BILLING_QUOTA_CACHE_TTL_MS) return null
  return cached.data
}

/**
 * 写入 Grok 套餐额度模块缓存。
 *
 * @param accountId Grok 账号 ID。
 * @param quota 最近一次成功的套餐额度快照。
 * @returns 无返回值。
 */
export function setCachedGrokBillingQuota(accountId: number, quota: GrokBillingQuota): void {
  billingRefreshCache.set(accountId, { data: quota, ts: Date.now() })
}

/**
 * 将 Grok 套餐额度请求加入全局并发队列。
 *
 * @param request 实际发起独立 Billing 请求的异步函数。
 * @returns 请求完成后的结果 Promise。
 */
export function enqueueGrokBillingQuotaRequest<T>(request: () => Promise<T>): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    billingQueue.push(() => {
      request()
        .then(resolve)
        .catch(reject)
        .finally(() => {
          billingActiveCount = Math.max(0, billingActiveCount - 1)
          runNextBillingTask()
        })
    })
    runNextBillingTask()
  })
}
