# Database Guidelines

> 本项目数据库、迁移、查询和事务约定。

---

## Overview

本项目后端使用 PostgreSQL、Ent ORM 和少量原生 SQL。SQL migrations 是 schema 演进的权威来源，Ent schema 和生成代码用于类型安全查询与实体访问。

关键事实：

- `backend/go.mod` 当前声明 `go 1.26.4`，`DEV_GUIDE.md` 中仍记录 CI 需要 Go `1.25.7`。涉及工具链时要同时核对 CI 配置和 `go.mod`，不要只凭其中一处判断。
- PostgreSQL 连接和自动迁移在 `backend/internal/repository/ent.go` 的 `InitEnt` 中完成。
- migrations 通过 `backend/migrations/migrations.go` 的 `//go:embed *.sql` 嵌入。
- 迁移执行器在 `backend/internal/repository/migrations_runner.go`，使用 `schema_migrations` 表记录 filename、checksum、applied_at。
- Redis 缓存相关实现位于 `backend/internal/repository/*_cache.go` 和 `backend/internal/repository/redis.go`。

---

## Query Patterns

优先使用 Ent 查询构造器，并从生成的 predicate 包导入字段谓词：

```go
m, err := r.client.User.Query().
	Where(dbuser.IDEQ(id)).
	Only(ctx)
```

复杂过滤可组合 predicate，分页使用项目已有 `internal/pkg/pagination` 或 repository 层已有分页工具。不要在 handler 层拼查询条件。

需要绕过软删除时，必须显式使用 `mixins.SkipSoftDelete(ctx)`。示例来自 `backend/internal/repository/user_repo.go`：

```go
ctx = mixins.SkipSoftDelete(ctx)
m, err := r.client.User.Query().Where(dbuser.IDEQ(id)).Only(ctx)
```

原生 SQL 只用于 Ent 不适合表达的场景，例如迁移执行、批量统计、特殊索引修复、导入导出和部分高性能聚合。相关模式可参考：

- `backend/internal/repository/migrations_runner.go`
- `backend/internal/repository/ops_repo_dashboard.go`
- `backend/internal/repository/sql_scan.go`

---

## Transactions

跨多表写入或需要与 allowed_groups、auth_identity 等关联数据保持一致时使用 Ent 事务。现有模式是：

- `r.client.Tx(ctx)` 开启事务。
- 如果遇到 `ent.ErrTxStarted`，复用 context 中已有事务。
- `defer tx.Rollback()` 作为失败兜底。
- 只有所有步骤成功后才 `tx.Commit()`。
- 事务内使用 `tx.Client()`，并用 `ent.NewTxContext(ctx, tx)` 传递事务上下文。

示例依据：`backend/internal/repository/user_repo.go` 的 `Create` 和 `Update`。

不要混用 `*sql.Tx` 手动构造 Ent client。代码中已有注释说明这样会导致 ExecQuerier 断言错误。

---

## Migrations

迁移文件位于 `backend/migrations/`，命名格式为 `NNN_description.sql`，描述部分使用小写 snake_case。已经应用到任何环境的迁移文件不允许修改、删除、重命名或重排。

执行规则来自 `backend/migrations/README.md` 和 `backend/internal/repository/migrations_runner.go`：

- 普通 `*.sql` 在事务中执行。
- `*_notx.sql` 非事务执行，只用于 `CREATE INDEX CONCURRENTLY` 或 `DROP INDEX CONCURRENTLY`。
- `*_notx.sql` 必须使用 `CREATE INDEX CONCURRENTLY IF NOT EXISTS` 或 `DROP INDEX CONCURRENTLY IF EXISTS`。
- 迁移内容会计算 SHA256 checksum，已应用文件被修改会导致启动失败。
- migration runner 会使用 PostgreSQL Advisory Lock 防止多实例并发迁移。

新增迁移时只写 forward-only SQL。不要在同一个文件里放 goose 风格的 Down SQL，因为当前 runner 会按完整 SQL 文件执行，不解析 Up/Down 段落。

---

## Ent Schema

Ent schema 源文件在 `backend/ent/schema/`。修改 schema 后必须运行：

```bash
cd backend
go generate ./ent
```

生成代码位于 `backend/ent/`，需要随 schema 变更一起提交。软删除使用 `backend/ent/schema/mixins/soft_delete.go`，默认查询会自动追加 `deleted_at IS NULL`。

Ent schema 字段和 migration 必须保持一致。新增字段时同时检查：

- `backend/ent/schema/<entity>.go`
- 对应 migration SQL
- repository mapper
- handler DTO
- 前端 `frontend/src/types/` 和 API 使用处

---

## Naming Conventions

- 表名和列名使用 snake_case。
- JSON 字段与数据库/API 习惯保持 snake_case。
- 索引名应可读并包含表/字段语义，例如 `idx_scheduler_outbox_pending_dedup_key`。
- 并发索引迁移文件使用 `_notx.sql` 后缀。
- migration 数字前缀需要零填充，并按现有最大编号继续增加。

---

## Docker PostgreSQL 18 Volume Layout

官方 PostgreSQL 18 镜像可能在父目录声明 Docker卷，而实际 `PGDATA` 位于其子目录。设计 Compose或辅助容器前，必须先检查当前固定镜像的元数据：

```bash
docker image inspect --format '{{json .Config.Volumes}}' "${POSTGRES_IMAGE}"
```

当镜像声明 `/var/lib/postgresql` 时：

- 命名卷挂载到 `/var/lib/postgresql`，显式设置 `PGDATA=/var/lib/postgresql/data`。
- Compose长语法启用 `volume.nocopy: true`，初始化辅助容器使用等价的 `volume-nocopy` 挂载。
- `pg_basebackup`、权限修正和配置写入辅助容器必须使用同一个父目录挂载契约，不能只在主服务上修正。
- 启动后检查容器只有一个位于 `/var/lib/postgresql` 的目标命名卷，不得存在承载真实数据的匿名父级卷。

禁止把命名卷只挂到 `/var/lib/postgresql/data`。父级镜像卷会遮蔽子挂载，或者先把镜像内的 `data -> .` 内容复制到空卷，导致 `pg_basebackup` 报目标目录非空。

---

## Common Mistakes

- 不要修改已应用 migration。需要修复时新增迁移。
- 不要在 `*_notx.sql` 中混入普通 DDL/DML 或事务控制语句。
- 不要忘记 `pnpm-lock.yaml` 之外的后端生成文件：Ent schema 改动必须提交 `backend/ent/` 生成结果。
- 不要在 service/handler 中直接依赖 DB 或 Redis。数据库访问应留在 repository 层。
- 不要用 `localhost` 假设 PostgreSQL 连接一定走 IPv4；`DEV_GUIDE.md` 建议本地使用 `127.0.0.1` 避免 Windows IPv6 回退问题。
- 不要在未检查固定镜像 `Config.Volumes` 的情况下沿用 PostgreSQL旧版本卷挂载路径。
