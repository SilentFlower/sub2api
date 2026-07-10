# Brief — 修复 OpenAI GPT-5.6 Luna Codex 客户端版本兼容

## Goal

- 修复 OpenAI OAuth 转发 `gpt-5.6-luna` 时因项目内置 Codex 客户端身份低于官方最低版本而返回 404 的问题，并避免合法模型误触发 30 分钟模型级冷却。

## Scope

- 在 `backend/internal/service` 新增唯一版本源 `openAICodexClientVersion = "0.144.1"`。
- 由唯一版本源派生 CLI UA、TUI 默认 UA、compact `Version` 和模型/用量探测版本。
- 删除 `openai_gateway_service.go`、`account_usage_service.go`、`setting_gateway_runtime.go` 中重复的 `0.125.0` 生产声明。
- 覆盖 HTTP Responses、Chat Completions bridge、compact、OAuth passthrough、WebSocket、模型目录、账号用量探测、账号测试和后台默认 UA 的身份一致性。
- 增加 Sol、Terra、Luna 请求构造与关键 Header 回归测试。

## Non-Goals

- 不实现基于 `minimal_client_version` 的动态版本协商、升级或回退。
- 不修改模型映射、定价、上下文窗口或前端展示。
- 不修改 OAuth、TLS/设备指纹、会话隔离、账号调度或 404 冷却策略。
- 不新增配置、DTO、数据库字段或 migration。
- 不覆盖管理员或账号显式配置的 User-Agent，不使用真实 OAuth 凭据执行线上 A/B。

## Key Context

- OpenAI Codex `rust-v0.144.1` 模型目录声明 Sol、Terra、Luna 的 `minimal_client_version` 均为 `0.144.0`。
- 当前生产代码在四个身份声明点仍使用 `0.125.0`；历史 `#1933` 已证明同类旧版本硬编码会导致新模型路径被上游拒绝。
- `gateway.force_codex_cli` 默认保持 `false`；普通非 passthrough HTTP 路径继续保留入站 UA，只有现有强制/兜底规则使用新版内置身份。
- 全局 `openai_codex_user_agent` 的持久化默认值为空，因此更新内置默认 UA 不需要迁移；显式配置继续优先。
- `min_codex_version` / `max_codex_version` 属于入站客户端限制策略，不能作为上游身份版本来源。

## Acceptance

- 非测试生产 Go 代码不再用 `0.125.0` 构造 Codex 上游身份。
- 所有内置版本消费者统一派生自 `0.144.1`，相关 UA/Version 一致。
- Sol、Terra、Luna 模型名原样转发，并在触发内置身份时使用 `0.144.1`。
- 自定义 UA、显式模型目录 `client_version`、`force_codex_cli=false` 和现有路由优先级保持不变。
- 现有 404 model-not-found 冷却测试继续通过。
- 定向与完整后端单元测试、`golangci-lint`、生产字面量扫描和 `git diff --check` 通过，或明确记录无关既有失败。

## Next Step

- 用户确认 planning artifacts 和本 brief 后，运行 `task.py start`，再进入 `trellis-route(implement)` 选择实现执行方式。
