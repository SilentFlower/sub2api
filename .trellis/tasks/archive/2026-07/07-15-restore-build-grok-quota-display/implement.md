# 恢复 build 版 Grok 额度显示 - Implement

## Phase 1: 独立 Billing 领域模型

- 新增 `internal/pkg/xai/billingquota`，从合并前 build 行为恢复专用 payload、snapshot、URL、header、解析和合并逻辑。
- 使用独立命名，禁止引用 main `xai.BillingSummary`、`BuildBillingSummary` 或 `MergeBillingProbeResult`。
- 覆盖 camelCase/snake_case、周/月、产品用量、on-demand、剩余值、套餐推导、缺字段、非法 JSON 和部分成功测试。

## Phase 2: 服务、快照与 API

- 新增 `GrokBillingQuotaService` 及 wire provider，共享账号仓储、代理仓储、`GrokTokenProvider` 和 `HTTPUpstream`。
- 实现 weekly/monthly 请求、短超时、代理、错误脱敏与独立快照持久化。
- 使用新键 `grok_billing_quota_snapshot`，断言不修改 `grok_billing_snapshot` 和 `grok_usage_snapshot`。
- 为 `GrokOAuthHandler` 注入独立服务，新增 `QueryBillingQuota` 和管理端 GET 路由。
- 更新 wire 生成代码，并补 handler、route、service 测试。

## Phase 3: 账号 usage 解耦

- `UsageInfo` 增加 `grok_billing_quota`，从独立 extra 快照读取并标记 30 分钟 stale。
- 移除 `getGrokUsage` 的 main `ProbeBilling` 自动调用，保留 main `/quota` 手动入口。
- 保持 header quota、本地 usage、reauth/forbidden 和现有 main API 字段兼容。
- 测试账号 usage 首次加载、缓存过期和 force 模式都不会自动调用 main Billing 或 Responses probe。

## Phase 4: 前端独立组件

- 恢复独立 TypeScript DTO、`queryBillingQuota` API 和 `AccountUsageInfo.grok_billing_quota`。
- 恢复 `GrokBillingQuotaCell.vue`，实现紧凑/展开、loading/empty/error/stale/partial、刷新、30 分钟缓存和最多 2 个账号并发。
- 使用当前 `UsageProgressBar`、格式化工具、图标组件和现有中英文 `grokBilling*` 文案；只补缺失 key。
- 新增组件测试，覆盖新鲜缓存不请求、过期自动刷新、手动强刷、错误保留旧值、字段缺失和并发上限。

## Phase 5: 账号列表集成

- 在 Grok OAuth 区块挂载独立组件并合并 `updated` 事件。
- 删除 main `grok_billing` weekly bar 和 Billing 展示判断，独立快照成为唯一套餐展示来源。
- 保留免费 24h、header 请求/Token、本地 usage、entitlement、reauth/forbidden。
- `handleGrokProbed` 和 `GrokQuotaProbeCell` 不再展示或合并 main Billing 明细。
- 更新 `AccountUsageCell`、`GrokQuotaProbeCell` 与 API 测试，断言页面自动加载只调用独立接口。

## Phase 6: 规范与验证

- 更新 `.trellis/spec/backend/protocol-adapter-guidelines.md`，将“统一 Billing”旧决策改为独立套餐链路与 main 手动 quota 并存的契约。
- 运行格式化、目标单测、前端 typecheck/lint 和 diff 检查。
- 复核 mobile、desktop、dark mode 以及长套餐名/金额不溢出。

## Validation Commands

```bash
cd backend
gofmt -w <本任务修改的 Go 文件>
go test -tags=unit ./internal/pkg/xai/billingquota ./internal/service ./internal/handler/admin ./internal/server/routes -run 'Grok|BillingQuota|AccountUsage' -count=1
go test -tags=unit ./internal/pkg/xai ./internal/service ./internal/handler/admin ./internal/server/routes -run 'Grok|Billing|Quota' -count=1

cd ../frontend
pnpm vitest run src/api/__tests__/admin.grok.spec.ts src/components/account/__tests__/GrokBillingQuotaCell.spec.ts src/components/account/__tests__/GrokQuotaProbeCell.spec.ts src/components/account/__tests__/AccountUsageCell.spec.ts
pnpm typecheck
pnpm lint:check

cd ..
git diff --check
```

## Completion Conditions

- 账号列表缓存过期时只出现一组独立 weekly/monthly Billing 请求。
- 独立链路失败不会清空 main quota/header 状态，main `/quota` 失败也不会清空独立套餐快照。
- 新旧 extra 键和前端字段没有交叉写入。
- PRD 验收项、后端错误矩阵、前端状态测试和更新后的项目规范全部通过。
