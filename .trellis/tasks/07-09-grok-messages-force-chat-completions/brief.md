# Brief — Grok messages force chat completions

## Goal

- 允许管理员为 Grok 账号显式设置 `openai_responses_mode=force_chat_completions`，让 Grok 处理入站 `/v1/messages` 时走 xAI `/chat/completions`，默认行为仍保持 `/responses`。

## Scope

- 后端：扩展 `/v1/messages` 的 OpenAI 兼容分流判断，使 Grok OAuth/APIKey 在显式 `force_chat_completions` 时进入现有 `forwardAnthropicViaRawChatCompletions`。
- 后端：确保 forced Chat 模式下 usage/upstream endpoint 记录为 `/v1/chat/completions`。
- 前端：在 Grok 账号创建/编辑界面暴露与 OpenAI APIKey 相同的 Responses/Chat Completions 模式选择，并保存到 `extra.openai_responses_mode`。
- 测试：覆盖 Grok OAuth forced chat、Grok 默认 responses、Grok APIKey forced chat，以及前端配置保存。

## Non-Goals

- 不把所有 Grok OAuth 默认改成 Chat Completions。
- 不新增数据库字段或迁移。
- 不改 xAI OAuth 登录、刷新、额度查询、图片/视频生成路径。
- 不调整普通 `/v1/chat/completions` 入站路径的既有行为，除非为保持记录字段正确必须做最小修正。

## Key Context

- Grok `/v1/messages` 入口在 `backend/internal/server/routes/gateway.go:97`，进入 `OpenAIGatewayHandler.Messages`。
- 当前 Grok 调度平台来自 `backend/internal/handler/openai_gateway_handler.go:100` 和 `backend/internal/handler/openai_gateway_handler.go:809`。
- 当前 `ForwardAsAnthropic` 只有 OpenAI APIKey 不使用 Responses 时走 raw Chat fallback，见 `backend/internal/service/openai_gateway_messages.go:35`。
- 现有配置字段和值来自 `backend/internal/pkg/openai_compat/upstream_capability.go:52`。
- raw Chat 桥接已有实现位于 `backend/internal/service/openai_gateway_messages_chat_fallback.go:20`。
- 前端当前只对 OpenAI APIKey 显示模式选择，见 `frontend/src/components/account/EditAccountModal.vue:1560` 和 `frontend/src/components/account/CreateAccountModal.vue:2913`。
- 约束：保存 Grok `extra` 时必须保留现有键，例如 `grok_usage_snapshot`、`email`、限额配置；不得暴露 token/APIKey。

## Acceptance

- Grok OAuth/APIKey 设置 `extra.openai_responses_mode="force_chat_completions"` 后，`POST /v1/messages` 调用 xAI `/chat/completions` 而不是 `/responses`。
- Grok 账号未设置该字段或设置为 `auto` 时，`POST /v1/messages` 仍保持现有 `/responses` 路径。
- Chat Completions 强制模式下，下游仍收到 Anthropic Messages 兼容响应。
- forced Chat 模式下 usage 记录的 upstream endpoint 反映 `/v1/chat/completions`。
- 前端 Grok 创建/编辑页能设置该模式，保存 payload 包含 `extra.openai_responses_mode` 并保留已有 `extra`。
- 后端和前端验证命令按 `implement.md` 执行或说明未执行原因。

## Next Step

- 用户确认 planning artifacts 和本 brief 后，运行 `task.py start` 激活任务，再进入实现路由。
