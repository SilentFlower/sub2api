# 实施计划

## Step 1: 建立单一 Codex 版本源

- 新增 `backend/internal/service/openai_codex_client_identity.go`。
- 定义唯一生产版本字面量 `openAICodexClientVersion = "0.144.1"`。
- 由该值派生 `codexCLIUserAgent`、`codexCLIVersion`、`openAICodexProbeVersion` 和 `DefaultOpenAICodexUserAgent`。
- 从 `openai_gateway_service.go`、`account_usage_service.go`、`setting_gateway_runtime.go` 删除原有重复声明。
- 保持所有现有消费者的方法签名和调用位置不变。

## Step 2: 锁定身份一致性

- 新增 `openai_codex_client_identity_test.go`。
- 断言唯一版本为 `0.144.1`，且不低于任务约定的 GPT-5.6 最低版本。
- 断言 compact 版本、探测版本、CLI UA、TUI UA 都从同一版本源派生。
- 断言生产默认身份不再包含 `0.125.0`。

## Step 3: 覆盖 GPT-5.6 和关键请求路径

- 在 OAuth passthrough 请求测试中增加 Sol、Terra、Luna 表驱动用例：模型名原样转发，非 Codex UA 触发新版 CLI UA 兜底。
- 扩展模型目录默认版本测试，同时断言 query `client_version`、`Version` 和 CLI UA 一致；保留显式客户端版本透传测试。
- 增加或扩展 WebSocket header 测试，覆盖 OAuth 非 Codex UA 兜底使用新版 CLI UA，以及账号自定义 UA 不被无条件覆盖。
- 保留并运行现有 HTTP compact、passthrough compact、账号 compact 测试，它们已经断言 `codexCLIVersion` 和 `codexCLIUserAgent`。
- 如现有 Chat Completions bridge 测试未覆盖强制 Codex UA，则补一个 Luna 用例，验证 bridge 仍复用统一身份且模型映射不变。

## Step 4: 静态扫描与定向验证

- 扫描非测试生产 Go 文件，确认 `0.125.0` 不再参与 Codex 上游请求构造。
- 允许纯测试输入、日志解析样例或历史兼容样例保留旧版本，但必须确认它们不被生产代码引用。
- 运行定向身份、模型目录、compact、passthrough、WebSocket、账号测试和 404 冷却测试。

## Validation Commands

```bash
cd backend
go test -tags=unit ./internal/service -run 'TestOpenAICodexClientIdentity|TestFetchCodexModelsManifest|TestOpenAIGatewayService_OAuthPassthrough_.*GPT56|TestOpenAI.*Compact|TestAccountTestService_.*OpenAICompact|TestOpenAI.*WS.*Header|TestIsUpstreamModelNotFoundError|TestRateLimitService_HandleUpstreamError_ModelNotFoundUsesModelRateLimit|TestOpenAIModelNotFound_DoesNotRuntimeBlockWholeAccount'
```

```bash
cd backend
go test -tags=unit ./internal/service
golangci-lint run ./...
```

```bash
rg -n --glob '*.go' --glob '!**/*_test.go' '0\.125\.0' backend/internal
git diff --check
```

## Risk Files

- `backend/internal/service/openai_codex_client_identity.go`
- `backend/internal/service/openai_gateway_service.go`
- `backend/internal/service/account_usage_service.go`
- `backend/internal/service/setting_gateway_runtime.go`
- `backend/internal/service/openai_oauth_passthrough_test.go`
- `backend/internal/service/openai_codex_models_service_test.go`
- `backend/internal/service/openai_ws_*_test.go`
- `backend/internal/service/openai_gateway_chat_completions_test.go`

## Review Gates

- 版本字面量只能在唯一身份文件中出现一次。
- 所有新增测试验证实际请求头或上游请求体，不只验证常量彼此相等。
- 自定义 UA、显式模型目录 `client_version`、`force_codex_cli=false` 和现有路由策略必须保持。
- 不引入配置、数据库、前端或调度策略改动。

## Rollback Points

- 身份文件及三个原声明点可作为一个原子改动回滚。
- 各路径测试可独立回滚，不影响生产逻辑。
- 无 migration、持久化数据或外部配置需要恢复。
