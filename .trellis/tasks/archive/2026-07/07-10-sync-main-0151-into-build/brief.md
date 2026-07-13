# Brief — 同步 main 0.1.151 到 build 并保护定制特性

## Goal

- 将 `main` 的 0.1.151 增量完整合入 `build`，解决全部文本和语义冲突，保留 `build` 的 OpenAI、Codex、Anthropic、Grok 定制能力，并通过跨层验证后普通推送到 `origin/build`。

## Scope

- 实施前 fetch 并固定 `origin/main`、`origin/build`；远端变化时重新生成冲突矩阵。
- 创建合并前备份分支，以 `git merge --no-commit --no-ff origin/main` 保留真实冲突现场。
- 处理 4 个已知文本冲突，并逐项复核其余 21 个自动合并文件。
- 吸收 MCP/custom tools、cache creation usage、Codex 身份配对、用户级 Fast/Flex、GPT-5.6 计费、生图 namespace、setup-token 刷新和 writer nil 防护。
- 保留 Anthropic 直连 Chat、缓存稳定、Grok 路由与额度、provider 最终 effort、GPT-5.6 `max`、Codex 单点版本及生图配置。
- 运行后端、前端、迁移和 Git 质量门槛，创建双父 merge commit 并普通推送 `origin/build`。

## Non-Goals

- 不重构无关协议或业务架构。
- 不删除 `build` 功能来规避冲突。
- 不修改已发布 migration，不合入其它分支，不 force push，不改写历史。

## Key Context

- 当前预期：`build=939b896b`、`main=e316ebf5`、merge base `9a2f11b4`；`build/main` 独有提交为 82/31。
- 已知文本冲突：`openai_gateway_grok.go`、`openai_gateway_grok_test.go`、`openai_gateway_messages_chat_fallback.go`、`openai_oauth_passthrough_test.go`。
- Grok effort 使用 provider 归一化后的最终请求体；测试同时覆盖嵌套别名和扁平兼容字段。
- Messages fallback 保留 `build` 的 Anthropic/Chat 直连桥接、custom UA、Beta Fast、failover、quota 和缓存契约，不恢复旧 Responses 中转实现。
- OAuth passthrough 同时保留 GPT-5.6 当前身份和 codex-tui originator/UA 配对测试。
- `openai_codex_client_identity.go` 继续唯一持有 0.144.1 版本常量；新增身份配对 helper 只能复用该常量。
- 新 migration 173 可顺序追加，历史重复编号迁移保持不动。
- 若 `origin/build` 并发前进或普通 push 被拒绝，停止并重新规划，绝不 force push。

## Acceptance

- 备份分支准确，目标 main 提交固定，全部 4 个文本冲突和 21 个自动合并文件完成记录与复核。
- `main` 0.1.151 修复完整进入结果，`build` 核心能力仍有代码和测试闭环。
- 无未解决索引项、冲突标记、重复常量/入口或失效签名，Go 格式正确。
- 后端全量 unit、相关 lint、前端 Vitest/typecheck/lint 和 migration 检查通过，或明确记录环境限制与剩余风险。
- 最终提交为双父 merge commit，`origin/main` 是其祖先；普通 push 后远端 `build` 与本地一致。

## Next Step

- 用户确认本 brief 和 planning artifacts 后，运行 `task.py start` 激活任务，再进入 `trellis-route(implement)`；确认前不执行实际合并。
