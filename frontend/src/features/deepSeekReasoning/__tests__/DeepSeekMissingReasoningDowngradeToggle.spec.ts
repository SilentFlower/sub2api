import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import DeepSeekMissingReasoningDowngradeToggle from '../DeepSeekMissingReasoningDowngradeToggle.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

describe('DeepSeekMissingReasoningDowngradeToggle', () => {
  it('展示领域文案并更新 v-model', async () => {
    const wrapper = mount(DeepSeekMissingReasoningDowngradeToggle, {
      props: {
        modelValue: true,
        'onUpdate:modelValue': (value: boolean) => wrapper.setProps({ modelValue: value })
      }
    })

    expect(wrapper.text()).toContain(
      'admin.settings.gatewayForwarding.deepSeekMissingReasoningAutoDowngrade'
    )
    expect(wrapper.text()).toContain(
      'admin.settings.gatewayForwarding.deepSeekMissingReasoningAutoDowngradeHint'
    )

    const toggle = wrapper.get('[data-testid="deepseek-missing-reasoning-auto-downgrade"]')
    expect(toggle.attributes('aria-checked')).toBe('true')

    await toggle.trigger('click')

    expect(toggle.attributes('aria-checked')).toBe('false')
  })
})
