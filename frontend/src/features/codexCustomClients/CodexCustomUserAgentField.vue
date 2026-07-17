<template>
  <div class="mt-4 border-l-2 border-gray-200 pl-4 dark:border-dark-600">
    <label class="input-label mb-1 block" :for="fieldId">
      {{ t('admin.accounts.openai.codexCLIOnlyCustomUA') }}
    </label>
    <textarea
      :id="fieldId"
      v-model="value"
      rows="3"
      class="input font-mono text-xs"
      :placeholder="t('admin.accounts.openai.codexCLIOnlyCustomUAPlaceholder')"
      :data-testid="testId"
    ></textarea>
    <p class="input-hint">{{ t('admin.accounts.openai.codexCLIOnlyCustomUADesc') }}</p>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

/** 自定义 Codex User-Agent 字段属性。 */
interface Props {
  fieldId: string
  testId: string
}

defineProps<Props>()
const value = defineModel<string>({ required: true })
const { t } = useI18n()
</script>
