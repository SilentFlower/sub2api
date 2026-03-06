# 错误处理

> 项目统一的错误处理体系和规范。

---

## 概述

项目使用自定义的 `ApplicationError` 作为统一错误类型，错误从 Repository 层产生，经过 Service 层传递，在 Handler 层统一转为 HTTP JSON 响应。

**核心文件**：
- `internal/pkg/errors/errors.go` — 核心错误结构体
- `internal/pkg/errors/types.go` — HTTP 状态码工厂函数
- `internal/pkg/errors/http.go` — 错误转 HTTP 响应
- `internal/pkg/response/response.go` — Gin 响应工具
- `internal/repository/error_translate.go` — 数据库错误翻译

---

## 错误类型

### ApplicationError 结构

```go
// internal/pkg/errors/errors.go
type Status struct {
    Code     int32             `json:"code"`
    Reason   string            `json:"reason,omitempty"`
    Message  string            `json:"message"`
    Metadata map[string]string `json:"metadata,omitempty"`
}

type ApplicationError struct {
    Status
    cause error  // 内部错误链（不暴露给客户端）
}
```

### 工厂函数

每个 HTTP 状态码对应一对构造/检测函数（`internal/pkg/errors/types.go`）：

| 工厂函数 | 状态码 | 用途 |
|---------|--------|------|
| `BadRequest(reason, message)` | 400 | 客户端请求参数错误 |
| `Unauthorized(reason, message)` | 401 | 未认证 |
| `Forbidden(reason, message)` | 403 | 无权限 |
| `NotFound(reason, message)` | 404 | 资源不存在 |
| `Conflict(reason, message)` | 409 | 资源冲突（如唯一约束） |
| `InternalServer(reason, message)` | 500 | 服务器内部错误 |
| `ServiceUnavailable(reason, message)` | 503 | 服务暂不可用 |
| `GatewayTimeout(reason, message)` | 504 | 网关超时 |
| `ClientClosed(reason, message)` | 499 | 客户端提前关闭连接 |

对应的检测函数：`IsBadRequest(err)`, `IsNotFound(err)`, `IsUnauthorized(err)` 等。

---

## 错误处理流程

```
数据库错误
  ↓ translatePersistenceError()
ApplicationError（带 cause 链）
  ↓ Service 层透传或包装
ApplicationError
  ↓ response.ErrorFrom(c, err)
JSON 响应 { code, message, reason, metadata }
```

### 1. Repository 层：数据库错误翻译

```go
// 正确写法
func (r *accountRepository) GetByID(ctx context.Context, id int) (*ent.Account, error) {
    account, err := r.client.Account.Get(ctx, id)
    if err != nil {
        return nil, translatePersistenceError(err,
            infraerrors.NotFound("ACCOUNT_NOT_FOUND", "账号不存在"),
            nil, // 此操作不会产生冲突错误
        )
    }
    return account, nil
}
```

### 2. Service 层：业务逻辑错误

```go
// 正确写法 — 使用工厂函数创建业务错误
func (s *AccountService) CreateAccount(ctx context.Context, req CreateAccountRequest) error {
    if req.Name == "" {
        return errors.BadRequest("INVALID_NAME", "账号名称不能为空")
    }
    // ...
    return nil
}
```

### 3. Handler 层：统一响应

```go
// 正确写法 — 使用 response.ErrorFrom 自动映射
func (h *AccountHandler) GetAccount(c *gin.Context) {
    account, err := h.accountService.GetByID(c.Request.Context(), id)
    if response.ErrorFrom(c, err) {
        return  // ErrorFrom 返回 true 表示已写入错误响应
    }
    response.Success(c, account)
}
```

---

## API 错误响应格式

### 标准响应结构

```go
// internal/pkg/response/response.go
type Response struct {
    Code     int               `json:"code"`
    Message  string            `json:"message"`
    Reason   string            `json:"reason,omitempty"`
    Metadata map[string]string `json:"metadata,omitempty"`
    Data     any               `json:"data,omitempty"`
}
```

### 成功响应（Code=0）

```json
{
    "code": 0,
    "message": "success",
    "data": { ... }
}
```

### 错误响应（Code=HTTP 状态码）

```json
{
    "code": 404,
    "message": "账号不存在",
    "reason": "ACCOUNT_NOT_FOUND"
}
```

### 响应工具函数

| 函数 | 用途 |
|------|------|
| `response.Success(c, data)` | 200 成功 |
| `response.Created(c, data)` | 201 创建成功 |
| `response.Paginated(c, items, total, page, pageSize)` | 分页数据 |
| `response.ErrorFrom(c, err)` | 从 error 自动映射状态码 |
| `response.BadRequest(c, msg)` | 400 快捷方法 |
| `response.Unauthorized(c, msg)` | 401 快捷方法 |

---

## Panic 恢复

全局 Recovery 中间件（`internal/server/middleware/recovery.go`）捕获所有 Panic，返回标准 JSON 错误信封。检测到 Broken Pipe 时不写响应。

---

## 常见错误

### 1. 直接返回原始错误给客户端

```go
// ❌ 错误 — 原始错误可能暴露内部信息
func (h *Handler) Get(c *gin.Context) {
    data, err := h.service.Get(ctx)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
}

// ✅ 正确 — 使用 response.ErrorFrom 自动映射
func (h *Handler) Get(c *gin.Context) {
    data, err := h.service.Get(ctx)
    if response.ErrorFrom(c, err) {
        return
    }
    response.Success(c, data)
}
```

### 2. Repository 未翻译数据库错误

```go
// ❌ 错误 — 原始 Ent 错误透传到 Service/Handler
return nil, err

// ✅ 正确 — 使用 translatePersistenceError
return nil, translatePersistenceError(err,
    infraerrors.NotFound("XXX_NOT_FOUND", "xxx不存在"),
    infraerrors.Conflict("XXX_CONFLICT", "xxx已存在"),
)
```

### 3. 500 错误未记录日志

`response.ErrorFrom` 内部已处理：当状态码 ≥ 500 时自动输出 `[ERROR]` 日志（含脱敏处理），无需在 Handler 层重复记录。
