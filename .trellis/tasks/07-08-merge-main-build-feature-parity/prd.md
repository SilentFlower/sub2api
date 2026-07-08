# 合并 main 到 build 并保留 build 功能

## Goal

将 `origin/main` 合入当前 `build` 分支，解决冲突，并保留 `build` 上已有的 OpenAI、Codex、Anthropic 相关功能。合并结果应以 `main` 的拆分后代码结构为基底，将 `build` 的功能迁移到新文件边界中，而不是回退到旧大文件结构。

## Background

- 用户已确认可以在当前 `build` 分支直接处理，且已有 `build-bak` 备份分支。
- 临时 worktree 探测 `origin/main` 合入 `origin/build` 时出现 9 个 Git 冲突文件：
  - `README_CN.md`
  - `backend/internal/handler/admin/setting_handler.go`
  - `backend/internal/service/openai_gateway_messages.go`
  - `backend/internal/service/openai_gateway_service.go`
  - `backend/internal/service/setting_service.go`
  - `frontend/src/components/account/CreateAccountModal.vue`
  - `frontend/src/components/account/EditAccountModal.vue`
  - `frontend/src/i18n/locales/en.ts`
  - `frontend/src/i18n/locales/zh.ts`
- `main` 已拆分大文件：
  - `backend/internal/service/openai_gateway_service.go` 从 build 的约 7774 行拆为约 1095 行，并拆出 `openai_gateway_*` / `gateway_*` / `openai_ws_forwarder_*` 等文件。
  - `backend/internal/service/setting_service.go` 从 build 的约 5429 行拆为约 263 行，并拆出 `setting_features.go`、`setting_update.go`、`setting_parse.go`、`setting_public.go`、`setting_gateway_runtime.go`、`setting_oauth.go`。
  - `backend/internal/handler/admin/setting_handler.go` 从 build 的约 3863 行拆为约 468 行，并拆出 `setting_handler_update.go`、`setting_handler_runtime.go`、`setting_handler_email.go`、`setting_handler_audit.go`。
- `build` 上需要保留的功能包括：
  - Anthropic `/v1/messages` 到 OpenAI Chat Completions 的直连桥接与缓存稳定修复。
  - Anthropic messages 账号粘性优先级修复。
  - raw chat 调试日志补充。
  - OpenAI 生图主模型与 reasoning effort 可配置。
  - Codex 自定义 User-Agent 放行规则。
  - OpenAI Codex reset 邀请与 reset credit 过期时间展示。

## Requirements

- R1：合并 `origin/main` 到当前 `build`，不丢失 `main` 在 `0.1.146` 后的兼容性修复、大文件拆分和批量生图等新增内容。
- R2：保留 `build` 的 Anthropic Chat 直连桥接缓存稳定契约：
  - typed content part array；
  - Claude Code attribution system block 过滤；
  - assistant text/image 与 `tool_use` 合并为同一条 assistant message；
  - `tool_result` 保持 Chat tool adjacency；
  - 并行 tool_result 按上一条 assistant `tool_calls` 顺序稳定化；
  - 缺失上游 `tool_call.id` 时使用确定性 fallback id；
  - `thinking: {"type":"disabled"}` 透传且不输出 `reasoning_effort`。
- R3：以 `main` 的 `forwardAnthropicViaRawChatCompletions` 命名和拆分结构为主，迁移 `build` 的直连桥接语义；不要保留一套重复的 `forwardMessagesViaRawChatCompletions` 并行实现。
- R4：设置相关功能按 `main` 的拆分结构迁移，不回退到 build 的旧大文件：
  - OpenAI 生图主模型 / reasoning effort 设置；
  - Codex custom UA allowlist 设置；
  - Codex reset service/client 注入与管理端 API。
- R5：前端账号创建/编辑弹窗保留 `main` 的新能力，并合入 `build` 的 custom UA 能力：
  - 保留 `codexImageToolMode`，包括 `block` 策略；
  - 保留 Anthropic APIKey auth scheme；
  - 合入 Codex 自定义 UA 输入、读取、写入、测试钩子。
- R6：i18n 按 `main` 的模块化目录迁移。`frontend/src/i18n/locales/en.ts` / `zh.ts` 旧文件应删除；build 新增文案要迁移到 `frontend/src/i18n/locales/*/admin/accounts.ts` 或 `admin/settings.ts` 等对应模块。
- R7：迁移文件不修改已存在 SQL。若需要处理重复编号或排序问题，新增 migration 或调整仅限未发布的新合并结果文件，并记录原因。
- R8：合并完成后工作区不得残留冲突标记。

## Acceptance Criteria

- [ ] `git merge origin/main` 在 `build` 上完成，`git status --short` 不再包含 `UU` / `UD` / `DU` 等未解决冲突状态。
- [ ] `backend/internal/pkg/apicompat/anthropic_chatcompletions.go` 存在并满足协议适配 spec 中的缓存稳定契约。
- [ ] `/v1/messages` raw Chat fallback 使用 `forwardAnthropicViaRawChatCompletions` 入口，并保留 build 的直连 Anthropic Chat 桥接行为。
- [ ] `openai_gateway_service.go`、`setting_service.go`、`setting_handler.go` 保持 `main` 的拆分方向，不恢复成 build 的旧大文件。
- [ ] OpenAI 生图主模型 / reasoning effort、Codex custom UA、Codex reset 相关后端类型、API、前端组件和 i18n 都可在合并后的结构中找到。
- [ ] i18n 不再依赖旧 `en.ts` / `zh.ts` 大文件；新增文案迁移到模块化 locale 文件。
- [ ] 如 migration 编号存在重复，给出处理结果和风险说明。
- [ ] 至少运行并记录：
  - `cd backend && go test -tags=unit ./internal/pkg/apicompat`
  - `cd backend && go test -tags=unit ./internal/service -run 'Test.*Messages|Test.*Chat|Test.*Anthropic|Test.*OpenAI|TestOpenAIImage|TestOpenAICodex|TestCodex'`
  - `cd backend && go test -tags=unit ./internal/handler ./internal/repository -run 'OpenAICodexReset|Setting|Account'`
  - `cd frontend && pnpm typecheck`
