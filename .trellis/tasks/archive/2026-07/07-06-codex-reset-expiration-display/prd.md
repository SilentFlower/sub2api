# 显示 Codex 邀请重置过期时间

## Goal

在后台账号的「Codex 邀请重置」弹窗中展示每个 reset credit 的过期时间，帮助管理员判断哪些重置次数需要优先使用，避免只看到可用数量却不知道有效期。

## Background

- 现有入口位于 `frontend/src/components/admin/account/OpenAICodexResetModal.vue`，通过 `GET /api/v1/admin/accounts/:id/openai-codex-reset/status` 获取 credit 列表。
- 后端调用 ChatGPT Web 后端 `GET /backend-api/wham/rate-limit-reset-credits`，当前只透传 `id/status/title/description`。
- 联网资料可确认 Codex reset credit 属于“保存后稍后使用”的产品形态；字段级实现仍以当前 ChatGPT 后端 payload 为准，按可选 `expires_at` 兼容处理，避免上游缺字段时破坏旧响应。
- 当前实现前我已误把它当小改开始了两处代码编辑：`backend/internal/service/openai_codex_reset_service.go` 和 `backend/internal/repository/openai_codex_reset_client.go`。后续实现需基于本 PRD 复核并补齐，不继续扩大未规划改动。

## Requirements

- 后端 reset credit 摘要必须透传上游返回的 `expires_at`；允许同时透传 `granted_at` 作为非敏感诊断字段。
- 前端 API 类型必须同步新增字段，保持后端 JSON 的 snake_case 命名。
- 「Codex 邀请重置」弹窗的 credit 明细中，如果 `expires_at` 有效，必须显示本地化后的过期时间。
- 如果 `expires_at` 缺失或格式无效，不显示过期行，避免展示误导性的空值或错误时间。
- 展示文案必须走现有 i18n，至少同步 `zh.ts` 和 `en.ts`。
- 不打印、不保存 access token、refresh token、Cookie 或上游原始响应。

## Acceptance Criteria

- [ ] 后端 `OpenAICodexResetCreditStatus` 响应包含 `expires_at`，并在上游返回 RFC3339 字符串或 Unix 时间戳时正确透传/规范化。
- [ ] 前端弹窗在 credit 列表中显示「过期时间 / Expires」及格式化后的本地时间。
- [ ] 前端在缺少 `expires_at` 的旧响应下仍可正常渲染。
- [ ] 前端单测覆盖过期时间展示。
- [ ] 后端 repository 单测覆盖 `expires_at` 字段映射。
- [ ] 相关 Go 单测和前端组件单测通过，或记录明确的环境/无关失败原因。

## Notes

- 资料参考：公开文章确认 Codex 已支持保存 rate limit reset 后稍后使用；字段级过期时间来自当前上游 payload 兼容需求，需通过后端 DTO 与单测固化为本项目契约。
- 这是跨后端 DTO、上游 payload、前端类型和弹窗展示的小型跨层改动，PRD-only 足够；实现前需要完成任务激活。
