# Brief — 合并 main 到 build 并保留现有 feature

## Goal

- 将实施时最新的 `origin/main` 合入当前 `build`，解决全部冲突，同时保留 `build` 的 OpenAI、Codex、Anthropic、Grok 定制功能和 `main` 的最新修复。

## Scope

- 刷新并确认 `origin/main` / `origin/build`，重新预演实际冲突。
- 为合并前的 `build` HEAD 创建新的带提交号备份分支。
- 使用 `git merge --no-commit --no-ff origin/main` 合并，在验证通过前不创建最终 merge commit。
- 解决 7 个后端 service 文本冲突，并复核 8 个双方共同修改但自动合并的文件。
- 保留 Anthropic 直连 Chat 桥接、Grok messages 强制 Chat、Grok quota/effort、Codex custom UA/reset/统一版本身份、生图配置和 raw Chat 日志。
- 吸收 main 的 GPT-5.6 cache billing/effort、WS reset、compact SSE、并发计费恢复、类型和 i18n 修复。
- 完成定向与 package 级测试、前端 typecheck 和相关 Vitest，验证后创建 merge commit。

## Non-Goals

- 不重写协议适配架构，不通过删除 build feature 换取无冲突。
- 不为 Anthropic Messages raw Chat fallback 保留两套并行转换算法。
- 不修改已发布 migration。
- 不把 `build` 合并到其它分支，不 force push；验证通过后按 normal 模式推送当前 `build` 到 `origin/build`。

## Key Context

- 探测时 `build=75893bf9`、`main=origin/main=9a2f11b4`、merge base=`12d811bd`，独有提交数为 main 29 / build 78。
- 7 个冲突文件：`account_usage_service.go`、raw Chat、Messages fallback、Responses fallback、gateway service、WS HTTP bridge、settings runtime。
- 用户已确认：3 个 Codex 常量冲突保留 `openai_codex_client_identity.go` 单一来源，统一值为 `0.144.1`。
- 用户已确认：4 个 effort 冲突组合“provider 记录最终上游值”和“upstream/billing/original 多模型候选恢复后缀”。
- 用户已确认：Messages fallback 仅保留一个 `forwardAnthropicViaRawChatCompletions` 入口，内部使用 build 的直连 `AnthropicToChatCompletions`，保留 Beta Fast、Grok quota 和缓存稳定契约，并吸收 main 的错误处理、failover 与兼容修复。
- 用户已确认：仅 GPT-5.6 保留显式 `max`，其它 GPT/Codex 折叠为 `xhigh`；GLM/Grok 使用 provider-specific 映射。
- pricing JSON 使用 main 的 GPT-5.6 官方分档价格和 cache write 成本，同时保留 build 的 `supports_max_reasoning_effort=true`。
- 自动合并会同时保留 Anthropic sticky 与 compact keepalive、ChatThinking 与 parallel_tool_calls/cache creation usage、Grok 强制 Chat 与 main Responses 修复。
- 现有 `build-bak` 落后 96 个提交，必须创建新的可靠备份分支。

## Acceptance

- 新备份分支指向合并前 build HEAD，`origin/main` 成为 build 祖先，本次业务合并提交为双父 merge commit。
- 工作区无未解决冲突和冲突标记；7 个显式冲突及 8 个自动合并文件均已复核。
- Codex identity 保持唯一版本源；raw Chat、Messages/Responses fallback、WS bridge 同时通过最终 effort 与模型候选测试。
- GPT-5.6 `max`、非 GPT-5.6 `max -> xhigh`、GLM/Grok provider 映射均通过回归测试。
- Anthropic、Grok、Codex、生图等 build feature 保持完整，main 的最新修复已吸收。
- backend apicompat/service/handler/repository 测试、frontend typecheck 和相关 Vitest 通过。
- merge commit 与任务 snapshot bookkeeping commit（若生成）已通过普通 push 同步到 `origin/build`，远端与本地一致。

## Next Step

- Phase 2.2 全面检查已通过；下一步按用户变更后的 normal 模式执行 Phase 3.4，创建双父 merge commit、推送 `origin/build`，再写入并推送任务 snapshot。
