import { beforeEach, describe, expect, it } from 'vitest'
import { flushPromises } from '@vue/test-utils'

import {
  buildFeatureSettingsResponse,
  mountSettingsViewBuildFeature,
  openSettingsGatewayTab,
  resetSettingsViewBuildFeatureHarness,
  settingsViewBuildFeatureMocks
} from './settingsViewBuildFeatureHarness'

describe('SettingsView DeepSeek 缺失推理内容自动降级', () => {
  beforeEach(() => {
    resetSettingsViewBuildFeatureHarness()
  })

  it('加载关闭状态并提交新值', async () => {
    settingsViewBuildFeatureMocks.getSettings.mockResolvedValueOnce({
      ...buildFeatureSettingsResponse,
      enable_deepseek_missing_reasoning_auto_downgrade: false
    })
    const wrapper = mountSettingsViewBuildFeature()

    await flushPromises()
    await openSettingsGatewayTab(wrapper)
    const toggle = wrapper.find('[data-testid="deepseek-missing-reasoning-auto-downgrade"]')
    expect((toggle.element as HTMLInputElement).checked).toBe(false)

    await toggle.setValue(true)
    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(settingsViewBuildFeatureMocks.updateSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        enable_deepseek_missing_reasoning_auto_downgrade: true
      })
    )
  })

  it('后端缺失字段时保持默认开启', async () => {
    const withoutSetting: Record<string, unknown> = { ...buildFeatureSettingsResponse }
    delete withoutSetting.enable_deepseek_missing_reasoning_auto_downgrade
    settingsViewBuildFeatureMocks.getSettings.mockResolvedValueOnce(withoutSetting)
    const wrapper = mountSettingsViewBuildFeature()

    await flushPromises()
    await openSettingsGatewayTab(wrapper)

    expect(
      (wrapper.find('[data-testid="deepseek-missing-reasoning-auto-downgrade"]').element as HTMLInputElement)
        .checked
    ).toBe(true)
  })
})
