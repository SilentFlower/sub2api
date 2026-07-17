<template>
  <div>
    <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
      {{ t('admin.settings.gatewayForwarding.openaiImageMainModel') }}
    </label>
    <input
      v-model="mainModel"
      type="text"
      class="input max-w-xs font-mono text-sm"
      :placeholder="t('admin.settings.gatewayForwarding.openaiImageMainModelPlaceholder')"
    />
    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
      {{ t('admin.settings.gatewayForwarding.openaiImageMainModelHint') }}
    </p>
  </div>

  <div>
    <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
      {{ t('admin.settings.gatewayForwarding.openaiImageReasoningEffort') }}
    </label>
    <Select v-model="reasoningEffort" :options="reasoningEffortOptions" class="max-w-xs" />
    <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
      {{ t('admin.settings.gatewayForwarding.openaiImageReasoningEffortHint') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'

const mainModel = defineModel<string>('mainModel', { required: true })
const reasoningEffort = defineModel<string>('reasoningEffort', { required: true })
const { t } = useI18n()

const reasoningEffortOptions = computed(() => [
  { value: 'low', label: t('admin.settings.gatewayForwarding.openaiImageReasoningEffortLow') },
  { value: 'medium', label: t('admin.settings.gatewayForwarding.openaiImageReasoningEffortMedium') },
  { value: 'high', label: t('admin.settings.gatewayForwarding.openaiImageReasoningEffortHigh') },
  { value: 'xhigh', label: t('admin.settings.gatewayForwarding.openaiImageReasoningEffortXHigh') },
  { value: 'max', label: t('admin.settings.gatewayForwarding.openaiImageReasoningEffortMax') }
])
</script>
