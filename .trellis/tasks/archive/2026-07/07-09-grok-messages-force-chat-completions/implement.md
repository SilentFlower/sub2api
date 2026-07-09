# 实施计划

## 步骤

1. 后端分流
   - 新增 service 层判断函数，统一表达 OpenAI/Grok `/v1/messages` 是否走 raw Chat Completions。
   - 修改 `OpenAIGatewayService.ForwardAsAnthropic` 使用该判断。
   - 确认或补齐 Grok OAuth/APIKey 在 raw Chat fallback 中的目标 URL 和凭据处理。

2. Usage endpoint
   - 修改 `resolveOpenAIUpstreamEndpoint` 或其辅助逻辑，使 Grok `/v1/messages` 强制 Chat 模式记录 `/v1/chat/completions`。

3. 前端配置
   - 抽取 Responses 模式可配置条件，覆盖 OpenAI APIKey 和 Grok OAuth/APIKey。
   - 修改创建/编辑保存逻辑，Grok 账号也能写入或删除 `extra.openai_responses_mode`。
   - 检查文案是否可复用，必要时补最小 i18n。

4. 测试
   - 补后端单测覆盖 Grok OAuth forced chat、默认 responses、Grok APIKey forced chat。
   - 补或调整前端测试覆盖 Grok 配置入口和 payload。

5. 验证
   - `cd backend && go test -tags=unit ./internal/service -run 'Test.*Grok.*Messages|Test.*ForwardAsAnthropic|Test.*ChatCompletions'`
   - `cd backend && go test -tags=unit ./internal/handler -run 'Test.*OpenAI.*Endpoint|Test.*Grok|Test.*Gateway'`
   - `cd frontend && pnpm typecheck`
   - 如前端测试变更，运行对应 `pnpm vitest run ...`。

## 风险文件

- `backend/internal/service/openai_gateway_messages.go`
- `backend/internal/service/openai_gateway_messages_chat_fallback.go`
- `backend/internal/handler/openai_chat_completions.go`
- `frontend/src/components/account/EditAccountModal.vue`
- `frontend/src/components/account/CreateAccountModal.vue`

## 回滚点

- 后端分流判断和前端展示条件应保持小范围修改，便于单独回滚。
- 不做数据库迁移；如线上配置出现问题，删除账号 `extra.openai_responses_mode` 即可恢复默认 Responses 路径。
