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

## Scenario: OpenAI codex_cli_only 403 诊断响应

### 1. Scope / Trigger

- Trigger: 修改 `OpenAIGatewayService.Forward`、`OpenAIGatewayService.ForwardAsChatCompletions` 或 `CodexClientRestrictionMessage` 时，必须保持 OpenAI 兼容错误响应与诊断字段一致。
- Scope: OpenAI `/v1/responses` 与 `/v1/chat/completions` 网关入口。
- Exception: 这两个入口模拟 OpenAI 协议错误格式，允许绕过后台 API envelope；不要套用 `{code,message,reason,data}`。

### 2. Signatures

```go
func writeCodexClientRestrictionForbidden(c *gin.Context, result CodexClientRestrictionDetectionResult)
func codexCLIOnlyRejectedRequestUserAgent(c *gin.Context) string
func CodexClientRestrictionMessage(result CodexClientRestrictionDetectionResult) string
```

### 3. Contracts

- HTTP status 必须是 `403`。
- 响应 JSON 必须以 `error` 为顶层字段，匹配 OpenAI 兼容错误形态：

```json
{
  "error": {
    "type": "forbidden_error",
    "message": "This account only allows Codex official clients. Request User-Agent: my-codex-wrapper/1.2.3",
    "request_user_agent": "my-codex-wrapper/1.2.3"
  }
}
```

- `error.request_user_agent` 只在入站 `User-Agent` trim 后非空时输出。
- 输出 `request_user_agent` 时，`error.message` 必须追加 `. Request User-Agent: <value>`，方便 CLI 客户端只展示 message 时也能定位来源。
- `User-Agent` 使用 `codexCLIOnlyHeaderValueMaxBytes` 截断，并保持 UTF-8 有效。
- 空白 `User-Agent` 不输出 `request_user_agent`，message 只保留 `CodexClientRestrictionMessage(result)`。
- 响应中不得暴露 `Authorization`、`Cookie`、token、完整请求体或未白名单请求头。

### 4. Validation & Error Matrix

- `codex_cli_only` 未开启 -> 不走该 403 响应。
- `codex_cli_only` 开启且客户端命中官方/白名单策略 -> 不走该 403 响应。
- `codex_cli_only` 开启且客户端未命中策略，`User-Agent` 非空 -> `403` + `error.request_user_agent` + message 追加诊断。
- `codex_cli_only` 开启且客户端未命中策略，`User-Agent` 为空或全空白 -> `403` + 不输出 `request_user_agent`。
- `User-Agent` 超过 `codexCLIOnlyHeaderValueMaxBytes` -> 截断后输出，不能破坏 UTF-8。

### 5. Good/Base/Bad Cases

- Good: 非官方包装器 UA 为 `my-codex-wrapper/1.2.3` 时，`/v1/responses` 与 `/v1/chat/completions` 都返回同一 OpenAI 兼容 403 shape。
- Good: 超长中文 UA 被截断后仍是合法 UTF-8。
- Base: 空白 UA 被拒绝时，只返回 `type` 与 `message`，不出现空字符串诊断字段。
- Bad: 为了统一后台风格返回 `{code:403,message:"..."}`，会破坏 OpenAI/Codex 客户端兼容性。
- Bad: 把完整 headers、body 或 token 放进 403 payload，属于敏感信息泄露。

### 6. Tests Required

```bash
cd backend
go test -tags=unit ./internal/service -run 'TestOpenAIGatewayService_Forward_CodexCLIOnlyForbidden'
```

断言点：

- `/v1/responses` 与 `/v1/chat/completions` 拒绝路径状态码都是 `403`。
- 非空 UA 输出 `error.request_user_agent`，且 message 追加同一值。
- 空白 UA 不输出 `error.request_user_agent`。
- 超长 UA 被截断且仍是有效 UTF-8。

### 7. Wrong vs Correct

#### Wrong

```go
c.JSON(http.StatusForbidden, gin.H{
	"code":    http.StatusForbidden,
	"message": "This account only allows Codex official clients",
})
```

#### Correct

```go
writeCodexClientRestrictionForbidden(c, restrictionResult)
```

---

## Common Mistakes

- 不要直接 `c.JSON(status, gin.H{...})` 绕过统一 envelope，除非兼容外部协议必须如此。
- 不要在 handler 中吞掉错误后继续执行。
- 不要把 token、密钥、完整上游响应体直接放入错误 message 或 metadata。
- 不要为了前端展示而新增不稳定的英文 message 判断。稳定判断应依赖 `reason` 或明确字段。
