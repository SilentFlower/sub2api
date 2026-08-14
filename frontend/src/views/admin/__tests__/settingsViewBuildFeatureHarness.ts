import { vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

import SettingsView from '../SettingsView.vue'

const hoistedSettingsViewBuildFeatureMocks = vi.hoisted(() => ({
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
  getWebSearchEmulationConfig: vi.fn(),
  updateWebSearchEmulationConfig: vi.fn(),
  getAdminApiKey: vi.fn(),
  getOverloadCooldownSettings: vi.fn(),
  getRateLimit429CooldownSettings: vi.fn(),
  updateRateLimit429CooldownSettings: vi.fn(),
  getStreamTimeoutSettings: vi.fn(),
  getRectifierSettings: vi.fn(),
  getBetaPolicySettings: vi.fn(),
  getGroups: vi.fn(),
  listProxies: vi.fn(),
  getProviders: vi.fn(),
  updateProvider: vi.fn(),
  createProvider: vi.fn(),
  deleteProvider: vi.fn(),
  fetchPublicSettings: vi.fn(),
  adminSettingsFetch: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

/** SettingsView build 功能测试共享的 mock 集合。 */
export const settingsViewBuildFeatureMocks = hoistedSettingsViewBuildFeatureMocks

vi.mock('@/api', () => ({
  adminAPI: {
    settings: {
      getSettings: hoistedSettingsViewBuildFeatureMocks.getSettings,
      updateSettings: hoistedSettingsViewBuildFeatureMocks.updateSettings,
      getWebSearchEmulationConfig: hoistedSettingsViewBuildFeatureMocks.getWebSearchEmulationConfig,
      updateWebSearchEmulationConfig: hoistedSettingsViewBuildFeatureMocks.updateWebSearchEmulationConfig,
      getAdminApiKey: hoistedSettingsViewBuildFeatureMocks.getAdminApiKey,
      getOverloadCooldownSettings: hoistedSettingsViewBuildFeatureMocks.getOverloadCooldownSettings,
      getRateLimit429CooldownSettings: hoistedSettingsViewBuildFeatureMocks.getRateLimit429CooldownSettings,
      updateRateLimit429CooldownSettings: hoistedSettingsViewBuildFeatureMocks.updateRateLimit429CooldownSettings,
      getStreamTimeoutSettings: hoistedSettingsViewBuildFeatureMocks.getStreamTimeoutSettings,
      getRectifierSettings: hoistedSettingsViewBuildFeatureMocks.getRectifierSettings,
      getBetaPolicySettings: hoistedSettingsViewBuildFeatureMocks.getBetaPolicySettings
    },
    groups: {
      getAll: hoistedSettingsViewBuildFeatureMocks.getGroups
    },
    proxies: {
      list: hoistedSettingsViewBuildFeatureMocks.listProxies
    },
    payment: {
      getProviders: hoistedSettingsViewBuildFeatureMocks.getProviders,
      updateProvider: hoistedSettingsViewBuildFeatureMocks.updateProvider,
      createProvider: hoistedSettingsViewBuildFeatureMocks.createProvider,
      deleteProvider: hoistedSettingsViewBuildFeatureMocks.deleteProvider
    }
  }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: hoistedSettingsViewBuildFeatureMocks.showError,
    showSuccess: hoistedSettingsViewBuildFeatureMocks.showSuccess,
    showWarning: vi.fn(),
    showInfo: vi.fn(),
    fetchPublicSettings: hoistedSettingsViewBuildFeatureMocks.fetchPublicSettings
  })
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({
    fetch: hoistedSettingsViewBuildFeatureMocks.adminSettingsFetch
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() })
}))

vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: () => 'error'
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const translations: Record<string, string> = {
    'admin.settings.gatewayForwarding.openaiResponsesLiteBlockedModelEmpty': 'Responses Lite 阻止模型规则不能为空。',
    'admin.settings.gatewayForwarding.openaiResponsesLiteBlockedModelWildcardInvalid': 'Responses Lite 阻止模型规则仅支持一个位于末尾的 *。'
  }
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) =>
        (translations[key] ?? key).replace(/\{(\w+)\}/g, (_, token) => params?.[token] ?? `{${token}}`),
      locale: { value: 'zh-CN' }
    })
  }
})

const AppLayoutStub = { template: '<div><slot /></div>' }

const ToggleStub = defineComponent({
  props: {
    modelValue: {
      type: Boolean,
      default: false
    }
  },
  emits: ['update:modelValue'],
  inheritAttrs: false,
  setup(props, { attrs, emit }) {
    return () => h('input', {
      ...attrs,
      class: 'toggle-stub',
      type: 'checkbox',
      checked: props.modelValue,
      onChange: (event: Event) => {
        emit('update:modelValue', (event.target as HTMLInputElement).checked)
      }
    })
  }
})

const SelectStub = defineComponent({
  props: {
    modelValue: {
      type: [String, Number, Boolean, null],
      default: ''
    },
    options: {
      type: Array,
      default: () => []
    }
  },
  emits: ['update:modelValue', 'change'],
  setup(props, { emit }) {
    return () => h(
      'select',
      {
        class: 'select-stub',
        value: props.modelValue ?? '',
        onChange: (event: Event) => {
          const target = event.target as HTMLSelectElement
          emit('update:modelValue', target.value)
          const option = (props.options as Array<Record<string, unknown>>).find(
            (item) => String(item.value ?? '') === target.value
          ) ?? null
          emit('change', target.value, option)
        }
      },
      (props.options as Array<Record<string, unknown>>).map((option) => h(
        'option',
        {
          key: `${String(option.value ?? '')}:${String(option.label ?? '')}`,
          value: option.value as string
        },
        String(option.label ?? '')
      ))
    )
  }
})

/** build 功能测试使用的最小系统设置响应。 */
export const buildFeatureSettingsResponse = {
  registration_email_suffix_whitelist: [],
  default_subscriptions: [],
  default_platform_quotas: {},
  table_page_size_options: [10, 20, 50, 100],
  payment_load_balance_strategy: 'round-robin',
  backend_mode_enabled: false,
  openai_image_generation_main_model: 'gpt-5.4-mini',
  openai_image_generation_reasoning_effort: 'medium',
  openai_responses_lite_header_blocked_models: ['gpt-5.4', 'gpt-5.4-mini', 'gpt-5.5'],
  enable_deepseek_missing_reasoning_auto_downgrade: true
}

/**
 * 重置 SettingsView build 功能测试的 API 与提示 mock。
 *
 * @return 无返回值。
 */
export function resetSettingsViewBuildFeatureHarness(): void {
  for (const mock of Object.values(settingsViewBuildFeatureMocks)) {
    mock.mockReset()
  }

  settingsViewBuildFeatureMocks.getSettings.mockResolvedValue({ ...buildFeatureSettingsResponse })
  settingsViewBuildFeatureMocks.updateSettings.mockImplementation(async (payload) => ({
    ...buildFeatureSettingsResponse,
    ...payload
  }))
  settingsViewBuildFeatureMocks.getWebSearchEmulationConfig.mockResolvedValue({
    enabled: false,
    providers: []
  })
  settingsViewBuildFeatureMocks.updateWebSearchEmulationConfig.mockImplementation(async (payload) => payload)
  settingsViewBuildFeatureMocks.getAdminApiKey.mockResolvedValue({ exists: false, masked_key: '' })
  settingsViewBuildFeatureMocks.getOverloadCooldownSettings.mockResolvedValue({ enabled: true, cooldown_minutes: 10 })
  settingsViewBuildFeatureMocks.getRateLimit429CooldownSettings.mockResolvedValue({ enabled: true, cooldown_seconds: 5 })
  settingsViewBuildFeatureMocks.updateRateLimit429CooldownSettings.mockImplementation(async (payload) => payload)
  settingsViewBuildFeatureMocks.getStreamTimeoutSettings.mockResolvedValue({
    enabled: true,
    action: 'temp_unsched',
    temp_unsched_minutes: 5,
    threshold_count: 3,
    threshold_window_minutes: 10
  })
  settingsViewBuildFeatureMocks.getRectifierSettings.mockResolvedValue({
    enabled: true,
    thinking_signature_enabled: true,
    thinking_budget_enabled: true,
    apikey_signature_enabled: false,
    apikey_signature_patterns: []
  })
  settingsViewBuildFeatureMocks.getBetaPolicySettings.mockResolvedValue({ rules: [] })
  settingsViewBuildFeatureMocks.getGroups.mockResolvedValue([])
  settingsViewBuildFeatureMocks.listProxies.mockResolvedValue({ items: [] })
  settingsViewBuildFeatureMocks.getProviders.mockResolvedValue({ data: [] })
  settingsViewBuildFeatureMocks.fetchPublicSettings.mockResolvedValue(undefined)
  settingsViewBuildFeatureMocks.adminSettingsFetch.mockResolvedValue(undefined)
}

/**
 * 挂载 SettingsView，并替换与本组测试无关的复杂子组件。
 *
 * @return SettingsView 测试 wrapper。
 */
export function mountSettingsViewBuildFeature() {
  return mount(SettingsView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        Select: SelectStub,
        Toggle: ToggleStub,
        Icon: true,
        ConfirmDialog: true,
        PaymentProviderList: true,
        PaymentProviderDialog: true,
        GroupBadge: true,
        GroupOptionItem: true,
        ProxySelector: true,
        ImageUpload: true,
        BackupSettings: true
      }
    }
  })
}

/**
 * 切换到网关设置页签。
 *
 * @param wrapper SettingsView 测试 wrapper。
 * @return 页签切换完成后的 Promise。
 */
export async function openSettingsGatewayTab(
  wrapper: ReturnType<typeof mountSettingsViewBuildFeature>
): Promise<void> {
  const button = wrapper.findAll('button').find((node) => node.text().includes('admin.settings.tabs.gateway'))
  if (!button) throw new Error('未找到网关设置页签')
  await button.trigger('click')
  await flushPromises()
}
