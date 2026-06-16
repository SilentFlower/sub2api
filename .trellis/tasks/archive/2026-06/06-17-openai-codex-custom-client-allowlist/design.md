# OpenAI OAuth Codex 自定义客户端放行规则设计

## Scope

本任务在现有 `codex_cli_only` 限制上新增账号级自定义放行规则。数据继续存放在 `accounts.extra`，避免迁移和 Ent schema 改动。

## Current Flow

1. 前端在 OpenAI OAuth 账号创建/编辑/批量编辑中写入 `extra.codex_cli_only`。
2. 后端 `Account.IsCodexCLIOnlyEnabled()` 判断账号是否启用限制。
3. `OpenAIGatewayService.Forward()` 调用 `detectCodexClientRestriction()`。
4. detector 依次匹配官方 UA、官方 `originator`、账号级命名 preset、全局 preset。
5. 未匹配时返回 403。

## Proposed Data Contract

新增账号 extra 字段：

```json
{
  "codex_cli_only_custom_user_agent_prefixes": [
    "my-codex-wrapper/*",
    "another-wrapper/*"
  ]
}
```

字段含义：

- `codex_cli_only_custom_user_agent_prefixes`：账号级自定义 `User-Agent` 前缀规则列表；模式支持 `*`。

候选限制：

- 单账号规则数量设置合理上限，避免热路径配置过大。
- 单条 pattern 长度设置合理上限，避免异常配置造成性能或日志风险。
- 空白 pattern、空规则必须被忽略或判为不匹配。
- UA 值匹配默认大小写不敏感，与现有 Codex 头匹配行为保持一致。

## Matching Semantics

本期只匹配 `User-Agent`。多个 pattern 按 OR 匹配，任一 UA pattern 命中即视为自定义规则命中。

`*` 通配符建议只表示“任意字符序列”，不引入正则语法。实现上应转义其他字符后编译为安全的内部匹配，或使用小型 glob 匹配函数，避免直接把用户输入当正则。

## Backend Boundaries

- `internal/service/account.go`：新增 getter，集中从 `Extra` 解码自定义规则。
- `internal/pkg/openai`：新增自定义规则类型和匹配函数，和现有官方/preset 匹配逻辑并列。
- `internal/service/openai_client_restriction_detector.go`：在官方/preset 之后增加自定义规则匹配分支，并新增 reason。
- `internal/service/openai_gateway_service.go`：保持拦截入口不变，必要时扩展诊断字段但不记录敏感 header 值。

## Frontend Boundaries

- `CreateAccountModal.vue` / `EditAccountModal.vue`：在 `codexCLIOnlyEnabled` 下方增加自定义放行规则编辑区域，使用多行文本框，每行一个 UA pattern。
- `BulkEditAccountModal.vue`：纳入本期。沿用现有批量编辑模式，增加一个“是否修改自定义 UA 规则”的 checkbox；勾选后写入同一 extra 字段结构，未勾选则不提交该字段。
- `i18n/locales/en.ts` / `zh.ts`：新增文案。
- 测试跟随现有账号 modal spec。

## Compatibility

- 未配置新字段时行为完全等同当前版本。
- 旧字段 `codex_cli_only_allowed_clients` 保持原语义。
- 非 OpenAI OAuth 账号读取新字段时返回空。
- 非 bool 的 `codex_cli_only` 仍按关闭处理。

## Risks

- 该功能基于请求头，不能提供强认证；管理员配置过宽会弱化 `codex_cli_only`。
- `*` 过宽例如 `*` 或 `Mozilla*` 可能放行大量非目标客户端。UI 需要提示风险，后端至少避免空规则误放行。
- 本期不支持 header 匹配，无法表达“UA + 自定义 header 双因子”规则；换取更简单的配置和更小实现面。
- 批量编辑会同时覆盖多个账号的放行规则，UI 需要明确“未勾选不修改，勾选且为空表示清空”的语义。
