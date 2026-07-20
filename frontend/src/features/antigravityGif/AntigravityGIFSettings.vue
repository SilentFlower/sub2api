<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t('admin.settings.gatewayForwarding.antigravityGifTitle') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.gatewayForwarding.antigravityGifDescription') }}
      </p>
    </div>

    <div class="space-y-5 p-6">
      <div class="flex items-center justify-between gap-4">
        <div>
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.settings.gatewayForwarding.antigravityGifEnabled') }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.gatewayForwarding.antigravityGifEnabledHint') }}
          </p>
        </div>
        <Toggle
          v-model="settings.enabled"
          data-testid="antigravity-gif-enabled"
          class="disabled:cursor-not-allowed disabled:opacity-50"
          :disabled="loading || saving || !loaded"
        />
      </div>

      <div class="max-w-xs">
        <label
          for="antigravity-gif-max-frames"
          class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300"
        >
          {{ t('admin.settings.gatewayForwarding.antigravityGifMaxFrames') }}
        </label>
        <input
          id="antigravity-gif-max-frames"
          v-model.number="settings.max_frames_per_gif"
          data-testid="antigravity-gif-max-frames"
          type="number"
          min="1"
          max="16"
          step="1"
          class="input w-full"
          :disabled="loading || saving || !loaded || !settings.enabled"
        />
        <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.gatewayForwarding.antigravityGifMaxFramesHint') }}
        </p>
      </div>

      <div class="flex justify-end">
        <button
          type="button"
          class="btn btn-primary min-w-24"
          data-testid="antigravity-gif-save"
          :disabled="loading || saving || !loaded"
          @click="save"
        >
          <Icon
            v-if="saving"
            name="refresh"
            size="sm"
            class="mr-1 animate-spin"
          />
          <Icon v-else name="check" size="sm" class="mr-1" />
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import Icon from '@/components/icons/Icon.vue'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  getAntigravityGIFCompatibilitySettings,
  updateAntigravityGIFCompatibilitySettings,
  type AntigravityGIFCompatibilitySettings
} from './api'

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const saving = ref(false)
const loaded = ref(false)
const settings = reactive<AntigravityGIFCompatibilitySettings>({
  enabled: true,
  max_frames_per_gif: 8
})

/** 从管理 API 加载反重力 GIF 多帧兼容设置。 */
async function load(): Promise<void> {
  loading.value = true
  loaded.value = false
  try {
    const loadedSettings = await getAntigravityGIFCompatibilitySettings()
    settings.enabled = loadedSettings.enabled
    settings.max_frames_per_gif = loadedSettings.max_frames_per_gif
    loaded.value = true
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  } finally {
    loading.value = false
  }
}

/** 校验并保存反重力 GIF 多帧兼容设置。 */
async function save(): Promise<void> {
  if (!loaded.value || loading.value || saving.value) {
    return
  }

  const maxFrames = Number(settings.max_frames_per_gif)
  if (!Number.isInteger(maxFrames) || maxFrames < 1 || maxFrames > 16) {
    appStore.showError(t('admin.settings.gatewayForwarding.antigravityGifInvalidFrames'))
    return
  }

  saving.value = true
  try {
    const updated = await updateAntigravityGIFCompatibilitySettings({
      enabled: settings.enabled,
      max_frames_per_gif: maxFrames
    })
    settings.enabled = updated.enabled
    settings.max_frames_per_gif = updated.max_frames_per_gif
    appStore.showSuccess(t('admin.settings.gatewayForwarding.antigravityGifSaved'))
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  void load()
})
</script>
