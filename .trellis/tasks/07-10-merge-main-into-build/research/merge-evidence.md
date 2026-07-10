# 合并证据记录

## 分支状态

- 探测时间：2026-07-10。
- `build`：`75893bf9`，与 `origin/build` 一致。
- `main`：`9a2f11b4`，与 `origin/main` 一致。
- merge base：`12d811bd`。
- 独有提交：`main` 29，`build` 78。
- 旧 `build-bak`：`57a11b4c`，落后 `build` 96 个提交。

## Git 预演

`git merge-tree --write-tree --messages build main` 返回 7 个内容冲突，均位于 `backend/internal/service`。双方共同修改 15 个文件，除冲突文件外还有 8 个自动合并文件需要人工语义复核。

## Build 侧相关提交

- `792c51ff`：统一 Codex 客户端版本身份。
- `05918460`：对齐 Grok 4.5 effort 归一化。
- `60d36274`：支持 GPT-5.6 max effort。
- `57e409da` / `cbd34d3a` / `a328495d`：Grok 套餐额度进度。
- `421df83b`：Grok messages 强制走 Chat Completions。
- `89e05bdc`：raw Chat 调试日志。
- `524b9b7a`：OpenAI 生图主模型与思考预算配置。
- `416943fe`：Codex 自定义 UA 放行。
- `e1a089e4` / `c084180c` / `d53e8b6c` / `c9d52416`：Codex reset 与过期时间。
- `e086ca5d` / `014d69de` / `d6d3f1bf` / `8f070522`：Anthropic 直连桥接、缓存稳定和账号粘性。

## Main 侧相关提交

- `c3ae5fc3`：effort 提取改用模型候选列表，修复后缀元数据丢失。
- `80b3d4c1`：兼容 GPT-5.6 max 推理强度。
- `657c4f97`：Codex 客户端版本升级到 0.144.1。
- `4a2b10c9` / `383f61d0` / `062af81f`：GPT-5.6 cache write 计费与价格。
- `0a5f34a2`：Windows websocket reset 兼容。
- `2cffe1cf` / `ae9a01d8` / `000f6dc6`：compact SSE/心跳修复。
- `fc66a30f`：billing concurrency 与 payment recovery 加固。

## 结论

- 用户已确认：3 个 Codex 常量冲突保留 `build` 的集中定义，并使用 `main` 已验证的 0.144.1 值。
- 用户已确认：4 个 effort 冲突组合 `build` 的最终上游值语义与 `main` 的多模型候选推导，不单选任一侧。
- 用户已确认：Anthropic Messages raw Chat fallback 只保留一个入口，采用 build 的直连 Chat 转换，并吸收 main 的错误处理、failover、effort 与兼容性修复。
- 自动合并已确认可直接保留的组合：Anthropic sticky + compact keepalive、ChatThinking + parallel_tool_calls/cache creation usage、Grok 强制 Chat + main Responses 修复。
- pricing JSON 的虚拟合并结果有效：采用 main 的 GPT-5.6 sol/terra/luna 分档价格与 cache write 成本，同时保留 build 的 `supports_max_reasoning_effort=true`。
- 用户已确认：request body 采用 main 的模型感知规则，仅 GPT-5.6 保留显式 `max`，其它 GPT/Codex 折叠为 `xhigh`；GLM/Grok 继续走 provider-specific 映射。
- 当前没有必须由用户补充的产品决策；“保留现有 feature”按保留所有 build 独有业务能力解释。

## 实施结果（提交前）

- `git fetch origin main build` 后远端未前进：`origin/main=9a2f11b4`，`origin/build=build=75893bf9`。
- 已创建备份分支 `backup/build-before-main-merge-75893bf9`，指向合并前 `build` HEAD。
- 已执行 `git merge --no-commit --no-ff origin/main`，实际冲突仍为预演中的 7 个文件，全部解决且 `git ls-files -u` 为空。
- Codex 常量只保留 `openai_codex_client_identity.go` 单一来源，版本为 `0.144.1`。
- raw Chat、Messages fallback、Responses fallback、WS bridge 使用统一的最终上游 effort helper，并按 mapped -> billing -> original 候选顺序恢复后缀。
- Anthropic Messages fallback 保持直连 `AnthropicToChatCompletions`、Beta Fast、Grok quota 和缓存稳定契约。
- 自动合并复核发现并修复了旧通用 `max` 语义：仅 GPT-5.6 保留 `max`，其它模型归一化为 `xhigh`；Responses、Chat Completions 和 Gemini 兼容路径均传入模型候选。
- 验证结果：
  - `cd backend && go test -tags=unit ./... -count=1`：通过。
  - `cd frontend && pnpm typecheck`：通过。
  - `cd frontend && pnpm lint:check`：通过。
  - 相关 7 个 Vitest 文件共 94 项测试：通过。
  - `git diff --cached --check`：通过。
- 尚未创建 merge commit；用户已将执行模式变更为 normal，确认计划后将普通推送当前 `build` 到 `origin/build`，不额外合并其它分支。
