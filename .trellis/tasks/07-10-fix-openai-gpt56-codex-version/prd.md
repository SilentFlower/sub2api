# 修复 OpenAI GPT-5.6 Luna Codex 客户端版本兼容

## Goal

修复 OpenAI OAuth 转发 `gpt-5.6-luna` 时因内置 Codex 客户端身份低于上游最低版本要求而返回 404 的问题，并统一各条 Codex 上游链路使用的内置版本来源，避免合法模型触发错误的模型级冷却。

## Background

- 来源 issue：<https://github.com/Wei-Shaw/sub2api/issues/3895>。
- `v0.1.149` 与当前上游 `main` 仍在多个位置内置 `0.125.0`：
  - `backend/internal/service/openai_gateway_service.go:39` 的 `codexCLIUserAgent`。
  - `backend/internal/service/openai_gateway_service.go:53` 的 `codexCLIVersion`。
  - `backend/internal/service/account_usage_service.go:114` 的 `openAICodexProbeVersion`。
  - `backend/internal/service/setting_gateway_runtime.go:86` 的 `DefaultOpenAICodexUserAgent`。
- OpenAI 官方 Codex `rust-v0.144.1` 模型目录将 `gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna` 的 `minimal_client_version` 均声明为 `0.144.0`。
- 提交 `6cea1c35` 已增加 GPT-5.6 三模型的名称、映射与计费配置，但未同步客户端版本身份。
- `backend/internal/service/ratelimit_service.go:2007` 将上游 404 model-not-found 记录为 30 分钟模型级冷却；因此一次客户端身份不兼容会被放大为后续调度排除，并可能最终表现为 502/503。
- 历史 issue `#1933` 曾因同类硬编码旧版本导致 GPT-5.5 compact 失败，提交 `1e57e88e` 通过同步更新 Codex UA、Version 与探测版本修复。

## Requirements

- **R1：统一内置版本来源。** Codex CLI 版本、强制/兜底 User-Agent、模型目录默认 `client_version`、compact `Version`、账号用量探测和账号测试不得继续分别维护互相独立的版本字面量。
- **R2：使用固定已发布版本。** 所有内置 Codex 客户端身份统一使用 `0.144.1`，满足官方声明的 GPT-5.6 最低版本 `0.144.0`；本任务不引入动态版本协商。
- **R3：保持现有路由策略。** 修复版本身份时不得改变 `gateway.force_codex_cli` 的默认值、OpenAI passthrough 开关、`originator` 选择、浏览器 UA 兜底范围或 HTTP/WS 路由决策。
- **R4：覆盖全部内置身份消费者。** 至少核对并统一 HTTP Responses、Chat Completions bridge、Responses compact、OAuth passthrough、WebSocket 握手、模型目录、账号用量探测、账号测试和后台默认 Codex UA。
- **R5：保留用户显式配置。** 账号自定义 User-Agent、入站官方 Codex UA 和后台显式设置不得被无条件覆盖；只更新项目内置默认值和既有强制/兜底行为使用的版本来源。
- **R6：防止回归。** 测试必须验证统一版本值被各条相关链路正确使用，并至少覆盖 GPT-5.6 三个模型的版本兼容性相关转发/请求头行为。
- **R7：保持错误语义。** 不修改现有上游 404 model-not-found 的识别和模型级冷却策略；本任务通过避免发送过旧客户端身份来消除误触发，不扩大错误处理范围。

## Acceptance Criteria

- [x] 项目内不再存在用于生产 Codex 上游身份的 `0.125.0` 硬编码；历史日志样例或纯测试输入若保留，必须不参与生产请求构造。
- [x] 所有项目内置 Codex 版本消费者从同一来源派生，且统一版本为 `0.144.1`。
- [x] 使用内置强制/兜底身份构造 HTTP、compact、passthrough、WebSocket、模型目录、探测和账号测试请求时，UA/Version 中的版本一致。
- [x] 入站客户端或账号显式配置的 User-Agent 仍按现有优先级生效，`force_codex_cli=false` 的既有行为不变。
- [x] GPT-5.6 Sol、Terra、Luna 的请求构造测试确认模型名原样转发，并在触发内置身份时使用 `0.144.1`，不再发送低于官方最低要求的项目默认身份。
- [x] 现有 404 model-not-found 识别与模型级冷却测试继续通过，证明错误策略没有被本修复改写。
- [x] 相关后端单元测试通过：`cd backend && go test -tags=unit ./internal/service -run 'Test.*Codex|Test.*OpenAI.*(Version|UserAgent|Passthrough|Compact|WebSocket|Model)'`。
- [x] 完整后端单元测试、静态检查和 `git diff --check` 通过，或明确记录与本改动无关的既有失败。

## Verification Results

- 定向身份、模型目录、GPT-5.6 passthrough、Chat Completions bridge、compact、WebSocket、账号测试和 404 冷却测试通过。
- `cd backend && go test -tags=unit ./internal/service -count=1` 通过。
- `cd backend && go test -tags=unit ./... -count=1` 通过。
- `GOTOOLCHAIN=go1.26.5 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.9.0 run --timeout=30m --build-tags=unit --new ./...` 通过，结果为 `0 issues`。
- `rg -n --glob '*.go' --glob '!**/*_test.go' '0\.125\.0' backend/internal` 无匹配；`git diff --check` 通过。
- 仓库当前未安装独立 `golangci-lint` 可执行文件，因此使用 CI 锁定版本通过 `go run` 执行。无 `unit` build tag 的默认 lint 会被既有 `settingValuesRepoStub` build-tag 问题阻断；全仓无增量过滤 lint 另有 50 个既有问题，均不属于本任务改动。

## Technical Notes

- 全局 `openai_codex_user_agent` 的持久化默认值为空，运行时空值才回退到内置 UA；因此本任务不需要 migration。
- 管理员显式保存的 User-Agent 和账号 `credentials.user_agent` 属于用户配置，继续保持现有优先级，不自动改写。
- `min_codex_version` / `max_codex_version` 用于入站 `codex_cli_only` 策略，不作为上游身份版本来源。
- 验证使用官方版本契约和本地请求构造测试，不使用真实用户 OAuth 凭据执行有额度消耗的线上 A/B。

## Out of Scope

- 不调整 GPT-5.6 的模型映射、定价、上下文窗口或前端展示。
- 不修改 OpenAI OAuth、TLS 指纹、设备指纹、会话隔离或账号调度算法。
- 不通过真实用户 OAuth 凭据执行付费或有额度消耗的线上请求。
- 不改变 404 model-not-found 的 30 分钟冷却规则。
- 不实现根据模型目录 `minimal_client_version` 动态选择、升级或回退客户端版本。
