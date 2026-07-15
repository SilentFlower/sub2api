# Brief — 同步 main 0.1.155 到 build 并保留特性

## Goal

- 将 `origin/main` 0.1.155 合并到本地 `build`：main 等价覆盖的 build 实现允许替换，仍有独立价值或兼容契约的 build 功能继续保留，并输出冲突记录、主线更新摘要和特性处置清单。

## Scope

- 实施前 fetch 并重新固定 `build`、`origin/build`、`origin/main`、merge base 和虚拟冲突矩阵，创建合并前备份分支。
- 使用 `git merge --no-commit --no-ff origin/main`，逐项解决规划时 16 个显式冲突并语义复核 37 个自动合并文件。
- Grok 配额与 Billing 收敛到 main 的统一查询、探测、缓存、本地窗口和 SSO 数据流，删除 build 重复的独立 Billing API、旧快照类型和前端组件。
- Codex 图片工具采用 main 的统一 namespace/`additional_tools` 识别，同时保留 build 的 namespace 不注入旧工具、不追加旧提示、不改 `tool_choice` 保护；旧原生图片桥接继续支持 `none→auto`。
- 保留 build 的生图主模型与思考预算设置；main 默认 `gpt-5.4-mini` 仅作为空配置回退。
- 保持 build `OpenAICodexResetService`、status/consume/invite API 和独立弹窗完整独立，同时吸收 main 的 OpenAI quota reset API 与单元格。
- 保留 build 的 Grok Messages 强制 Chat、Grok 4.5 effort、Anthropic 直连、Alpha Search、HA/DR、Trellis 和手动镜像构建等独立能力。
- 运行 Wire/Ent 生成、后端和前端定向测试、完整 unit、typecheck、lint、migration 与 Git 完整性检查；验证通过后创建本地双父 merge commit。

## Non-Goals

- 不重新实现已回滚的 JSON Schema 账号降级或 `request_permissions` 文本恢复。
- 不修改或提交 `.trellis/tasks/07-14-deepseek-response-format-json-schema-compat/`。
- 不部署、不执行数据库 migration、不发布镜像。
- 不自动 push `build`，不删除合并前备份分支。

## Key Context

- 规划引用：`build=origin/build=a9ad55b3`，`origin/main=7c717365`，共同基点 `a1930ea6`；build/main 分别独有 118/112 个提交。引用变化时必须重新规划。
- 虚拟合并报告 16 个显式冲突：10 个 Grok Billing 跨层文件、2 个 Codex 图片工具文件、4 个协议测试文件；双方共同修改共 53 个文件。
- GPT-5.6 `max` 使用 main 的模型感知和候选模型实现，不保留 build 的重复旧解析。
- `image_gen` namespace 与旧 `image_generation` 必须使用不同分支，不能让 main 的统一 predicate 扩大 `none→auto` 的范围。
- Wire 必须同时注入 build Codex reset、main OpenAI quota reset 和 main Grok quota/import prober；最终生成文件来自 `go generate ./cmd/server`。
- migration、Ent schema 和生成代码以 main 结构为基线并通过 `go generate ./ent` 核对，禁止手工拼接不一致产物。
- 合并期间禁止 `git add -A`、`git add .` 和全仓 `git stash -u`，避免吸收其它未跟踪任务。

## Acceptance

- `origin/main` 已进入本地 build，`git ls-files -u` 为空，merge commit 两个父提交正确，备份分支可恢复。
- Grok 只保留 main 统一 Billing/探测数据流，不存在旧独立 API、重复缓存、新旧快照或旧前端组件的生产引用。
- namespace 不注入旧图片工具、不追加旧桥接提示、不改显式 `none`；旧原生图片桥接缺失或 `none` 会变为 `auto`。
- 生图主模型和思考预算设置仍可读写并实际进入请求。
- build 独立 Codex reset 三个 API、弹窗和邀请约束，与 main reset API/单元格同时可用且互不替代。
- Grok 4.5 effort、Messages 强制 Chat、Anthropic 直连、Alpha Search、HA/DR 和 Trellis 特性没有静默回退。
- Wire/Ent/migration 一致性、后端定向与完整 unit、前端相关测试/typecheck/lint、`git diff --check` 全部通过，或清楚记录环境阻断。
- 其它未跟踪任务保持未修改、未暂存；合并结果仅在本地，未 push。

## Next Step

- 用户确认本 brief 和三件套后，运行 `task.py start` 激活任务，再进入 `trellis-route(implement)` 执行真实合并。
