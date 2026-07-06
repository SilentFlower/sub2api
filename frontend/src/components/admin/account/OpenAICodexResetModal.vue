<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.openaiCodexReset.title')"
    width="wide"
    @close="handleClose"
  >
    <div class="space-y-4">
      <div class="flex flex-col gap-3 rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-gray-700 dark:bg-gray-800/60 sm:flex-row sm:items-center sm:justify-between">
        <div class="min-w-0">
          <div class="truncate text-sm font-semibold text-gray-900 dark:text-white">
            {{ account?.name || status?.account.name || '-' }}
          </div>
          <div class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">
            {{ displayEmail || t('admin.accounts.openaiCodexReset.noEmail') }}
          </div>
        </div>
        <button
          type="button"
          class="btn btn-secondary shrink-0"
          :disabled="loadingStatus || !account"
          @click="loadStatus"
        >
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': loadingStatus }" />
          <span>{{ t('common.refresh') }}</span>
        </button>
      </div>

      <div v-if="errorMessage" class="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-300">
        {{ errorMessage }}
      </div>

      <div class="grid gap-4 lg:grid-cols-2">
        <section class="space-y-3">
          <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-dark-800">
            <div class="flex items-start justify-between gap-3">
              <div>
                <div class="text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                  {{ t('admin.accounts.openaiCodexReset.availableCredits') }}
                </div>
                <div class="mt-2 text-3xl font-semibold text-gray-900 dark:text-white">
                  {{ status?.available_count ?? 0 }}
                </div>
              </div>
              <span class="rounded-full bg-blue-50 px-2.5 py-1 text-xs font-medium text-blue-700 dark:bg-blue-900/30 dark:text-blue-300">
                {{ t('admin.accounts.openaiCodexReset.totalCredits', { count: status?.credit_count ?? 0 }) }}
              </span>
            </div>

            <button
              type="button"
              class="btn btn-primary mt-4 w-full"
              :disabled="loadingStatus || consuming || !hasAvailableCredit"
              @click="consumeCredit"
            >
              <Icon name="bolt" size="sm" />
              <span>{{ consuming ? t('admin.accounts.openaiCodexReset.consuming') : t('admin.accounts.openaiCodexReset.consume') }}</span>
            </button>

            <div v-if="consumeResult" class="mt-3 rounded-md bg-emerald-50 px-3 py-2 text-sm text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">
              {{ t('admin.accounts.openaiCodexReset.consumeSuccess', { credit: consumeResult.credit_id }) }}
            </div>
            <div v-else-if="!hasAvailableCredit && !loadingStatus" class="mt-3 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openaiCodexReset.noAvailableCredit') }}
            </div>
          </div>

          <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-dark-800">
            <div class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.accounts.openaiCodexReset.creditList') }}
            </div>
            <div v-if="loadingStatus" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
              {{ t('common.loading') }}
            </div>
            <div v-else-if="creditStatuses.length === 0" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openaiCodexReset.noCredits') }}
            </div>
            <div v-else class="max-h-64 space-y-2 overflow-y-auto">
              <div
                v-for="credit in creditStatuses"
                :key="credit.id"
                class="rounded-md border border-gray-100 p-3 dark:border-gray-700"
              >
                <div class="flex items-center justify-between gap-3">
                  <span class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ credit.title || credit.id }}</span>
                  <span :class="creditStatusClass(credit.status)">
                    {{ credit.status || t('admin.accounts.openaiCodexReset.availableStatus') }}
                  </span>
                </div>
                <p v-if="credit.description" class="mt-1 line-clamp-2 text-xs text-gray-500 dark:text-gray-400">
                  {{ credit.description }}
                </p>
                <p v-if="creditExpiresAtTexts[credit.id]" class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.accounts.openaiCodexReset.expiresAt', { time: creditExpiresAtTexts[credit.id] }) }}
                </p>
              </div>
            </div>
          </div>
        </section>

        <section class="space-y-3">
          <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-dark-800">
            <label class="block text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.accounts.openaiCodexReset.inviteEmails') }}
            </label>
            <textarea
              v-model="inviteText"
              class="input mt-2 min-h-[128px] w-full resize-y"
              :placeholder="t('admin.accounts.openaiCodexReset.invitePlaceholder')"
            ></textarea>
            <div class="mt-2 flex items-center justify-between gap-3 text-xs">
              <span :class="emailParseError ? 'text-red-600 dark:text-red-400' : 'text-gray-500 dark:text-gray-400'">
                {{ emailParseError || t('admin.accounts.openaiCodexReset.inviteHint', { count: parsedEmails.length }) }}
              </span>
              <span class="text-gray-400">{{ t('admin.accounts.openaiCodexReset.maxEmails') }}</span>
            </div>

            <label class="mt-3 flex items-start gap-2 text-sm text-gray-700 dark:text-gray-300">
              <input
                v-model="consentConfirmed"
                type="checkbox"
                class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              />
              <span>{{ t('admin.accounts.openaiCodexReset.consent') }}</span>
            </label>

            <button
              type="button"
              class="btn btn-primary mt-4 w-full"
              :disabled="sendingInvite || !canSendInvite"
              @click="sendInvites"
            >
              <Icon name="mail" size="sm" />
              <span>{{ sendingInvite ? t('admin.accounts.openaiCodexReset.sending') : t('admin.accounts.openaiCodexReset.sendInvite') }}</span>
            </button>

            <div v-if="inviteResult" class="mt-3 rounded-md bg-emerald-50 px-3 py-2 text-sm text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">
              {{ t('admin.accounts.openaiCodexReset.inviteSuccess', { count: inviteResult.invited_count ?? inviteResult.emails.length }) }}
              <div v-if="inviteResult.failed_emails?.length" class="mt-1 text-xs">
                {{ t('admin.accounts.openaiCodexReset.failedEmails') }}: {{ inviteResult.failed_emails.join(', ') }}
              </div>
            </div>
          </div>

          <details class="rounded-lg border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-dark-800">
            <summary class="cursor-pointer text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.accounts.openaiCodexReset.rules') }}
            </summary>
            <pre class="mt-3 max-h-52 overflow-auto rounded-md bg-gray-50 p-3 text-xs text-gray-700 dark:bg-dark-900 dark:text-gray-300">{{ rulesPreview }}</pre>
          </details>
        </section>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import { formatDateTime } from '@/utils/format'
import type { Account } from '@/types'
import type {
  OpenAICodexInviteResult,
  OpenAICodexResetConsumeResult,
  OpenAICodexResetCreditStatus,
  OpenAICodexResetStatus
} from '@/api/admin/accounts'

const props = defineProps<{
  show: boolean
  account: Account | null
}>()

const emit = defineEmits<{
  close: []
}>()

const { t } = useI18n()

const loadingStatus = ref(false)
const consuming = ref(false)
const sendingInvite = ref(false)
const status = ref<OpenAICodexResetStatus | null>(null)
const consumeResult = ref<OpenAICodexResetConsumeResult | null>(null)
const inviteResult = ref<OpenAICodexInviteResult | null>(null)
const errorMessage = ref('')
const inviteText = ref('')
const consentConfirmed = ref(false)

const displayEmail = computed(() => {
  const extra = props.account?.extra as Record<string, unknown> | undefined
  const credentials = props.account?.credentials as Record<string, unknown> | undefined
  const candidates = [
    status.value?.account.email,
    extra?.email_address,
    extra?.email,
    credentials?.email
  ]
  for (const candidate of candidates) {
    if (typeof candidate === 'string' && candidate.trim()) return candidate.trim()
  }
  return ''
})

const creditStatuses = computed<OpenAICodexResetCreditStatus[]>(() => status.value?.credit_statuses ?? [])
const hasAvailableCredit = computed(() => (status.value?.available_count ?? 0) > 0)
const creditExpiresAtTexts = computed<Record<string, string>>(() => {
  const texts: Record<string, string> = {}
  for (const credit of creditStatuses.value) {
    const formatted = formatDateTime(credit.expires_at)
    if (formatted) texts[credit.id] = formatted
  }
  return texts
})

const parsedEmails = computed(() => {
  const parts = inviteText.value
    .split(/[\s,;]+/)
    .map(item => item.trim())
    .filter(Boolean)
  const seen = new Set<string>()
  const result: string[] = []
  for (const email of parts) {
    const key = email.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    result.push(email)
  }
  return result
})

const emailParseError = computed(() => {
  if (parsedEmails.value.length > 5) return t('admin.accounts.openaiCodexReset.tooManyEmails')
  const invalid = parsedEmails.value.find(email => !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email))
  if (invalid) return t('admin.accounts.openaiCodexReset.invalidEmail', { email: invalid })
  return ''
})

const canSendInvite = computed(() => {
  return Boolean(props.account) &&
    parsedEmails.value.length > 0 &&
    !emailParseError.value &&
    consentConfirmed.value
})

const rulesPreview = computed(() => {
  const payload = {
    eligibility: status.value?.eligibility ?? null,
    rules: status.value?.rules ?? null
  }
  return JSON.stringify(payload, null, 2)
})

const resetLocalState = () => {
  status.value = null
  consumeResult.value = null
  inviteResult.value = null
  errorMessage.value = ''
  inviteText.value = ''
  consentConfirmed.value = false
}

const errorMessageOf = (error: unknown, fallback: string) => {
  if (error instanceof Error && error.message) return error.message
  if (typeof error === 'object' && error && 'message' in error) {
    const message = (error as { message?: unknown }).message
    if (typeof message === 'string' && message.trim()) return message
  }
  return fallback
}

const loadStatus = async () => {
  if (!props.account) return
  loadingStatus.value = true
  errorMessage.value = ''
  try {
    status.value = await adminAPI.accounts.getOpenAICodexResetStatus(props.account.id)
  } catch (error) {
    errorMessage.value = errorMessageOf(error, t('admin.accounts.openaiCodexReset.loadFailed'))
  } finally {
    loadingStatus.value = false
  }
}

const consumeCredit = async () => {
  if (!props.account || !hasAvailableCredit.value) return
  consuming.value = true
  errorMessage.value = ''
  consumeResult.value = null
  try {
    consumeResult.value = await adminAPI.accounts.consumeOpenAICodexResetCredit(props.account.id)
    await loadStatus()
  } catch (error) {
    errorMessage.value = errorMessageOf(error, t('admin.accounts.openaiCodexReset.consumeFailed'))
  } finally {
    consuming.value = false
  }
}

const sendInvites = async () => {
  if (!props.account || !canSendInvite.value) return
  sendingInvite.value = true
  errorMessage.value = ''
  inviteResult.value = null
  try {
    inviteResult.value = await adminAPI.accounts.sendOpenAICodexInvites(props.account.id, {
      emails: parsedEmails.value,
      consent_confirmed: consentConfirmed.value
    })
  } catch (error) {
    errorMessage.value = errorMessageOf(error, t('admin.accounts.openaiCodexReset.inviteFailed'))
  } finally {
    sendingInvite.value = false
  }
}

const creditStatusClass = (statusText?: string) => {
  const normalized = statusText?.toLowerCase() ?? ''
  if (!normalized || normalized === 'available') {
    return 'rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
  }
  return 'rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600 dark:bg-gray-700 dark:text-gray-300'
}

const handleClose = () => {
  emit('close')
}

watch(
  () => props.show,
  (visible) => {
    if (visible) {
      resetLocalState()
      loadStatus()
    }
  },
  { immediate: true }
)
</script>
