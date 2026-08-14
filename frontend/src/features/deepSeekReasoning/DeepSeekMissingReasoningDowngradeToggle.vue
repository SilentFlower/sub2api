<template>
  <div class="flex items-center justify-between gap-4">
    <div>
      <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.settings.gatewayForwarding.deepSeekMissingReasoningAutoDowngrade') }}
      </label>
      <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.gatewayForwarding.deepSeekMissingReasoningAutoDowngradeHint') }}
      </p>
    </div>
    <Toggle
      v-model="enabled"
      data-testid="deepseek-missing-reasoning-auto-downgrade"
    />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

import Toggle from '@/components/common/Toggle.vue'

const enabled = defineModel<boolean>({ required: true })
const { t } = useI18n()
</script>
