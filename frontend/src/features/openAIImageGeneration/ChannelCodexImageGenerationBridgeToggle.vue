<template>
  <div class="border-t border-gray-200 pt-3 dark:border-dark-600">
    <div class="flex items-center justify-between gap-4">
      <div>
        <label class="text-xs font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.channels.form.codexImageGenerationBridge') }}
        </label>
        <p class="mt-0.5 text-[11px] text-amber-600 dark:text-amber-400">
          {{ t('admin.channels.form.codexImageGenerationBridgeHint') }}
        </p>
      </div>
      <Toggle v-model="value" data-testid="channel-codex-image-generation-bridge-toggle" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'

const value = defineModel<boolean>({ required: true })
const { t } = useI18n()
</script>
