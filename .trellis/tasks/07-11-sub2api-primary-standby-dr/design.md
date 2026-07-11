# Design: Sub2API 主备容灾部署

## 1. 架构概览

正常状态：

```text
A
├── 现有 Sub2API：运行
├── 现有 PostgreSQL：主库
├── 现有 Redis：主库
└── 独立复制转发层：后续新增，不重建现有容器
            │
            │ PostgreSQL异步流复制 / Redis主从复制
            ▼
B
├── 现有 Sub2API单机栈：保持不变
└── sub2api-dr 容灾栈
    ├── PostgreSQL：只读备库
    ├── Redis：从库
    └── Sub2API：停止
```

故障提升后：

```text
B sub2api-dr PostgreSQL：主库
B sub2api-dr Redis：主库
B sub2api-dr Sub2API：运行并监听 18080
公共入口：切换到 B:18080
```

## 2. B 侧隔离边界

| 类型 | B 现有部署 | 新容灾部署 |
|------|------------|------------|
| 目录 | `/root/sub2api` | `/root/sub2api-dr` |
| Compose 项目 | `sub2api` | `sub2api-dr` |
| 应用容器 | `sub2api` | `sub2api-dr-app` |
| PostgreSQL容器 | `sub2api-postgres` | `sub2api-dr-postgres` |
| Redis容器 | `sub2api-redis` | `sub2api-dr-redis` |
| 网络 | `sub2api_sub2api-network` | `sub2api-dr-network` |
| 应用卷 | `sub2api_sub2api_data` | `sub2api-dr-app-data` |
| PostgreSQL卷 | `sub2api_postgres_data` | `sub2api-dr-postgres-data` |
| Redis卷 | `sub2api_redis_data` | `sub2api-dr-redis-data` |
| 应用端口 | `8080` | 提升后 `18080` |

新 PostgreSQL、Redis仅加入 `sub2api-dr-network`，不发布宿主机端口。备用应用通过内部服务名连接它们。

## 3. B 侧文件结构

```text
/root/sub2api-dr/
├── compose.yaml
├── .env.example
├── compose.recovery-export.yaml
├── config/
│   └── redis-standby.conf
├── scripts/
│   ├── preflight.sh
│   ├── prepare-runtime-env.sh
│   ├── import-app-data.sh
│   ├── init-postgres-standby.sh
│   ├── verify-replication.sh
│   ├── switch-mode.sh
│   ├── promote.sh
│   ├── prepare-recovery-source.sh
│   ├── restore-standby-from-a.sh
│   └── verify-service.sh
└── state/
```

首轮只创建上述文件、拉取固定镜像并验证 Compose。真实 `.env` 在后续同步 A 配置时生成，不进入版本库；首轮不会启动任何容灾服务，也不会初始化数据库。

## 4. Compose 生命周期

- `postgres-dr`：使用 `profiles: ["standby", "promoted"]`，数据初始化完成后显式启动，正常状态为 PostgreSQL备库。
- `redis-dr`：使用 `profiles: ["standby", "promoted"]`，A 复制出口准备完成后显式启动，正常状态为 Redis从库。
- `app-dr`：使用 `profiles: ["promoted"]` 或等价方式，正常 `docker compose up -d` 不启动该服务。
- 不带 profile 的 Compose 操作不得启动任何服务，防止误初始化空数据库或提前运行备用应用。
- 应用端口映射固定为 `18080:8080`，仅在提升阶段启动应用时生效。
- Compose 对容器、网络和卷使用显式 `name`，确保实际资源名均为 `sub2api-dr-*`。
- PostgreSQL 18 镜像声明父级卷 `/var/lib/postgresql`；容灾命名卷必须挂载到该父目录并启用 `volume.nocopy`，同时显式设置 `PGDATA=/var/lib/postgresql/data`，避免镜像创建的父级匿名卷遮蔽真实数据。
- 所有镜像使用明确版本；Sub2API使用 A 当前运行的可拉取 `仓库@sha256` 镜像引用。

## 5. A 侧无重启复制出口

A 使用独立目录 `/root/sub2api-ha-export` 和 Compose 项目 `sub2api-ha-export`。两个转发容器使用固定摘要的 socat 镜像，加入已有 `deploy_sub2api-network`，对外监听独立复制端口并转发到：

```text
B -> A:15432 -> postgres:5432
B -> A:16379 -> redis:6379
```

监听器通过 socat `range=<B 公网 IP>/32` 只接受 B 的固定公网来源。由于 PostgreSQL看到的实际客户端是转发容器，`pg_hba.conf` 为专用复制用户放行 A 的 `deploy_sub2api-network` 子网，而不是 B 公网地址。

新增转发容器不会触发 A 现有 Compose 项目的容器重建。PostgreSQL复制所需变更使用在线 SQL、`pg_hba.conf` reload 和复制槽，不修改现有容器的端口映射。`wal_keep_size` 设为 2 GiB，`max_slot_wal_keep_size` 设为 8 GiB，避免 B 长时间离线时无限保留 WAL占满 A 的剩余磁盘。

A 的现有 `/root/sub2api-ha-export` 目录同时承载回切控制文件，避免引入第三个运维目录：

```text
/root/sub2api-ha-export/
├── compose.yaml
├── compose.recovery.yaml
├── compose.recovery-promoted.yaml
├── .env.example
├── scripts/
│   ├── switch-mode.sh
│   ├── init-postgres-from-b.sh
│   ├── sync-app-data.sh
│   └── verify-cutback.sh
└── state/
```

`compose.recovery.yaml` 作为 A 原 `/root/sub2api/deploy/docker-compose.yml` 的覆盖文件，使用 Compose `!override` 将 PostgreSQL、Redis和应用数据切换到新的明确命名卷。PostgreSQL卷挂载到 `/var/lib/postgresql`，并设置 `PGDATA=/var/lib/postgresql/data`；故障前旧卷保持不变。`compose.recovery-promoted.yaml` 只负责将 Redis持久化启动配置从 B 从库切换为 A 主库。

## 6. PostgreSQL复制流程

1. A 创建最小权限 replication 用户。
2. A 配置 B 来源地址并 reload。
3. A 创建物理复制槽。
4. B 停止容灾 PostgreSQL，确保目标卷为空。
5. B 运行 `pg_basebackup -R -X stream -S <slot>` 初始化数据目录。
6. B 启动 PostgreSQL，验证恢复状态和 WAL 接收进程。
7. A 验证 `pg_stat_replication`，B 验证回放延迟。

使用异步复制。秒级 RPO 优先于跨厂商同步复制带来的主库写阻塞风险。

## 7. Redis复制流程

B Redis使用独立配置文件描述从库状态。提升脚本必须移除或切换 `replicaof` 配置后再执行 `REPLICAOF NO ONE`，保证 B 重启后仍以主库身份启动，不能重新指向已故障的 A。

## 8. 应用配置同步

主数据由 PostgreSQL、Redis复制。建立复制阶段先盘点 A 的 `/app/data`，同步全部非临时业务文件，不同步日志；已知至少包括：

```text
config.yaml
.installed
pages/
实际启用的自定义模板或资源
```

同时以 A 当前 Compose 环境为源生成 B 的容灾 `.env`，保留数据库/Redis凭据、JWT/TOTP密钥及其他业务参数，仅覆盖容灾栈内部服务地址、资源名和 `18080` 端口。任务文件和版本库只保留无真实凭据的 `.env.example`。

备用应用在备库状态下保持停止。它只在 PostgreSQL和 Redis均完成提升后启动。

## 9. 提升状态机

```text
standby
  -> operator_confirmed
  -> postgres_promoted
  -> redis_promoted
  -> app_started
  -> ingress_switched
  -> verified
```

每一步写入本地状态记录。脚本检测到已完成步骤时应验证当前状态，而不是盲目重复执行。任一步失败即停止，禁止越过失败步骤继续切换入口。

模式控制脚本提供四个入口：

```text
status   -> 只读识别 standby / active / active-stopped / inconsistent
standby  -> 仅在 PostgreSQL=in_recovery 且 Redis=slave 时停止应用并验证复制
enable   -> 调用 promote.sh，保留人工 fencing 确认后按状态机提升
freeze   -> B 已启用时只停止应用并验证 18080 不再提供写入，不改变数据库角色
```

`active` 或 `active-stopped` 不能通过 `standby` 原地恢复为备库。B 提升后若要重新成为 A 的备库，必须先停止 B 写入并从新的 A 主库重新执行基础备份和 Redis全量同步。

## 10. 双端脚本与回切状态机

B 的统一入口固定为：

```text
/root/sub2api-dr/scripts/switch-mode.sh status
/root/sub2api-dr/scripts/switch-mode.sh standby [--dry-run]
/root/sub2api-dr/scripts/switch-mode.sh enable [--dry-run]
/root/sub2api-dr/scripts/switch-mode.sh freeze [--dry-run]
```

A 的统一入口固定为：

```text
/root/sub2api-ha-export/scripts/switch-mode.sh status
/root/sub2api-ha-export/scripts/switch-mode.sh prepare-from-b [--dry-run]
/root/sub2api-ha-export/scripts/switch-mode.sh cutback-to-a [--dry-run]
/root/sub2api-ha-export/scripts/switch-mode.sh restore-b-standby [--dry-run]
```

`prepare-from-b` 的状态流：

```text
B active
  -> A 旧服务停止并隔离
  -> B 临时开启仅允许 A 来源的 PostgreSQL/Redis恢复出口
  -> A 新命名卷完成 pg_basebackup、Redis全量同步和应用数据同步
  -> A PostgreSQL=in_recovery、Redis=slave、应用停止
```

B 恢复出口使用独立临时 Compose覆盖文件，建议端口为 `25432`、`26379`，只接受 A 固定公网来源。A 脚本通过标准 `ssh` 调用 B 本地辅助脚本并检查结果，不保存 SSH密码，也不使用 `sshpass`；部署时在 A 创建专用私钥 `/root/.ssh/sub2api_dr_ed25519`，只把公钥授权到 B。

`cutback-to-a` 的状态流：

```text
A standby-from-b
  -> 调用 B freeze，停止 B 应用写入
  -> 等待 A PostgreSQL LSN和 Redis offset追平
  -> 提升 A PostgreSQL
  -> 持久化提升 A Redis
  -> 启动 A 应用并验证 8080
  -> 等待人工切换公共入口
```

`restore-b-standby` 的状态流：

```text
A active + B active-stopped
  -> A 重建 B 专用复制槽和复制出口
  -> B 停止旧主数据库并在确认后重建容灾卷
  -> B 从新 A 主节点执行 pg_basebackup和 Redis全量同步
  -> B PostgreSQL=in_recovery、Redis=slave、应用停止
  -> 恢复 A active / B standby
```

A 提升后会产生新 PostgreSQL时间线，因此 B 不能原地声明为备库。恢复 B 时必须重新执行物理基础备份；若未来改用 `pg_rewind`，需另行设计、验证并更新本任务契约。

每个变更命令先输出两端角色、复制位置、目标卷和下一步动作；`--dry-run` 只执行状态检查和计划输出。破坏性阶段使用不同确认口令，避免一次确认覆盖整个回切过程。公共入口方式尚未确定时，脚本最多报告“节点已就绪，等待入口切换”，不能标记完整回切完成。

## 11. 回滚与保护

- B-only 准备：删除 `/root/sub2api-dr` 中新增文件以及未使用的 `sub2api-dr-*` 空资源，不影响现有栈。
- 复制初始化前：若失败，清空且仅清空新容灾卷后重试。
- 提升前：B 保持备库，不切换入口。
- 提升后：不尝试自动降级 B；按故障恢复流程处理 A。
- 模式控制：检测到数据库角色与应用状态不一致时停止，不自动猜测或修复主从拓扑。
- A 重建：默认创建并切换到新的恢复卷，故障前旧卷不删除；初始化失败时停止新恢复容器并保留失败卷供检查。
- A 回切失败：只要公共入口尚未切回 A，就保持 B 数据库为冻结写入后的权威主库；不得同时启动 B 应用恢复写入和继续提升 A。
- B 重新入备失败：保持 A 为唯一写入主节点，B 应用继续停止；清理并重试的范围只限 `sub2api-dr-*` 容灾卷。
