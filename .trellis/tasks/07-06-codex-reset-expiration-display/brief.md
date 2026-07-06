# Brief — 显示 Codex 邀请重置过期时间

## Goal

- 在后台账号的「Codex 邀请重置」弹窗中展示每个 reset credit 的过期时间，帮助管理员判断哪些重置次数需要优先使用。

## Scope

- 后端 reset credit 摘要透传上游返回的可选 `expires_at`，允许同时透传 `granted_at` 作为非敏感诊断字段。
- 前端 API 类型同步新增 snake_case 字段。
- 弹窗 credit 明细在 `expires_at` 有效时显示本地化后的过期时间。
- 同步 `zh.ts` 和 `en.ts` 文案。
- 补充后端 repository 单测和前端组件单测。

## Non-Goals

- 不保存上游原始响应。
- 不记录 access token、refresh token、Cookie 或未脱敏上游响应。
- 不扩大到邀请发送、reset credit 消耗流程或数据库结构调整。

## Key Context

- 现有前端入口：`frontend/src/components/admin/account/OpenAICodexResetModal.vue`。
- 现有前端 API 类型：`frontend/src/api/admin/accounts.ts`。
- 现有后端状态接口：`GET /api/v1/admin/accounts/:id/openai-codex-reset/status`。
- 后端调用 ChatGPT Web 后端 `GET /backend-api/wham/rate-limit-reset-credits`，当前只透传 `id/status/title/description`。
- `expires_at` 是可选上游字段；缺失或格式无效时前端不显示过期行，避免误导。
- 当前已有两处提前开始的代码编辑，需要在实现阶段复核并纳入本任务：`backend/internal/service/openai_codex_reset_service.go`、`backend/internal/repository/openai_codex_reset_client.go`。
- 任务上下文清单已补充：`implement.jsonl`、`check.jsonl` 包含跨层、前端组件、前端类型、后端日志与质量规范。

## Acceptance

- 后端 `OpenAICodexResetCreditStatus` 响应包含 `expires_at`，并在上游返回 RFC3339 字符串或 Unix 时间戳时正确透传/规范化。
- 前端弹窗在 credit 列表中显示「过期时间 / Expires」及格式化后的本地时间。
- 前端在缺少 `expires_at` 的旧响应下仍可正常渲染。
- 前端单测覆盖过期时间展示。
- 后端 repository 单测覆盖 `expires_at` 字段映射。
- 相关 Go 单测和前端组件单测通过，或记录明确的环境/无关失败原因。

## Next Step

- 用户确认 planning artifacts 和本 brief 后，运行 `task.py start .trellis/tasks/07-06-codex-reset-expiration-display`；任务进入 `in_progress` 后先走 `trellis-route(implement)`，再继续实现。
