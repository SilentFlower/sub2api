<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="flex items-center justify-between gap-4">
      <div>
        <label for="codex-web-search-bridge-mode" class="input-label mb-0">
          {{ t('admin.accounts.openai.codexWebSearchBridge') }}
        </label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.openai.codexWebSearchBridgeDesc') }}
        </p>
      </div>
      <select
        id="codex-web-search-bridge-mode"
        v-model="value"
        class="input w-32 text-sm"
        data-testid="codex-web-search-bridge-mode-select"
      >
        <option value="inherit">{{ t('admin.accounts.openai.codexWebSearchBridgeInherit') }}</option>
        <option value="enabled">{{ t('admin.accounts.openai.codexWebSearchBridgeEnabled') }}</option>
        <option value="disabled">{{ t('admin.accounts.openai.codexWebSearchBridgeDisabled') }}</option>
      </select>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { CodexWebSearchBridgeMode } from './codexBridge'

const value = defineModel<CodexWebSearchBridgeMode>({ required: true })
const { t } = useI18n()
</script>
