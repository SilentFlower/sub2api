# Grok /v1/messages 强制走 Chat Completions

## 目标

让管理员可以像配置 OpenAI APIKey 账号一样，为 Grok 账号显式设置 `openai_responses_mode=force_chat_completions`，使 Grok 账号处理入站 `/v1/messages` 时走 xAI `/chat/completions`，而不是默认 `/responses`。

## 背景与已确认事实

- `/v1/messages` 在 Grok 分组下会进入 `OpenAIGatewayHandler.Messages`，而不是普通 Claude 网关；证据：`backend/internal/server/routes/gateway.go:97`。
- Grok 分组会把请求平台固定为 `grok` 并只调度 Grok 账号；证据：`backend/internal/handler/openai_gateway_handler.go:100`、`backend/internal/handler/openai_gateway_handler.go:809`。
- 现有 `ForwardAsAnthropic` 只有 `APIKey + !ShouldUseResponsesAPI(extra)` 会走 `forwardAnthropicViaRawChatCompletions`；Grok OAuth 账号不会进入该分支；证据：`backend/internal/service/openai_gateway_messages.go:35`。
- `openai_responses_mode=force_chat_completions` 已存在于 OpenAI 兼容能力判断中；证据：`backend/internal/pkg/openai_compat/upstream_capability.go:52`。
- `forwardAnthropicViaRawChatCompletions` 已能把 Anthropic Messages 请求转换为 Chat Completions，并把上游流式响应转回 Anthropic 格式；证据：`backend/internal/service/openai_gateway_messages_chat_fallback.go:20`。
- 前端当前只在 `platform=openai && type=apikey` 时显示 Responses 模式选择；Grok 账号编辑页没有配置入口；证据：`frontend/src/components/account/EditAccountModal.vue:1560`、`frontend/src/components/account/CreateAccountModal.vue:2913`。
- 远程账号 `3815` 是 `platform=grok`、`type=oauth`，因此当前手工写入 OpenAI APIKey 专用开关不会影响它的 `/v1/messages` 行为。

## 需求

1. Grok 账号在 `extra.openai_responses_mode == "force_chat_completions"` 时，入站 `/v1/messages` 必须走 Chat Completions 直连桥接路径。
2. Grok OAuth 和 Grok APIKey 都应支持该显式强制模式。
3. 默认行为必须保持不变：缺失该配置或配置为 `auto` 时，Grok `/v1/messages` 仍走现有 Responses 路径。
4. 继续复用现有字段名和值：`openai_responses_mode`、`auto`、`force_responses`、`force_chat_completions`，避免新增一套 Grok 专用配置。
5. 前端账号创建/编辑界面应允许 Grok 账号配置该模式，并把非 `auto` 值持久化到 `extra.openai_responses_mode`。
6. 保存 Grok 账号配置时不得覆盖或丢失既有 `extra` 键，例如 `grok_usage_snapshot`、`email`、限额配置等。
7. 不改变 Grok token 刷新、OAuth 凭据结构、模型默认映射和账号调度优先级。
8. 不在日志、错误响应或前端展示中暴露 Grok token、APIKey 或完整敏感凭据。

## 验收标准

- [ ] Grok OAuth 账号设置 `extra.openai_responses_mode="force_chat_completions"` 后，`POST /v1/messages` 会调用 xAI `/chat/completions`，不会调用 `/responses`。
- [ ] Grok APIKey 账号设置 `extra.openai_responses_mode="force_chat_completions"` 后，`POST /v1/messages` 会调用 xAI `/chat/completions`。
- [ ] Grok 账号未设置该字段或设置为 `auto` 时，`POST /v1/messages` 仍保持现有 `/responses` 路径。
- [ ] Chat Completions 强制模式下，下游仍收到 Anthropic Messages 兼容响应，流式和非流式至少覆盖一个测试路径。
- [ ] 该强制模式下 usage 记录的 upstream endpoint 能反映实际 `/v1/chat/completions` 路径。
- [ ] 前端 Grok 账号创建/编辑页能设置该模式，保存后请求 payload 包含 `extra.openai_responses_mode`，并保留已有 `extra`。
- [ ] 相关后端单测覆盖 Grok OAuth forced chat、Grok 默认 responses、现有 OpenAI APIKey 行为不回退。
- [ ] 相关前端单测或类型检查覆盖 Grok 配置入口和保存 payload。

## 非目标

- 不把所有 Grok OAuth 默认改为 Chat Completions。
- 不新增数据库字段或迁移。
- 不改 xAI OAuth 登录、刷新、额度查询、图片/视频生成路径。
- 不调整普通 `/v1/chat/completions` 入站路径的既有行为，除非为保持记录字段正确必须做最小修正。

## 开放问题

当前无阻塞开放问题。默认采用“显式配置才切换”的保守方案。
