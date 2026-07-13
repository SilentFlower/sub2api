# Brief — 同步 main 0.1.152 到 build 并保护定制特性

## Goal

- 将 `origin/main` 的 0.1.152 增量安全合入 `build`，吸收最新修复，同时保留 build 的 OpenAI、Codex、Anthropic、Grok、容灾和 Trellis 定制能力。

## Scope

- 实施前重新 fetch 并固定 `origin/main`、`build`、`origin/build` 和 merge base；远端变化时重新规划。
- 纯快进本地 `main`，创建合并前备份分支，选择性 stash 用户已有的 8 个 Trellis 路径。
- 使用 `--no-commit --no-ff` 合并，逐项解决 12 个文本冲突，并复核 31 个自动合并文件。
- 合并 Alpha Search 成功按次计费、migration 174、Ent/DTO/cache/billing/前端价格字段完整链路。
- 组合保留 Grok reasoning effort、cache identity、Composer 清理、APIKey/OAuth、quota 和实际端点记录。
- 保留 Anthropic Messages 直连 Chat Completions 的强制上游 SSE、调试快照、custom UA 和协议稳定性。
- 保持 Codex 客户端版本单点定义，修复旧调用签名和协议规范漂移。
- 运行后端定向/全量 unit、golangci-lint、迁移检查，以及前端 Vitest、typecheck、lint。
- 验证通过后创建本地双父 merge commit，恢复选择性 stash，然后停止等待用户确认。

## Non-Goals

- 不把 `build` 反向合入 `main`。
- 不删除 build 产品能力来降低冲突数量，不做无关架构重构。
- 不修改、删除或重命名已发布 migration。
- 不 force push，不改写已有历史；未经再次确认不推送 `origin/build`。

## Key Context

- 规划基线：`build=origin/build=f59991b5`，`origin/main=a1930ea6`，本地 `main=e316ebf5`，merge base 为 `e316ebf5`。
- 分叉：build 独有 100 个提交，main 独有 40 个提交；虚拟合并树为 `62c6d7bdb64a74cd5d67be64d1d76862d26afc4f`。
- 合并范围：118 个文件变化；双方共同修改 43 个文件，其中 12 个文本冲突、31 个自动合并复核文件。
- 关键冲突集中在 Alpha Search、实际上游端点、Grok raw/Responses、Messages Chat fallback、Codex 版本常量和 Grok 前端测试。
- Git 未直接报告的风险：`endpoint_test.go` 旧两参数调用、`codexCLIVersion` 重复声明、协议规范仍记录旧 Alpha Search/端点签名。
- 当前任务目录必须保持可见；只选择性 stash 旧任务和用户已有规范改动，合并期间禁止 `git add -A`。
- 详细冲突矩阵见 `design.md`，完整文件清单和提交证据见 `research/merge-evidence.md`。

## Acceptance

- 12 个文本冲突全部按组合语义解决，31 个自动合并文件全部完成人工复核。
- main 0.1.152 的 Alpha Search 计费、Grok/xAI、Codex/Responses 和前端增量完整进入 build。
- build 的 Codex 身份/生图、Anthropic 直连、Grok effort/路由、GPT-5.6、容灾和 Trellis 能力仍有代码与测试闭环。
- migration 174、Ent 生成代码、DTO/cache、usage/billing 和前端字段形成完整跨层链路。
- Git 无未解决项和冲突标记，格式、重复定义、调用签名、后端/前端/迁移质量门槛通过。
- 本地 merge commit 具有正确的两个父提交；用户已有 8 个工作区路径恢复且未混入 merge commit。
- merge commit 创建后停止，展示结果；未经用户再次确认不执行 push。

## Next Step

- 用户确认本 brief 和规划三件套后，运行 `task.py start`；进入实现阶段后先执行 `trellis-route(implement)`，再按 `implement.md` 固定远端并开始真实合并。
