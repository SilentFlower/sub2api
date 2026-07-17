<template>
  <div class="border-t border-gray-200 pt-3 dark:border-dark-600">
    <div class="flex items-center justify-between">
      <div>
        <label class="text-xs font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.channels.form.webSearchEmulation') }}
        </label>
        <p class="mt-0.5 text-[11px] text-red-500 dark:text-red-400">
          {{ t('admin.channels.form.webSearchEmulationHint') }}
        </p>
      </div>
      <Toggle v-model="value" data-testid="channel-web-search-emulation-toggle" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'

const value = defineModel<boolean>({ required: true })
const { t } = useI18n()
</script>
