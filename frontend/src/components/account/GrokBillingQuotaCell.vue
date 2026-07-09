<template>
  <div v-if="visible" class="max-w-[240px] space-y-1">
    <div class="flex items-center gap-1.5">
      <span class="shrink-0 rounded bg-emerald-50 px-1.5 py-0.5 text-[9px] font-medium text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300">
        {{ t('admin.accounts.usageWindow.grokBillingTitle') }}
      </span>
      <span v-if="planLabel" class="min-w-0 truncate text-[9px] text-gray-500 dark:text-gray-400">
        {{ planLabel }}
      </span>
      <button
        type="button"
        class="ml-auto inline-flex h-5 w-5 shrink-0 items-center justify-center rounded text-gray-500 transition-colors hover:bg-gray-200 hover:text-gray-700 disabled:cursor-not-allowed disabled:opacity-50 dark:text-gray-400 dark:hover:bg-gray-800 dark:hover:text-gray-200"
        :disabled="loading"
        :title="t('admin.accounts.usageWindow.grokBillingRefresh')"
        @click="refreshBilling(true)"
      >
        <svg
          class="h-3 w-3"
          :class="{ 'animate-spin': loading }"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
          />
        </svg>
      </button>
      <button
        type="button"
        data-testid="grok-billing-toggle"
        class="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded text-gray-500 transition-colors hover:bg-gray-200 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-gray-800 dark:hover:text-gray-200"
        :title="expandTitle"
        :aria-expanded="expanded"
        @click="expanded = !expanded"
      >
        <svg
          class="h-3 w-3 transition-transform"
          :class="{ 'rotate-180': expanded }"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
        </svg>
      </button>
    </div>

    <div v-if="currentQuota" class="space-y-1">
      <div
        v-for="row in compactRows"
        :key="row.key"
        class="flex items-center gap-1"
      >
        <span :class="['w-[32px] shrink-0 rounded px-1 text-center text-[10px] font-medium', compactLabelClass(row.color)]">
          {{ row.shortLabel }}
        </span>
        <div class="h-1.5 w-8 shrink-0 overflow-hidden rounded-full bg-gray-200 dark:bg-gray-700">
          <div
            :class="['h-full transition-all duration-300', compactBarClass(row.usagePercent)]"
            :style="{ width: barWidth(row.usagePercent) }"
          ></div>
        </div>
        <span :class="['w-[32px] shrink-0 text-right text-[10px] font-medium', compactTextClass(row.usagePercent)]">
          {{ formatUsedPercent(row.usagePercent) }}
        </span>
        <span v-if="row.resetAt" class="shrink-0 text-[10px] text-gray-400">
          {{ formatCompactResetTime(row.resetAt) }}
        </span>
      </div>
    </div>

    <div v-else-if="loading" class="flex items-center gap-1">
      <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      <div class="h-1.5 w-8 animate-pulse rounded-full bg-gray-200 dark:bg-gray-700"></div>
      <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
    </div>

    <div v-else class="text-[10px] text-gray-500 dark:text-gray-400">
      {{ t('admin.accounts.usageWindow.grokBillingEmpty') }}
    </div>

    <div v-if="expanded && currentQuota" class="space-y-1 rounded-md border border-gray-200 bg-gray-50 px-2 py-1.5 dark:border-gray-700 dark:bg-gray-900/40">
      <div v-if="monthlyRow" class="space-y-0.5">
        <div class="flex items-center justify-between gap-2 text-[10px]">
          <span class="font-medium text-gray-700 dark:text-gray-200">
            {{ monthlyRow.label }}
          </span>
          <span class="shrink-0 text-gray-500 dark:text-gray-400">
            {{ monthlyRow.meta }}
          </span>
        </div>
        <div class="flex items-center gap-1.5">
          <div class="h-1.5 flex-1 overflow-hidden rounded-full bg-gray-200 dark:bg-gray-700">
            <div
              :class="['h-full transition-all duration-300', barClass(monthlyRow.percent)]"
              :style="{ width: barWidth(monthlyRow.percent) }"
            ></div>
          </div>
          <span class="w-[34px] shrink-0 text-right text-[10px] font-medium text-gray-600 dark:text-gray-300">
            {{ formatRemainingPercent(monthlyRow.percent) }}
          </span>
        </div>
        <div v-if="monthlyRow.resetLabel" class="text-[9px] text-gray-400 dark:text-gray-500">
          {{ monthlyRow.resetLabel }}
        </div>
      </div>

      <div v-if="weeklyRow" class="space-y-0.5">
        <div class="flex items-center justify-between gap-2 text-[10px]">
          <span class="text-gray-600 dark:text-gray-300">{{ weeklyRow.label }}</span>
          <span class="shrink-0 text-gray-500 dark:text-gray-400">{{ weeklyRow.meta }}</span>
        </div>
        <div v-if="weeklyRow.resetLabel" class="text-[9px] text-gray-400 dark:text-gray-500">
          {{ weeklyRow.resetLabel }}
        </div>
      </div>

      <div
        v-for="row in productRows"
        :key="row.key"
        class="flex items-center justify-between gap-2 text-[9px] text-gray-500 dark:text-gray-400"
      >
        <span class="min-w-0 truncate">{{ row.label }}</span>
        <span class="shrink-0">{{ row.meta }}</span>
      </div>

      <div v-if="payAsYouGoRow" class="space-y-0.5">
        <div class="flex items-center justify-between gap-2 text-[10px]">
          <span class="text-gray-600 dark:text-gray-300">{{ payAsYouGoRow.label }}</span>
          <span class="shrink-0 text-gray-500 dark:text-gray-400">{{ payAsYouGoRow.meta }}</span>
        </div>
        <div class="flex items-center gap-1.5">
          <div class="h-1 flex-1 overflow-hidden rounded-full bg-gray-200 dark:bg-gray-700">
            <div
              :class="['h-full transition-all duration-300', barClass(payAsYouGoRow.percent)]"
              :style="{ width: barWidth(payAsYouGoRow.percent) }"
            ></div>
          </div>
          <span class="w-[34px] shrink-0 text-right text-[9px] text-gray-500 dark:text-gray-400">
            {{ formatRemainingPercent(payAsYouGoRow.percent) }}
          </span>
        </div>
      </div>
      <div v-else class="flex items-center justify-between gap-2 text-[9px] text-gray-500 dark:text-gray-400">
        <span>{{ t('admin.accounts.usageWindow.grokBillingPayAsYouGo') }}</span>
        <span>{{ t('admin.accounts.usageWindow.grokBillingPayAsYouGoDisabled') }}</span>
      </div>

      <div v-if="statusLabel" class="text-[9px] text-gray-400 dark:text-gray-500">
        {{ statusLabel }}
      </div>
    </div>

    <div v-if="error" class="truncate text-[10px] text-red-600 dark:text-red-400" :title="error">
      {{ error }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { Account, GrokBillingQuota } from '@/types'
import { formatRelativeTime } from '@/utils/format'

type BillingQueueTask = () => void

interface BillingRow {
  key: string
  label: string
  shortLabel: string
  percent: number | null
  usagePercent: number | null
  meta: string
  resetAt?: string
  resetLabel?: string
  color: 'indigo' | 'emerald'
}

const GROK_BILLING_CACHE_TTL = 30 * 60 * 1000
const GROK_BILLING_MAX_CONCURRENT = 2

const _billingRefreshCache = new Map<number, { data: GrokBillingQuota; ts: number }>()
const _billingQueue: BillingQueueTask[] = []
let _billingActiveCount = 0

const props = defineProps<{
  account: Account
  quota?: GrokBillingQuota | null
}>()

const emit = defineEmits<{
  updated: [quota: GrokBillingQuota]
}>()

const { t } = useI18n()

const currentQuota = ref<GrokBillingQuota | null>(props.quota ?? null)
const loading = ref(false)
const error = ref<string | null>(null)
const expanded = ref(false)

const visible = computed(() => props.account.platform === 'grok' && props.account.type === 'oauth')

const runNextBillingTask = () => {
  if (_billingActiveCount >= GROK_BILLING_MAX_CONCURRENT) return
  const task = _billingQueue.shift()
  if (!task) return
  _billingActiveCount += 1
  task()
}

const enqueueBillingRefresh = <T,>(fn: () => Promise<T>): Promise<T> => {
  return new Promise<T>((resolve, reject) => {
    const task = () => {
      fn()
        .then(resolve)
        .catch(reject)
        .finally(() => {
          _billingActiveCount = Math.max(0, _billingActiveCount - 1)
          runNextBillingTask()
        })
    }
    _billingQueue.push(task)
    runNextBillingTask()
  })
}

const parseUpdatedAt = (quota: GrokBillingQuota | null): number | null => {
  if (!quota?.updated_at) return null
  const ts = new Date(quota.updated_at).getTime()
  return Number.isFinite(ts) ? ts : null
}

const isQuotaFresh = (quota: GrokBillingQuota | null): boolean => {
  if (!quota || quota.stale) return false
  const updatedAt = parseUpdatedAt(quota)
  if (updatedAt === null) return false
  return Date.now() - updatedAt < GROK_BILLING_CACHE_TTL
}

const shouldAutoRefresh = computed(() => visible.value && !isQuotaFresh(currentQuota.value))

const extractErrorMessage = (e: unknown): string => {
  const err = e as {
    message?: string
    reason?: string
    response?: { data?: { message?: string; error?: string } }
  }
  return (
    err?.message ||
    err?.reason ||
    err?.response?.data?.message ||
    err?.response?.data?.error ||
    t('common.error')
  )
}

const refreshBilling = async (force = false) => {
  if (!visible.value || loading.value) return
  if (!force) {
    const cached = _billingRefreshCache.get(props.account.id)
    if (cached && Date.now() - cached.ts < GROK_BILLING_CACHE_TTL) {
      currentQuota.value = cached.data
      emit('updated', cached.data)
      return
    }
    if (isQuotaFresh(currentQuota.value)) return
  }

  loading.value = true
  error.value = null
  try {
    const result = await enqueueBillingRefresh(() => adminAPI.grok.queryBillingQuota(props.account.id))
    if (result.snapshot) {
      currentQuota.value = result.snapshot
      _billingRefreshCache.set(props.account.id, { data: result.snapshot, ts: Date.now() })
      emit('updated', result.snapshot)
    } else {
      error.value = t('admin.accounts.usageWindow.grokBillingEmpty')
    }
  } catch (e) {
    error.value = extractErrorMessage(e)
  } finally {
    loading.value = false
  }
}

const clampPercent = (value: number | null | undefined): number | null => {
  if (value == null || !Number.isFinite(value)) return null
  return Math.max(0, Math.min(100, value))
}

const remainingPercentFromUsed = (usedPercent: number | null | undefined): number | null => {
  const used = clampPercent(usedPercent)
  return used === null ? null : Math.max(0, 100 - used)
}

const remainingPercentFromAmount = (
  remainingCents: number | null | undefined,
  limitCents: number | null | undefined
): number | null => {
  if (remainingCents == null || limitCents == null || limitCents <= 0) return null
  return clampPercent((remainingCents / limitCents) * 100)
}

const usedPercentFromAmount = (
  usedCents: number | null | undefined,
  limitCents: number | null | undefined
): number | null => {
  if (usedCents == null || limitCents == null || limitCents <= 0) return null
  return clampPercent((usedCents / limitCents) * 100)
}

const usedPercentFromRemaining = (remainingPercent: number | null | undefined): number | null => {
  const remaining = clampPercent(remainingPercent)
  return remaining === null ? null : Math.max(0, 100 - remaining)
}

const formatCents = (cents: number | null | undefined): string => {
  if (cents == null || !Number.isFinite(cents)) return '--'
  return new Intl.NumberFormat(undefined, {
    style: 'currency',
    currency: 'USD'
  }).format(cents / 100)
}

const formatAmountPair = (
  remainingCents: number | null | undefined,
  limitCents: number | null | undefined
): string => {
  const remaining = formatCents(remainingCents)
  if (limitCents == null) return remaining
  return `${remaining} / ${formatCents(limitCents)}`
}

const formatRemainingPercent = (percent: number | null): string => {
  if (percent === null) return '--'
  return `${Math.round(percent)}%`
}

const formatUsedPercent = (percent: number | null): string => {
  if (percent === null) return '--'
  return `${Math.round(percent)}%`
}

const barWidth = (percent: number | null): string => {
  if (percent === null) return '0%'
  return `${Math.max(0, Math.min(100, percent))}%`
}

const barClass = (percent: number | null): string => {
  if (percent === null) return 'bg-gray-300 dark:bg-gray-600'
  if (percent <= 20) return 'bg-red-500'
  if (percent <= 50) return 'bg-amber-500'
  return 'bg-green-500'
}

const compactBarClass = (percent: number | null): string => {
  if (percent === null) return 'bg-gray-300 dark:bg-gray-600'
  if (percent >= 100) return 'bg-red-500'
  if (percent >= 80) return 'bg-amber-500'
  return 'bg-green-500'
}

const compactTextClass = (percent: number | null): string => {
  if (percent === null) return 'text-gray-500 dark:text-gray-400'
  if (percent >= 100) return 'text-red-600 dark:text-red-400'
  if (percent >= 80) return 'text-amber-600 dark:text-amber-400'
  return 'text-gray-600 dark:text-gray-400'
}

const compactLabelClass = (color: BillingRow['color']): string => {
  if (color === 'emerald') {
    return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
  }
  return 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-300'
}

const formatCompactResetTime = (value: string): string => {
  const date = new Date(value)
  const diffMs = date.getTime() - Date.now()
  if (!Number.isFinite(diffMs) || diffMs <= 0) {
    return t('usage.resetPending')
  }
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60))
  const diffMins = Math.floor((diffMs % (1000 * 60 * 60)) / (1000 * 60))
  if (diffHours >= 24) {
    const days = Math.floor(diffHours / 24)
    return `${days}d ${diffHours % 24}h`
  }
  if (diffHours > 0) {
    return `${diffHours}h ${diffMins}m`
  }
  return `${diffMins}m`
}

const monthlyRow = computed<BillingRow | null>(() => {
  const quota = currentQuota.value
  if (!quota || (quota.monthly_limit_cents == null && quota.monthly_used_cents == null)) return null
  const remainingPercent =
    remainingPercentFromUsed(quota.monthly_used_percent) ??
    remainingPercentFromAmount(quota.monthly_remaining_cents, quota.monthly_limit_cents)
  const usagePercent =
    clampPercent(quota.monthly_used_percent) ??
    usedPercentFromAmount(quota.monthly_used_cents, quota.monthly_limit_cents) ??
    usedPercentFromRemaining(remainingPercent)
  const resetLabel = quota.billing_period_end
    ? t('admin.accounts.usageWindow.grokBillingReset', { time: formatRelativeTime(quota.billing_period_end) })
    : undefined
  return {
    key: 'monthly',
    label: t('admin.accounts.usageWindow.grokBillingMonthly'),
    shortLabel: t('admin.accounts.usageWindow.grokBillingMonthlyShort'),
    percent: remainingPercent,
    usagePercent,
    meta: formatAmountPair(quota.monthly_remaining_cents, quota.monthly_limit_cents),
    resetAt: quota.billing_period_end,
    resetLabel,
    color: 'indigo'
  }
})

const weeklyRow = computed<BillingRow | null>(() => {
  const quota = currentQuota.value
  if (!quota || quota.weekly_used_percent == null) return null
  const usagePercent = clampPercent(quota.weekly_used_percent)
  const percent = remainingPercentFromUsed(usagePercent)
  return {
    key: 'weekly',
    label: t('admin.accounts.usageWindow.grokBillingWeekly'),
    shortLabel: t('admin.accounts.usageWindow.grokBillingWeeklyShort'),
    percent,
    usagePercent,
    meta: t('admin.accounts.usageWindow.grokBillingRemainingPercent', {
      percent: formatRemainingPercent(percent)
    }),
    resetAt: quota.weekly_reset_at,
    resetLabel: quota.weekly_reset_at
      ? t('admin.accounts.usageWindow.grokBillingReset', { time: formatRelativeTime(quota.weekly_reset_at) })
      : undefined,
    color: 'emerald'
  }
})

const productRows = computed<BillingRow[]>(() => {
  const usage = currentQuota.value?.product_usage
  if (!usage || usage.length === 0) return []
  return usage.map((item) => {
    const used = clampPercent(item.usage_percent)
    return {
      key: `product-${item.product}`,
      label: t('admin.accounts.usageWindow.grokBillingProductUsage', { product: item.product }),
      shortLabel: item.product,
      percent: remainingPercentFromUsed(item.usage_percent),
      usagePercent: used,
      meta: t('admin.accounts.usageWindow.grokBillingUsedPercent', {
        percent: formatUsedPercent(used)
      }),
      color: 'emerald'
    }
  })
})

const payAsYouGoRow = computed<BillingRow | null>(() => {
  const quota = currentQuota.value
  if (!quota || !quota.on_demand_cap_cents || quota.on_demand_cap_cents <= 0) return null
  const percent =
    remainingPercentFromUsed(quota.on_demand_used_percent) ??
    remainingPercentFromAmount(quota.on_demand_remaining_cents, quota.on_demand_cap_cents)
  return {
    key: 'pay-as-you-go',
    label: t('admin.accounts.usageWindow.grokBillingPayAsYouGo'),
    shortLabel: t('admin.accounts.usageWindow.grokBillingPayAsYouGo'),
    percent,
    usagePercent:
      clampPercent(quota.on_demand_used_percent) ??
      usedPercentFromAmount(quota.on_demand_used_cents, quota.on_demand_cap_cents) ??
      usedPercentFromRemaining(percent),
    meta: formatAmountPair(quota.on_demand_remaining_cents, quota.on_demand_cap_cents),
    color: 'emerald'
  }
})

const planLabel = computed(() => {
  const label = currentQuota.value?.plan_label
  if (label === 'supergrok') return t('admin.accounts.usageWindow.grokBillingPlanSuperGrok')
  if (label === 'supergrok_heavy') return t('admin.accounts.usageWindow.grokBillingPlanSuperGrokHeavy')
  return null
})

const statusLabel = computed(() => {
  const quota = currentQuota.value
  if (!quota?.updated_at) return null
  const time = formatRelativeTime(quota.updated_at)
  return quota.stale
    ? t('admin.accounts.usageWindow.grokBillingStale', { time })
    : t('admin.accounts.usageWindow.grokBillingUpdated', { time })
})

const compactRows = computed(() => {
  return [monthlyRow.value, weeklyRow.value].filter((row): row is BillingRow => row !== null)
})

const expandTitle = computed(() => {
  return expanded.value
    ? t('admin.accounts.usageWindow.grokBillingCollapse')
    : t('admin.accounts.usageWindow.grokBillingExpand')
})

watch(
  () => props.quota,
  (quota) => {
    currentQuota.value = quota ?? null
  }
)

watch(
  () => props.account.id,
  () => {
    currentQuota.value = props.quota ?? null
    loading.value = false
    error.value = null
    expanded.value = false
  }
)

onMounted(() => {
  if (shouldAutoRefresh.value) {
    refreshBilling(false)
  }
})
</script>
