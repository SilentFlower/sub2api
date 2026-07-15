# Brief — 恢复 build 版 Grok 额度显示

## Goal

- 恢复合并 main 前 build 的完整 Grok 套餐额度展示，并把套餐 Billing 与 main 统一 quota 链路隔离，避免 main 频繁调整影响账号列表。

## Scope

- 恢复独立 `GET /admin/grok/accounts/:id/billing-quota`、独立服务、DTO、解析、快照、前端 API 与 `GrokBillingQuotaCell`。
- 独立链路请求 weekly/monthly Grok CLI Billing，展示套餐、月/周额度、产品用量、按量付费、更新时间、过期和部分失败状态。
- 使用新快照键 `extra.grok_billing_quota_snapshot` 和 usage 字段 `grok_billing_quota`，不复用 main 的 `grok_billing_snapshot` / `grok_billing`。
- 账号列表只由独立链路自动刷新和展示 Billing；取消 account usage 对 main `ProbeBilling` 的自动调用，避免一次加载形成两套 Billing 请求。
- main `/quota` 保留原后端编排、返回结构和手动探测兼容，但它的 Billing 结果不再进入账号列表 Billing 展示或独立状态。
- 保留免费 24h、request/token header、本地 usage、entitlement、reauth/forbidden 等现有 Grok 信息。
- 独立组件使用 30 分钟缓存、最多 2 个账号并发，并支持手动强制刷新。

## Non-Goals

- 不修改 main `/quota` 后端的 Billing/Responses 探测编排或返回结构。
- 不修改 Grok 调度、账号选择、计费、本地 usage 统计和免费 2M Token 估算规则。
- 不迁移或猜测当前 `grok_billing_snapshot` 的结构；新链路首次刷新后写入新键。
- 不复制 OAuth Token、账号仓储、代理仓储或 HTTP transport 基础设施。

## Key Context

- 历史来源是 build 提交 `57e409da`；原实现业务链路独立，但共享 Token、代理和 HTTP 基础设施。
- 当前 main 已占用旧 `grok_billing_snapshot` 键并自动运行 `ProbeBilling`，直接恢复旧代码会发生结构冲突和最多 4 个上游 GET。
- 新解析放入专用 `internal/pkg/xai/billingquota`，不得引用或转换 main `xai.BillingSummary`。
- 新服务只写 `grok_billing_quota_snapshot`；任一链路失败不得清空或覆盖另一条链路状态。
- 当前协议规范仍禁止独立 Billing 链路，完成实现后必须更新为本任务确认的双链路隔离契约。
- 管理端 API 必须使用标准 envelope；日志和错误不得暴露 Token、Authorization 或完整上游正文。

## Acceptance

- 月额度、套餐、周额度、产品用量和按量付费按实际可用字段正确展示，未知值不伪装为 0、100% 或“未启用”。
- 自动与手动套餐刷新只调用独立 `/billing-quota`，不触发 Responses 模型请求。
- 账号列表首次加载或缓存过期时只发起一组 weekly/monthly Billing 请求，不同时触发 main `ProbeBilling`。
- main `/quota` 手动探测继续可用，独立和 main 状态互不覆盖。
- 独立缓存命中、过期、强刷、并发限制、部分失败和错误保留旧值均有测试。
- Grok 后端单测、前端组件/API 测试、typecheck、lint 和 `git diff --check` 通过。

## Next Step

- 用户确认 planning artifacts 与本 brief 后，运行 `task.py start`，再通过 `trellis-route(implement)` 进入实现。
