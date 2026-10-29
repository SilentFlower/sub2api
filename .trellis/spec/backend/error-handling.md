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

## Scenario: Responses 流式错误执行账号停止状态码规则

### 1. Scope / Trigger

- 修改 HTTP `/v1/responses` 原生或透传 SSE 的 `error`、`response.failed` 分类、终止处理或账号副作用时，必须同时检查两个入口。
- 共用分类器还服务于非流式请求接收上游 SSE 的路径，必须保持其错误 JSON 转换行为。
- 本契约不扩展 Chat Completions、WebSocket 或全局账号错误策略。

### 2. Signatures

```go
func isOpenAIStreamCreditBalanceExhausted(payload []byte) bool
func (s *OpenAIGatewayService) handleOpenAIStreamTerminalAccountSideEffects(
	c *gin.Context,
	account *Account,
	payload []byte,
	message string,
	headers http.Header,
	canonicalModel ...string,
) (int, bool)
```

实现入口位于 `backend/internal/service/openai_gateway_passthrough.go`；原生流式消费者位于 `backend/internal/service/openai_gateway_response_handling.go`。

### 3. Contracts

- 余额耗尽识别要求合法 JSON：依次读取 `response.error.code`、`error.code`，缺失时再读顶层 `code`；匹配 `credit_balance_exhausted` 或 `insufficient_quota`，或嵌套错误 `type=insufficient_quota`。不得仅凭自由文本中出现余额、配额关键词识别。
- 保持既有 `openAIStreamFailureStatus` 状态推断。示例这种没有显式 HTTP 状态的余额错误仍归为 `502`，不得为了识别余额错误改成 `429` 或 `402`，导致已配置的 `[502]` 无法命中。
- 保留既有 `401`、账号级 `403`、`429`、`529` 分支。其它状态只有在 API Key 账号开启自定义错误码、命中列表且合法载荷通过上游失败分类时，才进入账号错误入口；空列表沿用“不过滤状态码”的既有语义。
- 沿用参数、策略、上下文、容量降载等请求级错误排除。无效 JSON、普通断流的空载荷或本地合成 `502` 不能据此永久停用正常账号。
- 复用 `handleOpenAIAccountUpstreamError` 和既有停调流程。自定义状态码分支的 `response.error` 必须归一化为账号错误入口可读取的形态，保留错误原因；持久化由 repository `SetError` 设置 `status=error`、`schedulable=false`，并通知、同步调度快照，不直接另写数据库。
- 首次输出前的裸 `error` 也必须经过账号策略；仍可安全切号时沿用既有 failover。已经输出后只处理终止错误和账号副作用，不能重放请求。需要等待权威终止事件的路径挂起裸错误，随后失败或 EOF 才执行一次；成功 `response.completed` / `response.done` 必须清除挂起错误。
- 若账号错误入口已决定停用，则同账号池内重试资格必须为 false，即使池模式显式允许重试 `502`。不能在停用后继续消耗同一账号。

### 4. Validation & Error Matrix

| 条件 | 预期 |
| --- | --- |
| 结构化余额错误、开关开启、列表 `[502]` | 按 `502` 执行既有停调流程 |
| 同一错误，开关关闭或列表未命中 | 不因自定义停止规则停用 |
| 开关开启、空列表、可归因的上游失败 | 沿用既有不过滤语义 |
| 首次输出前裸 `error`，或最终 `response.failed` | 两个流式入口均执行适用的账号策略 |
| 已输出后失败 | 执行适用的停调副作用，保留已收到的 usage，不重放 |
| 等待权威终止的裸错误后成功完成 | 取消挂起的停调副作用 |
| 普通断流、无效 JSON、文本回显或请求级错误 | 不误停正常账号 |
| 池模式显式允许重试 `502`，同时命中停用 | 禁止同账号重试 |

### 5. Scenarios and Examples

API Key 账号开启停止状态码并配置 `[502]`，上游通过 SSE 返回：

```json
{
  "type": "error",
  "error": {
    "code": "credit_balance_exhausted",
    "type": "insufficient_quota",
    "message": "You have no credits remaining."
  }
}
```

正确行为：按既有 `502` 推断进入账号错误入口，持久化错误状态与停止调度，并保留余额耗尽原因。

错误做法：只让监控显示 `502`，或只扩充账号 helper 的状态码分支，却让首次输出前的裸 `error` 提前返回。两者都会留下配置存在但未实际执行的路径。

### 6. Tests Required

```bash
cd backend
go test -tags=unit ./internal/service -run '^TestOpenAIStreamErrorPolicy' -count=1
```

- 覆盖原生与透传、裸 `error` 与 `response.failed`、输出前后、开关与列表过滤，以及非流式 SSE 转 JSON。
- 断言持久化错误状态、后续读取账号的调度资格、错误原因与另一健康账号资格；不能仅断言日志或运行时通知次数。
- 断言停用优先于显式配置的池内重试，并保留输出后的 usage。
- 覆盖结构化嵌套字段、顶层 `code`、无效 JSON、文本回显、请求级错误、普通断流和权威成功终止的负例。

---

## Common Mistakes

- 不要直接 `c.JSON(status, gin.H{...})` 绕过统一 envelope，除非兼容外部协议必须如此。
- 不要在 handler 中吞掉错误后继续执行。
- 不要把 token、密钥、完整上游响应体直接放入错误 message 或 metadata。
- 不要为了前端展示而新增不稳定的英文 message 判断。稳定判断应依赖 `reason` 或明确字段。
