<template>
  <div v-if="visible" class="max-w-[240px] space-y-1">
    <div class="flex min-w-0 items-center gap-1.5">
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
        <Icon name="refresh" size="xs" :class="{ 'animate-spin': loading }" />
      </button>
      <button
        type="button"
        data-testid="grok-billing-toggle"
        class="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded text-gray-500 transition-colors hover:bg-gray-200 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-gray-800 dark:hover:text-gray-200"
        :title="expandTitle"
        :aria-expanded="expanded"
        @click="expanded = !expanded"
      >
        <Icon name="chevronDown" size="xs" class="transition-transform" :class="{ 'rotate-180': expanded }" />
      </button>
    </div>

    <div v-if="currentQuota && compactRows.length > 0" class="space-y-1">
      <UsageProgressBar
        v-for="row in compactRows"
        :key="row.key"
        :label="row.shortLabel"
        :utilization="row.remainingPercent"
        :resets-at="row.resetAt"
        :remaining-capacity="true"
        :color="row.color"
      />
    </div>
    <div v-else-if="loading" class="flex items-center gap-1">
      <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
      <div class="h-1.5 w-8 animate-pulse rounded-full bg-gray-200 dark:bg-gray-700"></div>
      <div class="h-3 w-[32px] animate-pulse rounded bg-gray-200 dark:bg-gray-700"></div>
    </div>
    <div v-else-if="!currentQuota" class="text-[10px] text-gray-500 dark:text-gray-400">
      {{ t('admin.accounts.usageWindow.grokBillingEmpty') }}
    </div>

    <div v-if="expanded && currentQuota" class="space-y-1 border-l-2 border-gray-200 pl-2 dark:border-gray-700">
      <div v-if="monthlyMeta" class="flex items-center justify-between gap-2 text-[10px]">
        <span class="text-gray-600 dark:text-gray-300">
          {{ t('admin.accounts.usageWindow.grokBillingMonthly') }}
        </span>
        <span class="min-w-0 truncate text-right text-gray-500 dark:text-gray-400">
          {{ monthlyMeta }}
        </span>
      </div>
      <div v-if="monthlyResetLabel" class="text-[9px] text-gray-400 dark:text-gray-500">
        {{ monthlyResetLabel }}
      </div>

      <div v-if="weeklyMeta" class="flex items-center justify-between gap-2 text-[10px]">
        <span class="text-gray-600 dark:text-gray-300">
          {{ t('admin.accounts.usageWindow.grokBillingWeekly') }}
        </span>
        <span class="shrink-0 text-gray-500 dark:text-gray-400">{{ weeklyMeta }}</span>
      </div>

      <div
        v-for="product in productRows"
        :key="product.key"
        class="flex items-center justify-between gap-2 text-[9px] text-gray-500 dark:text-gray-400"
      >
        <span class="min-w-0 truncate">{{ product.label }}</span>
        <span class="shrink-0">{{ product.meta }}</span>
      </div>

      <div
        v-if="payAsYouGoRemainingPercent !== null"
        :title="t('admin.accounts.usageWindow.grokBillingPayAsYouGo')"
      >
        <UsageProgressBar
          :label="t('admin.accounts.usageWindow.grokBillingPayAsYouGoShort')"
          :utilization="payAsYouGoRemainingPercent"
          :remaining-capacity="true"
          color="amber"
        />
      </div>
      <div
        v-else-if="payAsYouGoExplicitlyDisabled"
        class="flex items-center justify-between gap-2 text-[9px] text-gray-500 dark:text-gray-400"
      >
        <span>{{ t('admin.accounts.usageWindow.grokBillingPayAsYouGo') }}</span>
        <span>{{ t('admin.accounts.usageWindow.grokBillingPayAsYouGoDisabled') }}</span>
      </div>
      <div v-if="payAsYouGoMeta" class="text-right text-[9px] text-gray-400 dark:text-gray-500">
        {{ payAsYouGoMeta }}
      </div>

      <div v-if="statusLabel" class="text-[9px] text-gray-400 dark:text-gray-500" :title="failedWindowsTitle">
        {{ statusLabel }}
      </div>
    </div>

    <div v-if="error" class="truncate text-[10px] text-red-600 dark:text-red-400" :title="error">
      {{ error }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import Icon from '@/components/icons/Icon.vue'
import type { Account, GrokBillingQuota } from '@/types'
import { formatRelativeTime } from '@/utils/format'
import {
  enqueueGrokBillingQuotaRequest,
  getCachedGrokBillingQuota,
  GROK_BILLING_QUOTA_CACHE_TTL_MS,
  setCachedGrokBillingQuota
} from '@/utils/grokBillingQuotaQueue'
import UsageProgressBar from './UsageProgressBar.vue'

interface CompactRow {
  key: string
  shortLabel: string
  remainingPercent: number
  resetAt?: string
  color: 'indigo' | 'emerald'
}

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
const unmounted = ref(false)

const visible = computed(() => props.account.platform === 'grok' && props.account.type === 'oauth')

const parseUpdatedAt = (quota: GrokBillingQuota | null): number | null => {
  if (!quota?.updated_at) return null
  const timestamp = new Date(quota.updated_at).getTime()
  return Number.isFinite(timestamp) ? timestamp : null
}

const isQuotaFresh = (quota: GrokBillingQuota | null): boolean => {
  if (!quota || quota.stale) return false
  const updatedAt = parseUpdatedAt(quota)
  return updatedAt !== null && Date.now() - updatedAt < GROK_BILLING_QUOTA_CACHE_TTL_MS
}

const extractErrorMessage = (value: unknown): string => {
  const candidate = value as { message?: string; reason?: string }
  return candidate?.message || candidate?.reason || t('common.error')
}

const refreshBilling = async (force = false) => {
  if (!visible.value || loading.value) return
  if (!force) {
    const cached = getCachedGrokBillingQuota(props.account.id)
    if (cached) {
      currentQuota.value = cached
      emit('updated', cached)
      return
    }
    if (isQuotaFresh(currentQuota.value)) return
  }

  loading.value = true
  error.value = null
  try {
    const result = await enqueueGrokBillingQuotaRequest(() => adminAPI.grok.queryBillingQuota(props.account.id))
    if (unmounted.value) return
    if (!result.snapshot) {
      error.value = t('admin.accounts.usageWindow.grokBillingEmpty')
      return
    }
    currentQuota.value = result.snapshot
    setCachedGrokBillingQuota(props.account.id, result.snapshot)
    emit('updated', result.snapshot)
  } catch (caught) {
    if (!unmounted.value) error.value = extractErrorMessage(caught)
  } finally {
    if (!unmounted.value) loading.value = false
  }
}

const clampPercent = (value: number | null | undefined): number | null => {
  if (value == null || !Number.isFinite(value)) return null
  return Math.max(0, Math.min(100, value))
}

const remainingFromUsed = (used: number | null | undefined): number | null => {
  const normalized = clampPercent(used)
  return normalized === null ? null : 100 - normalized
}

const remainingFromAmount = (
  remaining: number | null | undefined,
  limit: number | null | undefined
): number | null => {
  if (remaining == null || limit == null || limit <= 0) return null
  return clampPercent((remaining / limit) * 100)
}

const formatCents = (value: number | null | undefined): string => {
  if (value == null || !Number.isFinite(value)) return '--'
  return new Intl.NumberFormat(undefined, { style: 'currency', currency: 'USD' }).format(value / 100)
}

const formatAmountPair = (
  remaining: number | null | undefined,
  limit: number | null | undefined
): string | null => {
  if (remaining == null && limit == null) return null
  if (remaining == null) return formatCents(limit)
  if (limit == null) return formatCents(remaining)
  return `${formatCents(remaining)} / ${formatCents(limit)}`
}

const monthlyRemainingPercent = computed(() => {
  const quota = currentQuota.value
  if (!quota) return null
  return remainingFromAmount(quota.monthly_remaining_cents, quota.monthly_limit_cents)
    ?? remainingFromUsed(quota.monthly_used_percent)
})

const weeklyRemainingPercent = computed(() => {
  return remainingFromUsed(currentQuota.value?.weekly_used_percent)
})

const compactRows = computed<CompactRow[]>(() => {
  const rows: CompactRow[] = []
  if (monthlyRemainingPercent.value !== null) {
    rows.push({
      key: 'monthly',
      shortLabel: t('admin.accounts.usageWindow.grokBillingMonthlyShort'),
      remainingPercent: monthlyRemainingPercent.value,
      resetAt: currentQuota.value?.billing_period_end,
      color: 'indigo'
    })
  }
  if (weeklyRemainingPercent.value !== null) {
    rows.push({
      key: 'weekly',
      shortLabel: t('admin.accounts.usageWindow.grokBillingWeeklyShort'),
      remainingPercent: weeklyRemainingPercent.value,
      resetAt: currentQuota.value?.weekly_reset_at,
      color: 'emerald'
    })
  }
  return rows
})

const monthlyMeta = computed(() => {
  const quota = currentQuota.value
  return quota ? formatAmountPair(quota.monthly_remaining_cents, quota.monthly_limit_cents) : null
})

const monthlyResetLabel = computed(() => {
  const resetAt = currentQuota.value?.billing_period_end
  return resetAt
    ? t('admin.accounts.usageWindow.grokBillingReset', { time: formatRelativeTime(resetAt) })
    : null
})

const weeklyMeta = computed(() => {
  const remaining = weeklyRemainingPercent.value
  if (remaining === null) return null
  return t('admin.accounts.usageWindow.grokBillingRemainingPercent', {
    percent: `${Math.round(remaining)}%`
  })
})

const productRows = computed(() => {
  return (currentQuota.value?.product_usage ?? []).map((product) => ({
    key: product.product,
    label: t('admin.accounts.usageWindow.grokBillingProductUsage', { product: product.product }),
    meta: t('admin.accounts.usageWindow.grokBillingUsedPercent', {
      percent: product.usage_percent == null ? '--' : `${Math.round(clampPercent(product.usage_percent) ?? 0)}%`
    })
  }))
})

const payAsYouGoRemainingPercent = computed(() => {
  const quota = currentQuota.value
  if (!quota || quota.on_demand_cap_cents == null || quota.on_demand_cap_cents <= 0) return null
  return remainingFromAmount(quota.on_demand_remaining_cents, quota.on_demand_cap_cents)
    ?? remainingFromUsed(quota.on_demand_used_percent)
})

const payAsYouGoExplicitlyDisabled = computed(() => currentQuota.value?.on_demand_cap_cents === 0)

const payAsYouGoMeta = computed(() => {
  const quota = currentQuota.value
  if (!quota || quota.on_demand_cap_cents == null || quota.on_demand_cap_cents <= 0) return null
  return formatAmountPair(quota.on_demand_remaining_cents, quota.on_demand_cap_cents)
})

const planLabel = computed(() => {
  const label = currentQuota.value?.plan_label?.trim()
  if (!label) return null
  if (label === 'supergrok') return t('admin.accounts.usageWindow.grokBillingPlanSuperGrok')
  if (label === 'supergrok_heavy') return t('admin.accounts.usageWindow.grokBillingPlanSuperGrokHeavy')
  return label
})

const statusLabel = computed(() => {
  const quota = currentQuota.value
  const labels: string[] = []
  if (quota?.updated_at) {
    const time = formatRelativeTime(quota.updated_at)
    labels.push(quota.stale
      ? t('admin.accounts.usageWindow.grokBillingStale', { time })
      : t('admin.accounts.usageWindow.grokBillingUpdated', { time }))
  }
  if (quota?.partial) labels.push(t('admin.accounts.usageWindow.grokBillingPartial'))
  return labels.join(' · ')
})

const failedWindowsTitle = computed(() => currentQuota.value?.failed_windows?.join(', ') || undefined)

const expandTitle = computed(() => expanded.value
  ? t('admin.accounts.usageWindow.grokBillingCollapse')
  : t('admin.accounts.usageWindow.grokBillingExpand'))

watch(() => props.quota, (quota) => {
  currentQuota.value = quota ?? null
})

watch(() => props.account.id, () => {
  currentQuota.value = props.quota ?? null
  loading.value = false
  error.value = null
  expanded.value = false
})

onMounted(() => {
  if (!isQuotaFresh(currentQuota.value)) void refreshBilling(false)
})

onBeforeUnmount(() => {
  unmounted.value = true
})
</script>
