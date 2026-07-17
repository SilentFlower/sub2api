<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="mb-3 flex items-center justify-between">
      <div class="flex-1 pr-4">
        <label
          id="bulk-edit-openai-codex-custom-ua-label"
          class="input-label mb-0"
          for="bulk-edit-openai-codex-custom-ua-enabled"
        >
          {{ t('admin.accounts.openai.codexCLIOnlyCustomUA') }}
        </label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.openai.codexCLIOnlyCustomUABulkDesc') }}
        </p>
      </div>
      <input
        id="bulk-edit-openai-codex-custom-ua-enabled"
        v-model="enabled"
        type="checkbox"
        aria-controls="bulk-edit-openai-codex-custom-ua"
        class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
      />
    </div>
    <textarea
      id="bulk-edit-openai-codex-custom-ua"
      v-model="value"
      rows="3"
      class="input font-mono text-xs"
      :class="!enabled && 'cursor-not-allowed opacity-50'"
      :disabled="!enabled"
      :placeholder="t('admin.accounts.openai.codexCLIOnlyCustomUAPlaceholder')"
      data-testid="bulk-edit-openai-codex-custom-ua"
    ></textarea>
    <p class="input-hint">{{ t('admin.accounts.openai.codexCLIOnlyCustomUABulkHint') }}</p>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const enabled = defineModel<boolean>('enabled', { required: true })
const value = defineModel<string>('value', { required: true })
const { t } = useI18n()
</script>
