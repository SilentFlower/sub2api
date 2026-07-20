import { apiClient } from '@/api/client'

/** 反重力 GIF 多帧兼容的全局设置。 */
export interface AntigravityGIFCompatibilitySettings {
  enabled: boolean
  max_frames_per_gif: number
}

/**
 * 获取反重力 GIF 多帧兼容设置。
 *
 * @return 当前全局设置。
 */
export async function getAntigravityGIFCompatibilitySettings(): Promise<AntigravityGIFCompatibilitySettings> {
  const { data } = await apiClient.get<AntigravityGIFCompatibilitySettings>(
    '/admin/settings/antigravity-gif'
  )
  return data
}

/**
 * 更新反重力 GIF 多帧兼容设置。
 *
 * @param settings 待保存的全局设置。
 * @return 保存后的全局设置。
 */
export async function updateAntigravityGIFCompatibilitySettings(
  settings: AntigravityGIFCompatibilitySettings
): Promise<AntigravityGIFCompatibilitySettings> {
  const { data } = await apiClient.put<AntigravityGIFCompatibilitySettings>(
    '/admin/settings/antigravity-gif',
    settings
  )
  return data
}
