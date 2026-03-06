# 目录结构

> 后端代码的组织方式和命名规范。

---

## 概述

本项目后端使用 **Go** 语言，采用经典的三层架构（Handler → Service → Repository），通过 **Google Wire** 进行编译期依赖注入，使用 **Ent ORM** 进行数据库访问，HTTP 框架为 **Gin**。

---

## 目录布局

```
backend/
├── cmd/
│   ├── server/                  # 主程序入口
│   │   ├── main.go              # 启动逻辑（两阶段日志初始化 + 优雅关闭）
│   │   ├── wire.go              # Wire DI 定义（Application 结构体 + injector）
│   │   ├── wire_gen.go          # Wire 自动生成代码（勿手动编辑）
│   │   └── wire_gen_test.go     # Wire 生成代码的编译检查
│   └── jwtgen/                  # JWT 生成工具（独立 CLI）
│       └── main.go
├── ent/                         # Ent ORM 自动生成代码（勿手动编辑，除 schema/）
│   ├── schema/                  # 【手写】数据库 Schema 定义
│   │   ├── account.go
│   │   ├── user.go
│   │   ├── api_key.go
│   │   ├── mixins/              # Schema 复用组件
│   │   │   ├── time.go          # created_at, updated_at 自动填充
│   │   │   └── soft_delete.go   # 软删除（Interceptor + Hook 方式）
│   │   └── ...
│   ├── {entity}/                # 每个实体的常量和 Where 条件（自动生成）
│   ├── {entity}_create.go       # 创建操作（自动生成）
│   ├── {entity}_query.go        # 查询操作（自动生成）
│   ├── {entity}_update.go       # 更新操作（自动生成）
│   ├── {entity}_delete.go       # 删除操作（自动生成）
│   ├── migrate/                 # 数据库迁移（自动生成）
│   ├── runtime/                 # 运行时注册（自动生成）
│   └── ...
├── internal/
│   ├── config/                  # 配置加载（Viper + mapstructure）
│   │   ├── config.go            # Config 结构体定义 + 加载逻辑
│   │   └── wire.go              # Config ProviderSet
│   ├── domain/                  # 领域常量和模型
│   │   ├── constants.go         # 全局常量定义
│   │   └── announcement.go      # 领域模型
│   ├── handler/                 # HTTP 处理器（用户端）
│   │   ├── handler.go           # Handlers 聚合结构体
│   │   ├── auth_handler.go      # 认证相关 Handler
│   │   ├── gateway_handler.go   # 网关 Handler
│   │   ├── logging.go           # Handler 层日志工具函数
│   │   ├── wire.go              # Handler ProviderSet
│   │   ├── dto/                 # 数据传输对象
│   │   │   ├── types.go         # DTO 结构体定义
│   │   │   ├── mappers.go       # Entity ↔ DTO 映射函数
│   │   │   └── settings.go      # 设置相关 DTO
│   │   └── admin/               # 管理端 Handler
│   │       ├── account_handler.go
│   │       ├── ops_handler.go
│   │       └── ...
│   ├── middleware/              # 功能中间件（如限流器）
│   │   └── rate_limiter.go      # Redis 滑动窗口限流
│   ├── model/                   # 内部模型
│   │   └── error_passthrough_rule.go
│   ├── repository/              # 数据访问层
│   │   ├── account_repo.go      # 账号仓库实现
│   │   ├── billing_cache.go     # 计费缓存
│   │   ├── error_translate.go   # DB 错误转 ApplicationError
│   │   ├── ent.go               # Ent Client 初始化
│   │   ├── wire.go              # Repository ProviderSet
│   │   └── ...
│   ├── service/                 # 业务逻辑层
│   │   ├── gateway_service.go   # 网关服务
│   │   ├── account.go           # 账号服务
│   │   ├── wire.go              # Service ProviderSet
│   │   ├── openai_ws_v2/        # OpenAI WebSocket 子模块
│   │   └── prompts/             # 提示词模板
│   ├── server/                  # HTTP Server 配置
│   │   ├── http.go              # Server 创建 + ProviderSet
│   │   ├── router.go            # 路由编排（全局中间件挂载）
│   │   ├── middleware/          # 全局/框架级中间件
│   │   │   ├── jwt_auth.go      # JWT 认证
│   │   │   ├── admin_auth.go    # 管理员权限
│   │   │   ├── api_key_auth.go  # API Key 认证
│   │   │   ├── cors.go          # CORS 配置
│   │   │   ├── recovery.go      # Panic 恢复
│   │   │   ├── security_headers.go  # 安全响应头
│   │   │   ├── request_logger.go    # 请求日志
│   │   │   └── wire.go          # Middleware ProviderSet
│   │   └── routes/              # 路由注册（按模块分文件）
│   │       ├── auth.go          # /auth/* 路由
│   │       ├── gateway.go       # /v1/* 网关路由
│   │       ├── admin.go         # /admin/* 管理路由
│   │       ├── user.go          # /user/* 用户路由
│   │       └── ...
│   ├── pkg/                     # 内部工具包
│   │   ├── errors/              # 统一错误类型
│   │   ├── logger/              # 日志系统（Zap）
│   │   ├── response/            # HTTP 响应工具
│   │   ├── openai/              # OpenAI API 客户端
│   │   ├── claude/              # Claude API 客户端
│   │   ├── gemini/              # Gemini API 客户端
│   │   ├── antigravity/         # Antigravity API 客户端
│   │   ├── httpclient/          # 通用 HTTP 客户端
│   │   ├── pagination/          # 分页工具
│   │   └── ...
│   ├── setup/                   # 首次启动设置向导
│   ├── web/                     # 前端静态资源嵌入
│   ├── testutil/                # 测试工具
│   │   ├── fixtures.go          # 测试数据构造
│   │   ├── httptest.go          # HTTP 测试辅助
│   │   └── stubs.go             # 接口桩实现
│   ├── integration/             # 端到端集成测试
│   │   └── e2e_gateway_test.go
│   └── util/                    # 通用工具
│       ├── logredact/           # 日志脱敏
│       ├── responseheaders/     # 响应头工具
│       └── urlvalidator/        # URL 校验
├── migrations/                  # 数据库迁移脚本
│   └── migrations.go
├── resources/                   # 静态资源文件
│   └── model-pricing/          # 模型定价数据
└── config.yaml                  # 默认配置文件
```

---

## 模块组织

### 新增功能模块的标准步骤

1. **Schema**: 在 `ent/schema/` 中定义实体，运行 `go generate ./ent` 生成代码
2. **Repository**: 在 `internal/repository/` 中创建 `{资源}_repo.go`，实现 Service 层定义的接口
3. **Service**: 在 `internal/service/` 中创建 `{资源}_service.go` 或 `{资源}.go`
4. **Handler**: 在 `internal/handler/` 中创建 `{资源}_handler.go`
5. **Route**: 在 `internal/server/routes/` 中注册路由
6. **Wire**: 在各层的 `wire.go` 中注册 Provider

### 分层依赖规则（由 depguard linter 强制执行）

```
Handler → Service → Repository → Ent Client
   ↓         ↓          ↓
  禁止直接跨层依赖（Handler 不能直接调用 Repository）
  禁止引入 gorm / redis 包（只允许在 Repository 层使用）
```

---

## 命名规范

| 类别 | 规则 | 示例 |
|------|------|------|
| Go 文件 | snake_case | `auth_handler.go`, `account_repo.go` |
| Handler 文件 | `{资源}_handler.go` | `gateway_handler.go` |
| Repository 文件 | `{资源}_repo.go` 或 `{资源}_cache.go` | `account_repo.go`, `billing_cache.go` |
| Service 文件 | `{资源}_service.go` 或 `{资源}.go` | `gateway_service.go`, `account.go` |
| Schema 文件 | snake_case（与数据库表对应） | `api_key.go`, `usage_log.go` |
| 测试文件 | `{文件名}_test.go` | `account_repo_test.go` |
| Wire 文件 | 固定为 `wire.go` | 每个需要 DI 的包一个 |

---

## 示例

### 典型的新增功能文件清单

以添加"公告"功能为例：

```
ent/schema/announcement.go            # Schema 定义
internal/repository/announcement_repo.go  # 数据访问
internal/service/announcement.go       # 业务逻辑
internal/handler/admin/announcement_handler.go  # 管理端 Handler
internal/server/routes/admin.go        # 路由注册（在现有文件中添加）
```
