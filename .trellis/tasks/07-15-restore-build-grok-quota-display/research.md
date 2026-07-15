# 恢复 build 版 Grok 额度显示 - Research

## 历史实现

- build 提交 `57e409da` 首次引入独立 Grok CLI Billing 链路。
- 合并 main 前基线 `a9ad55b3817c416fcf24804308faa19097a3b244` 中，该链路包含：
  - `GET /api/v1/admin/grok/accounts/:id/billing-quota`。
  - 独立 `BillingPayload`、`BillingSnapshot`、`GrokBillingQuotaResult` 与前端 `GrokBillingQuota`。
  - weekly `/billing?format=credits` 与 monthly `/billing` 两次上游请求。
  - `extra.grok_billing_snapshot` 持久化、`grok_billing_quota` usage 投影。
  - `GrokBillingQuotaCell.vue` 的 30 分钟模块缓存、最多 2 个账号并发、自动与手动刷新。
- 原链路共享账号仓储、OAuth Token、代理解析和 `HTTPUpstream`，但 Billing 请求、解析、快照与 UI 不依赖 rate-limit header quota。

## 当前 main 状态

- 当前 main 已将 `extra.grok_billing_snapshot` 用于 `xai.BillingSummary`，通过统一 `/quota` 返回 `GrokQuotaProbeResult.billing`。
- `AccountUsageService.getGrokUsage` 会在缓存缺失或过期时自动调用 main `ProbeBilling`。
- `AccountUsageCell.vue` 会直接展示 `grok_billing` 的 weekly bar，并用它判断免费/付费窗口。
- 如果直接恢复旧组件，账号列表缓存过期时会同时触发两套 weekly/monthly 请求，最多形成 4 个 Billing 上游 GET。
- 当前 main `BillingSummary` 不包含旧 build 展示所需的 on-demand cap/used/remaining 字段，不能仅靠前端恢复原展示。

## 已确认边界

- 独立链路成为账号列表唯一自动 Billing 刷新和 Billing 展示来源。
- main `/quota` 保留手动探测兼容，但账号 usage 自动加载不再调用 main `ProbeBilling`，前端也不再展示或合并其 Billing 结果。
- 独立链路恢复自己的请求、解析、DTO、快照、接口、缓存和组件。
- 仅共享 OAuth Token、账号仓储、代理仓储和 `HTTPUpstream`。

## 兼容冲突与处理

| 冲突 | 处理 |
| --- | --- |
| main 已占用 `extra.grok_billing_snapshot` | 独立链路使用新键 `extra.grok_billing_quota_snapshot` |
| main 已定义 `xai.BillingPayload/BillingSummary` | 独立解析放入专用 `xai/billingquota` 包，避免同名和行为耦合 |
| main usage DTO 已有 `grok_billing` | 恢复并行字段 `grok_billing_quota`，两者不互相回填 |
| main `/quota` 仍会主动探测 Billing | 只保留手动 API；独立组件不调用 `/quota`，main 结果不进入 Billing UI |
| 存量独立快照曾使用旧键 | 不猜测旧键结构；首次独立刷新写入新键，无数据库迁移 |

## 规范漂移

- 当前 `.trellis/spec/backend/protocol-adapter-guidelines.md` 的“Grok 统一 Billing 与配额探测数据流”明确禁止独立 `/billing-quota`、`BillingSnapshot` 和组件。
- 该约束来自上次 main 同步决策，已被本任务的新产品决定替代。
- 完成实现后必须更新该场景，记录双链路的职责、不同快照键、触发规则和禁止重复自动探测的契约。
