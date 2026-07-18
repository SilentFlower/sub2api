import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import WebSearchEmulationSettings from '../WebSearchEmulationSettings.vue'
import type { WebSearchEmulationConfig } from '@/api/admin/settings'

const webSearchSettingsMocks = vi.hoisted(() => ({
  getWebSearchEmulationConfig: vi.fn(),
  updateWebSearchEmulationConfig: vi.fn(),
  resetWebSearchUsage: vi.fn(),
  testWebSearchEmulation: vi.fn(),
  listProxies: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  copyToClipboard: vi.fn()
}))

vi.mock('@/api', () => ({
  adminAPI: {
    settings: {
      getWebSearchEmulationConfig: webSearchSettingsMocks.getWebSearchEmulationConfig,
      updateWebSearchEmulationConfig: webSearchSettingsMocks.updateWebSearchEmulationConfig,
      resetWebSearchUsage: webSearchSettingsMocks.resetWebSearchUsage,
      testWebSearchEmulation: webSearchSettingsMocks.testWebSearchEmulation
    },
    proxies: {
      list: webSearchSettingsMocks.listProxies
    }
  }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: webSearchSettingsMocks.showError,
    showSuccess: webSearchSettingsMocks.showSuccess
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: webSearchSettingsMocks.copyToClipboard
  })
}))

vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: () => 'error'
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) =>
        key.replace(/\{(\w+)\}/g, (_, token) => params?.[token] ?? `{${token}}`)
    })
  }
})

const ToggleStub = defineComponent({
  props: {
    modelValue: {
      type: Boolean,
      default: false
    }
  },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () => h('input', {
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
      type: String,
      default: ''
    },
    options: {
      type: Array,
      default: () => []
    }
  },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () => h(
      'select',
      {
        class: 'select-stub',
        value: props.modelValue,
        onChange: (event: Event) => {
          emit('update:modelValue', (event.target as HTMLSelectElement).value)
        }
      },
      (props.options as Array<Record<string, unknown>>).map((option) => h(
        'option',
        {
          key: String(option.value ?? ''),
          value: option.value as string
        },
        String(option.label ?? '')
      ))
    )
  }
})

/** 组件公开给 SettingsView 的保存接口。 */
interface WebSearchEmulationSettingsPublic {
  /**
   * 保存当前组件内的 Web Search 配置。
   *
   * @return 保存成功时返回 true，校验或提交失败时返回 false。
   */
  save: () => Promise<boolean>
}

/** 可人工 resolve/reject 的异步结果。 */
interface Deferred<T> {
  /** 外部等待的 Promise。 */
  promise: Promise<T>
  /** 让 Promise 成功完成。 */
  resolve: (value: T | PromiseLike<T>) => void
  /** 让 Promise 失败完成。 */
  reject: (reason?: unknown) => void
}

/**
 * 创建用于测试加载竞态的延迟 Promise。
 *
 * @return 包含 Promise 与 resolve/reject 控制函数的对象。
 */
function createDeferred<T>(): Deferred<T> {
  let resolve!: (value: T | PromiseLike<T>) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve
    reject = promiseReject
  })
  return { promise, resolve, reject }
}

/**
 * 挂载 Web Search Emulation 设置组件并等待初始加载完成。
 *
 * @return 挂载后的组件 wrapper。
 */
async function mountWebSearchEmulationSettings() {
  const wrapper = mount(WebSearchEmulationSettings, {
    global: {
      stubs: {
        Toggle: ToggleStub,
        Select: SelectStub,
        ProxySelector: true
      }
    }
  })
  await flushPromises()
  return wrapper
}

describe('WebSearchEmulationSettings', () => {
  beforeEach(() => {
    for (const mock of Object.values(webSearchSettingsMocks)) {
      mock.mockReset()
    }
    webSearchSettingsMocks.listProxies.mockResolvedValue({ items: [] })
    webSearchSettingsMocks.updateWebSearchEmulationConfig.mockImplementation(async (payload) => payload)
  })

  it('允许 AnySearch API Key 留空并保存', async () => {
    webSearchSettingsMocks.getWebSearchEmulationConfig.mockResolvedValue({
      enabled: true,
      providers: [
        {
          type: 'anysearch',
          api_key: '',
          api_key_configured: false,
          quota_limit: 1000,
          subscribed_at: null,
          proxy_id: null,
          expires_at: null
        }
      ]
    })

    const wrapper = await mountWebSearchEmulationSettings()
    const ok = await (wrapper.vm as unknown as WebSearchEmulationSettingsPublic).save()

    expect(ok).toBe(true)
    expect(webSearchSettingsMocks.updateWebSearchEmulationConfig).toHaveBeenCalledWith({
      enabled: true,
      providers: [
        expect.objectContaining({
          type: 'anysearch',
          api_key: '',
          quota_limit: 1000
        })
      ]
    })
  })

  it('阻止 Brave 缺少 API Key 的启用配置保存', async () => {
    webSearchSettingsMocks.getWebSearchEmulationConfig.mockResolvedValue({
      enabled: true,
      providers: [
        {
          type: 'brave',
          api_key: '',
          api_key_configured: false,
          quota_limit: 1000,
          subscribed_at: null,
          proxy_id: null,
          expires_at: null
        }
      ]
    })

    const wrapper = await mountWebSearchEmulationSettings()
    const ok = await (wrapper.vm as unknown as WebSearchEmulationSettingsPublic).save()

    expect(ok).toBe(false)
    expect(webSearchSettingsMocks.showError).toHaveBeenCalledWith(
      'admin.settings.webSearchEmulation.apiKeyRequired'
    )
    expect(webSearchSettingsMocks.updateWebSearchEmulationConfig).not.toHaveBeenCalled()
  })

  it('保存前将非正 quota_limit 清洗为 null', async () => {
    webSearchSettingsMocks.getWebSearchEmulationConfig.mockResolvedValue({
      enabled: true,
      providers: [
        {
          type: 'brave',
          api_key: '',
          api_key_configured: true,
          quota_limit: 0,
          subscribed_at: null,
          proxy_id: null,
          expires_at: null
        }
      ]
    })

    const wrapper = await mountWebSearchEmulationSettings()
    const ok = await (wrapper.vm as unknown as WebSearchEmulationSettingsPublic).save()

    expect(ok).toBe(true)
    expect(webSearchSettingsMocks.updateWebSearchEmulationConfig).toHaveBeenCalledWith({
      enabled: true,
      providers: [
        expect.objectContaining({
          type: 'brave',
          quota_limit: null
        })
      ]
    })
  })

  it('保存会等待初始加载完成，避免默认空配置覆盖远端配置', async () => {
    const configDeferred = createDeferred<WebSearchEmulationConfig>()
    webSearchSettingsMocks.getWebSearchEmulationConfig.mockReturnValue(configDeferred.promise)

    const wrapper = mount(WebSearchEmulationSettings, {
      global: {
        stubs: {
          Toggle: ToggleStub,
          Select: SelectStub,
          ProxySelector: true
        }
      }
    })
    await flushPromises()

    const savePromise = (wrapper.vm as unknown as WebSearchEmulationSettingsPublic).save()
    await flushPromises()
    expect(webSearchSettingsMocks.updateWebSearchEmulationConfig).not.toHaveBeenCalled()

    configDeferred.resolve({
      enabled: true,
      providers: [
        {
          type: 'anysearch',
          api_key: '',
          api_key_configured: false,
          quota_limit: 1000,
          subscribed_at: null,
          proxy_id: null,
          expires_at: null
        }
      ]
    })

    const ok = await savePromise

    expect(ok).toBe(true)
    expect(webSearchSettingsMocks.updateWebSearchEmulationConfig).toHaveBeenCalledWith({
      enabled: true,
      providers: [
        expect.objectContaining({
          type: 'anysearch',
          quota_limit: 1000
        })
      ]
    })
  })
})
