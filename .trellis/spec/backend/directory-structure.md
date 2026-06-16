# Directory Structure

> 本项目后端代码的组织方式。

---

## Overview

后端位于 `backend/`，是 Go 项目，模块名为 `github.com/Wei-Shaw/sub2api`。当前实际结构是 Gin HTTP 层、service 业务层、repository 基础设施层、Ent 生成代码和 SQL migrations 并存。

新增功能时优先沿用已有层次：

- HTTP 路由注册放在 `backend/internal/server/routes/`。
- Gin handler 放在 `backend/internal/handler/`，管理员子域优先放在 `backend/internal/handler/admin/`。
- 请求/响应 DTO 和实体映射放在 `backend/internal/handler/dto/`。
- 业务接口、业务模型和服务实现放在 `backend/internal/service/`。
- Ent、Redis、外部存储、迁移执行等基础设施实现放在 `backend/internal/repository/`。
- 第三方协议适配和可复用基础包放在 `backend/internal/pkg/`。
- 小型通用工具放在 `backend/internal/util/`，例如日志脱敏、响应头、URL 校验。
- 数据库 schema 定义放在 `backend/ent/schema/`，生成代码在 `backend/ent/`。
- 手写 SQL 迁移放在 `backend/migrations/`，通过 embed 进入二进制。

示例依据：

- `backend/internal/server/router.go` 统一装配中间件、前端静态资源和 `/api/v1` 路由。
- `backend/internal/server/routes/admin.go` 将 admin 路由按业务域拆成 `register*Routes`。
- `backend/internal/handler/payment_handler.go` 展示用户侧 payment handler 的依赖注入、请求绑定和响应返回方式。
- `backend/internal/service/user.go` 同时定义 service 模型、repository interface 和业务 service。
- `backend/internal/repository/user_repo.go` 实现 service 层声明的 repository interface。

---

## Directory Layout

```text
backend/
├── cmd/
│   ├── server/          # 服务入口、版本生成
│   └── jwtgen/          # 辅助命令
├── ent/
│   ├── schema/          # Ent schema 源文件
│   └── ...              # go generate 生成的 Ent 查询、创建、更新代码
├── internal/
│   ├── config/          # 配置结构和校验
│   ├── domain/          # 领域常量和小型领域逻辑
│   ├── handler/         # Gin handler、DTO、admin handler
│   ├── integration/     # e2e / integration 测试
│   ├── middleware/      # 历史中间件目录，新增优先看 server/middleware
│   ├── model/           # 少量跨层模型
│   ├── payment/         # 支付领域纯逻辑和 provider 抽象
│   ├── pkg/             # 协议适配、日志、错误、分页等可复用包
│   ├── repository/      # Ent/SQL/Redis/外部服务基础设施适配
│   ├── server/          # HTTP server、router、routes、middleware
│   ├── service/         # 业务服务、端口接口、业务数据结构
│   ├── setup/           # 初始安装向导相关逻辑
│   ├── testutil/        # 测试辅助
│   └── util/            # 小型工具包
├── migrations/          # forward-only SQL migrations
└── resources/           # 内置资源，例如模型定价数据
```

---

## Module Organization

分层依赖方向按现有 lint 约束执行：

- `handler` 可以依赖 `service`、`pkg/response`、`pkg/errors` 等，但不能直接依赖 `repository`、`gorm` 或 Redis。
- `service` 定义业务接口并调用接口，不应直接依赖 `repository`。`backend/.golangci.yml` 对 `internal/service/**` 配置了 depguard 例外列表，新增代码不要扩大例外。
- `repository` 实现 service 层接口，负责 Ent 查询、SQL、Redis、外部基础设施调用和持久化错误翻译。
- `server/routes` 只做路由分组和中间件挂载，不放业务逻辑。

典型新增 API 的落点：

1. 在 `backend/internal/service/<domain>.go` 定义请求结构、返回结构、repository interface 和 service 方法。
2. 在 `backend/internal/repository/<domain>_repo.go` 实现 interface。
3. 在 `backend/internal/handler/<domain>_handler.go` 绑定 JSON、调用 service、用 `response.Success` 或 `response.ErrorFrom` 返回。
4. 在 `backend/internal/server/routes/<domain>.go` 或已有 route 文件注册路径。
5. 如果涉及 DB 结构变化，新增 `backend/migrations/NNN_description.sql`，必要时更新 `backend/ent/schema/*.go` 后运行 `go generate ./ent`。

---

## Naming Conventions

- Go package 使用小写短名，目录名与 package 对齐，例如 `internal/pkg/logredact`、`internal/pkg/urlvalidator`。
- 文件名使用 snake_case，例如 `payment_webhook_handler.go`、`user_platform_quota_repo.go`。
- 测试文件与被测行为靠近，使用 `_test.go`，集成测试可带 `_integration_test.go`。
- 路由注册函数使用 `Register*Routes` 或内部 `register*Routes`，例如 `RegisterPaymentRoutes`、`registerOpsRoutes`。
- repository 构造函数使用 `New*Repository`，返回 service 层接口，例如 `NewUserRepository(client, sqlDB) service.UserRepository`。
- service 构造函数使用 `New*Service`，handler 构造函数使用 `New*Handler`。
- 后端 JSON 字段沿用 snake_case，例如 `page_size`、`group_id`、`created_at`。

---

## Examples

路由装配示例来自 `backend/internal/server/router.go`：

```go
v1 := r.Group("/api/v1")
routes.RegisterAuthRoutes(v1, h, jwtAuth, redisClient, settingService)
routes.RegisterUserRoutes(v1, h, jwtAuth, settingService)
routes.RegisterAdminRoutes(v1, h, adminAuth, settingService)
```

handler 返回约定示例来自 `backend/internal/handler/payment_handler.go`：

```go
cfg, err := h.configService.GetPaymentConfig(c.Request.Context())
if err != nil {
	response.ErrorFrom(c, err)
	return
}
response.Success(c, cfg)
```

repository 事务模式示例来自 `backend/internal/repository/user_repo.go`：

```go
tx, err := r.client.Tx(ctx)
if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
	return err
}
defer func() { _ = tx.Rollback() }()
```

---

## Common Mistakes

- 不要在 handler 中直接查 DB 或 Redis。`backend/.golangci.yml` 已通过 depguard 禁止 `internal/handler/**` 导入 `internal/repository` 和 Redis。
- 不要在 service 中新增 repository 的直接导入。先在 service 层定义端口接口，再由 repository 实现。
- 不要把新业务逻辑塞进 `server/routes`。routes 只负责路径、HTTP 方法、中间件和 handler 绑定。
- 不要手写使用不存在的 Ent 字段或 DTO 字段。改动前必须读取对应 `ent/schema`、service model、DTO mapper。
