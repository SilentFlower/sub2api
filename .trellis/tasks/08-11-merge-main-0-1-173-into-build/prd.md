# 合并 main 0.1.173 到 build 并处理冲突

## Goal

将 `main` 的 `0.1.173` 代码（`0b3fe95af`）完整合并到 `build`，在吸收上游功能、修复和数据结构变化的同时，保留 `build` 已有私有能力，消除文本冲突与语义回退，并形成可验证、可提交的合并结果。

## Background

- 目标分支：`build`，合并前提交为 `222cf5aeb`，后端版本为 `0.1.169`。
- 合并来源：`main@0b3fe95af`，后端版本为 `0.1.173`。
- 共同基线：`682c4fe0e`（2026-07-31）。
- 分支差异：`build` 有 221 个独有提交，`main` 有 279 个独有提交。
- 已执行 `git merge --no-commit --no-ff main`，当前 merge 尚未提交。
- 当前有 36 个冲突文件、52 个冲突块，集中在设置体系、OpenAI/Grok 网关、账户配额、Wire 依赖注入和前端管理界面。
- 该工作共享一个 Git merge 状态，后端 DTO/Service/Wire 与前端 API/类型/组件必须整体闭环，因此不拆分为独立子任务。

## Requirements

### R1. Build 私有能力隔离与保留

- 每个冲突必须对照共同基线、`build`、`main`、调用方和测试单独判断，禁止套用统一的 `ours`、`theirs` 或“双保留”规则。
- `main` 已经完整替代、修复或统一某项 `build` 实现时，采用 `main` owner，并迁移仍然有效的 `build` 测试或约束；不为保留历史形态继续维护重复实现。
- 两侧能力职责独立、数据来源不同或面向不同消费者时，才同时保留，并明确主路径、回退路径和唯一状态源。
- 只有运行时确实需要两种行为且现有配置无法表达时才新增兼容开关；不得为了降低冲突处理难度默认增加开关。
- 需要重点逐项评估的 `build` 能力包括 OpenAI 生图设置、Responses Lite 策略、Grok 独立 Billing Quota、Codex Reset、Fast Policy 和 Grok 路由覆盖。
- 遵循 Build 私有功能隔离指南：业务规则归属领域文件或前端 feature 目录，共享热点只保留不可拆分字段、稳定注册和薄调用。
- 对 `main` 已提供等价或更完整实现的能力，复用 `main` owner，不保留重复常量、重复状态源或仅用于保存历史形态的 wrapper。

### R2. 设置契约完整合并

- 后端设置 DTO、更新请求、默认值、解析、持久化、审计、缓存失效和返回视图必须同时支持：
  - `build` 的生图主模型、生图 reasoning effort、Responses Lite 屏蔽模型。
  - `main` 的 Codex 客户端版本、自动同步开关和已同步版本只读值。
- `openai_codex_client_version_synced` 只能由同步服务写入，后台保存设置不得覆盖或清空。
- 前端 API 类型、表单默认值、加载、提交、组件和中英文文案必须与后端 JSON 字段一致并完成往返。

### R3. OpenAI/Codex/Grok 网关行为合并

- Responses 请求必须同时保留工具 Schema 修复、Responses Lite 模型策略、Lite Header 收口、Routing Hint 和诊断日志。
- Anthropic Messages 路由必须覆盖 OpenAI API Key 与 Grok 显式 `force_chat_completions`，不得把探测字段误当作强制 Chat 信号。
- Reasoning effort、Fast Policy、Thinking fallback、上游响应模型审计和 usage 记录必须基于最终上游请求体保持一致。
- HTTP、passthrough、WebSocket、Responses/Chat fallback 入口的 Codex 身份和版本声明必须同源，不能出现重复常量或 UA/version 漂移。
- Grok 转发结果必须保留响应 ID、搜索/图片计数、reasoning effort 和计费所需字段。

### R4. Grok 配额与账户用量兼容

- 后端同时保留 `build` 的独立 `GrokBillingQuota` 快照和 `main` 的 `GrokBilling`、`SevenDay`、`ThirtyDay` 兼容字段，但 `main` Billing plan 不得投影到账号列表套餐状态。
- 主动探测、Billing 探测、被动快照和本地用量不得互相覆盖有效字段。
- 前端套餐标签、周/月额度、产品用量和按量付费只由独立 Billing Quota 快照展示；缺失时只回退到被动 header、免费 24h、credentials 和 entitlement，不使用 `main` Billing plan/周月进度填充套餐 UI。
- 探测成功后必须清理陈旧 forbidden/error 状态，并以服务端持久化结果完成最终刷新。
- Grok API 同时支持独立 Billing 查询、Capabilities、SSO Token 和密码授权入口。

### R5. 账户编辑与批量编辑兼容

- 编辑账号时只修改目标 `extra` 字段，保留 Grok 快照、邮箱、限额和其它未知字段。
- 保留 `build` 的 OpenAI compatibility helper、Codex 自定义 UA 控件和现有配额阈值设置。
- 吸收 `main` 将上游计费探测扩展到全部可支持 API Key 平台的行为。

### R6. 生成文件与依赖注入

- 先合并 Provider、constructor 和 `wire.go` 源定义，再重新生成 `wire_gen.go`，禁止手工塑形生成代码。
- Wire 最终同时注册 Grok 主额度服务、Grok 独立 Billing Quota 服务、SettingService、Codex Reset 和 Codex 版本同步服务，但不得为了“能力并集”把无关服务注入每个消费方。
- `AccountUsageService` 不注入 `GrokQuotaService`，账号用量查询只读已有快照；`main` 主额度和 `build` 独立 Billing Quota 各由明确入口触发。
- Ent 或其它生成文件只有在源 schema 需要调整时才重新生成，不手工修复生成文件差异。

### R7. 冲突清理与范围控制

- 清除全部 unmerged index、冲突标记、重复定义、失效导入和被静默覆盖的 locale/API export。
- 对 Git 自动合并的共享热点做语义复核，不能以“无冲突标记”代替行为正确性验证。
- 不进行与本次 `main -> build` 同步无关的重构，不修改已应用 migration 的历史内容。
- 本任务只把 merge 处理到可提交状态；提交和推送仍需后续明确授权。

## Acceptance Criteria

- [ ] `git diff --name-only --diff-filter=U` 无输出，仓库不存在 `<<<<<<<`、`=======`、`>>>>>>>` 冲突标记。
- [ ] `backend/cmd/server/VERSION` 为 `0.1.173`，`MERGE_HEAD` 对应 `main@0b3fe95af`，合并结果尚未误提交或推送。
- [ ] Build 私有功能仍由明确领域 owner 承载，共享 gateway、SettingsView 和 locale 只保留薄接入或必要中央字段。
- [ ] 每个行为型冲突在 `design.md` 中记录采用 `build`、采用 `main`、组合或删除旧实现的依据；不存在无证据的批量“双保留”。
- [ ] Codex UA、originator、version 和自动同步值同源，默认行为及回滚开关均有测试覆盖。
- [ ] OpenAI Responses、Messages、passthrough、WebSocket 和 Grok 路径的最终 reasoning effort、响应模型与计费字段一致。
- [ ] Grok 免费、付费、周/月 Billing、主动探测和陈旧 forbidden 清理场景均有后端或前端回归测试。
- [ ] 账号列表不从 `main` `grok_billing_snapshot.plan` 派生套餐等级，也不渲染第二套周/月 Billing UI。
- [ ] 设置字段从数据库/SettingService 到 handler DTO、前端 API 类型、表单加载与提交完成往返验证。
- [ ] `cd backend && go generate ./cmd/server` 可重复生成，`wire_gen.go` 无手工漂移。
- [ ] 后端定向测试、`go test -tags=unit ./...` 和必要的生成/静态检查通过。
- [ ] 前端 `pnpm lint:check`、`pnpm typecheck`、`pnpm test:run`、`pnpm build` 通过。
- [ ] 重新执行双方修改文件检查和 `git merge-tree`，未新增硬冲突或静默覆盖已知语义。

## Out Of Scope

- 将当前工作拆分到新 worktree 或重建分支历史。
- 重写无关模块、统一所有旧代码风格或顺带清理历史技术债。
- 改造现有数据库迁移编号或调整已经发布的 migration 内容。
- 处理 `07-18-websearch-settings-thin-layer`、`07-20-antigravity-gif-frames` 两个独立进行中任务。
- 自动创建合并提交、推送远端或部署上线。
