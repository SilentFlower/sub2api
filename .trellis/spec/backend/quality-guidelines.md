# Quality Guidelines

> 后端代码质量、测试和评审标准。

---

## Overview

后端质量基线来自 `backend/.golangci.yml`、`backend/Makefile`、`DEV_GUIDE.md` 和现有测试组织。

常用命令：

```bash
cd backend
go test -tags=unit ./...
go test -tags=integration ./...
golangci-lint run ./...
go generate ./ent
```

`Makefile` 也提供：

- `make test-unit`
- `make test-integration`
- `make test-e2e-local`
- `make generate`

本地 Windows 没有 `make` 时，按 `DEV_GUIDE.md` 直接运行底层 `go test` 命令。

---

## Forbidden Patterns

- handler 直接导入 `internal/repository`、`gorm.io/gorm` 或 Redis。
- service 直接导入 `internal/repository`、`gorm.io/gorm` 或 Redis，除 `.golangci.yml` 中已有历史例外外不要新增例外。
- 修改已应用 migration 文件。
- 修改 Ent schema 后不运行 `go generate ./ent`。
- 新增 interface 方法后只改生产实现，不补测试 stub/mock。
- 在 API 中返回非统一 envelope，破坏前端 `apiClient` 解包逻辑。
- 通过字符串猜测 DTO、Ent 字段或配置字段。改动前必须读取定义。
- 在日志、错误或测试快照中暴露密钥、token、密码、支付凭据。

---

## Required Patterns

- Go 代码必须通过 gofmt。项目 formatter 配置保留 `gofmt`，不启用 gofmt simplify。
- 业务错误用 `internal/pkg/errors` 定义稳定 reason，handler 使用 `response.ErrorFrom`。
- 新增 DB 结构变化同时检查 migration、Ent schema、repository mapper、前端类型。
- 跨表写入要使用事务，优先参考 `userRepository.Create/Update` 的 Ent 事务模式。
- 高风险输入要在边界校验，service 层保持业务不变量，repository 层保证持久化约束。
- 测试应覆盖修复过的 bug 或新增业务分支，不写只验证 mock 被调用的空测试。

---

## Testing Requirements

按风险选择测试层级：

- 纯函数和业务计算：放在同 package `_test.go`，例如 `backend/internal/payment/amount_test.go`。
- repository 行为：优先复用现有 integration harness，例如 `backend/internal/repository/*_integration_test.go`。
- HTTP 路由和中间件：参考 `backend/internal/server/routes/*_test.go` 和 `backend/internal/server/middleware/*_test.go`。
- gateway、支付、认证等跨层流程：参考 `backend/internal/integration/e2e_*_test.go`。

构建标签约定：

- 单元测试：`go test -tags=unit ./...`
- 集成测试：`go test -tags=integration ./...`
- 本地 e2e：`go test -tags=e2e -v -timeout=300s ./internal/integration/...`

如果改了 interface，必须搜索测试 stub/mock 并补齐方法：

```bash
cd backend
rg "type .*Stub|type .*Mock" internal
```

---

## Code Review Checklist

评审时至少检查：

- 分层依赖是否符合 depguard。
- API 响应是否保持 `{code,message,reason,metadata,data}` envelope。
- 错误 reason 是否稳定，message 是否不会泄露敏感信息。
- migration 是否 forward-only、幂等，是否误改旧文件。
- Ent schema、生成代码、migration、DTO、前端类型是否同步。
- 是否存在跨层字段名漂移，例如后端 `page_size` 前端写成 `pageSize` 直接传 API。
- 测试是否覆盖新增分支、失败分支和历史 bug。
- 新增依赖是否必要，Go module 或 pnpm lock 是否同步。

---

## Common Mistakes

- `DEV_GUIDE.md` 中记录 CI Go `1.25.7`，而 `backend/go.mod` 当前为 `go 1.26.4`。涉及版本判断时先核对实际 CI 文件，不要直接改工具链版本。
- pnpm 和 npm 混用会导致前端 lock/node_modules 问题，后端改动同时触及前端依赖时仍要使用 pnpm。
- PowerShell 中 bcrypt hash 的 `$` 会被解释，执行 SQL 时优先写入文件再 `psql -f`。
- 批量修改账号或模型映射前要按平台分组，避免 OpenAI、Antigravity、Gemini 的映射策略互相覆盖。
