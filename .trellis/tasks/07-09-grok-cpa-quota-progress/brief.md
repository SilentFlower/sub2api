# Brief — 新增 Grok 套餐额度进度条

## Goal

- 在管理端账号列表的 Grok 账号用量区域新增独立“Grok 套餐额度”进度条，展示从 CLIProxyAPI 管理面板迁移来的 Grok CLI Billing subscription 数据，同时保持现有 Grok rate-limit header 额度逻辑不变。

## Scope

- 新增 Grok CLI Billing 查询链路，请求 `https://cli-chat-proxy.grok.com/v1/billing?format=credits` 和 `https://cli-chat-proxy.grok.com/v1/billing`。
- 使用 Grok OAuth access token 和现有 token 获取链路发起请求；如果该 Grok 账号已绑定代理，则沿用该代理，未绑定则直连。
- 新增独立缓存字段 `extra.grok_billing_snapshot` 和 Usage DTO 字段 `grok_billing_quota`。
- 前端新增独立“Grok 套餐额度”展示：月 credits 为主进度条，周 credits 有有效数据时显示附加小进度条；产品用量和按量付费状态与 CLIProxyAPI Management Center 对齐展示。
- 查询触发方式为缓存优先、低频懒刷新、支持主动刷新；刷新失败只影响套餐额度区块。

## Non-Goals

- 不重构现有 Grok quota snapshot、active probe、自动暂停或调度逻辑。
- 不修改或覆盖 `extra.grok_usage_snapshot`。
- 不改变现有 `/admin/grok/accounts/:id/quota` 主动 probe 行为。
- 不实现 Grok subscription quota reset。
- 不使用 xAI API Management Billing 替代 Grok CLI Billing。

## Key Context

- 现有 Grok 额度链路：`backend/internal/pkg/xai/quota.go` 解析 response headers，保存到 `extra.grok_usage_snapshot`。
- 现有 Usage 字段：`grok_request_quota`、`grok_token_quota`、`grok_quota_snapshot_state`、`grok_last_quota_probe_at` 等。
- 现有前端展示在 `frontend/src/components/account/AccountUsageCell.vue`，包含请求/Token 进度条和 `GrokQuotaProbeCell`。
- 新增链路必须保存非敏感展示字段，不能保存 access token、refresh token、Authorization、完整上游响应或非 allowlist 响应头。
- 单账号 TTL 建议 30 分钟，前端刷新队列限制同一时间 1-2 个外部 billing 请求。
- 相关规范：`.trellis/spec/backend/protocol-adapter-guidelines.md`、`.trellis/spec/backend/quality-guidelines.md`、`.trellis/spec/frontend/quality-guidelines.md`。

## Acceptance

- 现有 `grok_request_quota` / `grok_token_quota` 进度条、未知提示和主动 probe 行为保持不变。
- Grok OAuth 账号可显示独立“Grok 套餐额度”区块；月/周额度、产品用量和按量付费状态与 CLIProxyAPI Management Center 对齐；查询失败不影响旧 Grok 额度展示。
- 后端不把 Grok CLI Billing 数据写入 `grok_usage_snapshot`，使用独立 DTO/字段返回前端。
- 后端解析覆盖 monthly billing 与 `format=credits` billing 的成功、缺字段、失败状态。
- 前端在 mobile、desktop、dark mode 下具备 loading、empty、error、success 状态。
- 测试覆盖新增 API 字段、解析逻辑和用户可见展示。

## Next Step

- 用户确认 planning artifacts 和本 brief 后，运行 `task.py start .trellis/tasks/07-09-grok-cpa-quota-progress` 进入 in_progress；随后按 Trellis 路由进入实现。
