# OpenAI 账号功能

## Goal

参考 `/root/project/my/codex-rate-res-main`，在 Sub2API 管理端为 OpenAI OAuth 账号新增 Codex 邀请与额度重置能力，让管理员可以在不暴露 OpenAI OAuth token 的前提下：

- 查看账号可用 Codex rate-limit reset credit。
- 对明确选中的 OpenAI OAuth 账号消耗可用 reset credit。
- 用选中账号向邮箱列表发送 Codex 邀请。

该功能使用 ChatGPT/Codex backend 行为，不属于公开 OpenAI API；实现必须保持账号凭证脱敏，不自动清理 Sub2API 本地限流状态。

## Confirmed Facts

- 参考项目核心脚本为 `scripts/codex_rate_reset.mjs`，支持 `status`、`credits`、`eligibility`、`rules`、`consume`、`invite`。
- 参考脚本调用 `https://chatgpt.com/backend-api`：
  - `GET /wham/rate-limit-reset-credits`
  - `POST /wham/rate-limit-reset-credits/consume`
  - `GET /referrals/invite/eligibility?referral_key=codex_referral_persistent_invite`
  - `GET /wham/referrals/eligibility_rules?referral_key=codex_referral_persistent_invite`
  - `POST /wham/referrals/invite`
- 参考脚本从 Sub2API 读取账号时筛选 `accounts.deleted_at is null`、`platform = 'openai'`、`type = 'oauth'`，使用 `credentials.access_token` 和 `credentials.chatgpt_account_id`，邮箱来自 `credentials.email` 或 `extra.email`。
- 当前 Sub2API 后端 `Account` 实体已有 `platform`、`type`、`credentials`、`extra`、`rate_limited_at`、`rate_limit_reset_at` 等字段。
- 当前 Sub2API DTO 会通过 `RedactCredentials` 剥离 `access_token`、`refresh_token` 等敏感字段，只向前端暴露 `credentials_status.has_<key>`。
- 当前前端账号类型已包含 Codex 使用快照字段：`codex_5h_used_percent`、`codex_7d_used_percent`、`codex_5h_reset_at`、`codex_7d_reset_at` 等。
- 当前管理端账号路由已有 `/admin/accounts/:id/usage`、`/clear-rate-limit`、`/recover-state`、`/reset-quota` 等账号操作，适合新增 OpenAI Codex 专属账号操作。
- 截图展示的是一个“Codex 邀请重置”弹窗：顶部选择/展示账号，左侧展示可用重置次数并提供刷新/使用重置次数，右侧输入邀请邮箱、勾选确认后发送邀请；一次最多邀请 5 个邮箱。

## Requirements

- 管理员可以从 OpenAI OAuth 账号入口打开“Codex 邀请重置”功能。
- 本次 MVP 只做单个账号弹窗操作，不做扫描所有满额账号并批量自动重置。
- 功能仅允许用于 `platform=openai` 且 `type=oauth` 的账号；非 OpenAI OAuth 账号必须给出明确错误。
- 后端必须使用已保存账号凭证发起 ChatGPT backend 请求，不向前端返回 access token、refresh token、id token 或 `chatgpt_account_id` 原值。
- 后端查询 reset credit 时返回：
  - 可用 credit 数量。
  - credit 列表的非敏感状态信息，例如 `id`、`status`、`title`、`description`。
  - 账号标识的非敏感展示信息，例如账号 ID、名称、邮箱。
- 后端消耗 reset credit 时：
  - 只对明确传入的账号 ID 操作。
  - 默认消耗第一个可用 credit，允许传入指定 credit ID。
  - 若无可用 credit，返回可读错误，不发起 consume。
  - 不自动清理 Sub2API 本地 `rate_limited_at`、`rate_limit_reset_at`、Redis 调度缓存或 Codex usage 快照。
- 后端发送邀请时：
  - 邮箱支持逗号、空格、换行分隔。
  - 去重后一次最多 5 个邮箱。
  - 校验邮箱格式，失败时返回明确错误。
  - 必须由前端显式确认“已获得收件人同意”后才能提交。
- 前端弹窗需要展示：
  - 当前账号展示名/邮箱。
  - 可用重置次数、刷新状态、无可用机会提示。
  - 使用 reset credit 的按钮及处理中/成功/失败状态。
  - 邮箱输入框、最多 5 个邮箱提示、收件人同意确认框、发送邀请按钮及结果提示。
- 日志和错误提示必须避免泄露 token、cookie、完整认证头。

## Acceptance Criteria

- [ ] 管理员在账号页可对 OpenAI OAuth 账号打开 Codex 邀请重置弹窗。
- [ ] 查询 reset credit 成功时，前端展示可用次数和 credit 状态；失败时展示后端返回的可读错误。
- [ ] 无可用 reset credit 时，“使用重置次数”不可执行或执行后返回 `no_available_credit` 类错误。
- [ ] 有可用 reset credit 时，管理员点击使用后，后端向 `/wham/rate-limit-reset-credits/consume` 提交 `credit_id` 和新的 `redeem_request_id`。
- [ ] 消耗 reset credit 后不修改本地账号限流字段，除非未来另有独立需求明确要求清理。
- [ ] 邀请邮箱输入支持逗号、空格、换行分隔；重复邮箱只发送一次；超过 5 个或格式错误时不能提交。
- [ ] 未勾选收件人同意确认时不能发送邀请。
- [ ] 发送邀请成功后展示成功数量、失败邮箱和后端消息；失败时展示错误且不暴露敏感凭证。
- [ ] 后端单元测试覆盖非 OpenAI OAuth 拒绝、缺失 access token、credit 解析、无可用 credit、consume payload、invite 邮箱校验。
- [ ] 前端单元测试覆盖弹窗按钮状态、邮箱解析限制、确认框限制、API 成功/失败渲染。

## Out of Scope

- 不实现自动批量扫描所有满额账号并自动消耗 reset credit；后续如需批量能力，作为独立任务设计确认。
- 不实现定时任务自动重置。
- 不修改本地 Sub2API 限流/调度状态清理逻辑。
- 不持久化 ChatGPT backend 返回的 eligibility/rules 原始大对象，除非实现时发现前端必须展示。

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
