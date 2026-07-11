# 建立复制阶段执行结果

执行时间：2026-07-11。

## 已完成

- B 直接从 A 拉取原应用环境文件、PostgreSQL复制密钥和应用数据归档，文件未经过本地工作区；校验通过后生成 `/root/sub2api-dr/.env`，权限为 `0600`。
- 真实 `.env` 完整继承 A 的业务环境，追加 B 容灾项目、内部复制端口和固定镜像覆盖；Redis主库和从库密码均保持为空。
- A 原 `.env` 中 `JWT_SECRET` 为空；线上 `config.yaml` 和 PostgreSQL `security_secrets` 表已有持久化 JWT密钥，因此 B 保留原空环境值，没有生成新密钥。
- A `/app/data` 总量约 158 MiB，其中日志约 157.8 MiB；已同步全部非日志文件和目录到 `sub2api-dr-app-data`，导入后约 236 KiB，所有权和权限与 A 一致。
- B PostgreSQL使用 A 当前 PostgreSQL 18.1 精确镜像摘要完成在线 `pg_basebackup`，并启动为只读备库。
- B Redis使用 A 当前 Redis 8.4.0 精确镜像摘要完成初次同步并保持从库状态。
- 临时传输目录和 A 侧临时归档、校验清单均已删除；B 只保留真实 `.env`、容灾卷、网络和运行中的数据库备库。

## PostgreSQL验证

- B `pg_is_in_recovery()` 为 `true`，`transaction_read_only` 为 `on`。
- B WAL receiver 为 `streaming`，使用物理槽 `sub2api_b_standby`，接收与回放 LSN差值为 0。
- A `pg_stat_replication` 显示应用 `sub2api-b` 为 `streaming/async`，最终采样的写入、刷盘和回放延迟均约 6 毫秒。
- A 物理槽为 active、`wal_status=reserved`，最终 WAL积压为 0 bytes。
- A/B PostgreSQL系统标识一致，B 位于时间线 1。
- B PostgreSQL容器只有 `sub2api-dr-postgres-data` 一个父目录命名卷挂载，目标为 `/var/lib/postgresql`，没有承载真实数据的匿名卷。

## Redis验证

- B 为 `role:slave`，`master_link_status=up`，`master_sync_in_progress=0`。
- B 主从 offset一致，A 显示一个已连接从库和一个 slave客户端。
- 连续 4 次、45 秒采样中，Redis offset差值始终为 0，PostgreSQL接收与回放差值也始终为 0。

## 隔离与零重启证据

- A 原 Sub2API、PostgreSQL、Redis容器和 PostgreSQL进程均未重启；A 原 `8080/health` 正常。
- B 原 Sub2API三个容器基线未变化，原 `8080/health` 正常。
- B 容灾 PostgreSQL和 Redis没有发布宿主机端口。
- `sub2api-dr-app` 不存在，宿主机 `18080` 空闲。
- B 的应用、PostgreSQL和 Redis卷均带有 `sub2api-dr` Compose项目和逻辑卷标签。

## 初始化问题与修复

- PostgreSQL 18.1 镜像声明父级卷 `/var/lib/postgresql`。首次子目录挂载触发镜像内容复制，`pg_basebackup` 因目标非空在连接 A 前退出。
- 仅给子目录增加 `volume-nocopy` 后，父级匿名卷仍遮蔽子挂载，权限辅助容器同样在连接 A 前退出。
- 两次失败都没有建立 A 复制连接、没有激活复制槽、没有写入基础备份数据；对应 B 新卷在核对内容后被删除。
- 最终使用父级命名卷挂载、`volume.nocopy` 和子目录 `PGDATA`，通过实际容器挂载检查后完成初始化。完整复盘见 `postgresql18-volume-root-cause.md`。

## 全面检查修复

- 初版 `verify-replication.sh` 会检查 PostgreSQL恢复状态、Redis从库角色和主链路，但没有硬性断言 PostgreSQL WAL receiver 为 `streaming`，也没有断言 Redis初次同步或重同步已经结束。
- 已增加 `pg_stat_wal_receiver.status=streaming` 和 `master_sync_in_progress=0` 两项失败即退出的断言，并在 B 当前备库上重新执行通过。
- 修复只覆盖 B 的验证脚本，没有重建或重启 PostgreSQL、Redis或任何现有业务容器；B 原栈基线、备用应用停止和 `18080` 空闲检查再次通过。

## 待处理

- B 当前报告 NTP未同步，A 报告 NTP已同步；最近一次采样的时钟偏差约 1.2 秒，并仍可能继续漂移，会让 PostgreSQL回放时间差显示为负值。当前复制状态与 LSN/offset差值正常，但在提升演练前应单独处理 B 的时间同步。
- 阶段 4 的公共入口方式和最终提升脚本仍需单独确认；本阶段没有执行提升、启动备用应用或切换流量。
