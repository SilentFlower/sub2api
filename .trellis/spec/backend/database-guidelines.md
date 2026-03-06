# 数据库规范

> 数据库访问模式、Schema 定义和迁移规范。

---

## 概述

- **ORM**: Ent（`entgo.io/ent`）
- **数据库**: PostgreSQL 16
- **缓存**: Redis（用于限流、Token 缓存等）
- **迁移方式**: Ent 自动迁移 + `migrations/` 目录的手动 SQL 脚本

---

## Schema 定义模式

所有 Schema 定义在 `backend/ent/schema/` 目录下。修改 Schema 后**必须**运行：

```bash
cd backend && go generate ./ent
```

### 基本结构

参考：`backend/ent/schema/account.go`

```go
type Account struct {
    ent.Schema
}

// Annotations 指定数据库表名
func (Account) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "accounts"},
    }
}

// Mixin 复用时间戳和软删除
func (Account) Mixin() []ent.Mixin {
    return []ent.Mixin{
        mixins.TimeMixin{},      // 自动添加 created_at, updated_at
        mixins.SoftDeleteMixin{}, // 自动添加 deleted_at + 软删除逻辑
    }
}

// Fields 定义字段
func (Account) Fields() []ent.Field {
    return []ent.Field{
        field.String("name").MaxLen(100).NotEmpty(),
        field.String("platform").MaxLen(50).NotEmpty(),
        field.JSON("credentials", map[string]any{}).
            Default(func() map[string]any { return map[string]any{} }).
            SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
        field.Time("last_used_at").Optional().Nillable().
            SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
    }
}

// Edges 定义关联关系
func (Account) Edges() []ent.Edge {
    return []ent.Edge{
        edge.To("groups", Group.Type).Through("account_groups", AccountGroup.Type), // 多对多
        edge.To("proxy", Proxy.Type).Field("proxy_id").Unique(),                    // 一对一
        edge.To("usage_logs", UsageLog.Type),                                       // 一对多
    }
}
```

### Mixin 使用规范

| Mixin | 功能 | 文件 |
|-------|------|------|
| `mixins.TimeMixin{}` | 自动添加 `created_at`、`updated_at` 字段 | `ent/schema/mixins/time.go` |
| `mixins.SoftDeleteMixin{}` | 添加 `deleted_at` + 自动过滤已删除记录 + DELETE→UPDATE 转换 | `ent/schema/mixins/soft_delete.go` |

**所有新建 Schema 必须包含这两个 Mixin**。

### 字段定义规范

| 字段类型 | 写法 |
|---------|------|
| 必填字符串 | `field.String("name").MaxLen(100).NotEmpty()` |
| 可选字符串 | `field.String("name").Optional().Nillable()` |
| JSON 字段 | `field.JSON("data", map[string]any{}).SchemaType(map[string]string{dialect.Postgres: "jsonb"})` |
| 时间字段 | `field.Time("xxx_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"})` |
| 枚举字段 | `field.Enum("status").Values("active", "disabled")` |
| 布尔字段 | `field.Bool("is_admin").Default(false)` |

---

## Repository 层模式

### 标准实现

参考：`backend/internal/repository/account_repo.go`

```go
// 结构体（小写，不导出）
type accountRepository struct {
    client *dbent.Client
    sql    sqlExecutor
}

// 构造函数返回 Service 层定义的接口（依赖反转）
func NewAccountRepository(client *dbent.Client, sqlDB *sql.DB) service.AccountRepository {
    return &accountRepository{client: client, sql: sqlDB}
}
```

### 事务支持

通过 `dbent.TxFromContext(ctx)` 获取事务客户端：

```go
func clientFromContext(ctx context.Context, defaultClient *dbent.Client) *dbent.Client {
    if tx := dbent.TxFromContext(ctx); tx != nil {
        return tx.Client()
    }
    return defaultClient
}
```

### 错误翻译

所有 Repository 方法必须使用 `translatePersistenceError` 将数据库错误转为业务错误：

```go
// backend/internal/repository/error_translate.go
func translatePersistenceError(err error, notFound, conflict *infraerrors.ApplicationError) error {
    if notFound != nil && (errors.Is(err, sql.ErrNoRows) || dbent.IsNotFound(err)) {
        return notFound.WithCause(err)
    }
    if conflict != nil && isUniqueConstraintViolation(err) {
        return conflict.WithCause(err)
    }
    return err
}
```

---

## 查询模式

### Ent 查询

```go
// 基本查询
account, err := r.client.Account.Query().
    Where(account.IDEQ(id)).
    Only(ctx)

// 带关联的查询
accounts, err := r.client.Account.Query().
    Where(account.PlatformEQ(platform)).
    WithGroups().
    WithProxy().
    All(ctx)

// 分页查询
items, err := r.client.Account.Query().
    Offset(offset).
    Limit(pageSize).
    Order(ent.Desc(account.FieldCreatedAt)).
    All(ctx)
```

### 原生 SQL

当 Ent 查询能力不足时，可使用原生 SQL：

```go
type sqlExecutor interface {
    QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
    ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}
```

---

## 命名规范

| 类别 | 规则 | 示例 |
|------|------|------|
| Schema 文件名 | snake_case | `api_key.go`, `usage_log.go` |
| 表名 | 复数 snake_case | `accounts`, `api_keys`, `usage_logs` |
| 字段名 | snake_case | `created_at`, `last_used_at` |
| Edge 名 | 小写复数 | `groups`, `usage_logs` |

---

## 常见错误

### 1. 修改 Schema 后忘记重新生成

**问题**：修改 `ent/schema/*.go` 后，代码不生效或编译报错。

**解决**：
```bash
cd backend
go generate ./ent   # 重新生成
git add ent/         # 生成的文件也要提交
```

### 2. 接口新增方法后 test stub 未补全

**问题**：给 interface 新增方法后，编译报错 `does not implement interface`。

**解决**：
```bash
grep -r "type.*Stub.*struct" internal/
grep -r "type.*Mock.*struct" internal/
# 逐一为 stub/mock 补全新方法
```

### 3. 软删除绕过

**注意**：`SoftDeleteMixin` 默认拦截所有查询和删除。如需查询已删除记录：

```go
ctx = mixins.SkipSoftDelete(ctx)
items, err := r.client.Account.Query().All(ctx)
```
