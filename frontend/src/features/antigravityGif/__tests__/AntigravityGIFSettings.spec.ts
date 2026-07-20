import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AntigravityGIFSettings from '../AntigravityGIFSettings.vue'

const mocks = vi.hoisted(() => ({
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('../api', () => ({
  getAntigravityGIFCompatibilitySettings: mocks.getSettings,
  updateAntigravityGIFCompatibilitySettings: mocks.updateSettings
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: mocks.showError,
    showSuccess: mocks.showSuccess
  })
}))

vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: () => 'api-error'
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
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
  setup(props, { emit, attrs }) {
    return () => h('input', {
      ...attrs,
      type: 'checkbox',
      checked: props.modelValue,
      onChange: (event: Event) => {
        emit('update:modelValue', (event.target as HTMLInputElement).checked)
      }
    })
  }
})

/** 挂载反重力 GIF 设置组件。 */
function mountSettingsComponent() {
  return mount(AntigravityGIFSettings, {
    global: {
      stubs: {
        Toggle: ToggleStub,
        Icon: true
      }
    }
  })
}

/** 挂载组件并等待初始设置加载完成。 */
async function mountSettings() {
  const wrapper = mountSettingsComponent()
  await flushPromises()
  return wrapper
}

describe('AntigravityGIFSettings', () => {
  beforeEach(() => {
    for (const mock of Object.values(mocks)) {
      mock.mockReset()
    }
    mocks.getSettings.mockResolvedValue({
      enabled: true,
      max_frames_per_gif: 8
    })
    mocks.updateSettings.mockImplementation(async (settings) => settings)
  })

  it('加载默认开启和 8 帧配置', async () => {
    const wrapper = await mountSettings()

    const toggle = wrapper.get('[data-testid="antigravity-gif-enabled"]')
    const input = wrapper.get<HTMLInputElement>('[data-testid="antigravity-gif-max-frames"]')
    expect((toggle.element as HTMLInputElement).checked).toBe(true)
    expect(input.element.value).toBe('8')
  })

  it('保存开关和帧数配置', async () => {
    const wrapper = await mountSettings()
    const toggle = wrapper.get('[data-testid="antigravity-gif-enabled"]')
    const input = wrapper.get('[data-testid="antigravity-gif-max-frames"]')

    await toggle.setValue(false)
    await toggle.setValue(true)
    await input.setValue('12')
    await wrapper.get('[data-testid="antigravity-gif-save"]').trigger('click')
    await flushPromises()

    expect(mocks.updateSettings).toHaveBeenCalledWith({
      enabled: true,
      max_frames_per_gif: 12
    })
    expect(mocks.showSuccess).toHaveBeenCalledWith(
      'admin.settings.gatewayForwarding.antigravityGifSaved'
    )
  })

  it('阻止越界帧数提交', async () => {
    const wrapper = await mountSettings()
    await wrapper.get('[data-testid="antigravity-gif-max-frames"]').setValue('17')

    await wrapper.get('[data-testid="antigravity-gif-save"]').trigger('click')

    expect(mocks.updateSettings).not.toHaveBeenCalled()
    expect(mocks.showError).toHaveBeenCalledWith(
      'admin.settings.gatewayForwarding.antigravityGifInvalidFrames'
    )
  })

  it.each([1, 16])('允许保存边界帧数 %i', async (maxFrames) => {
    const wrapper = await mountSettings()
    await wrapper.get('[data-testid="antigravity-gif-max-frames"]').setValue(String(maxFrames))

    await wrapper.get('[data-testid="antigravity-gif-save"]').trigger('click')
    await flushPromises()

    expect(mocks.updateSettings).toHaveBeenCalledWith({
      enabled: true,
      max_frames_per_gif: maxFrames
    })
  })

  it('加载失败时展示错误并阻止默认值覆盖现有配置', async () => {
    mocks.getSettings.mockRejectedValue(new Error('load failed'))

    const wrapper = await mountSettings()

    expect(mocks.showError).toHaveBeenCalledWith('api-error')
    expect(wrapper.get('[data-testid="antigravity-gif-enabled"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="antigravity-gif-max-frames"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="antigravity-gif-save"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-testid="antigravity-gif-save"]').trigger('click')
    expect(mocks.updateSettings).not.toHaveBeenCalled()
  })

  it('保存失败时展示错误且不提示成功', async () => {
    mocks.updateSettings.mockRejectedValue(new Error('save failed'))
    const wrapper = await mountSettings()

    await wrapper.get('[data-testid="antigravity-gif-save"]').trigger('click')
    await flushPromises()

    expect(mocks.showError).toHaveBeenCalledWith('api-error')
    expect(mocks.showSuccess).not.toHaveBeenCalled()
  })

  it('保存期间禁用开关和保存按钮', async () => {
    let resolveUpdate: ((value: { enabled: boolean; max_frames_per_gif: number }) => void) | undefined
    mocks.updateSettings.mockImplementation(() => new Promise((resolve) => {
      resolveUpdate = resolve
    }))
    const wrapper = await mountSettings()

    await wrapper.get('[data-testid="antigravity-gif-save"]').trigger('click')

    expect(wrapper.get('[data-testid="antigravity-gif-enabled"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="antigravity-gif-save"]').attributes('disabled')).toBeDefined()

    resolveUpdate?.({ enabled: true, max_frames_per_gif: 8 })
    await flushPromises()
  })
})
