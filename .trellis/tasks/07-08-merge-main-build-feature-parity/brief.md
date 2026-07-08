# Brief — 合并 main 到 build 并保留 build 功能

## Goal

- 将 `origin/main` 合入当前 `build` 分支，解决冲突，并保留 `build` 上已有的 OpenAI、Codex、Anthropic 相关功能；合并结果以 `main` 的拆分后结构为目标形态。

## Scope

- 在当前 `build` 分支执行 `origin/main` merge 并解决冲突。
- 保留 main 在 `0.1.146` 后的兼容性修复、大文件拆分和批量生图等新增内容。
- 将 build 的 Anthropic Messages ↔ Chat Completions 直连桥接、账号粘性、raw chat 调试日志、OpenAI 生图设置、Codex custom UA、Codex reset 功能迁移到 main 的新结构。
- 处理前端账号创建/编辑弹窗冲突，并将 build 新增 i18n 文案迁移到 main 的模块化 locale 文件。
- 检查 migration 编号重复与排序风险，必要时记录或新增后续修正。

## Non-Goals

- 不回退到 build 的旧大文件结构。
- 不用整文件 `ours` 覆盖 main 的拆分结果。
- 不修改已经发布或已应用的 migration 内容。
- 不在本任务中重新设计 Anthropic 或 OpenAI 网关协议，只做合并与功能迁移。

## Key Context

- `main` 的 `forwardAnthropicViaRawChatCompletions` 原始方案解决 `/v1/messages` fallback 可用性，但通过 Anthropic -> Responses -> Chat 两段转换；它默认不能满足 build 已记录的 Chat prefix cache 稳定契约。
- build 的直连桥接更适合缓存稳定：typed content part array、attribution 过滤、tool_result 顺序稳定、确定性 tool id、thinking disabled 与 reasoning effort 互斥。
- 合并后应使用 `main` 的函数名和拆分结构，但实现语义要保留 build 的直连桥接。
- 冲突探测已确认 9 个 Git 冲突文件：`README_CN.md`、`backend/internal/handler/admin/setting_handler.go`、`backend/internal/service/openai_gateway_messages.go`、`backend/internal/service/openai_gateway_service.go`、`backend/internal/service/setting_service.go`、`frontend/src/components/account/CreateAccountModal.vue`、`frontend/src/components/account/EditAccountModal.vue`、`frontend/src/i18n/locales/en.ts`、`frontend/src/i18n/locales/zh.ts`。
- 关键规范：`.trellis/spec/backend/protocol-adapter-guidelines.md`、`.trellis/spec/backend/database-guidelines.md`、`.trellis/spec/frontend/component-guidelines.md`。

## Acceptance

- `git merge origin/main` 在 `build` 上完成，工作区无未解决冲突状态和冲突标记。
- `backend/internal/pkg/apicompat/anthropic_chatcompletions.go` 存在并满足协议适配缓存稳定契约。
- `/v1/messages` raw Chat fallback 使用 `forwardAnthropicViaRawChatCompletions` 入口，并保留 build 的直连 Anthropic Chat 桥接行为。
- `openai_gateway_service.go`、`setting_service.go`、`setting_handler.go` 保持 main 的拆分方向，不恢复旧大文件。
- OpenAI 生图主模型 / reasoning effort、Codex custom UA、Codex reset 相关后端类型、API、前端组件和 i18n 都在合并后的结构中闭环。
- i18n 不再依赖旧 `en.ts` / `zh.ts` 大文件。
- 运行并记录 PRD 中列出的后端 apicompat/service/handler/repository 测试和前端 typecheck。

## Next Step

- 启动任务后，在当前 `build` 分支执行 `git merge origin/main`，按实现清单逐类解决冲突。
