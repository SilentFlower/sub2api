<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="flex items-center justify-between">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.anthropic.webSearchEmulation') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.anthropic.webSearchEmulationDesc') }}
        </p>
      </div>
      <select
        v-model="value"
        class="input w-24 text-sm"
        data-testid="web-search-emulation-mode-select"
      >
        <option value="default">{{ t('admin.accounts.anthropic.webSearchDefault') }}</option>
        <option value="enabled">{{ t('admin.accounts.anthropic.webSearchEnabled') }}</option>
        <option value="disabled">{{ t('admin.accounts.anthropic.webSearchDisabled') }}</option>
      </select>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { WebSearchEmulationMode } from './types'

const value = defineModel<WebSearchEmulationMode>({ required: true })
const { t } = useI18n()
</script>
