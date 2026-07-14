import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { listChannelsMock, createChannelMock, getGroupsMock, getWebSearchConfigMock } = vi.hoisted(() => ({
  listChannelsMock: vi.fn(),
  createChannelMock: vi.fn(),
  getGroupsMock: vi.fn(),
  getWebSearchConfigMock: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    channels: {
      list: listChannelsMock,
      create: createChannelMock,
      update: vi.fn(),
      remove: vi.fn(),
      syncPricingModels: vi.fn().mockResolvedValue({ models: [] }),
    },
    groups: {
      getAll: getGroupsMock,
    },
    settings: {
      getWebSearchEmulationConfig: getWebSearchConfigMock,
    },
    accounts: {
      list: vi.fn().mockResolvedValue({ items: [], total: 0 }),
      getById: vi.fn(),
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

import ChannelsView from '../ChannelsView.vue'

const LayoutStub = defineComponent({
  template: '<div><slot /><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>',
})

const BaseDialogStub = defineComponent({
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})

const ToggleStub = defineComponent({
  props: { modelValue: { type: Boolean, default: false } },
  emits: ['update:modelValue'],
  template: '<button type="button" class="toggle-stub" @click="$emit(\'update:modelValue\', !modelValue)">{{ modelValue }}</button>',
})

function mountView() {
  return mount(ChannelsView, {
    global: {
      stubs: {
        AppLayout: LayoutStub,
        TablePageLayout: LayoutStub,
        DataTable: LayoutStub,
        BaseDialog: BaseDialogStub,
        Toggle: ToggleStub,
        Pagination: true,
        ConfirmDialog: true,
        EmptyState: true,
        Select: true,
        Icon: true,
        PlatformIcon: true,
        PricingEntryCard: true,
      },
    },
  })
}

describe('ChannelsView Web Search 默认配置', () => {
  beforeEach(() => {
    listChannelsMock.mockReset().mockResolvedValue({ items: [], total: 0 })
    createChannelMock.mockReset().mockResolvedValue({})
    getGroupsMock.mockReset().mockResolvedValue([
      {
        id: 11,
        name: 'OpenAI Group',
        platform: 'openai',
        rate_multiplier: 1,
        account_count: 1,
      },
    ])
    getWebSearchConfigMock.mockReset().mockResolvedValue({
      enabled: true,
      providers: [{ type: 'anysearch' }],
    })
  })

  it('通过可见开关把 OpenAI Web Search 默认值写入 features_config', async () => {
    const wrapper = mountView()
    await flushPromises()

    const createButton = wrapper
      .findAll('button')
      .find((button) => button.text().includes('admin.channels.createChannel'))
    expect(createButton).toBeDefined()
    await createButton?.trigger('click')
    await flushPromises()

    await wrapper.get('form#channel-form input[type="text"]').setValue('Web Search Channel')
    const platformLabel = wrapper
      .findAll('label')
      .find((label) => label.text().includes('admin.groups.platforms.openai'))
    expect(platformLabel).toBeDefined()
    await platformLabel?.get('input[type="checkbox"]').setValue(true)

    const openAITab = wrapper
      .findAll('button')
      .find((button) => button.text().includes('admin.groups.platforms.openai'))
    expect(openAITab).toBeDefined()
    await openAITab?.trigger('click')

    const groupLabel = wrapper.findAll('label').find((label) => label.text().includes('OpenAI Group'))
    expect(groupLabel).toBeDefined()
    await groupLabel?.get('input[type="checkbox"]').setValue(true)

    const webSearchLabel = wrapper
      .findAll('label')
      .find((label) => label.text().includes('admin.channels.form.webSearchEmulation'))
    expect(webSearchLabel).toBeDefined()
    const webSearchSection = webSearchLabel?.element.parentElement?.parentElement?.parentElement
    expect(webSearchSection).not.toBeNull()
    const toggle = wrapper.findAll('.toggle-stub').find((button) => webSearchSection?.contains(button.element))
    expect(toggle).toBeDefined()
    await toggle?.trigger('click')

    await wrapper.get('form#channel-form').trigger('submit.prevent')
    await flushPromises()

    expect(createChannelMock).toHaveBeenCalledTimes(1)
    expect(createChannelMock.mock.calls[0]?.[0]?.features_config?.web_search_emulation).toEqual({
      openai: true,
    })
  })
})
