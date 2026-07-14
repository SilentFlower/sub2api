import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const source = readFileSync(
  resolve(process.cwd(), 'src/components/account/CreateAccountModal.vue'),
  'utf8'
)

describe('CreateAccountModal OpenAI JSON Schema compatibility', () => {
  it('shows the toggle only for OpenAI API Key accounts and persists the extra key', () => {
    expect(source).toContain("form.platform === 'openai' && accountCategory === 'apikey'")
    expect(source).toContain('data-testid="openai-json-schema-downgrade-toggle"')
    expect(source).toContain('extra.openai_json_schema_to_json_object = true')
    expect(source).toContain('delete extra.openai_json_schema_to_json_object')
  })
})
