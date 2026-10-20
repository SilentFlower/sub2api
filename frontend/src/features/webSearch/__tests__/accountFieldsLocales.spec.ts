import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'
import WebSearchEmulationField from '../WebSearchEmulationField.vue'
import AccountCodexWebSearchBridgeField from '../AccountCodexWebSearchBridgeField.vue'

// 测试构建使用不含编译器的运行时，装入最终聚合文案的预编译函数。
// 本处字段没有插值；全部语言文案的语法另由 localesMessageCompile 测试校验。
const compileFields = (fields: Record<string, string>) =>
  Object.fromEntries(Object.entries(fields).map(([key, value]) => [key, () => value]))

const messages = Object.fromEntries(Object.entries({ en, zh }).map(([locale, source]) => [
  locale,
  { admin: { accounts: {
    anthropic: compileFields(source.admin.accounts.anthropic),
    openai: compileFields(source.admin.accounts.openai)
  } } }
]))

describe('合并后搜索字段的实际多语言渲染', () => {
  it.each(['en', 'zh'] as const)('%s 同时显示上游搜索说明与 build 桥接文案', (locale) => {
    const i18n = createI18n({ legacy: false, locale, fallbackLocale: false, messages })
    const search = mount(WebSearchEmulationField, {
      props: { modelValue: 'enabled' },
      global: { plugins: [i18n] }
    })
    const bridge = mount(AccountCodexWebSearchBridgeField, {
      props: { modelValue: 'inherit' },
      global: { plugins: [i18n] }
    })
    expect(search.text()).toContain(i18n.global.t('admin.accounts.anthropic.webSearchEmulationDesc'))
    expect(bridge.text()).toContain(i18n.global.t('admin.accounts.openai.codexWebSearchBridgeDesc'))
    expect(search.text()).not.toContain('admin.accounts.')
    expect(bridge.text()).not.toContain('admin.accounts.')
    if (locale === 'zh') {
      expect(search.text()).toMatch(/[\u4e00-\u9fff]/)
      expect(bridge.text()).toMatch(/[\u4e00-\u9fff]/)
    }
    search.unmount()
    bridge.unmount()
  })
})
