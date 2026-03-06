# 日志规范

> 日志系统的使用方式和最佳实践。

---

## 概述

- **日志库**: `go.uber.org/zap`
- **封装位置**: `internal/pkg/logger/`
- **输出方式**: stdout + 文件轮转（`lumberjack`）
- **格式**: JSON（生产环境）或 Console（开发环境）

---

## 两阶段初始化

```go
// main.go 第一阶段：引导日志（仅 stdout，在配置加载前使用）
logger.InitBootstrap()

// main.go 第二阶段：正式日志（从配置读取完整选项）
logger.Init(logger.OptionsFromConfig(cfg.Log))
```

---

## 使用方式

### 全局日志

```go
import "github.com/Wei-Shaw/sub2api/internal/pkg/logger"

// 获取全局 Logger
log := logger.L()
log.Info("操作完成", zap.String("account_id", id))

// 获取 SugaredLogger（模板语法）
slog := logger.S()
slog.Infof("处理了 %d 个请求", count)
```

### Context 传播

```go
// 将 Logger 注入 Context（通常在中间件中完成）
ctx = logger.IntoContext(ctx, log.With(zap.String("request_id", rid)))

// 从 Context 获取 Logger
log := logger.FromContext(ctx)
log.Info("处理请求")
```

### Handler 层快捷方式

```go
// internal/handler/logging.go
func requestLogger(c *gin.Context, component string, fields ...zap.Field) *zap.Logger {
    base := logger.L()
    if c != nil && c.Request != nil {
        base = logger.FromContext(c.Request.Context())
    }
    if component != "" {
        fields = append([]zap.Field{zap.String("component", component)}, fields...)
    }
    return base.With(fields...)
}

// 使用
log := requestLogger(c, "account-handler")
log.Info("创建账号", zap.String("name", req.Name))
```

---

## 日志级别

| 级别 | 使用场景 | 示例 |
|------|---------|------|
| `Debug` | 调试信息，仅开发环境 | 请求/响应详细数据、中间状态 |
| `Info` | 正常业务事件 | 用户登录、账号创建、定时任务完成 |
| `Warn` | 可恢复的异常情况 | Redis 连接失败但 Fail-Open、Token 即将过期 |
| `Error` | 不可恢复的错误 | 数据库查询失败、外部 API 调用失败 |

### 级别使用规范

```go
// ✅ 正确
log.Info("账号创建成功", zap.Int("id", account.ID))
log.Warn("Redis 不可用，使用 Fail-Open 策略")
log.Error("数据库查询失败", zap.Error(err))

// ❌ 错误 — 不要用 Error 记录可恢复的情况
log.Error("缓存未命中")  // 应该用 Debug 或 Info
```

---

## 结构化字段

使用 `zap.Field` 而非字符串拼接：

```go
// ✅ 正确 — 结构化字段
log.Info("处理请求",
    zap.String("method", "POST"),
    zap.String("path", "/api/v1/accounts"),
    zap.Int("status", 200),
    zap.Duration("latency", elapsed),
)

// ❌ 错误 — 字符串拼接
log.Info(fmt.Sprintf("处理请求 POST /api/v1/accounts 状态=%d 耗时=%v", 200, elapsed))
```

---

## 日志 Sink 机制

日志事件可转发到外部系统（如 Ops 系统日志索引）：

```go
// 注册 Sink
logger.RegisterSink(mySink)

// 发送 Sink 事件
logger.WriteSinkEvent(level, component, message, fields)
```

---

## 敏感信息脱敏

项目内置了日志脱敏工具（`internal/util/logredact/`），`response.ErrorFrom` 内部已自动调用 `logredact.RedactText` 对 500+ 错误日志进行脱敏。

### 禁止记录的信息

| 类型 | 说明 |
|------|------|
| 密码 | 用户密码、API 密钥明文 |
| Token | JWT Token、OAuth Token、刷新 Token |
| 凭证 | 第三方 API 的 Secret Key |
| PII | 用户邮箱的完整地址（可记录脱敏后的） |

---

## 配置项

```yaml
log:
  level: "info"          # 日志级别：debug/info/warn/error
  format: "json"         # 输出格式：json/console
  output:
    stdout: true         # 输出到标准输出
    file: ""             # 输出到文件（可选）
  rotation:
    max_size: 100        # 单文件最大 MB
    max_backups: 3       # 保留旧文件数
    max_age: 28          # 保留天数
  sampling:
    initial: 100         # 每秒允许的初始日志数
    thereafter: 100      # 之后每 N 条记录一次
```

---

## 常见错误

### 1. 混用 log.Printf 和 zap

标准库的 `log.Printf` 和 `slog` 已被桥接到 zap（`logger.go` 中实现），所以旧代码的 `log.Printf` 不会丢失，但**新代码应始终使用 `logger.L()` 或 `logger.S()`**。

### 2. 忘记传递 component 字段

```go
// ❌ 难以追踪来源
logger.L().Info("操作完成")

// ✅ 带 component 便于过滤
logger.L().Info("操作完成", zap.String("component", "account-service"))
```
