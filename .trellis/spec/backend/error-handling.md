# Error Handling

> 本项目后端错误传播和 API 错误响应约定。

---

## Overview

后端 API 使用统一响应 envelope，错误优先通过 `internal/pkg/errors.ApplicationError` 表达业务语义，再由 `internal/pkg/response` 转成 HTTP JSON。

标准响应结构来自 `backend/internal/pkg/response/response.go`：

```go
type Response struct {
	Code     int               `json:"code"`
	Message  string            `json:"message"`
	Reason   string            `json:"reason,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Data     any               `json:"data,omitempty"`
}
```

成功响应 `code=0`，错误响应的 `code` 通常等于 HTTP status code。前端 `frontend/src/api/client.ts` 会自动解包 `{ code, message, data }`，因此后端新增 API 必须保持这个 envelope。

---

## Error Types

业务错误类型定义在 `backend/internal/pkg/errors/`：

- `ApplicationError` 包含 `Code`、`Reason`、`Message`、`Metadata` 和可选 cause。
- `BadRequest`、`Unauthorized`、`Forbidden`、`NotFound`、`Conflict`、`TooManyRequests`、`ServiceUnavailable`、`GatewayTimeout` 等构造函数会映射到对应 HTTP status。
- `WithCause` 用于保留底层错误，`WithMetadata` 用于向客户端返回结构化补充信息。
- `FromError` 支持 wrapped error，不是 `ApplicationError` 的错误会退化为 500 `internal error`。

service 层常见做法是在包级变量中定义可复用错误，例如 `backend/internal/service/user.go`：

```go
ErrUserNotFound = infraerrors.NotFound("USER_NOT_FOUND", "user not found")
ErrPasswordIncorrect = infraerrors.BadRequest("PASSWORD_INCORRECT", "current password is incorrect")
```

---

## Error Handling Patterns

handler 中优先使用 `response.ErrorFrom(c, err)`，它会调用 `errors.ToHTTP` 并保持 response envelope：

```go
cfg, err := h.configService.GetPaymentConfig(c.Request.Context())
if err != nil {
	response.ErrorFrom(c, err)
	return
}
response.Success(c, cfg)
```

请求绑定错误使用 `response.BadRequest` 或业务错误，不要 panic：

```go
if err := c.ShouldBindJSON(&req); err != nil {
	response.BadRequest(c, "Invalid request: "+err.Error())
	return
}
```

repository 层要把持久化错误翻译为 service 层业务错误。示例见 `backend/internal/repository/user_repo.go` 的 `translatePersistenceError(err, service.ErrUserNotFound, service.ErrEmailExists)`。

service 层包装底层错误时使用 `%w` 保留 error chain，例如 `fmt.Errorf("get user: %w", err)`，不要丢失 cause。

---

## API Error Responses

新增 API 错误响应必须满足：

- 仍由 `response.Error`、`response.ErrorWithDetails` 或 `response.ErrorFrom` 输出。
- 客户端可见错误包含稳定 `reason`，例如 `USER_NOT_FOUND`、`PASSWORD_INCORRECT`。
- `metadata` 只放可安全暴露给前端的结构化字段。
- 500 错误不要把敏感内部详情直接作为 `message` 返回给客户端。

panic 处理由 `backend/internal/server/middleware/recovery.go` 负责。它会把 panic 转成统一 500 envelope；broken pipe 或 client closed 不再写响应。

---

## Client Compatibility

前端 `apiClient` 对错误有固定假设：

- `code === 0` 被视为成功，返回 `data`。
- 非 0 会 reject 一个对象，包含 `status`、`code`、`message`、`reason`、`metadata`。
- 特殊状态如 401、423、ops disabled 404 有拦截器逻辑。

因此后端不要为同一类 API 临时返回裸对象、HTML 错误页或不含 `code/message` 的 JSON。

---

## Common Mistakes

- 不要直接 `c.JSON(status, gin.H{...})` 绕过统一 envelope，除非兼容外部协议必须如此。
- 不要在 handler 中吞掉错误后继续执行。
- 不要把 token、密钥、完整上游响应体直接放入错误 message 或 metadata。
- 不要为了前端展示而新增不稳定的英文 message 判断。稳定判断应依赖 `reason` 或明确字段。
