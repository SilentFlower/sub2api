# OpenAI OAuth Codex 自定义客户端放行规则

## Goal

在现有 OpenAI OAuth 账号“仅允许 Codex 官方客户端”限制之上，允许管理员为单个账号额外配置自定义客户端放行规则。该功能用于兼容可信的非官方客户端或包装器，同时保持默认限制逻辑不变，避免未配置账号被意外放宽。

## Confirmed Facts

- 现有开关只对 OpenAI OAuth 账号生效，存储在 `accounts.extra.codex_cli_only`。
- 现有额外放行 Claude Code 插件存储在 `accounts.extra.codex_cli_only_allowed_clients`，只引用后端内置命名预设。
- 网关转发 OpenAI 请求前会执行 `detectCodexClientRestriction`；开启限制但未匹配时直接返回 403，不再请求上游。
- 官方客户端匹配当前基于 `User-Agent` 与 `originator` 请求头。
- 前端创建、编辑、批量编辑账号都已经有 OpenAI OAuth Codex 限制相关 UI。

## Requirements

- 仅在 OpenAI OAuth 账号且 `codex_cli_only=true` 时启用自定义放行规则。
- 管理员可以在账号创建和编辑界面配置自定义放行规则。
- 自定义规则本期只支持 `User-Agent` 前缀匹配，匹配模式支持 `*` 通配符。
- 前端使用单个多行文本框配置自定义 UA pattern，每行一个 pattern；空行忽略。
- 自定义规则命中时，应像官方客户端或命名预设一样放行请求。
- 自定义规则未配置、为空、格式非法或账号未开启 `codex_cli_only` 时，不应放宽现有拦截。
- 现有官方客户端匹配、Claude Code 内置预设、全局 Claude Code 放行开关必须保持兼容。
- 批量编辑纳入本期：仅在管理员勾选“修改自定义 UA 放行规则”时覆盖所选账号；未勾选时不改变已有规则。
- 后端必须集中解析和校验自定义规则，前端不能成为唯一校验点。
- 自定义规则不得记录或暴露敏感请求头完整内容；拒绝日志继续遵循现有白名单和截断策略。
- `codex_cli_only` 拒绝返回 403 时，错误响应体应包含本次请求的 `User-Agent`，便于下游排查实际请求头；该值需要裁剪空白并限制长度。
- 新增用户可见文案必须同时提供中英文 i18n。

## Acceptance Criteria

- [ ] 创建 OpenAI OAuth 账号时，可配置自定义 UA 前缀规则，并正确写入账号 `extra`。
- [ ] 编辑 OpenAI OAuth 账号时，可回显、修改、清空自定义规则；关闭 `codex_cli_only` 后自定义规则不参与放行。
- [ ] 批量编辑纳入本期：勾选对应配置项后能写入/清空同一 UA 规则结构；未勾选时不影响账号已有规则。
- [ ] 创建、编辑、批量编辑均使用多行文本框；每行一个 pattern，空行忽略，重复项可按后端规范去重或安全忽略。
- [ ] 后端能识别并匹配 `*` 通配符，大小写和空白处理规则明确且有测试覆盖。
- [ ] 非 OpenAI OAuth 账号、未开启 `codex_cli_only` 的账号、非法规则配置均不会被自定义规则放行。
- [ ] 已有官方客户端和 Claude Code 放行测试保持通过。
- [ ] 新增后端单元测试覆盖 UA 前缀、通配符、非法配置、优先级/兼容路径。
- [ ] `codex_cli_only` 拒绝返回 403 时，下游错误体可看到本次请求 UA；空 UA 不增加噪音，超长 UA 会截断。
- [ ] 新增或更新前端测试覆盖表单写入和回显。

## Out of Scope

- 不引入数据库迁移；继续使用 `accounts.extra`。
- 不实现强身份认证或客户端签名证明；该功能仍是基于请求头的网关层兼容放行。
- 不改变 OpenAI 官方客户端匹配列表。
- 不开放管理员自定义 Claude Code 内置 preset 的内部匹配规则。
- 不在本期支持任意 header 匹配；如后续确需更强识别，可在 UA 规则稳定后另开任务。

## Open Questions

- 无。
