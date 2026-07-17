import { beforeEach, describe, expect, it } from 'vitest'
import { flushPromises } from '@vue/test-utils'

import {
  buildFeatureSettingsResponse,
  mountSettingsViewBuildFeature,
  resetSettingsViewBuildFeatureHarness,
  settingsViewBuildFeatureMocks
} from './settingsViewBuildFeatureHarness'

describe('SettingsView OpenAI 生图设置', () => {
  beforeEach(() => {
    resetSettingsViewBuildFeatureHarness()
  })

  it('提交生图主模型和思考预算', async () => {
    settingsViewBuildFeatureMocks.getSettings.mockResolvedValueOnce({
      ...buildFeatureSettingsResponse,
      openai_image_generation_main_model: 'gpt-5.6-sol',
      openai_image_generation_reasoning_effort: 'max'
    })
    const wrapper = mountSettingsViewBuildFeature()

    await flushPromises()
    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(settingsViewBuildFeatureMocks.updateSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        openai_image_generation_main_model: 'gpt-5.6-sol',
        openai_image_generation_reasoning_effort: 'max'
      })
    )
  })
})
