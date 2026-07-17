<template>
  <div class="flex items-center justify-between gap-4">
    <div>
      <label class="input-label mb-0">{{ t('admin.accounts.openai.jsonSchemaDowngrade') }}</label>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.openai.jsonSchemaDowngradeDesc') }}
      </p>
    </div>
    <button
      type="button"
      role="switch"
      data-testid="openai-json-schema-downgrade-toggle"
      :aria-checked="value"
      :class="[
        'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
        value ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
      ]"
      @click="value = !value"
    >
      <span
        :class="[
          'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
          value ? 'translate-x-5' : 'translate-x-0'
        ]"
      />
    </button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const value = defineModel<boolean>({ required: true })
const { t } = useI18n()
</script>
