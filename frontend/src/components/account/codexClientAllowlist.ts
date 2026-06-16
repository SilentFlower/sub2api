export const CODEX_CUSTOM_UA_EXTRA_KEY = 'codex_cli_only_custom_user_agent_prefixes'

/**
 * 解析自定义 Codex User-Agent 放行规则输入。
 *
 * @param input 多行文本，每行一个 User-Agent pattern。
 * @return 去掉空行和重复项后的 pattern 列表。
 */
export function parseCodexCustomUserAgentPatterns(input: string): string[] {
  const seen = new Set<string>()
  const result: string[] = []
  for (const line of input.split(/\r?\n/)) {
    const pattern = line.trim()
    if (!pattern) continue
    const key = pattern.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    result.push(pattern)
  }
  return result
}

/**
 * 将账号 extra 中的自定义 User-Agent 规则格式化为多行文本。
 *
 * @param value 从账号 extra 读取到的未知字段值。
 * @return 可直接回显到 textarea 的多行文本。
 */
export function formatCodexCustomUserAgentPatterns(value: unknown): string {
  if (!Array.isArray(value)) return ''
  return value
    .filter((item): item is string => typeof item === 'string' && item.trim() !== '')
    .map(item => item.trim())
    .join('\n')
}

/**
 * 按单账号创建/编辑语义写入自定义 Codex User-Agent 规则。
 *
 * @param extra 待提交到后端的账号 extra 对象。
 * @param enabled 是否启用 codex_cli_only；关闭时会移除自定义放行字段。
 * @param input 多行 User-Agent pattern 输入。
 */
export function writeCodexCustomUserAgentPatterns(
  extra: Record<string, unknown>,
  enabled: boolean,
  input: string
): void {
  if (!enabled) {
    delete extra[CODEX_CUSTOM_UA_EXTRA_KEY]
    return
  }
  const patterns = parseCodexCustomUserAgentPatterns(input)
  if (patterns.length > 0) {
    extra[CODEX_CUSTOM_UA_EXTRA_KEY] = patterns
  } else {
    delete extra[CODEX_CUSTOM_UA_EXTRA_KEY]
  }
}

/**
 * 按批量编辑语义写入自定义 Codex User-Agent 规则。
 *
 * @param extra 批量编辑 payload 中的 extra 对象。
 * @param input 多行 User-Agent pattern 输入，空文本会写入空数组以清空旧规则。
 */
export function writeCodexCustomUserAgentPatternsForBulkEdit(
  extra: Record<string, unknown>,
  input: string
): void {
  extra[CODEX_CUSTOM_UA_EXTRA_KEY] = parseCodexCustomUserAgentPatterns(input)
}
