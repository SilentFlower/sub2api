# Brief — 合并 main 0.1.173 到 build 并处理冲突

## Goal

- 将 `main@0b3fe95af` 的 `0.1.173` 完整合并到 `build`，逐项解决 36 个冲突文件和自动合并的语义风险，交付保留有效 `build` 能力、吸收 `main` 新架构且通过跨层验证的未提交 merge 结果。

## Scope

- 合并后端 Settings DTO/Service/handler/audit/runtime cache，同时支持 `build` 生图与 Responses Lite 设置、`main` Codex 版本覆写/自动同步/只读同步值。
- 合并 OpenAI Responses、Anthropic Messages -> Chat fallback、passthrough、WebSocket bridge 和 Grok 转发行为，统一最终 reasoning effort、响应模型、ResponseID、搜索/图片计数和计费字段。
- 将 Codex 身份收敛到 `main` 动态 resolver 和版本同步服务，删除 `build` 重复的静态常量源，迁移仍有效的回归测试。
- 保留 `main` Grok 主额度/主动探测与 `build` 独立 Billing Quota 两条不同数据链路；独立快照是账号列表套餐标签、周/月额度、产品用量和按量付费的唯一 owner。
- 合并账号编辑与批量编辑的 `extra` 保留逻辑、OpenAI compatibility helper、Codex 自定义 UA 和通用 upstream billing probe。
- 合并 SettingsView、Grok 用量展示、中英文 locale 与 API export，避免重复额度展示和 locale spread 静默覆盖。
- 修复源 Provider/constructor 后重新生成 `wire_gen.go`，完成后端、前端、Git 冲突完整性和自动合并语义复核。

## Non-Goals

- 不拆分或重建 worktree/分支历史，不重跑当前 merge。
- 不处理 `07-18-websearch-settings-thin-layer`、`07-20-antigravity-gif-frames` 等其它进行中任务。
- 不进行无关重构、历史 migration 改写或全局代码风格整理。
- 不自动创建 merge commit、push、部署或上线。

## Key Decisions

- 每个冲突对照基线、双分支、调用方和测试单独判断；只有职责、数据源或消费者确实独立时才双保留，不使用统一 `ours`/`theirs`/“能力并集”规则。
- Codex 身份采用 `main` 动态版本 resolver 与现有回滚开关作为唯一 owner；删除 `build` 静态 identity 常量源，不新增兼容开关。
- Responses ingress 先执行 `main` tool null-type 修复，再执行 `build` 按最终模型决定的 Lite 归一化和 Header 收口，最后保留 `main` routing hint 与诊断。
- Anthropic Messages 路由使用覆盖 OpenAI API Key 与 Grok `force_chat_completions` 的统一 helper；只保留直接 Chat bridge，按 reasoning policy -> Fast Policy 顺序处理最终 body。
- Grok 主额度与独立 Billing Quota 因 API、快照键和 UI 消费者不同而同时保留；两者不互相触发、写快照或覆盖错误状态。
- `AccountUsageService` 不注入 `GrokQuotaService`、不主动 `ProbeBilling`；主额度手动/reconciler/Responses 入口和独立 `/billing-quota` 入口各自保留。
- 独立 `GrokBillingQuotaCell` 是套餐类 UI 的唯一 owner；账号列表不从 `main` Billing plan/SevenDay/ThirtyDay 渲染套餐标签、周/月进度、预付或按量付费。缺失独立快照时只回退被动 header、免费 24h、credentials 和 entitlement。
- Wire 同时注册所需 service，但按业务边界注入消费方；`wire_gen.go` 只由源 Provider/constructor 稳定后重新生成。

## Key Context

- 当前分支为 `build`，合并前 HEAD 为 `222cf5aeb`；合并来源为 `main@0b3fe95af`，共同基线为 `682c4fe0e`。
- 当前处于未提交 merge 状态，已有 36 个冲突文件、52 个冲突块；后端 Settings/OpenAI/Grok/Wire 与前端 API/类型/组件必须整体闭环。
- 高风险 owner 包括 `backend/internal/service/openai_codex_identity.go`、`openai_gateway_*`、`account_usage_service.go`、`grok_quota_service.go`、`wire.go`、`frontend/src/components/account/AccountUsageCell.vue` 和 Settings locale feature spread。
- Build 私有业务规则应位于具名领域 owner；共享 gateway、SettingsView、locale 和 Wire 只保留不可避免的中央接入。
- Go 公开 API 需保持中文 Javadoc 式注释，前后端 JSON 字段使用精确 snake_case，前端使用 pnpm，Go 生成文件不手工塑形。

## Risks / Deferred

- Git 已在一个未提交 merge 现场中，需避免重跑 merge、误改其它进行中任务或把用户已有改动当成可回滚内容。
- 自动合并可能在 Codex 重复常量、Wire 签名、locale spread、API re-export 和 Grok 重复 UI 中造成无 marker 的语义回退，必须单独复核。
- 当前 Codex 身份规范仍记录旧的 `0.144.1` 静态常量契约，相对 `main 0.1.173` 已过期；实施以当前 `main` 动态架构为准，完成后更新规范。
- 删除静态 Codex identity owner 和拒绝 `AccountUsageService -> GrokQuotaService` 注入都会影响构造器与测试签名，需在 Wire 生成前全局搜索消费方。
- 提交、推送和部署明确延后，不在本任务实施阶段自动执行。

## Acceptance

- `git ls-files -u` 和 `git diff --name-only --diff-filter=U` 无输出，仓库无冲突 marker、重复 Codex 常量、失效 import/export 或被静默覆盖的最终文案。
- `backend/cmd/server/VERSION` 为 `0.1.173`，`MERGE_HEAD` 仍对应 `0b3fe95af`，且未误提交或推送。
- 设置字段从 SettingService/数据库到 handler DTO、前端 API、SettingsView 加载与提交完整往返，synced Codex 版本不被面板更新覆盖。
- Codex UA、originator、version 和自动同步值同源，默认、覆写、回退和禁用路径有回归覆盖。
- OpenAI Responses、Messages、passthrough、WebSocket 和 Grok 路径的最终 reasoning effort、响应模型、Fast Policy 与计费字段符合设计顺序。
- Grok 主额度与独立 Billing Quota 快照互不改写，账号 usage 不隐式触发主 Billing；账号列表不从 main Billing plan/周月数据生成套餐 UI，免费/付费/陈旧 forbidden 清理和独立 Billing 展示有回归覆盖。
- `cd backend && go generate ./cmd/server` 可重复生成，后端定向测试、`go test -tags=unit ./...` 和现有静态检查通过。
- 前端 `pnpm lint:check`、`pnpm typecheck`、`pnpm test:run`、`pnpm build` 通过，并完成双方修改交集与 `git merge-tree` 语义重审。

## Next Step

- 在用户确认本次规范对齐后的 Brief 后，继续当前 `in_progress` 任务，先从后端 Settings 契约和 Codex identity owner 收敛开始解冲突。
