# 新增 Grok 套餐额度进度条 - Design

## Architecture

新增“Grok 套餐额度”链路，与现有 Grok rate-limit header 额度链路并行存在。

现有链路保持不变：

- 上游响应头解析：`backend/internal/pkg/xai/quota.go`
- 快照存储：`extra.grok_usage_snapshot`
- Usage DTO 字段：`grok_request_quota`、`grok_token_quota` 等
- 前端展示：`AccountUsageCell.vue` 中的请求/Token 进度条和 `GrokQuotaProbeCell`

新增链路使用独立字段和 API：

- 上游来源：
  - `GET https://cli-chat-proxy.grok.com/v1/billing?format=credits`
  - `GET https://cli-chat-proxy.grok.com/v1/billing`
- 后端缓存字段：`extra.grok_billing_snapshot`
- 后端 DTO 字段：`grok_billing_quota`
- 前端标题：`Grok 套餐额度`
- 前端组件：新增独立组件或在 `AccountUsageCell.vue` 中独立区块，不能复用旧 Grok 请求/Token quota bar 的数据。

## Data Flow

1. 管理端账号用量接口 `GET /api/v1/admin/accounts/:id/usage` 返回现有 Grok 用量信息，并附带缓存中的 `grok_billing_quota`。
2. 前端账号行优先展示缓存快照；无缓存时显示轻量 empty 状态。
3. 如果账号行进入视口且缓存缺失或超过 TTL，前端通过独立队列触发后台刷新。
4. 管理员点击刷新图标时，前端强制刷新当前账号。
5. 后端刷新接口获取 Grok OAuth access token；如果该 Grok 账号已绑定代理，则沿用该代理请求两个 Grok CLI billing URL，未绑定代理则直连；然后解析并合并结果。
6. 成功后保存到 `extra.grok_billing_snapshot` 并返回；失败只返回该区块错误，不修改 `extra.grok_usage_snapshot`。

## Backend Contract

新增 snapshot 只保存非敏感展示字段：

- `period_type`
- `weekly_used_percent`
- `weekly_reset_at`
- `product_usage`
- `monthly_limit_cents`
- `monthly_used_cents`
- `monthly_remaining_cents`
- `monthly_used_percent`
- `billing_period_start`
- `billing_period_end`
- `on_demand_cap_cents`
- `on_demand_used_cents`
- `on_demand_remaining_cents`
- `on_demand_used_percent`
- `plan_label`
- `updated_at`
- `stale`

不保存：

- access token
- refresh token
- Authorization
- 完整上游响应
- 非 allowlist 响应头

## Frontend Contract

`AccountUsageInfo` 新增可选字段：

```ts
grok_billing_quota?: GrokBillingQuota | null
```

独立刷新 API 返回同一 DTO 或包装结果：

```ts
GET /api/v1/admin/grok/accounts/:id/billing-quota
```

前端展示规则：

- 标题为“Grok 套餐额度”。
- 月 credits 是主进度条，显示剩余百分比、剩余额度/总额度和重置时间。
- 周 credits 有有效数据时显示为附加小进度条。
- 如果返回产品用量，则显示产品用量行。
- 如果返回 `on_demand_cap_cents > 0`，显示按量付费进度/剩余额度；否则显示按量付费未启用状态。
- 缓存过期时仍显示旧值，但标记更新时间/已过期。
- 刷新失败只显示该区块错误，不覆盖旧值。

## Compatibility

- 不迁移或改写已有 `extra.grok_usage_snapshot`。
- 不改变现有 `/admin/grok/accounts/:id/quota` 主动 probe 行为。
- 不改变 Grok 调度、自动暂停、上游错误处理和 token 刷新策略。
- 非 Grok OAuth 账号不显示该区块，也不触发刷新。

## Risk Controls

- 单账号 TTL 默认建议 30 分钟。
- 前端刷新队列限制同一时间 1-2 个外部 billing 请求。
- 后端请求超时沿用 Grok quota probe 级别的短超时。
- 错误日志必须脱敏并截断上游 body。
