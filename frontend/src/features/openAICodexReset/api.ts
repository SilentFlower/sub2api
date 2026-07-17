import { apiClient } from '@/api/client'
import type {
  OpenAICodexInviteRequest,
  OpenAICodexInviteResult,
  OpenAICodexResetConsumeResult,
  OpenAICodexResetStatus
} from './types'

/**
 * 查询单个 OpenAI OAuth 账号的 Codex reset 状态。
 *
 * @param id 账号 ID。
 * @return Codex reset 状态。
 */
export async function getOpenAICodexResetStatus(id: number): Promise<OpenAICodexResetStatus> {
  const { data } = await apiClient.get<OpenAICodexResetStatus>(
    `/admin/accounts/${id}/openai-codex-reset/status`
  )
  return data
}

/**
 * 消耗单个 OpenAI OAuth 账号的 Codex reset credit。
 *
 * @param id 账号 ID。
 * @param creditId 可选的 reset credit ID；为空时由后端选择第一个可用 credit。
 * @return reset credit 消耗结果。
 */
export async function consumeOpenAICodexResetCredit(
  id: number,
  creditId?: string
): Promise<OpenAICodexResetConsumeResult> {
  const { data } = await apiClient.post<OpenAICodexResetConsumeResult>(
    `/admin/accounts/${id}/openai-codex-reset/consume`,
    { credit_id: creditId ?? '' }
  )
  return data
}

/**
 * 使用单个 OpenAI OAuth 账号发送 Codex 邀请。
 *
 * @param id 账号 ID。
 * @param payload 邀请邮箱和收件人同意确认。
 * @return 邀请发送结果。
 */
export async function sendOpenAICodexInvites(
  id: number,
  payload: OpenAICodexInviteRequest
): Promise<OpenAICodexInviteResult> {
  const { data } = await apiClient.post<OpenAICodexInviteResult>(
    `/admin/accounts/${id}/openai-codex-reset/invite`,
    payload
  )
  return data
}
