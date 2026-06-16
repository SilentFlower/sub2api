# Logging Guidelines

> 本项目后端日志记录约定。

---

## Overview

后端日志封装在 `backend/internal/pkg/logger/`，底层使用 zap，并桥接 Go 标准库 `log` 和 `log/slog`。启动时通过 `logger.Init` 或 `logger.InitBootstrap` 初始化，全局访问入口是：

- `logger.L()` 返回 `*zap.Logger`
- `logger.S()` 返回 `*zap.SugaredLogger`
- `logger.With(...)` 返回带字段 logger
- `logger.IntoContext` / `logger.FromContext` 用于 request-scoped logger

Gin 请求入口由 `backend/internal/server/middleware/request_logger.go` 注入 request scoped logger，并设置 `X-Request-ID`。

---

## Log Levels

项目定义的等级来自 `backend/internal/pkg/logger/logger.go`：

- `debug`：本地或临时诊断信息，不能依赖它作为业务审计来源。
- `info`：正常生命周期事件、重要状态变化。
- `warn`：可恢复但需要关注的问题，例如配置缺失后降级、上游暂时异常。
- `error`：请求失败、后台任务失败、数据处理失败等需要排查的问题。
- `fatal`：进程无法继续运行时使用。

运行时日志级别支持调整，相关实现见 `logger.SetLevel` 和 ops runtime logging API。

---

## Structured Logging

新增结构化日志优先使用 zap field，不要把所有上下文拼进 message：

```go
requestLogger := logger.With(
	zap.String("component", "http"),
	zap.String("request_id", requestID),
	zap.String("path", c.Request.URL.Path),
	zap.String("method", c.Request.Method),
)
```

常用字段：

- `component`：模块名，例如 `http`、`payment`、`ops`。
- `request_id`：服务端生成或传入的请求 ID。
- `client_request_id`：客户端请求 ID。
- `path`、`method`：HTTP 上下文。
- 与业务相关的安全 ID，例如 `user_id`、`order_id`、`provider_key`，避免记录完整密钥。

`logger.WriteSinkEvent` 可绕过全局级别写入日志 sink，适用于 ops 系统日志索引等“业务可观测性入库”和“输出级别”需要解耦的场景。

---

## What to Log

应该记录：

- 服务启动、配置加载失败、迁移失败、后台任务失败。
- 支付回调、订单状态变化、退款、履约等关键状态转换。
- 上游 API 调用失败的分类信息和可安全排查字段。
- 安全和合规相关事件，例如管理员确认、权限失败、异常流量。
- 自动降级行为，例如内嵌前端配置注入失败后回退 legacy mode。

示例：`backend/internal/server/router.go` 在前端 server 创建失败时使用标准库 log 输出 warning。标准库 log 会被 logger bridge 接管。

---

## What NOT to Log

禁止记录：

- API key、access token、refresh token、JWT、支付密钥、webhook secret。
- 完整 Authorization header、Cookie、Set-Cookie。
- 明文密码、TOTP secret、OAuth code。
- 未脱敏的用户隐私数据或完整上游响应体。

需要输出错误详情时使用 `backend/internal/util/logredact` 脱敏。`response.ErrorFrom` 对 500 错误日志已经调用 `logredact.RedactText(err.Error())`。

---

## Common Mistakes

- 不要在热路径中大量 `fmt.Printf`。标准库 log 已桥接，但结构化日志仍优先。
- 不要把日志作为控制流或测试断言的唯一依据。
- 不要在库函数中静默吞掉错误，只记录日志后继续返回成功。
- 不要在循环内打印高频 debug/info 日志，尤其是 gateway、usage、streaming 相关路径。
