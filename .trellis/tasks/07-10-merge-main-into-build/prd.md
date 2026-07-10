# 合并 main 到 build 并保留现有 feature

## Goal

将最新 `origin/main` 合入当前 `build` 分支，解决所有冲突，同时保留 `build` 上已有的 OpenAI、Codex、Anthropic、Grok 定制功能。合并结果应同时具备 `main` 的最新修复和 `build` 的现有业务能力，不通过整文件选择 `ours` 或 `theirs` 丢弃任一侧有效语义。

## Background

- 当前分支为 `build`，工作区在创建本任务前干净，`build` 与 `origin/build` 一致。
- 探测时 `main` 与 `origin/main` 一致：`9a2f11b4`；`build` 为 `75893bf9`。
- 合并基点为 `12d811bd`；`main...build` 的独有提交数为 `29 / 78`。
- `git merge-tree --write-tree --messages build main` 预演确认 7 个文本冲突：
  - `backend/internal/service/account_usage_service.go`
  - `backend/internal/service/openai_gateway_chat_completions_raw.go`
  - `backend/internal/service/openai_gateway_messages_chat_fallback.go`
  - `backend/internal/service/openai_gateway_responses_chat_fallback.go`
  - `backend/internal/service/openai_gateway_service.go`
  - `backend/internal/service/openai_ws_http_bridge.go`
  - `backend/internal/service/setting_gateway_runtime.go`
- 双方共同修改了 15 个文件，其中 8 个会被 Git 自动合并，仍需做语义复核，不能只依赖“无冲突”结果。
- 现有 `build-bak` 停留在 `57a11b4c`，落后当前 `build` 96 个提交，不能作为本次合并的可靠回滚点。
- 2026-07-08 的同类任务已确认：以 `main` 的拆分结构为基底，保留 `build` 的 Anthropic Messages 直连 Chat Completions 桥接、缓存稳定契约、Codex custom UA/reset、生图配置等能力。

## Requirements

- R1：实施前拉取远端并确认待合入的 `origin/main` 最新提交；若远端在规划后前进，以实施时确认的最新提交为准并重新运行冲突预演。
- R2：创建指向合并前 `build` HEAD 的新备份分支，分支名包含当前短提交号，确保合并前后均可准确回滚。
- R3：使用 `git merge --no-commit --no-ff origin/main` 将上游改动合入 `build`，在验证通过前不创建最终 merge commit。
- R4：保留 `build` 已有功能及其测试契约，包括：
  - Anthropic `/v1/messages` 到 Chat Completions 的直连桥接、稳定 content/tool 前缀、确定性 tool id 和账号粘性；
  - Grok `/v1/messages` 显式强制走 Chat Completions、xAI 凭据与 quota snapshot 更新；
  - Grok 套餐额度进度、Grok 4.5 effort 归一化；
  - raw Chat 调试日志、OpenAI 生图主模型与 reasoning effort 配置；
  - Codex custom UA、Codex reset、GPT-5.6 max effort、统一 Codex 客户端版本身份。
- R5：吸收 `main` 的最新修复，包括 GPT-5.6 cache write 计费、reasoning effort 多模型候选推导、WS reset 兼容、compact SSE/心跳、并发计费与支付恢复、类型和 i18n 修复。
- R6（用户已确认）：Codex 版本常量继续由 `backend/internal/service/openai_codex_client_identity.go` 单点定义，统一采用 `main` 已升级的 `0.144.1`；不得重新在 usage、gateway 或 settings 文件中引入重复常量。
- R7（用户已确认）：reasoning effort 冲突采用组合策略，同时满足两项契约：
  - GLM/Grok 4.5 等 provider-specific 归一化后，usage 记录最终实际发送给上游的 effort；
  - GPT/Codex 后缀模型在映射或标准化剥离后缀时，仍能从 `upstreamModel`、`billingModel`、`originalModel` 候选中恢复 effort。
- R8（用户已确认）：Anthropic Messages 的 raw Chat fallback 只保留一个 `forwardAnthropicViaRawChatCompletions` 入口，内部采用 `build` 的 Anthropic -> Chat Completions 直连桥接，保留 Beta Fast Mode `service_tier=priority`，并吸收 `main` 的错误处理、failover、effort 和兼容性修复；不保留并行的 Anthropic -> Responses -> Chat 实现。
- R9：对 8 个自动合并文件逐项复核，重点检查 handler 路由、apicompat 类型、usage endpoint、messages 分流、request body effort helper、模型定价 JSON。
- R9.1（用户已确认）：采用 `main` 的模型感知 `max` 规则，仅 GPT-5.6 sol/terra/luna 保留 `max`；其它 GPT/Codex 模型将 `max` 折叠为 `xhigh`。GLM/Grok 4.5 继续使用各自 provider-specific 映射。
- R9.2（用户已确认）：模型定价采用 `main` 的 GPT-5.6 官方分档价格和 cache write 成本，同时保留 `build` 的 `supports_max_reasoning_effort=true` 标记。
- R10：合并后不得残留冲突状态、冲突标记、重复函数或重复常量；Go 文件保持 gofmt。
- R11（用户已变更授权）：验证通过后创建双父 merge commit，并以 normal 模式将当前 `build` 推送到 `origin/build`；不得 force push，也不额外合并到 `main`、`master` 或其它分支。

## Acceptance Criteria

- [ ] 新备份分支准确指向合并前的 `build` HEAD。
- [ ] `origin/main` 已成为 `build` 的祖先，本次业务合并提交包含合并前 `build` 与 `origin/main` 两个父提交；其后的任务 snapshot bookkeeping 提交不改变该祖先关系。
- [ ] `git status --short` 不包含 `UU`、`UD`、`DU`、`AA` 等未解决状态，仓库中无 `<<<<<<<`、`=======`、`>>>>>>>` 冲突标记。
- [ ] 7 个显式冲突均按 `design.md` 的逐文件策略解决，8 个自动合并文件已完成语义复核。
- [ ] `openAICodexClientVersion` 仍是 Codex 内置身份的唯一版本源，相关 UA、Version header、probe version 和默认 UA 保持一致。
- [ ] raw Chat、Messages fallback、Responses fallback、WS HTTP bridge 的 usage effort 同时通过 provider 最终值与模型后缀候选测试。
- [ ] GPT-5.6 显式 `max` 保持为 `max`，非 GPT-5.6 的 GPT/Codex 显式 `max` 归一化为 `xhigh`；GLM/Grok provider 映射不受影响。
- [ ] Anthropic 直连桥接、Grok 强制 Chat、Grok quota、Codex custom UA/reset、生图配置等 `build` feature 仍可在代码和测试中闭环找到。
- [ ] 至少运行并记录：
  - `cd backend && go test -tags=unit ./internal/pkg/apicompat`
  - `cd backend && go test -tags=unit ./internal/service`
  - `cd backend && go test -tags=unit ./internal/handler ./internal/repository`
  - `cd frontend && pnpm typecheck`
  - 与冲突及自动合并文件对应的前端 Vitest；若全量单测成本可接受，再补跑 `cd backend && go test -tags=unit ./...`。
- [ ] merge commit 与任务 snapshot bookkeeping commit（若生成）已通过普通 push 同步到 `origin/build`，远端 `build` 与本地一致。

## Out of Scope

- 不重写或重新设计现有 OpenAI/Anthropic/Grok 协议适配架构。
- 不删除 `build` 的产品功能来换取无冲突合并。
- 不修改已发布 migration 内容。
- 不把 `build` 合并到其它分支，不 force push，不改写远端历史。
