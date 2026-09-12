import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AlphaSearchViaResponsesToggle from '../AlphaSearchViaResponsesToggle.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

describe('AlphaSearchViaResponsesToggle', () => {
  it('渲染文案并通过 v-model 切换开关', async () => {
    const wrapper = mount(AlphaSearchViaResponsesToggle, {
      props: {
        modelValue: false,
        'onUpdate:modelValue': (value: boolean) => wrapper.setProps({ modelValue: value })
      }
    })

    expect(wrapper.text()).toContain('admin.accounts.openai.alphaSearchViaResponses')
    expect(wrapper.text()).toContain('admin.accounts.openai.alphaSearchViaResponsesDesc')
    const toggle = wrapper.get('[data-testid="alpha-search-via-responses-toggle"]')
    expect(toggle.attributes('aria-checked')).toBe('false')

    await toggle.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([true])
    expect(toggle.attributes('aria-checked')).toBe('true')
  })
})
