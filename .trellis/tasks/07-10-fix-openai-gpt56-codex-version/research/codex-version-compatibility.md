# OpenAI Codex 客户端版本兼容性核查

## 结论

`gpt-5.6-luna` 的 404 问题可按真实兼容性 BUG 处理。项目已经声明支持 GPT-5.6 三个模型，但内置 Codex 客户端身份仍停留在 `0.125.0`，低于 OpenAI 官方模型目录声明的最低客户端版本 `0.144.0`。

本任务采用固定 `0.144.1` 的最小修复：统一项目内置版本来源并补齐请求路径测试，不实现动态版本协商。

## 外部证据

- 问题报告：<https://github.com/Wei-Shaw/sub2api/issues/3895>。
- OpenAI 官方模型页：<https://developers.openai.com/api/docs/models/gpt-5.6-luna>。
- OpenAI Codex `rust-v0.144.1` 模型目录：<https://github.com/openai/codex/blob/rust-v0.144.1/codex-rs/models-manager/models.json>。
- 官方目录中以下模型均声明 `minimal_client_version: "0.144.0"`：
  - `gpt-5.6-sol`
  - `gpt-5.6-terra`
  - `gpt-5.6-luna`
- 历史同类修复：提交 `1e57e88e` 将 Codex 身份从 `0.104.0` 升至 `0.125.0`，修复 GPT-5.5 compact 的旧客户端拒绝问题，并关闭 issue `#1933`。

## 本地代码证据

### 当前生产字面量

| 位置 | 当前值 | 用途 |
| --- | --- | --- |
| `backend/internal/service/openai_gateway_service.go:39` | `codex_cli_rs/0.125.0 ...` | HTTP、passthrough、WS 等强制/兜底 UA |
| `backend/internal/service/openai_gateway_service.go:53` | `0.125.0` | compact 与账号测试 `Version` |
| `backend/internal/service/account_usage_service.go:114` | `0.125.0` | 模型目录默认版本和账号用量探测 |
| `backend/internal/service/setting_gateway_runtime.go:86` | `codex-tui/0.125.0 ...` | 浏览器 UA 替换和 Codex reset 默认 UA |

### 消费路径

- HTTP Responses 与 Chat Completions bridge：`buildUpstreamRequest`。
- OAuth passthrough：`buildUpstreamRequestOpenAIPassthrough`。
- Responses compact：HTTP 和 passthrough 路径均设置 `Version`。
- WebSocket：`buildOpenAIWSHeaders` 在强制或非 Codex OAuth 兜底时使用内置 UA。
- 模型目录：`FetchCodexModelsManifest` 在未传 `client_version` 时使用探测版本，并设置对应 `Version` 和 UA。
- 账号用量探测：`probeOpenAICodexSnapshot`。
- 账号测试：普通 OpenAI、compact 和相关探测路径。
- 后台默认 Codex UA：`GetOpenAICodexUserAgent` 空设置回退，以及 Codex reset 路径。

### 配置兼容性

- `gateway.force_codex_cli` 默认是 `false`，本任务不修改。
- 账号 `credentials.user_agent` 是显式自定义值，继续保持现有优先级。
- 全局 `openai_codex_user_agent` 的设置默认值是空字符串；运行时空值才回退到内置默认 UA。
- 因此不需要 migration，也不应自动覆盖管理员显式保存的 User-Agent。
- `min_codex_version` / `max_codex_version` 属于入站 `codex_cli_only` 策略，不是上游身份版本，不能复用。

## 调度影响

- `backend/internal/service/ratelimit_service.go` 将匹配的 404 model-not-found 写为 30 分钟模型级冷却。
- 该策略本身符合现有设计，本任务不修改。
- 修复点是在请求发往上游前使用满足最低要求的内置客户端身份，避免合法模型误触发该冷却。

## 已完成验证

- 官方模型页返回成功，模型 ID 存在。
- 官方 Codex `0.144.1` 目录确认三个 GPT-5.6 模型的最低版本均为 `0.144.0`。
- `v0.1.149` 和当前上游 `main` 均仍包含 `0.125.0` 生产字面量。
- 现有 404 冷却测试通过：
  - `TestIsUpstreamModelNotFoundError`
  - `TestRateLimitService_HandleUpstreamError_ModelNotFoundUsesModelRateLimit`
  - `TestOpenAIModelNotFound_DoesNotRuntimeBlockWholeAccount`
- 现有模型目录、compact、passthrough UA 和账号测试请求头测试通过。

## 验证限制

- 未使用真实用户 OAuth 凭据执行 `0.125.0` 与 `0.144.1` 的线上 A/B 请求，避免额度消耗和凭据风险。
- 本任务通过官方版本契约、项目代码路径、历史同类修复和本地请求构造测试验证修复。
