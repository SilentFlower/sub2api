import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import BulkCodexCustomUserAgentField from '../BulkCodexCustomUserAgentField.vue'
import CodexCustomUserAgentField from '../CodexCustomUserAgentField.vue'
import { readCodexCustomUserAgentPatterns } from '../extra'
import OpenAIJSONSchemaField from '@/features/openAICompatibility/OpenAIJSONSchemaField.vue'
import WebSearchEmulationField from '@/features/webSearch/WebSearchEmulationField.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

describe('账号私有功能字段组件', () => {
  it('从账号 extra 读取并格式化自定义 UA', () => {
    expect(readCodexCustomUserAgentPatterns({
      codex_cli_only_custom_user_agent_prefixes: [' my-client/* ', '', 1]
    })).toBe('my-client/*')
  })

  it('更新 Codex 自定义 User-Agent 输入', async () => {
    const wrapper = mount(CodexCustomUserAgentField, {
      props: {
        modelValue: '',
        fieldId: 'codex-custom-ua',
        testId: 'codex-custom-ua'
      }
    })

    await wrapper.get('textarea').setValue('my-client/*')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['my-client/*'])
  })

  it('批量字段分别更新启用状态和内容', async () => {
    const wrapper = mount(BulkCodexCustomUserAgentField, {
      props: { enabled: false, value: '' }
    })

    await wrapper.get('input[type="checkbox"]').setValue(true)
    await wrapper.setProps({ enabled: true })
    await wrapper.get('textarea').setValue('wrapper/*')

    expect(wrapper.emitted('update:enabled')?.[0]).toEqual([true])
    expect(wrapper.emitted('update:value')?.[0]).toEqual(['wrapper/*'])
  })

  it('JSON Schema 开关发出布尔更新', async () => {
    const wrapper = mount(OpenAIJSONSchemaField, {
      props: { modelValue: false }
    })

    await wrapper.get('[role="switch"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([true])
  })

  it('Web Search 下拉发出三态更新', async () => {
    const wrapper = mount(WebSearchEmulationField, {
      props: { modelValue: 'default' }
    })

    await wrapper.get('select').setValue('disabled')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['disabled'])
  })
})
