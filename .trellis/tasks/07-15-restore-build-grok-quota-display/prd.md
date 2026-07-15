# 恢复 build 版 Grok 额度显示

## Goal

恢复合并 main 前 build 分支中更完整、可扫描的 Grok 套餐额度展示，解决当前账号列表只显示单条周额度或回退窗口、无法直观看到月额度、套餐和产品用量的问题。

## Background

- build 提交 `57e409da` 新增了独立 `GrokBillingQuotaCell.vue`：展示套餐标签、月/周额度进度、产品用量、按量付费状态、刷新和展开详情。
- 同步 main 0.1.155 时，任务 `07-14-sync-main-0155-into-build` 选择 main 的统一 `QueryQuota` / `ProbeBilling` / `ProbeUsage` 数据流，并删除旧 `queryBillingQuota` API、`GrokBillingQuota` 类型和独立展示组件，理由是避免双请求、双缓存和重复状态。
- main 的统一额度探测链路仍在频繁调整。build 需要把 Grok 套餐额度恢复为独立业务链路，避免 main 后续修改 `/quota` 编排、探测回退或聚合模型时影响套餐额度展示。
- 当前后端并非缺少 Billing 数据。`xai.BillingSummary` 和前端 `GrokBillingSummary` 仍包含周使用比例、周窗口、月额度、月已用、产品用量、套餐、更新时间和部分失败状态。
- 当前 `AccountUsageCell.vue` 只把上述数据投影为一条周额度进度条，并搭配免费账号 24 小时估算或请求/Token header 回退；旧 build 的月额度主行、套餐标签、产品明细和展开详情没有对应展示。
- 旧组件的按量付费 cap/remaining 字段不在当前统一 `GrokBillingSummary` 中。若要求精确恢复该部分，需要扩展当前统一模型或恢复旧解析字段，不能在前端猜测。
- 中英文 `grokBilling*` 文案仍保留，可复用并按最终字段范围清理。

## Requirements

### R1. 展示能力

- Grok OAuth 账号在账号列表中重新显示独立、紧凑的“Grok 套餐额度”区块。
- 默认折叠态至少展示套餐标签以及可用的月/周额度进度；展开态展示可用的月额度金额、周窗口、产品用量、更新时间和部分失败状态。
- 保留当前 main 的免费账号 24 小时估算、请求/Token header 回退、本地 usage 和 entitlement/reauth 状态；详细 Billing 区块不能覆盖这些信息。
- 缺失某个窗口或字段时只隐藏对应行，不能把未知值显示为 0 或“未启用”。

### R2. 数据流边界

- 恢复 build 独立的 Grok 套餐额度业务链路，包括独立 `/admin/grok/accounts/:id/billing-quota` 路由、`queryBillingQuota` 前端 API、套餐额度 DTO、组件状态和缓存。
- 套餐额度展示不得依赖 main 的统一 `/admin/grok/accounts/:id/quota`、`GrokQuotaProbeResult` 或其 Billing/Responses 回退编排。
- Billing 上游请求、响应解析、快照聚合和持久化契约保持为 build 私有实现，不复用 main 的 `GrokBillingSummary` 聚合模型。
- 仅共享 Grok OAuth Token 获取、账号校验、代理解析和 `HTTPUpstream` 等通用基础设施，避免复制鉴权与网络传输能力。
- 当前 main 的统一 `/quota` 链路继续服务免费账号 24 小时估算、请求/Token header 回退、本地 usage、entitlement/reauth 等现有展示；两条链路互不覆盖状态。
- 账号列表自动加载套餐额度时只访问 Billing 上游，不得触发会消耗模型额度的 Responses probe。
- 独立链路需要有自己的并发限制和 TTL 缓存，避免账号列表展开或刷新造成 Billing 请求风暴。
- 账号列表只允许独立 `/billing-quota` 链路自动刷新和展示 Billing；当前 main 的 `ProbeBilling` 不再由账号 usage 自动刷新触发，`grok_billing` 也不再作为账号列表 Billing 展示来源。
- main `/quota` API 继续保留手动探测与兼容能力，可以在其内部使用 Billing 判断是否需要 Responses probe，但其结果不得写入或覆盖独立套餐额度状态。

### R3. 兼容与质量

- 独立套餐额度 DTO 使用 snake_case，并明确区分于 main 的 `GrokBillingSummary` / `GrokQuotaProbeResult` 契约。
- 展示组件支持 desktop/mobile、dark mode、loading、empty、partial 和 error 状态。
- 复用现有 `UsageProgressBar`、格式化工具和 i18n；恢复独立缓存和并发控制时需有明确失效、强制刷新和错误回退行为。
- 不改变 Grok 调度、计费、模型探测或账号选择逻辑。

## Acceptance Criteria

- [ ] 有月额度数据时，账号列表可直接看到月额度使用比例/剩余口径和套餐信息。
- [ ] 有周额度、产品用量和按量付费数据时，可在同一区块展开查看，字段与独立套餐额度 DTO 一致。
- [ ] 只有免费 24 小时估算或 header quota 时，当前 main 回退展示保持正常。
- [ ] 套餐额度的自动加载和手动刷新只调用独立 `/billing-quota` API，不依赖统一 `/quota` 的返回结构或探测分支。
- [ ] 套餐额度与 main 现有 quota/usage 状态相互独立；任一链路失败都不会清空或伪造另一条链路的数据。
- [ ] 账号列表首次加载或缓存过期时只发起一组独立 weekly/monthly Billing 请求，不会同时触发 main `ProbeBilling` 形成四个上游 GET。
- [ ] main `/quota` 手动探测接口继续可用，且独立套餐额度刷新不会触发 Responses 模型请求。
- [ ] 独立缓存命中、过期、强制刷新和并发限制有组件或服务测试覆盖。
- [ ] 缺失、部分失败和过期数据不会伪装为 0、100% 或“未启用”。
- [ ] Grok 相关组件测试、前端 typecheck/lint 和后端既有 Grok quota 测试通过。

## Out of Scope

- 修改 main 统一 `/quota` 的后端 Billing/Responses 探测编排或返回结构。
- 修改 xAI Billing 上游鉴权、本地 usage 统计、账号调度或计费逻辑。
- 调整 Grok 免费额度 2M Token 估算规则、账号调度或计费价格。

## Confirmed Decision

- 不是只恢复 UI；恢复 build 独立的 Grok 套餐额度业务链路，隔离 main 统一 quota 链路的频繁变化。
- 隔离边界按合并 main 前的 build 实现确定：Billing 请求、解析、DTO、快照、接口、缓存和 UI 独立；OAuth Token、账号校验、代理及 HTTP 传输基础设施共享。
- 独立链路是账号列表唯一的自动 Billing 刷新和 Billing 展示来源；main `/quota` 仅保留手动探测与兼容用途，避免两套链路在页面加载时重复请求 Billing 上游。
