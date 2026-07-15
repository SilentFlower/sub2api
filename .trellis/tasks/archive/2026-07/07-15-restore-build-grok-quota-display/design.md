# 恢复 build 版 Grok 额度显示 - Design

## Architecture

账号列表的 Grok 额度拆成两条职责明确的链路：

```text
独立套餐额度链路
GrokBillingQuotaCell
  -> GET /admin/grok/accounts/:id/billing-quota
  -> GrokBillingQuotaService
  -> xai/billingquota 私有请求与解析
  -> extra.grok_billing_quota_snapshot
  -> AccountUsageInfo.grok_billing_quota

main 手动 quota 兼容链路
GrokQuotaProbeCell
  -> GET /admin/grok/accounts/:id/quota
  -> GrokQuotaService.QueryQuota
  -> billing/header/local usage 兼容结果
  -> 不写入、不回填独立套餐额度状态
```

共享边界仅包括 `AccountRepository`、`ProxyRepository`、`GrokTokenProvider` 与 `HTTPUpstream`。独立链路不得导入或转换 main 的 `xai.BillingSummary`。

## Backend Design

### 1. 独立解析包

新增 `backend/internal/pkg/xai/billingquota`，负责：

- 构造 weekly 与 monthly Billing URL。
- 应用 Grok CLI Billing 专用请求头。
- 解析 camelCase/snake_case 的 cents、period、product usage 与 on-demand 字段。
- 将两个响应合并为独立 `Snapshot`。
- 推导月剩余、月使用比例、按量付费剩余/比例和套餐标签。

独立快照字段：

```text
period_type
weekly_used_percent
weekly_reset_at
product_usage
monthly_limit_cents
monthly_used_cents
monthly_remaining_cents
monthly_used_percent
billing_period_start
billing_period_end
on_demand_cap_cents
on_demand_used_cents
on_demand_remaining_cents
on_demand_used_percent
plan_label
updated_at
stale
partial
failed_windows
```

未知字段保持缺失，不用零值表示未知。`SuperGrok` 与 `SuperGrok Heavy` 仅按已验证的月额度常量推导。

### 2. 独立服务

新增 `GrokBillingQuotaService`，注入与 main 相同的仓储、Token Provider 和 HTTP transport，但不依赖 `GrokQuotaService`：

1. 校验账号存在、`platform=grok` 且 `type=oauth`。
2. 通过 `GrokTokenProvider.GetAccessToken` 获取有效 Token。
3. 沿用账号代理；无代理时直连。
4. 在同一个短超时上下文中依次请求 weekly 和 monthly，保持前端“最多 2 个账号并发”时上游并发上限稳定。
5. 任一窗口成功且可形成快照时返回部分结果；两个窗口都失败或都无有效数据时返回稳定业务错误。
6. 只写 `extra.grok_billing_quota_snapshot`，不得读写 main 的 `extra.grok_billing_snapshot` 或 `extra.grok_usage_snapshot`。

上游错误正文只允许脱敏、截断后进入服务日志，不向前端返回 Token、Authorization 或完整响应。

### 3. API 与 usage 投影

- 新增 `GET /api/v1/admin/grok/accounts/:id/billing-quota`。
- handler 使用标准 response envelope 和 `response.ErrorFrom`。
- `UsageInfo` 新增 `grok_billing_quota`，账号 usage 仅从新快照键读取并按 30 分钟 TTL 标记 `stale`。
- 不修改数据库结构；两个快照均位于账号 `extra` JSON。
- main 的 `grok_billing` 字段和 `/quota` 返回结构继续保留，保证 API 兼容，但不作为账号列表套餐额度来源。

### 4. 禁止重复自动探测

- `AccountUsageService.getGrokUsage` 删除账号列表自动调用 main `ProbeBilling` 的分支。
- usage 获取只读取已有 main 快照、header 快照和独立套餐快照，不因页面加载触发 Responses 请求。
- main `/quota` 的 `QueryQuota` 与 `ProbeBilling` 保持可调用，仅由 `GrokQuotaProbeCell` 等显式手动动作触发。
- main 手动结果不得覆盖 `grok_billing_quota`。

## Frontend Design

### 1. 类型与 API

- 恢复独立 `GrokBillingQuota` 和 `GrokBillingProductUsage` 类型。
- `AccountUsageInfo` 新增 `grok_billing_quota?: GrokBillingQuota | null`。
- `adminAPI.grok.queryBillingQuota(id)` 只调用独立 `/billing-quota`。
- 所有 API 字段保持后端 snake_case。

### 2. 展示组件

恢复 `GrokBillingQuotaCell.vue`，但按当前组件规范重新接入：

- 仅 Grok OAuth 账号渲染。
- 默认紧凑态显示套餐、月剩余额度进度和可用的周额度进度。
- 展开态显示月金额、周重置时间、产品用量、按量付费、更新时间、过期和部分失败状态。
- 缺失字段隐藏对应行；`on_demand_cap_cents` 明确为 0 时才显示“未启用”，缺失时不猜测。
- 刷新失败保留最后一次成功快照，只在区块内显示错误。
- 复用 `UsageProgressBar`、现有格式化函数、i18n 与项目图标组件；不新增卡片嵌套或手绘 SVG。

### 3. 缓存与并发

- 模块级缓存按账号 ID 保存，TTL 为 30 分钟。
- 初始 props 快照未过期时不请求。
- 缺失或 stale 时自动进入模块队列，最多同时刷新 2 个账号。
- 手动刷新绕过 TTL，但同一组件 loading 时不重复提交。
- 组件卸载后不得写入已失效的本地 UI 状态。

### 4. 与 main UI 解耦

- `AccountUsageCell` 在 Grok OAuth 分支挂载独立组件并将 `updated` 快照合并到 `grok_billing_quota`。
- 删除当前 `grok_billing` weekly bar；免费 24 小时、header 请求/Token、本地 usage、reauth/forbidden 继续显示。
- 免费/付费展示判断优先使用独立快照的套餐和月额度，再回退账号 credential/entitlement，不依赖 main Billing UI。
- `handleGrokProbed` 不再把 `result.billing` 合并到账号列表 Billing 状态。
- `GrokQuotaProbeCell` 不展示 main Billing 明细；它只反馈 header、retry、entitlement 和手动探测错误。

## Error Matrix

| 条件 | 行为 |
| --- | --- |
| 非 Grok 或非 OAuth | `400`，不请求上游；前端不渲染组件 |
| Token 缺失或刷新失败 | `502` 稳定 reason；不清空旧独立快照 |
| weekly 成功、monthly 失败 | 保存 weekly，`partial=true`、`failed_windows=["monthly"]` |
| monthly 成功、weekly 失败 | 保存 monthly，`partial=true`、`failed_windows=["weekly"]` |
| 两个窗口都失败 | 返回映射后的上游错误，不覆盖旧快照 |
| Billing JSON 非法 | `502` 解析错误，不记录完整正文 |
| 独立快照过期 | usage 返回旧值并标记 `stale=true`，前端排队刷新 |
| main `/quota` 返回 Billing | 保持 API 响应，但账号列表不展示、不合并为独立快照 |

## Migration And Rollback

- 新键首次部署没有快照，前端会按队列低频补齐；不从 main 旧键猜测转换。
- 回滚前端组件不会影响 main `/quota`。
- 回滚独立 API 后，新 extra 键可安全保留为未使用 JSON；无需 schema migration。
- 当前 Grok 统一数据流规范需在完成实现后改写为“双链路隔离”契约。
