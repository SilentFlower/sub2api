<template>
  <div class="flex items-center justify-between gap-4">
    <div>
      <label class="input-label mb-0">{{ t('admin.accounts.openai.responsesMode') }}</label>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.openai.responsesModeDesc') }}
      </p>
    </div>
    <div class="w-56">
      <Select
        v-model="value"
        :options="options"
        :disabled="disabled"
        data-testid="openai-responses-mode-select"
      />
    </div>
  </div>
  <div
    v-if="statusKey"
    class="rounded-lg bg-gray-50 px-3 py-2 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300"
  >
    <span class="font-medium">{{ t(statusKey) }}</span>
  </div>
  <p
    v-else-if="notApplicable"
    class="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-300"
    data-testid="openai-responses-mode-not-applicable"
  >
    {{ t('admin.accounts.openai.responsesModeTextDisabledHint') }}
  </p>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import type { OpenAIResponsesMode } from '@/types'
import type { OpenAIResponsesModeOption } from './types'

/** Responses 路由模式字段属性。 */
interface Props {
  options: OpenAIResponsesModeOption[]
  disabled?: boolean
  statusKey?: string
  notApplicable?: boolean
}

withDefaults(defineProps<Props>(), {
  disabled: false,
  statusKey: '',
  notApplicable: false
})
const value = defineModel<OpenAIResponsesMode>({ required: true })
const { t } = useI18n()
</script>
