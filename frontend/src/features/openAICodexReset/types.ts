/** OpenAI Codex reset 功能展示用账号摘要。 */
export interface OpenAICodexResetAccountSummary {
  id: number
  name: string
  email?: string
}

/** OpenAI Codex reset credit 的非敏感状态。 */
export interface OpenAICodexResetCreditStatus {
  id: string
  status: string
  title?: string
  description?: string
  granted_at?: string
  expires_at?: string
}

/** OpenAI Codex reset 状态查询结果。 */
export interface OpenAICodexResetStatus {
  account: OpenAICodexResetAccountSummary
  available_count: number
  credit_count: number
  available_credit_ids: string[]
  credit_statuses: OpenAICodexResetCreditStatus[]
  eligibility?: Record<string, unknown>
  rules?: Record<string, unknown>
}

/** OpenAI Codex reset credit 消耗结果。 */
export interface OpenAICodexResetConsumeResult {
  account: OpenAICodexResetAccountSummary
  credit_id: string
  code?: string
  available_count?: number
  remaining_credit_count?: number
}

/** OpenAI Codex 邀请发送结果。 */
export interface OpenAICodexInviteResult {
  account: OpenAICodexResetAccountSummary
  emails: string[]
  invited_count?: number
  failed_emails?: string[]
  message?: string
}

/** OpenAI Codex 邀请请求。 */
export interface OpenAICodexInviteRequest {
  emails: string[]
  consent_confirmed: boolean
}
