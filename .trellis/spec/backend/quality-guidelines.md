# 质量规范

> 后端代码质量标准、Linter 配置和测试要求。

---

## 概述

- **Go 版本**: 1.25.7
- **Linter**: golangci-lint v2.7
- **测试框架**: Go 原生 `testing` 包
- **CI**: GitHub Actions（backend-ci.yml）

---

## Linter 配置

配置文件：`backend/.golangci.yml`（617 行）

### 启用的 Linter

| Linter | 用途 |
|--------|------|
| `depguard` | **架构约束** — 强制分层依赖规则 |
| `errcheck` | 检查未处理的错误返回值 |
| `gosec` | 安全扫描 |
| `govet` | Go 静态分析 |
| `ineffassign` | 无效赋值检测 |
| `staticcheck` | 全量静态分析（SA/ST/S/QF 系列） |
| `unused` | 未使用代码检测 |

### 架构约束（depguard，最重要）

通过 linter 强制执行的分层规则：

```yaml
# Service 层禁止直接依赖
service-no-repository:
  files: ["**/internal/service/**"]
  deny:
    - pkg: github.com/Wei-Shaw/sub2api/internal/repository
    - pkg: gorm.io/gorm
    - pkg: github.com/redis/go-redis/v9

# Handler 层禁止直接依赖
handler-no-repository:
  files: ["**/internal/handler/**"]
  deny:
    - pkg: github.com/Wei-Shaw/sub2api/internal/repository
    - pkg: gorm.io/gorm
    - pkg: github.com/redis/go-redis/v9
```

**含义**: Handler 和 Service 层**禁止**直接引入 Repository 包、gorm、redis，必须通过接口调用。

### 自动格式化规则

```yaml
gofmt:
  rewrite-rules:
    - pattern: 'interface{}'
      replacement: 'any'        # interface{} → any
    - pattern: 'a[b:len(a)]'
      replacement: 'a[b:]'      # 简化切片语法
```

---

## 禁止的模式

| 模式 | 原因 | 替代方案 |
|------|------|---------|
| Handler 直接引入 Repository 包 | 违反分层架构 | 通过 Service 层中转 |
| 使用 `interface{}` | 代码风格统一 | 使用 `any` |
| 忽略 error 返回值 | 可能丢失错误 | 必须处理或显式 `_ =` |
| 在 Service/Handler 层直接操作 Redis | 违反分层依赖 | 将 Redis 操作封装在 Repository 层 |
| `git commit` 跳过 hook（`--no-verify`） | 绕过质量检查 | 修复问题后正常提交 |

---

## 必须的模式

| 模式 | 说明 |
|------|------|
| 构造函数返回接口 | Repository `New*` 函数返回 Service 定义的 interface |
| Schema 变更后 `go generate ./ent` | Ent 代码必须重新生成 |
| 新增 interface 方法后补全所有 stub | 所有 Mock/Stub 必须实现新方法 |
| 使用 `ApplicationError` 工厂函数 | 不要手动构造错误 |
| Handler 层使用 `response.*` 返回 | 不要直接使用 `c.JSON()` |

---

## 测试要求

### 测试类别

| 类别 | 标签 | 命令 | 位置 |
|------|------|------|------|
| 单元测试 | `unit` | `go test -tags=unit ./...` | 各模块的 `_test.go` |
| 集成测试 | `integration` | `go test -tags=integration ./...` | `internal/integration/` |

### 测试工具

- **Fixtures**: `internal/testutil/fixtures.go` — 测试数据构造
- **HTTP 测试**: `internal/testutil/httptest.go` — HTTP 测试辅助
- **Stubs**: `internal/testutil/stubs.go` — 接口桩实现

### 测试命名

```go
// 文件命名
account_repo_test.go

// 函数命名
func TestAccountRepository_GetByID(t *testing.T) { ... }
func TestAccountRepository_GetByID_NotFound(t *testing.T) { ... }
```

---

## PR 提交前检查清单

提交 PR 前**必须**本地验证：

- [ ] `cd backend && go test -tags=unit ./...` 通过
- [ ] `cd backend && go test -tags=integration ./...` 通过
- [ ] `cd backend && golangci-lint run ./...` 无新增问题
- [ ] 所有 test stub 补全新接口方法（如果改了 interface）
- [ ] Ent 生成的代码已提交（如果改了 schema）
- [ ] `pnpm-lock.yaml` 已同步（如果改了 frontend/package.json）

---

## CI/CD 流水线

| Workflow | 触发条件 | 检查内容 |
|----------|---------|---------|
| `backend-ci.yml` | push, pull_request | 单元测试 + 集成测试 + golangci-lint |
| `security-scan.yml` | push, PR, 每周一 | govulncheck + gosec + pnpm audit |
| `release.yml` | tag `v*` | 构建发布（PR 不触发） |

---

## 代码审查关注点

| 关注点 | 检查内容 |
|--------|---------|
| 分层合规 | Handler 是否直接调用了 Repository？ |
| 错误处理 | 是否使用 `ApplicationError`？是否翻译了 DB 错误？ |
| 日志规范 | 是否使用结构化字段？是否记录了敏感信息？ |
| 安全性 | 是否存在 SQL 注入、未授权访问风险？ |
| 测试覆盖 | 新增功能是否有对应测试？ |
| Ent 生成 | Schema 变更后是否重新生成？ |
