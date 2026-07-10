# OpenAI GPT-5.6 Codex 客户端版本兼容设计

## Architecture

本任务不新增配置、DTO、数据库字段或运行时版本协商。修复集中在 `backend/internal/service` 包：新增一个唯一的 Codex 客户端版本源，并让现有语义常量由它在编译期派生。

建议在独立文件 `openai_codex_client_identity.go` 中定义：

```go
const (
	openAICodexClientVersion = "0.144.1"
	codexCLIUserAgent        = "codex_cli_rs/" + openAICodexClientVersion + " (Ubuntu 22.4.0; x86_64) xterm-256color"
	codexCLIVersion          = openAICodexClientVersion
	openAICodexProbeVersion = openAICodexClientVersion

	// DefaultOpenAICodexUserAgent OpenAI Codex 默认 User-Agent。
	DefaultOpenAICodexUserAgent = "codex-tui/" + openAICodexClientVersion + " (Ubuntu 22.4.0; x86_64) xterm-256color (codex-tui; " + openAICodexClientVersion + ")"
)
```

保留 `codexCLIVersion` 和 `openAICodexProbeVersion` 两个语义别名，以减少消费者改动并维持代码可读性；生产代码中只有 `openAICodexClientVersion` 持有版本字面量。

## Identity Matrix

| 路径 | 版本载体 | 设计结果 |
| --- | --- | --- |
| HTTP Responses 强制 Codex | `User-Agent` | 使用派生的 `codexCLIUserAgent` |
| Chat Completions bridge 强制 Codex | `User-Agent` | 复用同一 HTTP request builder 和 UA |
| OAuth passthrough 非 Codex UA 兜底 | `User-Agent` | 使用派生的 `codexCLIUserAgent` |
| Responses compact | `Version` | 使用 `codexCLIVersion`，等于唯一版本源 |
| WebSocket OAuth 兜底 | `User-Agent` | 使用派生的 `codexCLIUserAgent` |
| 模型目录默认请求 | `client_version`、`Version`、`User-Agent` | 默认版本与 UA 来自同一版本源 |
| 账号用量探测 | `Version`、`User-Agent` | 探测版本和 CLI UA 一致 |
| 账号测试 | `Version`、`User-Agent` | 继续使用现有语义常量，值统一 |
| 浏览器 UA 替换 / Codex reset | `User-Agent` | 空设置回退到派生的 TUI UA |

## Request Priority Contracts

本任务只改变内置默认身份中的版本，不改变既有优先级：

1. 账号显式 `credentials.user_agent` 仍按现有路径优先。
2. `gateway.force_codex_cli=true` 时继续强制使用内置 CLI UA。
3. OAuth passthrough 和 WebSocket 的非 Codex UA 继续按现有逻辑兜底。
4. 普通 HTTP 非 passthrough 路径继续保留入站 UA；只有既有强制或浏览器兜底规则会替换。
5. 模型目录显式传入的 `client_version` 继续原样使用；只更新空值时的默认版本。
6. 后台显式保存的 `openai_codex_user_agent` 继续生效；空值才使用新版内置默认。

## Model Compatibility Tests

增加一组以 `gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna` 为表项的 OAuth 请求构造测试，验证：

- 模型名原样发往上游，不回退到旧模型。
- 触发项目内置 Codex 兜底时，UA 包含唯一版本源 `0.144.1`。
- compact、模型目录、WebSocket 和账号测试继续验证各自的 `Version` / UA。

同时增加身份一致性测试，直接锁定：

- `openAICodexClientVersion == "0.144.1"`。
- `codexCLIVersion` 与 `openAICodexProbeVersion` 等于唯一版本源。
- CLI 和 TUI 默认 UA 均包含同一个版本，且不再包含 `0.125.0`。

## Compatibility

- 不修改 `Account`、设置 DTO、前端类型或 API 响应。
- 不修改 `gateway.force_codex_cli` 默认值。
- 不修改 `openai_passthrough`、WebSocket 路由、`originator` 或会话隔离行为。
- 不修改 404 model-not-found 识别和 30 分钟模型级冷却。
- 不迁移管理员显式配置的 User-Agent；这类值属于用户控制范围。
- 不复用入站 `min_codex_version` / `max_codex_version` 策略设置。

## Risks And Rollback

- 风险：遗漏一个生产字面量会继续形成版本不一致。实现后使用 `rg` 扫描非测试 Go 文件中的 `0.125.0` 和相关常量。
- 风险：误改 UA 优先级会覆盖管理员自定义值或非 Codex 客户端。测试必须覆盖显式 UA 和 `force_codex_cli=false` 的保留行为。
- 风险：只更新 UA、不更新 compact 或模型目录的 `Version` 会重现历史 `#1933`。身份一致性测试必须同时锁定所有语义常量。
- 回滚：恢复身份文件中的唯一版本源及派生常量即可；没有数据迁移、缓存格式或 API 契约需要回滚。
