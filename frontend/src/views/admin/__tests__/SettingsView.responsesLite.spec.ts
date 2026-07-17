import { beforeEach, describe, expect, it } from 'vitest'
import { flushPromises } from '@vue/test-utils'

import {
  mountSettingsViewBuildFeature,
  resetSettingsViewBuildFeatureHarness,
  settingsViewBuildFeatureMocks
} from './settingsViewBuildFeatureHarness'

describe('SettingsView Responses Lite 阻止模型', () => {
  beforeEach(() => {
    resetSettingsViewBuildFeatureHarness()
  })

  it('加载、去重并归一化规则', async () => {
    const wrapper = mountSettingsViewBuildFeature()

    await flushPromises()
    const rows = wrapper.findAll('[data-testid="responses-lite-blocked-model-row"]')
    expect(rows).toHaveLength(3)
    expect((rows[0].element as HTMLInputElement).value).toBe('gpt-5.4')

    await rows[0].setValue(' gpt-5.4* ')
    await rows[1].setValue('gpt-5.4*')
    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(settingsViewBuildFeatureMocks.updateSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        openai_responses_lite_header_blocked_models: ['gpt-5.4*', 'gpt-5.5']
      })
    )
  })

  it('显式空列表按空数组提交', async () => {
    const wrapper = mountSettingsViewBuildFeature()

    await flushPromises()
    while (wrapper.findAll('[data-testid="responses-lite-blocked-model-remove"]').length > 0) {
      await wrapper.findAll('[data-testid="responses-lite-blocked-model-remove"]')[0].trigger('click')
    }
    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(settingsViewBuildFeatureMocks.updateSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        openai_responses_lite_header_blocked_models: []
      })
    )
  })

  it('拒绝空规则和非法通配符', async () => {
    const wrapper = mountSettingsViewBuildFeature()

    await flushPromises()
    await wrapper.find('[data-testid="responses-lite-blocked-model-add"]').trigger('click')
    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(settingsViewBuildFeatureMocks.updateSettings).not.toHaveBeenCalled()
    expect(settingsViewBuildFeatureMocks.showError).toHaveBeenCalledWith('Responses Lite 阻止模型规则不能为空。')

    settingsViewBuildFeatureMocks.showError.mockClear()
    const rows = wrapper.findAll('[data-testid="responses-lite-blocked-model-row"]')
    await rows.at(-1)!.setValue('gpt-5.*-mini')
    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(settingsViewBuildFeatureMocks.updateSettings).not.toHaveBeenCalled()
    expect(settingsViewBuildFeatureMocks.showError).toHaveBeenCalledWith('Responses Lite 阻止模型规则仅支持一个位于末尾的 *。')
  })
})
