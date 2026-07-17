export default {
  codexCLIOnlyCustomUA: '自定义放行 UA 前缀',
  codexCLIOnlyCustomUADesc: '仅在上方开关开启时生效。每行一个 User-Agent pattern，支持 * 通配符；命中任一行即放行。',
  codexCLIOnlyCustomUABulkDesc: '批量覆盖所选 OpenAI OAuth 账号的自定义 User-Agent 放行规则。',
  codexCLIOnlyCustomUABulkHint: '勾选后提交：每行一个 pattern，空内容会清空所选账号的自定义 UA 规则。',
  codexCLIOnlyCustomUAPlaceholder: 'my-client/*\ncustom-codex-wrapper/*'
}
