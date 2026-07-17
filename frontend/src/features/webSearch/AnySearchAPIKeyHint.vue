<template>
  <p v-if="isAnySearchProviderType(providerType)" class="mt-1 text-xs text-gray-400">
    {{ t('admin.settings.webSearchEmulation.anySearchApiKeyHint') }}
  </p>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { isAnySearchProviderType } from './anySearch'

/** AnySearch API Key 提示组件属性。 */
interface Props {
  providerType: string
}

defineProps<Props>()
const { t } = useI18n()
</script>
