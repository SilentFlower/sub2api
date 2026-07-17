import type { OpenAIResponsesMode } from '@/types'

/** Responses 路由模式下拉选项。 */
export interface OpenAIResponsesModeOption extends Record<string, unknown> {
  value: OpenAIResponsesMode
  label: string
}
