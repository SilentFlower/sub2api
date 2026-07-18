import { beforeEach, describe, expect, it } from 'vitest'
import { flushPromises } from '@vue/test-utils'

import {
  mountSettingsViewBuildFeature,
  openSettingsGatewayTab,
  resetSettingsViewBuildFeatureHarness,
  settingsViewBuildFeatureMocks
} from './settingsViewBuildFeatureHarness'

describe('SettingsView AnySearch provider', () => {
  beforeEach(() => {
    resetSettingsViewBuildFeatureHarness()
  })

  it('允许 API Key 留空并提交 provider', async () => {
    settingsViewBuildFeatureMocks.getWebSearchEmulationConfig.mockResolvedValue({
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

    const wrapper = mountSettingsViewBuildFeature()
    await flushPromises()
    await openSettingsGatewayTab(wrapper)

    const providerSelect = wrapper
      .findAll('.select-stub')
      .find((select) => select.findAll('option').some((option) => option.text() === 'AnySearch'))
    expect(providerSelect).toBeDefined()
    expect((providerSelect?.element as HTMLSelectElement).value).toBe('anysearch')

    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(settingsViewBuildFeatureMocks.updateWebSearchEmulationConfig).toHaveBeenCalledWith({
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

  it('Web Search 配置校验失败时不显示整页保存成功', async () => {
    settingsViewBuildFeatureMocks.getWebSearchEmulationConfig.mockResolvedValue({
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

    const wrapper = mountSettingsViewBuildFeature()
    await flushPromises()
    await openSettingsGatewayTab(wrapper)

    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(settingsViewBuildFeatureMocks.updateSettings).toHaveBeenCalled()
    expect(settingsViewBuildFeatureMocks.updateWebSearchEmulationConfig).not.toHaveBeenCalled()
    expect(settingsViewBuildFeatureMocks.showSuccess).not.toHaveBeenCalledWith('admin.settings.settingsSaved')
    expect(settingsViewBuildFeatureMocks.showError).toHaveBeenCalledWith(
      'admin.settings.webSearchEmulation.apiKeyRequired'
    )
  })
})
