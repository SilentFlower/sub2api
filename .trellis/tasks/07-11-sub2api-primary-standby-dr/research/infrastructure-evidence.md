# 基础设施现状证据

> 仅记录规划所需的非敏感事实，不记录服务器密码、数据库密码、Token 或公网 IP。

## A 主节点

- 系统：Debian 12，x86-64，KVM。
- 资源：约 9.7 GiB 内存，根盘 100 GiB，剩余约 30 GiB。
- Compose：`/root/sub2api/deploy/docker-compose.yml`，项目名 `deploy`。
- 业务容器：`sub2api`、`sub2api-postgres`、`sub2api-redis`。
- 应用镜像 digest：`sha256:ff1bd8e6494963642570706d0ab55e04a1a15d9fd79707fc555d0f844f357d52`。
- PostgreSQL：18.1，数据库约 2.8 GB，`wal_level=replica`、`max_wal_senders=10`、`hot_standby=on`、无备库、无复制槽、`archive_mode=off`。
- PostgreSQL真实目录：`/var/lib/postgresql/18/docker`，位于 PostgreSQL 18 自动匿名卷；Compose声明的旧挂载点不承载当前运行数据。
- Redis：8，主库，AOF已启用，数据目录约 59 MB，无从库。
- 初始建设约束：不得重启或重建现有三个业务容器。

## B 备节点

- 系统：Debian 12，x86-64，Alibaba Cloud ECS。
- 资源：约 7.5 GiB 内存、61 GiB 可用磁盘。
- Docker：29.6.1；Docker Compose：5.3.1。
- 已有 Compose 项目：`new-api`、`rustdesk`、`sub2api`。
- 已有 Sub2API路径：`/root/sub2api/docker-compose.yml`。
- 已有业务容器：`sub2api`、`sub2api-postgres`、`sub2api-redis`。
- 已有应用端口：宿主机 `8080`。
- 已有卷：`sub2api_sub2api_data`、`sub2api_postgres_data`、`sub2api_redis_data`。
- 已有网络：`sub2api_sub2api-network`。
- 当前可用资源足以新增一套停止应用、运行数据库备库的隔离容灾栈。
- 隔离约束：现有目录、容器、端口、卷和网络不可修改。

## 已确认决策

- A 主、B 备；不做双活。
- 不引入第三节点和自动 fencing。
- 采用人工确认后的半自动提升。
- PostgreSQL采用异步流复制，接受秒级 RPO。
- 采用公网固定 IP直连复制，不使用 Tailscale。
- B-only 准备先行；A 侧变更后续单独确认。
