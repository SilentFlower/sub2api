# 同步 main 0.1.152 到 build 并保护定制特性

## Goal

将远端 `main` 的 0.1.152 增量合入 `build`，完整识别并解决文本冲突与语义冲突，同时保留 `build` 已有的 OpenAI、Codex、Anthropic、Grok、容灾编排和 Trellis 定制。合并结果必须通过与变更范围匹配的后端、前端和数据库迁移验证，并具有明确的回滚点。

## Background

- 规划阶段已执行 `git fetch --prune origin main build`，`origin/main` 从 `e316ebf5` 前进到 `a1930ea6`，版本为 0.1.152。
- 本地 `main` 仍指向 `e316ebf5`，相对 `origin/main` 可纯快进 40 个提交；规划阶段刻意未移动本地分支。
- 当前 `build` 与 `origin/build` 均指向 `f59991b5`，两个分支的共同基点为 `e316ebf5`。该基点已经通过上一轮 merge commit `0712a147` 合入 `build`。
- 当前分叉为 `build` 独有 100 个提交、`origin/main` 独有 40 个提交，不能使用 fast-forward 完成 `main -> build` 合并。
- `git merge-tree --write-tree --messages build origin/main` 的虚拟合并树为 `62c6d7bdb64a74cd5d67be64d1d76862d26afc4f`，预计给 `build` 带来 118 个文件变化、5973 行新增和 382 行删除。
- 双方共同修改 43 个文件，其中 12 个产生 Git 文本冲突，另外 31 个自动合并文件需要语义复核。
- 创建本任务前工作区已有 8 个未提交 Trellis 路径；它们不与虚拟合并结果直接重叠，但实际合并前仍需使用包含未跟踪文件的选择性 stash 隔离，merge commit 完成后再恢复。当前任务目录不属于这 8 个路径。
- 上一轮 0.1.151 合并任务确立的保护契约继续适用：不得整文件选择 `ours`/`theirs`；Codex 客户端版本保持单点定义；Anthropic Messages 直连 Chat Completions、Grok 路由与 quota、provider-specific reasoning effort、GPT-5.6 `max`、Codex 生图和 Fast/Flex 定制能力不得回退。

## Text Conflicts

1. `backend/internal/handler/endpoint.go`
2. `backend/internal/handler/openai_alpha_search.go`
3. `backend/internal/handler/openai_chat_completions.go`
4. `backend/internal/service/openai_alpha_search.go`
5. `backend/internal/service/openai_alpha_search_test.go`
6. `backend/internal/service/openai_gateway_chat_completions_raw.go`
7. `backend/internal/service/openai_gateway_grok.go`
8. `backend/internal/service/openai_gateway_grok_test.go`
9. `backend/internal/service/openai_gateway_messages_chat_fallback.go`
10. `backend/internal/service/openai_gateway_messages_chat_fallback_test.go`
11. `backend/internal/service/openai_gateway_service.go`
12. `frontend/src/components/account/__tests__/EditAccountModal.spec.ts`

## Requirements

- R1：实施前再次 fetch 并固定目标 `origin/main`、合并前 `build` 和 `origin/build` 提交；任一远端引用变化时必须重新生成冲突清单并更新任务材料。
- R2：实际合并前创建准确指向合并前 `build` HEAD 的备份分支，并隔离当前未提交 Trellis 文件，保证可恢复。
- R3：本地 `main` 只允许纯快进到已固定的 `origin/main`；使用非 fast-forward、验证前不提交的方式把 `main` 合入 `build`。
- R4：12 个文本冲突必须逐项记录双方意图和最终方案；31 个自动合并文件必须按协议、计费、身份、前端和迁移链路完成语义复核。
- R5：不得用整文件 `ours` 或 `theirs` 丢弃任一侧有效语义；优先吸收 `main` 最新结构和修复，并组合保留 `build` 定制能力。
- R6：Alpha Search 采用 `main` 的成功调用按次计费和 `OpenAIForwardResult` 返回契约，同时保留 `build` 的路由、失败切换与中文公共 API 文档。
- R7：实际上游端点判定采用 `result -> runtime context -> account/inbound fallback` 的优先级；Grok 原生 Chat/Responses 动态路由必须由 result 或 runtime context 记录，不能仅凭入站 `/chat/completions` 猜测。fallback 继续覆盖 Messages 显式强制 Chat 和 OpenAI API Key raw Chat。
- R8：Grok raw Chat 同时保留 `build` 基于最终上游请求体的 reasoning effort 归一化/计费语义，以及 `main` 的 cache identity、prompt cache 字段清理和 Composer capability 清理。
- R9：Anthropic Messages 直连 Chat fallback 保留 `build` 的强制上游 SSE、调试快照和 custom UA，并适配 `main` 新增的 `sendCCUpstreamRequest` cache identity 参数。
- R10：Codex 内置版本继续由 `openai_codex_client_identity.go` 单点定义；`openai_gateway_service.go` 只吸收实际上游端点 context key，不得重复声明 `codexCLIVersion`。
- R11：前端冲突中的 Grok Responses override 与 xAI 默认 Base URL 为独立行为，两个测试都必须保留。
- R12：解决后不得残留未合并索引项、冲突标记、重复常量、重复入口或旧调用签名；修改的 Go 文件必须通过 gofmt。
- R13：运行冲突相关定向测试、后端全量 unit、golangci-lint、前端 Vitest/typecheck/lint 和 migration 检查；所有失败必须修复或明确记录环境限制与剩余风险。
- R14（用户已确认）：验证通过后创建包含合并前 `build` 与固定目标 `main` 两个父提交的本地 merge commit，然后暂停并等待用户单独确认；未经确认不得推送 `origin/build`，禁止 force push。
- R15：原样合入 migration `174_group_web_search_price_per_call.sql`、Ent schema/生成代码、service/DTO/API cache 和前端分组价格字段；不得修改既有 migration，并验证 `web_search_price_per_call` 从数据库到计费和管理界面的跨层闭环。
- R16：同步更新协议规范中已过期的 Alpha Search 返回签名和按次计费契约，以及实际上游端点判定签名，避免实现与 `.trellis/spec/` 漂移。

## Acceptance Criteria

- [ ] 固定并记录目标 `origin/main`、合并前 `build`、`origin/build`、merge base 和备份分支，远端变化有重新规划记录。
- [ ] 12 个文本冲突全部按文件级方案解决；31 个自动合并文件完成语义复核，覆盖全部 43 个双方共同修改文件。
- [ ] `main` 0.1.152 的 Grok、Alpha Search 按次计费、Codex/Responses 修复和前端能力完整进入合并结果。
- [ ] `build` 的 Codex 身份、生图、Anthropic 直连、Grok effort/路由、GPT-5.6、容灾编排与 Trellis 定制仍有代码和测试闭环。
- [ ] Git 无未解决状态和冲突标记，Go 格式、重复定义和调用签名检查通过。
- [ ] 后端、前端和迁移验证全部通过，或明确记录无法执行的环境限制与剩余风险。
- [ ] 本地 merge commit 具有两个正确父提交；创建后停止在本地并展示提交、验证和远端状态，未经用户再次确认不执行 push。
- [ ] 合并前暂存的 8 个 Trellis 工作区路径在 merge commit 后恢复，内容不丢失且不混入 merge commit。
- [ ] migration 174、Ent 生成代码、分组价格 DTO/cache、Alpha Search usage 和前端管理字段形成可验证闭环；协议规范与最终函数签名一致。

## Out of Scope

- 不把 `build` 反向合入 `main`。
- 不删除 `build` 产品能力来降低冲突数量。
- 不重构与本次增量无关的协议适配、计费或前端架构。
- 不修改、删除或重命名已发布 migration。
- 不 force push，不改写已有远端历史。
