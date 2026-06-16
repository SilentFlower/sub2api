# OpenAI 账号功能实现计划

## Checklist

1. 后端 service 合约与实现
   - 新增 OpenAI Codex reset 相关请求/响应模型。
   - 新增 `OpenAICodexResetClient` 端口接口。
   - 新增 `OpenAICodexResetService`，实现账号校验、邮箱规范化、status/consume/invite。

2. 后端 upstream client
   - 新增 HTTP client 实现，调用 ChatGPT backend endpoints。
   - 支持账号代理。
   - 实现 JSON 响应解析、错误摘要、敏感信息保护。

3. 后端 DI 与 routes
   - 注册 repository provider 和 service provider。
   - 将 `OpenAICodexResetService` 注入 `AccountHandler`。
   - 注册 `/admin/accounts/:id/openai-codex-reset/status|consume|invite`。
   - 更新必要测试 stub，保持已有 handler 测试可编译。

4. 后端测试
   - service 测试覆盖非 OpenAI OAuth、缺 token、credits 可用性、consume payload、invite 校验。
   - client 测试使用 `httptest.Server` 覆盖 headers、payload、错误响应脱敏。
   - handler 测试覆盖请求绑定和响应。

5. 前端类型与 API
   - 在 `frontend/src/types/index.ts` 或 `accounts.ts` 添加 OpenAI Codex reset 类型。
   - 在 `frontend/src/api/admin/accounts.ts` 添加 status/consume/invite 函数。

6. 前端 UI
   - 新增 `OpenAICodexResetModal.vue`。
   - `AccountActionMenu.vue` 增加 OpenAI OAuth 专属入口。
   - `AccountsView.vue` 挂载弹窗并处理打开/关闭。
   - 同步中英文 i18n 文案。

7. 前端测试
   - 添加组件测试覆盖邮箱解析、确认框、按钮状态和 API 结果。
   - 如测试成本过高，至少覆盖核心纯函数并运行现有相关测试。

8. 验证
   - 后端：`cd backend && go test ./internal/service ./internal/repository ./internal/handler/admin ./internal/server/routes`
   - 前端：`cd frontend && pnpm test -- --run OpenAICodexResetModal` 或相关 spec。
   - 全量轻验：`cd backend && go test ./internal/service/... ./internal/handler/...`、`cd frontend && pnpm typecheck`，视耗时调整。

## Risky Files

- `backend/internal/handler/admin/account_handler.go`：构造函数参数会影响多处测试。
- `backend/cmd/server/wire_gen.go`：若无法运行 wire，需要手工同步生成文件。
- `frontend/src/views/admin/AccountsView.vue`：大文件，改动应仅限 import、状态、弹窗挂载和事件处理。
- `frontend/src/i18n/locales/en.ts` / `zh.ts`：大对象，插入位置要保持语法正确。

## Rollback Points

- 后端路由和 handler 可独立回滚，不影响现有账号功能。
- 前端入口可先隐藏，仅保留后端 API。
- 若 ChatGPT backend 返回结构与参考脚本不一致，保持 `status` 原始摘要为 `map[string]any`，前端展示降级为 JSON 摘要。

## Pre-Start Review

- PRD 已确认：MVP 只做单账号弹窗，不做批量自动重置。
- 设计不涉及 DB schema 和 migration。
- 实现前需用户确认这些规划文件可以进入 `task.py start`。
