import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ResponsesLiteDowngradeToggle from '../ResponsesLiteDowngradeToggle.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

describe('ResponsesLiteDowngradeToggle', () => {
  it('渲染文案并通过 v-model 切换开关', async () => {
    const wrapper = mount(ResponsesLiteDowngradeToggle, {
      props: {
        modelValue: false,
        'onUpdate:modelValue': (value: boolean) => wrapper.setProps({ modelValue: value })
      }
    })

    expect(wrapper.text()).toContain('admin.accounts.openai.responsesLiteDowngrade')
    expect(wrapper.text()).toContain('admin.accounts.openai.responsesLiteDowngradeDesc')
    const toggle = wrapper.get('[data-testid="responses-lite-downgrade-toggle"]')
    expect(toggle.attributes('aria-checked')).toBe('false')

    await toggle.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([true])
    expect(toggle.attributes('aria-checked')).toBe('true')
  })
})
