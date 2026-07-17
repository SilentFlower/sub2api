<template>
  <div>
    <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
      {{ t('admin.settings.gatewayForwarding.openaiResponsesLiteBlockedModels') }}
    </label>
    <p class="mb-2 text-xs text-gray-500 dark:text-gray-400">
      {{ t('admin.settings.gatewayForwarding.openaiResponsesLiteBlockedModelsHint') }}
    </p>
    <div
      v-for="(_, index) in blockedModels"
      :key="`responses-lite-blocked-model-${index}`"
      class="mb-2 flex items-center gap-2"
    >
      <input
        v-model="blockedModels[index]"
        type="text"
        class="input flex-1 font-mono text-sm"
        data-testid="responses-lite-blocked-model-row"
        :placeholder="t('admin.settings.gatewayForwarding.openaiResponsesLiteBlockedModelPlaceholder')"
      />
      <button
        type="button"
        class="btn btn-secondary btn-sm shrink-0 text-red-600 hover:text-red-700 dark:text-red-400"
        data-testid="responses-lite-blocked-model-remove"
        :aria-label="t('admin.settings.gatewayForwarding.openaiResponsesLiteBlockedModelRemove')"
        :title="t('admin.settings.gatewayForwarding.openaiResponsesLiteBlockedModelRemove')"
        @click="blockedModels.splice(index, 1)"
      >
        <Icon name="trash" size="xs" />
      </button>
    </div>
    <button
      type="button"
      class="btn btn-secondary btn-sm"
      data-testid="responses-lite-blocked-model-add"
      @click="blockedModels.push('')"
    >
      <Icon name="plus" size="xs" />
      {{ t('admin.settings.gatewayForwarding.openaiResponsesLiteBlockedModelAdd') }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const blockedModels = defineModel<string[]>({ required: true })
const { t } = useI18n()
</script>
