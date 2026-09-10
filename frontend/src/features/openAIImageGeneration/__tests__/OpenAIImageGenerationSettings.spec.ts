import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'
import OpenAIImageGenerationSettings from '../OpenAIImageGenerationSettings.vue'

describe('生图主模型默认文案', () => {
  it.each(['en', 'zh'])('%s 显示新的默认模型和取值优先级', (locale) => {
    const wrapper = mount(OpenAIImageGenerationSettings, {
      props: { mainModel: '', reasoningEffort: 'medium' },
      global: {
        plugins: [createI18n({
          legacy: false,
          locale,
          messages: { en, zh },
          // 测试配置使用无编译器的运行时包；这些纯文本文案由此返回，语法另由 locale 编译测试覆盖。
          messageCompiler: (message) => {
            if (typeof message !== 'string') throw new Error('预期为纯文本消息')
            return () => message
          }
        })],
        stubs: { Select: true }
      }
    })
    expect(wrapper.get('input').attributes('placeholder')).toBe('gpt-5.6-luna')
    expect(wrapper.text()).toContain('SUB2API_IMAGES_MAIN_MODEL')
    expect(wrapper.text()).toContain('gpt-5.6-luna')
    expect(wrapper.text()).not.toContain('gpt-5.4-mini')
    expect(wrapper.text()).toContain(locale === 'zh' ? '优先使用此处非空配置' : 'A non-empty setting takes priority')
    wrapper.unmount()
  })
})
