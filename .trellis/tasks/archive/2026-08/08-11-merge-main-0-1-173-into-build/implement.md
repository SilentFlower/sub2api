# Implementation Plan: 合并 main 0.1.173 到 build 并处理冲突

## 1. 现场保护与基线确认

1. 确认当前分支仍为 `build`，`MERGE_HEAD` 仍为 `main@0b3fe95af`，不重跑 merge。
2. 记录 `git ls-files -u`、冲突文件列表和双方修改交集，作为最终复核基线。
3. 对任务外已有改动只读不改，不回滚用户或其它任务的工作区内容。

## 2. 后端设置契约

1. 合并 `domain_constants.go`、Settings DTO、service 结构和 defaults，建立双方字段并集。
2. 合并 parse/update/audit/view/runtime cache，确保每个字段在读取、写入、缓存失效和审计中闭环。
3. 明确排除 `openai_codex_client_version_synced` 的面板写回路径。
4. 合并两类 setting handler 测试，增加 synced 值不被普通更新覆盖的回归。

## 3. Codex 身份架构迁移

1. 对照 `main` 动态 identity resolver 的公共函数、SettingService 依赖和同步服务，确认所有 HTTP/WS/passthrough/probe 调用方。
2. 删除 `build` 静态 `openai_codex_client_identity.go` 的重复定义，将仍需要的常量或 helper 消费方迁移到 `main` owner。
3. 更新 passthrough 与 OAuth 测试，覆盖管理员覆写、自动同步、内置回退和现有身份禁用开关。
4. 全局搜索重复的 `codexCLIVersion`、UA 常量和 probe version，确保只有一个版本解析源。

## 4. OpenAI/Grok 网关冲突

1. 在 Responses ingress 中先接入 `main` tool null-type sanitizer，再执行 `build` 按最终模型决策的 Lite policy。
2. 保留最终 Lite Header 收口、`main` routing hint 和日志，补 managed/passthrough 的 allow/block 模型测试。
3. Messages 路由先启动上游响应模型观测，再使用统一 helper 判断 OpenAI API Key 与 Grok force-chat。
4. Chat fallback 保留直接 Anthropic -> Chat bridge、强制上游 stream 和本地折叠；按 reasoning policy -> Fast Policy 顺序处理最终 body。
5. 合并 HTTP、WS bridge 和 Grok result 字段，统一最终 reasoning effort、UpstreamResponseModel、ResponseID、搜索/图片计数和计费传播。
6. 运行 OpenAI/Grok 定向测试，在编译问题扩散到 Wire 之前稳定网关签名和数据结构。

## 5. Grok 双链路与账号用量

1. 在 `UsageInfo` 中同时保留 `GrokBillingQuota`、`GrokBilling`、`SevenDay`、`ThirtyDay` 及本地用量字段。
2. 保留 `main` Billing 与本地窗口的 API 兼容数据，但不把 `billing.Plan` 投影到 `SubscriptionTier`/`SubscriptionTierRaw`，也不写入或覆盖 `grok_billing_quota_snapshot`。
3. 从 `AccountUsageService` 移除 `GrokQuotaService` 注入和主动 `ProbeBilling`，保留只读快照与本地窗口投影。
4. 保留 `GrokQuotaService` 的手动/reconciler/Responses 入口和 `GrokBillingQuotaService` 的独立 `/billing-quota` 入口。
5. 保留导入探测超时注释，确认成功探测的 forbidden/error 清理条件未回退。
6. 补齐双链路测试：账号 usage 不触发主 Billing、两个快照键互不改写、主探测和独立刷新的错误状态互不覆盖。

## 6. 账号编辑与前端 API

1. 合并 `account.go` 的 import 和 extra 保留逻辑，对照实体/DTO 确认每个字段名。
2. 合并 `accounts.ts`、`grok.ts`、`settings.ts` 的导出和类型，保持 snake_case payload 与后端一致。
3. `EditAccountModal` 使用原 `extra` 副本逐 key 更新，保留 build compatibility helper，将 main 通用 billing probe 移到所有支持的 API Key 平台分支。
4. `BulkEditAccountModal` 同时保留 Codex 自定义 UA 和 main 通用 probe 能力判定。
5. 运行账号编辑、批量编辑和 API 定向测试，确认未知 extra、快照、邮箱和限额字段不被覆盖。

## 7. 前端 Grok 投影与 Settings UI

1. `AccountUsageCell` 以独立 `GrokBillingQuotaCell` 为套餐标签、周/月额度、产品用量与按量付费的唯一 owner；不从 main Billing 渲染第二套 UI。
2. `GrokQuotaProbeCell` 保留请求/Token/retry/entitlement 详情，只在不与独立 Billing UI 重复时展示 main 摘要。
3. 主探测 `persisted=true` 时重载服务端 usage；`persisted=false` 时仅在当前 cell 临时合并返回值。独立 Billing 刷新只合并 `grok_billing_quota`。
4. 重建 `AccountUsageCell.spec.ts` 的显示优先级和刷新矩阵，断言 main Billing plan/周月数据不会生成套餐 UI，缺失独立快照时只回退被动 header、免费 24h、credentials 与 entitlement。
5. `SettingsView` 保留 build 生图/Lite 组件，接入 main Codex 版本字段，不重复声明表单状态。
6. 合并中英文 locale，检查 feature spread 后的最终运行时值，保证 Codex 新架构语义不被旧 override 覆盖。

## 8. Wire 收口与生成

1. 统一 `NewAccountUsageService`、`ProvideAccountUsageService`、`ProvideAccountTestService`、`ProvideGrokQuotaService` 及所有测试构造调用。
2. `ProvideGrokQuotaService` 吸收 `main` 的 `SettingService`、config 和 usage log 依赖；`ProvideAccountUsageService` 不接收 `GrokQuotaService`。
3. ProviderSet 同时注册主额度、独立 Billing Quota、Codex Reset、Codex 版本同步和 main 新增服务。
4. 在源依赖图编译成功后执行 `cd backend && go generate ./cmd/server`，将冲突的 `wire_gen.go` 完全替换为生成结果。
5. 再次执行生成并比较 diff，确认生成可重复。

## 9. 格式化、定向验证与全量检查

1. 对所有解冲突的 Go 文件执行 `gofmt`，前端按现有 formatter/lint 规则处理。
2. 后端定向测试至少覆盖 Settings、Codex identity、Responses Lite、Messages fallback、WS bridge、Grok quota/billing、AccountUsage 和 Wire 构造。
3. 前端定向测试至少覆盖 admin Grok API、AccountUsageCell、编辑/批量编辑与 SettingsView 数据往返。
4. 执行后端全量 `go test -tags=unit ./...` 和项目现有静态检查。
5. 执行前端 `pnpm lint:check`、`pnpm typecheck`、`pnpm test:run`、`pnpm build`。
6. 若发现失败，回到对应 owner 修正，不通过放宽断言或删除不相关测试规避问题。

## 10. Git 完整性与交付

1. 确认 `git ls-files -u` 和 `git diff --name-only --diff-filter=U` 无输出。
2. 搜索冲突 marker、重复 Codex 常量、重复 Grok 额度展示和失效 API export。
3. 执行 `git diff --check`，核对 `backend/cmd/server/VERSION=0.1.173` 和 `MERGE_HEAD=0b3fe95af`。
4. 重新生成双方修改文件交集，逐类复核自动合并的 locale、re-export、Wire、DTO 和网关热点。
5. 运行 `git merge-tree` 语义重审，将实际结果与 `design.md` 决策矩阵逐项对照。
6. 保持 merge 未提交状态，向用户报告解冲突、测试结果和剩余风险；commit/push 需后续明确确认。
