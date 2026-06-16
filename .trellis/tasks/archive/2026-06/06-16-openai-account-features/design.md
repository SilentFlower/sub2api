# OpenAI 账号功能设计

## Architecture

本任务实现“单个 OpenAI OAuth 账号的 Codex 邀请与 reset credit 操作”，不做批量扫描或自动重置。

分层落点：

- 后端 service：新增 `OpenAICodexResetService`，负责账号校验、邮箱规范化、reset credit 选择、调用 ChatGPT backend 客户端、错误归一。
- 后端 repository/pkg 适配：新增 `OpenAICodexResetClient` 实现，封装 `https://chatgpt.com/backend-api` 的 HTTP 请求。
- 后端 handler：在 `AccountHandler` 上新增 `GetOpenAICodexResetStatus`、`ConsumeOpenAICodexResetCredit`、`SendOpenAICodexInvites`。
- 后端 routes：在 `/api/v1/admin/accounts/:id/...` 下注册 OpenAI Codex reset 专属路径。
- 前端 API：在 `frontend/src/api/admin/accounts.ts` 新增类型和函数。
- 前端 UI：新增 `frontend/src/components/admin/account/OpenAICodexResetModal.vue`，由 `AccountsView.vue` 控制打开；在 `AccountActionMenu.vue` 中仅对 `platform=openai` 且 `type=oauth` 的账号展示入口。

## Backend Contracts

新增管理员 API：

- `GET /api/v1/admin/accounts/:id/openai-codex-reset/status`
  - 查询 reset credits、invite eligibility、rules。
  - 返回非敏感状态信息。
- `POST /api/v1/admin/accounts/:id/openai-codex-reset/consume`
  - Body: `{ "credit_id": "optional" }`
  - 未传 `credit_id` 时使用第一个可用 credit。
  - 不清理本地限流状态。
- `POST /api/v1/admin/accounts/:id/openai-codex-reset/invite`
  - Body: `{ "emails": ["a@example.com"], "consent_confirmed": true }`
  - 后端再次校验邮箱、去重、数量上限和确认字段。

响应模型：

```go
type OpenAICodexResetAccountSummary struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

type OpenAICodexResetCreditStatus struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

type OpenAICodexResetStatus struct {
	Account            OpenAICodexResetAccountSummary `json:"account"`
	AvailableCount     int                            `json:"available_count"`
	CreditCount         int                            `json:"credit_count"`
	AvailableCreditIDs []string                       `json:"available_credit_ids"`
	CreditStatuses      []OpenAICodexResetCreditStatus `json:"credit_statuses"`
	Eligibility         map[string]any                 `json:"eligibility,omitempty"`
	Rules               map[string]any                 `json:"rules,omitempty"`
}

type OpenAICodexResetConsumeResult struct {
	Account              OpenAICodexResetAccountSummary `json:"account"`
	CreditID             string                         `json:"credit_id"`
	Code                 string                         `json:"code,omitempty"`
	AvailableCount       *int                           `json:"available_count,omitempty"`
	RemainingCreditCount *int                           `json:"remaining_credit_count,omitempty"`
}

type OpenAICodexInviteResult struct {
	Account      OpenAICodexResetAccountSummary `json:"account"`
	Emails       []string                       `json:"emails"`
	InvitedCount *int                           `json:"invited_count,omitempty"`
	FailedEmails []string                       `json:"failed_emails,omitempty"`
	Message      string                         `json:"message,omitempty"`
}
```

## Upstream Calls

固定常量：

- Base URL: `https://chatgpt.com/backend-api`
- Referral key: `codex_referral_persistent_invite`
- 最大邀请邮箱数：5

请求头：

- `Authorization: Bearer <access_token>`
- `Accept: application/json`
- `Content-Type: application/json`
- `OAI-Language: zh-CN`
- `originator: Codex Desktop`
- `X-OpenAI-Attach-Auth: 1`
- `X-OpenAI-Attach-Integrity-State: 1`
- `User-Agent`: 默认沿用项目已有 OpenAI Codex UA 配置；若注入设置服务不方便，先使用参考脚本默认 `Codex Desktop/0.0.0 (Linux; x86_64)`，实现时优先复用 `SettingService.GetOpenAICodexUserAgent`。
- `chatgpt-account-id`: 仅当账号 `credentials.chatgpt_account_id` 非空时发送。

代理：

- 若账号已关联代理，优先通过账号代理请求 ChatGPT backend。
- 若账号未关联代理，则使用直连；失败时返回上游请求错误，不自动切换代理。

## Validation

账号校验：

- 账号必须存在。
- `Platform == service.PlatformOpenAI`。
- `Type == service.AccountTypeOAuth`。
- `credentials.access_token` 必须存在。

邮箱校验：

- 前端可以支持文本拆分，但后端 API 只接收数组，后端仍统一 trim、大小写去重、格式校验。
- 去重后数量必须在 1 到 5 之间。
- 邀请 API 必须 `consent_confirmed=true`。

错误 reason 建议：

- `OPENAI_CODEX_RESET_ACCOUNT_UNSUPPORTED`
- `OPENAI_CODEX_RESET_ACCESS_TOKEN_MISSING`
- `OPENAI_CODEX_RESET_NO_AVAILABLE_CREDIT`
- `OPENAI_CODEX_RESET_CONFIRMATION_REQUIRED`
- `OPENAI_CODEX_RESET_INVALID_EMAIL`
- `OPENAI_CODEX_RESET_TOO_MANY_EMAILS`
- `OPENAI_CODEX_RESET_UPSTREAM_FAILED`

## Security

- 不向前端返回 `access_token`、`refresh_token`、`id_token`、`chatgpt_account_id` 或完整 Authorization header。
- Codex reset / invite 上游失败时，错误 message 可返回脱敏并截断后的上游响应体，用于管理员排查 ChatGPT backend 拒绝原因；不得返回未脱敏 token、cookie、完整 Authorization header 或账号内部凭证。
- 日志只记录账号 ID、HTTP status、操作类型、错误分类。
- 本任务不落库保存上游原始 eligibility/rules；仅当次返回给管理员查看。

## Frontend Design

`OpenAICodexResetModal.vue` 结构：

- 使用 `BaseDialog width="wide"`。
- 顶部显示账号名、邮箱和刷新按钮。
- 左侧展示可用 reset credit 数量、credit 状态列表、使用 reset credit 按钮。
- 右侧展示邮箱 `textarea`、最多 5 个提示、收件人同意 checkbox、发送邀请按钮和结果。
- “邀请规则”使用可折叠区域展示 eligibility/rules 的简要 JSON 或状态摘要。

前端状态：

- `loadingStatus`：打开弹窗时自动查询。
- `consuming`：消耗 credit 中。
- `sendingInvite`：发送邀请中。
- `statusResult`、`consumeResult`、`inviteResult`、`errorMessage` 分开保存，避免一个操作覆盖另一个结果。

## Data Flow

查询：

`AccountActionMenu` → `AccountsView` → `OpenAICodexResetModal` → `adminAPI.accounts.getOpenAICodexResetStatus` → handler → service → client → ChatGPT backend。

消耗：

`OpenAICodexResetModal` → consume API → service 先查询 credits → 选择可用 credit → upstream consume → 返回结果 → 前端刷新 status。

邀请：

`OpenAICodexResetModal` 本地拆分邮箱 → invite API → service 规范化/校验 → upstream invite → 返回成功/失败详情。

## Compatibility

- 不改数据库 schema，不新增 migration。
- 不改账号 DTO 脱敏策略。
- 不影响现有 `/admin/accounts/:id/usage`、`/clear-rate-limit`、`/recover-state`。
- 本地限流字段保持原样，避免误以为 reset credit 已立即恢复 Sub2API 调度状态。

## Testing

后端：

- service 单元测试使用 fake client / fake account repo。
- handler 测试覆盖路由绑定、请求校验和统一响应 envelope。
- 重点断言 consume 不调用 `ClearRateLimit` 之类本地状态清理。

前端：

- API 函数测试可沿用现有 axios mock 模式。
- 组件测试覆盖：
  - 打开后查询 status。
  - 无可用 credit 时按钮禁用。
  - 未勾选确认时不能发送邀请。
  - 邮箱拆分、去重、超过 5 个报错。
  - 成功/失败结果渲染。
