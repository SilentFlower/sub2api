# 同步 main 0.1.151 到 build 并保护定制特性

## Goal

将当前 `main` 的 0.1.151 增量合入 `build`，完整识别并解决文本冲突与语义冲突，同时保留 `build` 已有的 OpenAI、Codex、Anthropic、Grok 定制能力。合并结果必须能够通过与变更范围匹配的后端、前端和数据库迁移验证。

## Background

- 当前分支为 `build`，创建任务前工作区干净，`build` 与当前本地记录的 `origin/build` 一致。
- 当前 `main` 与当前本地记录的 `origin/main` 均指向 `e316ebf5`，版本为 0.1.151。
- 两个分支的共同基点为 `9a2f11b4`。该提交已经包含上一轮 `main -> build` 整合，因此本次只处理其后的增量。
- 当前分叉为 `build` 独有 82 个提交、`main` 独有 31 个提交；不能使用 fast-forward。
- 上一轮同类任务已经确立以下保护契约：Codex 身份版本单点定义、Anthropic Messages 直连 Chat Completions、缓存稳定与确定性工具 ID、Grok 显式强制 Chat 与 quota、provider-specific reasoning effort、GPT-5.6 `max` 边界及 build 定制能力标记。
- `git merge-tree --write-tree --messages build main` 预演确认 4 个文本冲突：
  - `backend/internal/service/openai_gateway_grok.go`
  - `backend/internal/service/openai_gateway_grok_test.go`
  - `backend/internal/service/openai_gateway_messages_chat_fallback.go`
  - `backend/internal/service/openai_oauth_passthrough_test.go`
- 双方共同修改 25 个文件；除上述 4 个文本冲突外，还有 21 个自动合并文件需要语义复核，主要覆盖 apicompat 类型、Codex 生图、raw Chat、Messages/Responses fallback、Fast/Flex 设置、GPT-5.6 定价和管理端设置页。
- 新迁移 `173_allow_cyber_blocked_usage_request_type.sql` 紧接当前最高编号 172，不与现有文件同名；历史上的重复编号迁移保持不动。

## Requirements

- R1：实施前刷新并固定待合入的远端 `main` 提交；若远端头部变化，必须重新生成冲突清单和处理方案。
- R2：在实际合并前创建准确指向合并前 `build` HEAD 的备份分支，提供明确回滚点。
- R3：使用非 fast-forward、验证前不提交的方式执行合并，保留真实冲突现场以逐项处理。
- R4：列出所有 Git 文本冲突，并对双方共同修改但自动合并的高风险文件进行语义冲突复核。
- R5：冲突处理不得用整文件 `ours` 或 `theirs` 丢弃任一侧有效语义；优先以 `main` 的最新结构为基底，组合保留 `build` 定制功能。
- R6：保留上一轮合并固化的协议和业务契约，包括 Codex 统一身份、Anthropic 直连桥接、Grok 路由与额度、reasoning effort 记录、GPT-5.6 `max` 与定价能力。
- R7：吸收 `main` 0.1.151 的新增修复，包括 MCP/custom tools 桥接、Anthropic cache creation usage、Codex originator/UA 配对、用户级 Fast/Flex 策略、Grok effort、GPT-5.6 用量计费、生图 namespace 处理、setup-token 自动刷新和 writer nil 防护。
- R8：不得修改、删除或重命名已发布 migration；新增迁移只能按现有顺序原样合入，并验证 migration 编号与约束兼容性。
- R9：解决后不得残留未合并索引项、冲突标记、重复常量、重复入口或失效调用签名；修改的 Go 文件必须保持 gofmt。
- R10：运行协议适配、service、handler、repository、前端 typecheck 与受影响 Vitest；根据实际改动补充全量后端 unit、lint 和数据库迁移检查。
- R11（用户已确认）：四个文本冲突按以下方向处理：
  - Grok 结果继续从 provider 归一化后的最终请求体记录 effort，同时补齐 main 对兼容字段的覆盖测试；
  - Messages fallback 保留 `build` 的 Anthropic 直连 Chat Completions 实现和 custom UA 处理，不把 main 的旧 Responses 中转代码误插入 header builder；
  - OAuth passthrough 同时保留 GPT-5.6 当前 Codex 身份测试与 main 新增的 originator/User-Agent 配对测试。
- R12：新增的 `openai_codex_identity.go` 作为身份配对逻辑保留，并继续复用 `openai_codex_client_identity.go` 中唯一的版本与默认 UA 常量；不得复制 0.144.1 字面量到其它生产文件。
- R13（用户已确认）：验证通过后创建包含合并前 `build` 与目标 `origin/main` 两个父提交的 merge commit，并以普通模式直接推送到 `origin/build`；禁止 force push，不推送到其它分支。

## Acceptance Criteria

- [ ] 记录并固定本次合入的 `main` 提交和合并前 `build` 提交，备份分支指向准确。
- [ ] 4 个文本冲突均按 R11 组合处理并记录文件级原因、双方意图和最终方案；其余 21 个自动合并文件完成语义复核，合计覆盖全部 25 个共同修改文件。
- [ ] `main` 的 0.1.151 增量完整进入合并结果，`build` 的既有核心定制功能仍有代码和测试闭环。
- [ ] Git 无未解决状态和冲突标记，Go 格式、重复定义与调用签名检查通过。
- [ ] 后端、前端及迁移相关验证全部通过，或明确记录无法执行的环境限制和剩余风险。
- [ ] merge commit 已按用户确认的边界通过普通 push 同步到 `origin/build`，远端与本地 `build` 一致，且未改写远端历史。

## Out of Scope

- 不重构与本次增量无关的协议适配或业务架构。
- 不删除 `build` 产品能力来换取低冲突合并。
- 不把 `build` 合入 `main`，不 force push，不改写已有提交历史。
