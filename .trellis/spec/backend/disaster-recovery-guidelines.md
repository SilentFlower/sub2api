# Disaster Recovery Guidelines

> 本项目双节点 Sub2API 主备容灾的部署、切换和恢复契约。

---

## Scenario: 双节点人工主备容灾

### 1. Scope / Trigger

- Trigger: 修改 A/B 容灾目录、Compose 文件、切换脚本、复制参数、端口、卷、网络、SSH 编排或操作手册时，必须按本节检查。
- Trigger: 修改 HA Agent 续租门禁、Worker 节点报告门禁、A 发布镜像自动同步 timer 或同标签更新流程时，也必须按本节检查。
- Scope: 两台服务器、Docker Compose、PostgreSQL 物理流复制、Redis 主从复制、应用数据同步，以及人工入口切换。
- Normal topology: A 是唯一写入主节点；B 的容灾 PostgreSQL 和 Redis 持续复制，B 容灾应用停止。
- Failure topology: 人工确认 A 已停止且不会继续写入后，提升 B 的容灾数据库并启动 B 容灾应用。
- Recovery topology: A 修复后先从 B 重建为备节点，冻结 B 写入并等待 A 追平，再提升 A，最后从新 A 主节点全量重建 B 备库。
- Constraint: 只有 A、B 两个节点，没有仲裁节点、自动 fencing 或自动入口切换。本方案是人工确认边界下的主备容灾，不是自动一致性集群。
- Security boundary: 规范和版本库不得记录服务器密码、数据库密码、Token、SSH 私钥、真实域名或真实公网 IP。

### 2. Signatures

#### 2.1 唯一推荐的操作入口

A 节点：

```text
/root/sub2api-ha-export/scripts/switch-mode.sh status
/root/sub2api-ha-export/scripts/switch-mode.sh sync-release [--dry-run]
/root/sub2api-ha-export/scripts/switch-mode.sh prepare-from-b [--dry-run]
/root/sub2api-ha-export/scripts/switch-mode.sh cutback-to-a [--dry-run]
/root/sub2api-ha-export/scripts/switch-mode.sh restore-b-standby [--dry-run]
/usr/local/libexec/sub2api-ha-sync-release-if-needed
systemctl status sub2api-ha-release-sync.timer
```

B 节点：

```text
/root/sub2api-dr/scripts/switch-mode.sh status
/root/sub2api-dr/scripts/switch-mode.sh standby [--dry-run]
/root/sub2api-dr/scripts/switch-mode.sh enable [--dry-run]
/root/sub2api-dr/scripts/switch-mode.sh freeze [--dry-run]
```

- `status` 必须是只读操作。
- 每个变更命令都必须支持 `--dry-run`；`--dry-run` 只能读取状态、验证前置条件并输出计划，不得停止或创建容器、提升数据库、改变复制方向、写入业务数据、创建或删除数据卷。
- `help`、`--help`、`-h` 不接受额外参数。
- 未知命令、未知参数或超过约定参数数量必须立即失败。
- `scripts/` 下其它文件是受统一入口编排的辅助脚本，不是日常人工操作入口。

#### 2.2 机器可读状态接口

跨节点编排依赖以下内部接口，字段名属于稳定契约：

```text
switch-mode.sh status --machine
```

A 必须输出：

```text
mode
postgres_container
postgres_recovery
postgres_lsn
postgres_volume
redis_container
redis_role
redis_link
redis_sync
redis_master_offset
redis_slave_offset
redis_volume
app_container
app_volume
app_image_digest
recovery_image_digest
recovery_image_cached
release_image_digest
release_source_ref
release_synced_at
dr_image_digest
dr_image_cached
dr_release_image_digest
dr_release_synced_at
image_sync
```

B 必须输出：

```text
mode
postgres_container
postgres_recovery
postgres_lsn
redis_container
redis_role
redis_link
redis_sync
redis_master_offset
redis_slave_offset
app_container
app_image_digest
app_image_cached
running_app_image_digest
release_image_digest
release_source_ref
release_synced_at
```

- 每行格式固定为 `key=value`，不得把说明文字写入标准输出。
- A 的回切脚本至少依赖 B 的 `mode`、`postgres_lsn` 和 `redis_master_offset`。
- A 的发布同步至少依赖 B 的 `mode`、数据库/Redis 复制状态、`app_image_digest`、`app_image_cached` 和 `release_image_digest`。
- `image_sync=ok` 只表示 A 当前运行镜像、A 恢复配置、B 容灾配置、B 本地缓存和双端最近同步记录一致，不表示数据库迁移支持向旧镜像回滚。
- 有效状态返回 `0`；检测到 `inconsistent` 返回 `2`；配置、命令、角色或前置条件错误返回非零。

### 3. Contracts

#### 3.1 部署路径与资源身份

当前部署不是任意路径可迁移的通用安装器。路径分为三类：

| 类型 | 当前契约 | 变更规则 |
|---|---|---|
| 脚本相对根目录 | `SCRIPT_DIR` 从脚本位置计算；A 的 `EXPORT_ROOT` 和 B 的 `DR_ROOT` 从 `SCRIPT_DIR/..` 计算 | 整个目录一起移动时，内部相对引用可以继续工作 |
| 节点间路径 | A 容灾目录 `/root/sub2api-ha-export`；B 容灾目录 `/root/sub2api-dr` | README、SSH 远程命令、环境变量和已部署文件必须一起更新 |
| 原部署路径 | A `/root/sub2api/deploy`；B `/root/sub2api` | 当前预检脚本仍有字面量路径，不能只改一个环境变量后直接迁移 |

可配置项不等于完整可迁移：

- A 的 `PRIMARY_COMPOSE_FILE`、`PRIMARY_ENV_FILE`、`PRIMARY_PROJECT_NAME` 可以覆盖原 Compose 调用参数。
- A 的 `B_DR_ROOT`、`B_SSH_TARGET`、`B_SSH_IDENTITY_FILE` 可以覆盖 B 的远程目录和 SSH 连接参数。
- 但 A 预检仍直接校验 `/root/sub2api/deploy/docker-compose.yml`、`/root/sub2api/deploy/.env`，B 预检仍直接校验 `/root/sub2api/docker-compose.yml`。
- 因此改变上述固定路径时，必须同步修改预检、环境文件、README、远程命令和基线，并重新跑完整验证；禁止只移动目录或只改 `.env`。

资源名是防止误操作另一套单机部署的身份边界，不能直接重命名：

| 范围 | 固定身份 |
|---|---|
| A 原部署 | Compose 项目 `deploy`；容器 `sub2api`、`sub2api-postgres`、`sub2api-redis`；网络 `deploy_sub2api-network` |
| A 复制出口 | Compose 项目 `sub2api-ha-export`；容器 `sub2api-ha-postgres-export`、`sub2api-ha-redis-export` |
| A 恢复卷 | `sub2api-ha-app-data`、`sub2api-ha-postgres-data`、`sub2api-ha-redis-data` |
| B 原部署 | Compose 项目 `sub2api`；容器 `sub2api`、`sub2api-postgres`、`sub2api-redis`；网络 `sub2api_sub2api-network`；目录 `/root/sub2api` |
| B 容灾栈 | Compose 项目 `sub2api-dr`；容器 `sub2api-dr-app`、`sub2api-dr-postgres`、`sub2api-dr-redis`；网络 `sub2api-dr-network` |
| B 容灾卷 | `sub2api-dr-app-data`、`sub2api-dr-postgres-data`、`sub2api-dr-redis-data` |
| B 临时恢复出口 | `sub2api-dr-postgres-recovery-export`、`sub2api-dr-redis-recovery-export` |

- A 回切后仍复用 A 原容器名和 Compose 项目名，但通过覆盖文件切到新的 `sub2api-ha-*` 恢复卷。
- A/B 的 Compose 辅助函数都显式传入固定 `--project-name`；只修改 `.env` 中的 `COMPOSE_PROJECT_NAME` 不会完成项目迁移。
- 创建、检查或删除卷时必须同时验证 `com.docker.compose.project` 和 `com.docker.compose.volume` 标签。
- B 的所有容灾清理只能作用于 `sub2api-dr-*` 资源，绝不能修改 B 原 `/root/sub2api` 单机部署。

#### 3.2 环境与连接参数

以下是容灾脚本的关键环境契约；真实值只存在于服务器受限文件中：

| 分组 | 环境键 | 契约 |
|---|---|---|
| A 原部署 | `PRIMARY_PROJECT_NAME`、`PRIMARY_COMPOSE_FILE`、`PRIMARY_ENV_FILE` | 指向 A 当前有效 Compose 项目和环境文件 |
| A 恢复卷 | `A_RECOVERY_APP_VOLUME`、`A_RECOVERY_POSTGRES_VOLUME`、`A_RECOVERY_REDIS_VOLUME` | 必须与 Compose 覆盖文件和卷标签一致 |
| A 到 B SSH | `B_SSH_TARGET`、`B_SSH_IDENTITY_FILE`、`B_DR_ROOT`、`SSH_CONNECT_TIMEOUT` | 使用标准 SSH 密钥；脚本不得保存 SSH 密码或使用 `sshpass` |
| A 复制出口 | `EXPORT_BIND_HOST`、`POSTGRES_EXPORT_PORT`、`REDIS_EXPORT_PORT`、`B_SOURCE_CIDR` | 来源限制必须是 B 的固定 IPv4 `/32` |
| B 临时恢复出口 | `RECOVERY_EXPORT_BIND_HOST`、`B_POSTGRES_RECOVERY_PORT`、`B_REDIS_RECOVERY_PORT`、`A_SOURCE_CIDR` | 只在 A 修复回切期间启动，来源限制必须是 A 的固定 IPv4 `/32` |
| PostgreSQL 复制 | `POSTGRES_REPLICATION_USER`、`POSTGRES_REPLICATION_PASSWORD`、`POSTGRES_REPLICATION_SLOT`、`POSTGRES_A_RECOVERY_SLOT` | 用户名和槽名必须是合法 PostgreSQL 标识符；两个方向使用不同物理槽 |
| PostgreSQL 保留 | `WAL_KEEP_SIZE`、`MAX_SLOT_WAL_KEEP_SIZE` | 格式必须为正整数加 `MB` 或 `GB`，防止复制断连时无限占用磁盘 |
| B 连接 A | `A_REPLICATION_HOST`、`A_POSTGRES_REPLICATION_PORT`、`A_REDIS_REPLICATION_PORT` | 必须与 A 对外复制端口一致 |
| A 连接 B | `B_REPLICATION_HOST`、`B_POSTGRES_RECOVERY_PORT`、`B_REDIS_RECOVERY_PORT` | 仅用于 B 已提升后重建 A |
| 固定镜像 | `SUB2API_IMAGE`、`POSTGRES_IMAGE`、`REDIS_IMAGE`、`SOCAT_IMAGE` | 生产部署使用已验证的不可变引用；B 运行环境生成要求前三者使用 `@sha256`，socat 同样固定摘要 |

- `.env.example` 只能包含占位值，真实 `.env`、`secrets.env` 权限必须为 `0600`。
- 所有必填变量必须拒绝空值和 `REPLACE_WITH_*` 占位值。
- `B_SSH_TARGET` 必须符合 `user@host` 形态；`B_DR_ROOT` 必须是受限字符组成的绝对路径；配置了 SSH 私钥时文件必须存在。
- 默认端口契约是：A 应用 `8080`、B 原应用 `8080`、B 容灾应用 `18080`、A PostgreSQL/Redis 复制出口 `15432/16379`、B 临时恢复出口 `25432/26379`。修改宿主机端口时，两端连接参数必须成对修改。

##### 3.2.1 应用发布镜像同步

- A 日常更新仍由原部署流程负责。A 应用更新并通过健康检查后，必须执行 `sync-release --dry-run` 和 `sync-release`，把当前运行容器对应的不可变镜像 digest 同步到容灾配置。
- 标签只用于记录来源，不能作为容灾版本身份。脚本必须用 A 运行容器的镜像 ID 锁定实际内容，再从该镜像的 `RepoDigests` 中选择与容器来源仓库一致的唯一 `仓库@sha256`；无匹配 digest、多个匹配结果或镜像 ID 不一致时必须失败。
- `sync-release` 只允许 A 为 `legacy-active` 或 `active-recovered`、A 应用健康、B 为健康 `standby` 且复制已追平时执行。B 已接管后，不得再从故障 A 推导发布版本。
- `--dry-run` 只能读取当前镜像、双端状态、配置和缓存，不得拉取镜像、写 `.env`、写状态文件、重启服务或启动 B 应用。
- 实际同步顺序固定为：B 拉取并验证精确 digest，原子更新 B `.env` 中唯一的 `SUB2API_IMAGE`，写 B `state/release-image.env`，原子更新 A `.env` 中唯一的 `SUB2API_IMAGE`，最后写 A `state/release-image.env`。
- `.env` 单键更新必须使用同目录临时文件加 `mv`，保持权限为 `0600`，且同步后 `SUB2API_IMAGE` 必须恰好出现一次。禁止重写或覆盖数据库、复制、端口及其它环境键。
- 双端发布状态文件只允许记录 `APP_IMAGE_DIGEST`、`PREVIOUS_APP_IMAGE_DIGEST`、`SOURCE_IMAGE_REF` 和 `SYNCED_AT`，权限必须为 `0600`，不得写入密码或 Token。
- 跨节点同步不具备分布式事务。若 B 已更新而 A 更新失败，状态必须显示 `drift`，不得自动回滚 B 或启动任何应用；修复连接或文件问题后通过同一命令幂等重跑。
- B `enable` 和 `promote.sh` 必须在 PostgreSQL 提升前验证：配置是固定 digest、镜像已缓存、最近同步记录与配置一致；任一条件不满足都必须拒绝提升。
- B 已接管后，A `prepare-from-b` 必须以 B 当前活动配置 digest 为权威，在停止 A 旧服务前确保 A 已缓存并使用同一 digest。
- `PREVIOUS_APP_IMAGE_DIGEST` 只用于诊断。数据库迁移可能不向后兼容，保留旧镜像不能被描述为可保证成功的应用回滚。

##### 3.2.2 A 发布镜像自动调和与租约门禁

- A 活动 owner 的续租门禁只判断 A 自身是否仍可安全写入：本地模式、应用 HTTP 健康、PostgreSQL/Redis 主库角色、restart policy 和 A HA Tunnel。`image_sync!=ok` 只表示 B 暂不可接管，不得单独撤销健康 A 的租约或触发 A self-fencing。
- B 的接管和 B_ACTIVE 续租门禁仍必须要求精确 digest、镜像缓存和发布记录一致。B 不得因为 A 仍在运行就跳过镜像门禁，也不得独立拉取 `latest`、`build-latest` 等可变标签。
- A 使用独立 `sub2api-ha-release-sync.timer` 每 60 秒触发一次 oneshot；timer 只安装在 A，不安装在 B。脚本使用 `/run/sub2api-ha-release-sync.lock` 非阻塞文件锁，禁止并发同步。
- 自动调和只接受 A `legacy-active` 或 `active-recovered`，要求应用容器运行且自本次 `StartedAt` 起稳定至少 120 秒。未达到稳定窗口时成功退出并等待下一轮，不得提前拉取或改配置。
- 达到稳定窗口后必须依次执行 `sync-release --dry-run` 和 `sync-release`，最后再次读取 `status --machine` 并要求 `image_sync=ok`。任何步骤失败都由 oneshot 返回非零，timer 后续周期幂等重试。
- timer 与 10 秒 HA Agent 心跳必须是独立进程。镜像拉取或 SSH 较慢时不能阻塞 A 续租，也不能为了完成同步延长 45 秒租约 TTL。
- `observe` 模式允许该受限调和动作修改镜像缓存、A/B 容灾 `SUB2API_IMAGE` 和发布状态文件，因为它不改变写入拓扑。它不得启动应用、重启 A、改变数据库/Redis 角色、修改卷、Tunnel、DNS、owner 或 `epoch`。
- 同步失败时 A 继续服务并续租，B 的 `b_failover_eligible()` 保持 false；修复网络、镜像仓库或配置后由同一 timer 自动重试，禁止自动回退 A 当前运行镜像。
- 应用 Compose healthcheck 必须使用镜像内真实存在的工具。修改 Compose 不会改变已创建容器的 healthcheck；只有容器重建后才会加载新命令。Docker healthcheck 工具缺失但外部 HTTP 健康时，应修复 Compose 并在正常更新窗口生效，不能仅为刷新 healthcheck 强制重启活动 A。

#### 3.3 人工 fencing 与入口切换

- 本方案没有自动 fencing。SSH 超时、健康检查失败或网络不可达都不能证明 A 已停止写入。
- 执行 B `enable` 前，操作者必须确认 A 已关机或已在云厂商控制台被停止，并输入 `A_IS_STOPPED_AND_WILL_NOT_WRITE`。
- 无法确认 A 是否仍可写时，必须停止提升，不能以“先切过去看看”代替隔离。
- 脚本不会自动修改 DNS、反向代理、负载均衡或公共域名。数据库和应用就绪后，只能输出“等待人工切换入口”。
- 任意时刻只能有一个应用节点接受写入。B `active-stopped` 表示数据库仍是权威主库，但应用写入已经冻结。

#### 3.4 状态机与确认边界

B 状态：

```text
uninitialized
  -> standby
  -> active
  -> active-stopped
  -> 从新 A 主节点全量重建
  -> standby
```

- `standby`: PostgreSQL `pg_is_in_recovery()=true`，Redis `role=slave`、`master_link_status=up`、`master_sync_in_progress=0`，容灾应用停止。
- `active`: PostgreSQL 是主库，Redis 是主库，容灾应用运行。
- `active-stopped`: PostgreSQL 和 Redis 仍是主库，容灾应用停止。
- `inconsistent`: 组件角色不符合任何完整状态；必须停止操作并排查。
- 已提升且产生写入的 B 不能通过 `standby` 原地降级，必须从新的 A 主库重新做基础备份和 Redis 全量同步。

A 状态：

```text
legacy-active
  -> B 接管
  -> standby-from-b
  -> cutback-postgres-promoted
  -> active-recovered-stopped
  -> active-recovered
```

- `prepare-from-b` 只接受 B `active` 或 `active-stopped`，并使用新命名卷重建 A；故障前旧卷必须保留。
- `cutback-to-a` 必须先冻结 B 应用，等待 A 的 PostgreSQL LSN 和 Redis offset 达到 B 冻结点，再提升 A。
- `restore-b-standby` 只允许 A `active-recovered`，并要求 B 已冻结或已经是有效 `standby`。

动作前置状态属于稳定契约：

| 动作 | 允许状态或幂等结果 |
|---|---|
| A `sync-release` | A 为 `legacy-active` 或 `active-recovered`，应用健康；B 为健康 `standby`，PostgreSQL/Redis 复制正常；同 digest 重跑时幂等刷新双端配置和记录 |
| A `prepare-from-b` | A 为 `legacy-active` 或 `offline`，且 B 为 `active` 或 `active-stopped`；A 已是 `standby-from-b` 时直接成功返回 |
| A `cutback-to-a` | A 为 `standby-from-b`、`cutback-postgres-promoted` 或 `active-recovered-stopped`，且 B 为 `active` 或 `active-stopped`；A 已是 `active-recovered` 时直接成功返回 |
| A `restore-b-standby` | A 必须为 `active-recovered`；B 为 `active-stopped` 时全量重建，B 已是 `standby` 时只同步非日志应用数据 |
| B `standby` | PostgreSQL 必须仍在恢复，Redis 必须是已追平的从库；若应用意外运行，只停止应用后重新验证 |
| B `enable` | 容灾 PostgreSQL、Redis 容器必须存在；发布镜像必须是已缓存且与最近同步记录一致的固定 digest；实际提升始终要求人工 fencing 口令，已完成阶段按状态标记和真实角色幂等复核 |
| B `freeze` | 只接受 `active` 或 `active-stopped`，且 PostgreSQL、Redis 均为主库；已冻结时只复核状态 |

独立确认口令：

| 阶段 | 确认口令 |
|---|---|
| A 故障后提升 B | `A_IS_STOPPED_AND_WILL_NOT_WRITE` |
| 从 B 重建 A | `STOP_AND_REBUILD_A_FROM_B` |
| 冻结 B 并提升 A | `FREEZE_B_AND_PROMOTE_A` |
| B 已是备库时同步应用数据 | `SYNC_A_APP_DATA_TO_B` |
| A 已恢复为主后重建 B | `A_IS_PRIMARY_REBUILD_B_STANDBY` |
| 直接冻结 B 写入 | `FREEZE_B_WRITES_FOR_CUTBACK` |
| B 删除容灾卷并从 A 重建 | `REBUILD_B_STANDBY_FROM_A` |

一次确认不能覆盖多个破坏性阶段。脚本失败后应重新读取两端状态，不能连续盲目重跑实际命令。

#### 3.5 PostgreSQL、Redis 与应用数据

- PostgreSQL 使用异步物理流复制；复制健康必须同时满足备库恢复状态和 `pg_stat_wal_receiver.status=streaming`。
- `pg_basebackup` 不复制源库的物理复制槽。A 从 B 重建并提升后，必须先在新 A 主库重新创建 B 专用物理槽，再让 B 验证恢复源和执行全量重建。
- A 提升后产生新时间线，B 不能只修改 `primary_conninfo` 假装重新入备。除非另行设计并验证 `pg_rewind`，当前契约必须重新执行基础备份。
- `.pgpass` 写入复制密码时，必须先把反斜杠 `\` 转义为 `\\`，再把冒号 `:` 转义为 `\:`；文件权限必须为 `0600`，所有者必须是 `postgres`。
- PostgreSQL 18 的父级卷布局必须遵循 [Database Guidelines](./database-guidelines.md#docker-postgresql-18-volume-layout)，不得把真实数据卷只挂到 `/var/lib/postgresql/data`。
- Redis 提升必须同时执行运行时 `REPLICAOF NO ONE` 和持久化 Compose 覆盖，保证容器重建后仍保持主库身份。
- 应用数据同步必须排除 `logs/`，并验证 `.installed` 和 `config.yaml` 存在；从 B 回写时只能在 B 已恢复为 `standby` 且应用停止后执行。

#### 3.6 来源受限的 socat 出口

- socat `range=<peer IPv4>/32` 是连接来源限制，不能从出口所在主机使用 `127.0.0.1` 证明真实复制协议可用。
- 出口所在节点只检查容器运行和宿主机端口监听；协议级验证必须从被允许的另一节点执行。
- PostgreSQL 验证至少执行 `IDENTIFY_SYSTEM` 和 `READ_REPLICATION_SLOT <slot>`；Redis 验证至少执行 `PING`。
- 必须在协议验证通过后，才允许停止旧容器、删除目标容灾卷或开始 `pg_basebackup`。

### 4. Validation & Error Matrix

| 条件 | 必须行为 |
|---|---|
| 必填环境变量为空或仍为占位值 | 立即失败，不启动 Compose 操作 |
| 原 Compose、原 `.env`、复制密钥或 SSH 私钥不存在 | 立即失败，不改变本地和远端状态 |
| 路径、SSH 目标、CIDR、复制用户名或槽名格式无效 | 立即失败并指出具体变量 |
| 目标恢复卷非空 | 拒绝覆盖，保留现场供检查 |
| 卷的 Compose 项目或逻辑卷标签不匹配 | 拒绝删除或复用该卷 |
| 同名容器、网络、卷或目标端口被其它资源占用 | 预检失败，不碰现有部署 |
| B PostgreSQL 不是备库，或 WAL receiver 不是 `streaming` | 不允许标记为 `standby` |
| B Redis 不是从库、主链路断开或正在同步 | 不允许标记为 `standby` 或继续回切 |
| A/B 状态为 `inconsistent` | `status` 返回 `2`，所有变更命令停止 |
| A 无法读取 B 状态 | A 命令在任何本地破坏性动作前失败 |
| B 不是 `active`/`active-stopped` | A `prepare-from-b` 拒绝执行 |
| A 不是 `standby-from-b` 或允许的中间恢复态 | `cutback-to-a` 拒绝执行 |
| B 写入冻结后 A 未追平目标 LSN 或 Redis offset | 不提升 A，保持 B 数据库为权威主库 |
| B 已提升后尝试原地 `standby` | 拒绝执行，要求从新主库全量重建 |
| A 运行镜像没有同仓库 RepoDigest、RepoDigest 与运行镜像 ID 不一致或结果不唯一 | `sync-release` 立即失败，不修改双端配置或状态 |
| A 应用不健康、B 不是健康 `standby` 或复制未追平 | `sync-release` 拒绝执行，不拉取镜像或修改 `.env` |
| B 拉取或验证精确 digest 失败 | 停止同步，双端 `.env` 保持原值，不在故障窗口退回可变标签 |
| 双端配置、缓存或发布记录不一致 | `status` 显示 `drift` 或 `unknown`；B `enable` 在提升 PostgreSQL 前失败 |
| A 自身应用、角色、restart policy 和 Tunnel 健康，但 `image_sync=drift` | A 继续续租；B 接管门禁关闭；A timer 等待稳定窗口后自动调和 |
| A 新容器运行不足 120 秒 | timer 成功跳过，不执行 dry-run、拉取或配置写入 |
| 自动调和 dry-run、SSH、B 拉取或配置更新失败 | oneshot 返回非零，A 和入口保持不变，B 应用保持停止，下一 timer 周期幂等重试 |
| 自动调和执行时间超过一次心跳周期 | A Agent 独立续租；同步进程不得持有 Agent 主循环锁或阻塞心跳 |
| Docker healthcheck 使用镜像内不存在的命令 | Docker 可显示 `unhealthy`；以真实 HTTP 检查确认业务，修复 Compose 后等待正常容器更新加载，不为此强制重启活动 A |
| 跨节点同步只完成 B 侧后失败 | 保持 B 应用停止，不自动回滚；修复故障后幂等重跑 `sync-release` |
| 来源受限出口只能从本机监听、另一节点协议验证失败 | 不停止旧服务、不删除卷、不开始基础备份 |
| B 未报告 NTP 同步 | `enable` 明确警告；真实演练前必须人工处理或确认时间同步，追平判断仍以 LSN/offset 为准 |
| 健康检查失败或确认口令不匹配 | 当前阶段立即停止，不自动越过或切换入口 |

### 5. Good/Base/Bad Cases

- Good: A 正常提供服务，B 显示 `standby`，PostgreSQL 为 `streaming`，Redis 链路为 `up` 且同步完成，B 容灾应用和 `18080` 均未运行。
- Good: A 更新后即使继续使用同一标签，也执行 `sync-release --dry-run` 和 `sync-release`；A/B 状态最终显示同一 digest 且 `image_sync=ok`。
- Good: A 同标签更新产生新 digest 时继续稳定续租，B 暂时不可接管；A 容器稳定 120 秒后 timer 自动同步精确 digest，A/B 业务容器均不重启。
- Good: A 故障时先在控制台确认 A 已停止，再执行 B `enable --dry-run` 和 `enable`，最后人工切换公共入口。
- Good: A 修复后使用新恢复卷从 B 重建，冻结 B 后按 LSN/offset 等待完全追平，再提升 A 并从新 A 全量重建 B。
- Base: 只修改 SSH 目标或外部端口时，同步修改两端环境文件并重新验证 Compose、SSH、协议连接和 `status --machine`。
- Base: `status` 可以长期执行；B 容灾数据库可以常开复制，但备用应用必须停止。
- Base: `image_sync=ok` 时 timer 只读取状态并成功退出；重复执行不拉取镜像或改写配置。
- Bad: 因为 A ping 不通或 SSH 超时就直接提升 B，这不构成 fencing，可能造成双主写入。
- Bad: 只改 `B_DR_ROOT` 后移动 B 目录，却不更新 README、远程命令、预检和服务器部署文件。
- Bad: 手工运行 `docker compose down`、删除卷、启动 B 容灾应用或绕过统一脚本修改数据库角色。
- Bad: 把 `build-latest` 等标签文本直接写入 B 容灾配置，或在 A 故障提升现场才临时拉取可变标签。
- Bad: 把 B 镜像未同步视为 A 自身不健康并停止 A 续租，这会把“备机暂不可接管”放大为主节点主动停机。
- Bad: 在 HA Agent 心跳线程内同步大镜像，导致 45 秒租约因下载或 SSH 延迟而过期。
- Bad: 在 B 本机用 `127.0.0.1` 自测只允许 A 来源的恢复出口，并把被拒绝误判为出口故障。
- Bad: A 提升后直接让 B 使用旧数据目录连接新时间线，或在创建新物理槽前先执行 B 恢复源预检。

### 6. Tests Required

静态修改至少执行：

```bash
bash -n /root/sub2api-ha-export/scripts/*.sh
bash -n /root/sub2api-dr/scripts/*.sh
shellcheck -S warning /root/sub2api-ha-export/scripts/*.sh
shellcheck -S warning /root/sub2api-dr/scripts/*.sh
bash -n /usr/local/libexec/sub2api-ha-sync-release-if-needed
systemd-analyze verify /etc/systemd/system/sub2api-ha-release-sync.service /etc/systemd/system/sub2api-ha-release-sync.timer
```

自动化产物至少执行：

```bash
PYTHONPATH=. python3 -m unittest discover -s test -p 'test_*.py'
./test/test_release_sync.sh
npm test -- --run
npm run check
```

Compose 修改至少执行：

```bash
cd /root/sub2api-dr
docker compose --env-file .env.example -f compose.yaml config >/dev/null
docker compose --env-file .env.example -f compose.yaml -f compose.promoted.yaml config >/dev/null
docker compose --env-file .env.example -f compose.yaml -f compose.recovery-export.yaml config >/dev/null

cd /root/sub2api-ha-export
docker compose --project-name deploy --env-file /root/sub2api/deploy/.env --env-file .env -f /root/sub2api/deploy/docker-compose.yml -f compose.recovery.yaml config >/dev/null
docker compose --project-name deploy --env-file /root/sub2api/deploy/.env --env-file .env -f /root/sub2api/deploy/docker-compose.yml -f compose.recovery.yaml -f compose.recovery-promoted.yaml config >/dev/null
```

运行态检查至少覆盖：

```bash
/root/sub2api-ha-export/scripts/switch-mode.sh status
/root/sub2api-ha-export/scripts/switch-mode.sh sync-release --dry-run
/root/sub2api-dr/scripts/switch-mode.sh status
/root/sub2api-dr/scripts/switch-mode.sh enable --dry-run
```

断言点：

- `status --machine` 的字段名、`key=value` 格式和退出码保持稳定。
- 正常态为 A `legacy-active` 或 `active-recovered`，B `standby`。
- B PostgreSQL 为恢复状态且 WAL receiver 为 `streaming`；Redis 为从库、链路 `up`、同步状态 `0`。
- B 容灾应用停止，`18080` 不监听；临时恢复阶段以外 `25432/26379` 不监听。
- B 原部署容器 ID、启动时间、端口、网络和卷基线不变；A 在线准备阶段原容器和 PostgreSQL 进程启动时间不变。
- 所有状态允许的变更命令先跑对应 `--dry-run`，并确认容器角色、卷列表、监听端口和状态文件没有变化。
- `sync-release --dry-run` 前后双端 `.env` 和 `state/release-image.env` 哈希必须不变；实际同步不得改变 A/B 数据库、Redis、应用容器 ID 或启动时间。
- 实际 `sync-release` 后双端 `SUB2API_IMAGE` 各自恰好出现一次且为同一 digest，B 已缓存该 digest，双端发布记录一致，A `status --machine` 显示 `image_sync=ok`。
- B `enable --dry-run` 必须在发布配置、缓存或同步记录漂移时于数据库提升前失败；同步成功后通过镜像门禁，并且前后 `.env`、状态文件、数据库角色和应用状态不变。
- 使用隔离 fixture 或 mock 覆盖：同一标签对应新 digest、无 RepoDigest、仓库不匹配、B 缓存缺失、双端记录漂移、B 非 `standby` 和 SSH 失败；不得用生产数据库提升来制造这些分支。
- 非法状态的 `--dry-run` 必须在角色保护处失败，不能为了展示计划而绕过状态机。
- 用包含反斜杠和冒号的测试密码验证 `.pgpass` 结果符合 PostgreSQL 转义规则，但不得把真实密码写入测试输出。
- 来源限制出口的 PostgreSQL 和 Redis 协议测试必须从真实允许的另一节点执行。
- 修改部署产物后，对比本地受管文件与服务器文件 SHA256，确保两端部署内容没有漂移。
- Python Agent 测试必须断言：A `image_sync=drift` 且其它门禁健康时不停止 A；B 镜像记录漂移时 `owner_healthy` 和 `b_failover_eligible` 均为 false。
- Worker 测试必须断言：A 节点报告 `imageSyncHealthy=false` 时仍可续租；B_ACTIVE 报告同字段为 false 时拒绝续租。
- timer shell 测试必须覆盖 `image_sync=ok` 无动作、新容器不足 120 秒跳过、稳定后按 dry-run→实际同步顺序执行、最终 `image_sync=ok`。
- 服务器验证必须确认 timer 只存在于 A，B 不存在同名 timer；手工启动 oneshot 时 `image_sync=ok` 路径不修改双端文件或容器。

实际提升、删除卷、改变复制方向和入口切换只允许在独立维护窗口演练，不能作为普通静态修改的自动测试。

### 7. Wrong vs Correct

#### Wrong

```text
A SSH 超时
-> 认为 A 已宕机
-> 直接提升 B
```

问题：网络分区时 A 可能仍在写入，最终形成双主和不可自动合并的数据分叉。

#### Correct

```text
A SSH 超时
-> 在云厂商控制台确认 A 已关机或停止
-> B enable --dry-run
-> B enable，并输入独立 fencing 确认口令
-> 验证 B 后人工切换入口
```

#### Wrong

```text
pg_basebackup 完成并提升新 A
-> 直接让 B 读取旧物理槽
-> 原地把 B 声明为 standby
```

问题：`pg_basebackup` 不复制物理槽，且提升后的 A 已进入新时间线。

#### Correct

```text
提升新 A
-> 启动 A 复制出口
-> 在新 A 创建 B 专用物理槽
-> 从 B 验证 IDENTIFY_SYSTEM 和 READ_REPLICATION_SLOT
-> 删除且仅删除 B 容灾卷
-> 从新 A 全量重建 B
```

#### Wrong

```text
A 使用 build-latest 更新成功
-> 只确认标签文字没有变化
-> 不同步容灾镜像
-> A 故障时让 B 按旧缓存启动
```

问题：同一标签可能已指向新镜像，数据库迁移已复制到 B，但备用应用仍是旧版本。

#### Correct

```text
A 更新并健康
-> A sync-release --dry-run
-> A sync-release
-> B 预缓存精确 digest
-> 双端 status 显示 image_sync=ok
-> B enable --dry-run 验证镜像门禁
```

#### Wrong

```text
A 使用同一标签更新到新 digest
-> image_sync=drift
-> A 因 B 镜像未就绪停止续租
-> 健康 A 在租约到期后被 self-fencing
```

问题：发布接管就绪度与主节点自身健康被错误合并，备机准备失败会主动制造业务中断。

#### Correct

```text
A 使用同一标签更新到新 digest
-> A 自身健康，继续续租
-> B 接管门禁关闭
-> A 独立 timer 等待容器稳定 120 秒
-> sync-release --dry-run
-> sync-release 精确同步 digest
-> image_sync=ok，B 恢复接管资格
```

---

## Scenario: HA 控制面心跳、租约与在线参数变更

### 1. Scope / Trigger

- Trigger: 修改 HA Agent 的 `interval_seconds`、`request_timeout_seconds`、`lease_state_command`、`lease_probe_timeout_seconds`、`detailed_probe_timeout_seconds`，或 Worker 的 `LEASE_TTL_SECONDS` 时，必须按本节同步代码、示例配置、测试、运行配置和额度估算。
- Trigger: 在线部署 Worker/Agent、切换 `observe`/`automatic`、轮换 `ADMIN_TOKEN` 或核对 Cloudflare 调用量时，也必须按本节执行。
- Scope: Cloudflare Worker + Durable Object、A/B HA Agent、管理员控制令牌、systemd Agent 服务和免费额度验证。
- 当前生产契约是：两个 Agent 每 10 秒各发送一次合并心跳，请求超时 4 秒，租约关键探测总预算 5 秒，详细探测总预算 20 秒，租约 TTL 固定为 45 秒。

### 2. Signatures

Agent 真实配置和示例配置必须包含：

```json
{
  "interval_seconds": 10,
  "request_timeout_seconds": 4,
  "lease_state_command": [
    "/root/sub2api-ha-export/scripts/verify-cutback.sh",
    "--machine"
  ],
  "lease_probe_timeout_seconds": 5,
  "detailed_probe_timeout_seconds": 20
}
```

Worker 变量必须包含：

```text
LEASE_TTL_SECONDS=45
CONTROL_REQUEST_DAILY_LIMIT=100000
```

管理员模式切换入口：

```bash
python3 -m sub2api_ha.cli observe \
  --config /etc/sub2api-ha/agent.json \
  --admin-token-file /etc/sub2api-ha/admin-token

python3 -m sub2api_ha.cli automatic \
  --config /etc/sub2api-ha/agent.json \
  --admin-token-file /etc/sub2api-ha/admin-token
```

运行配置只读核验：

```bash
python3 - <<'PY'
import json

with open("/etc/sub2api-ha/agent.json", encoding="utf-8") as config_file:
    config = json.load(config_file)
for key in (
    "interval_seconds",
    "request_timeout_seconds",
    "lease_state_command",
    "lease_probe_timeout_seconds",
    "detailed_probe_timeout_seconds",
):
    print(f"{key}={config[key]}")
PY
stat -c '%a %U:%G' /etc/sub2api-ha/agent.json /etc/sub2api-ha/admin-token
```

Cloudflare 调用量使用账户 GraphQL Analytics 的 `workersInvocationsAdaptive` 数据集核对，过滤目标 `scriptName`，至少汇总 `requests`、`errors` 和时间戳。

### 3. Contracts

#### 3.1 心跳、租约与调用量

- `request_timeout_seconds` 必须严格小于 `interval_seconds`；当前 `4 < 10`。
- 租约关键探测必须使用不依赖对端 SSH 的本地命令，当前总预算为 5 秒；A 使用 `verify-cutback.sh --machine`，不能使用会读取 B 状态的完整 `switch-mode.sh status --machine`。
- 租约关键探测、本地 HTTP 门禁和合并 report 必须发生在跨节点详细探测之前。A 稳态 `owner=A/state=A_ACTIVE` 不执行 B SSH。
- 非稳态详细探测总预算为 20 秒。超时后必须保留已完成的心跳，只执行本地写入门禁并跳过 acquire、提升、重建、回切和入口切换。
- 所有子命令共享同一个探测总预算，禁止把 5 秒或 20 秒分别应用到状态脚本、systemd、Docker 和 HTTP 后累加成更长阻塞。
- 缓存租约确认过期后，fail-closed 路径不得调用租约探测、详细探测或 B SSH。没有可用 `LocalState` 时必须直接尝试停止应用，不能为了确认容器是否运行而推迟 fencing。
- 当前租约必须满足至少四次完整失败重试的时间预算：`LEASE_TTL_SECONDS >= 4 * interval_seconds + request_timeout_seconds`，即 `45 >= 4 * 10 + 4`。
- 10 秒心跳配 30 秒租约只剩约三次机会，公网连续抖动时余量偏小；60 秒租约虽然更稳，但会把故障确认延长到约 50–60 秒。当前选择 45 秒，故障确认约为 35–45 秒。
- 健康稳态中，每个 Agent 每轮只发送一次合并状态报告；owner 的租约续期由该报告合并完成，不得再额外发送独立 `renew`。
- 理论日请求基线按 `节点数 * 86400 / interval_seconds` 计算。当前为 `2 * 86400 / 10 = 17280` 次/天；启动时的 `status`、管理员操作、告警和真实迁移动作属于少量附加请求。
- `observe` 和 `automatic` 使用相同心跳频率。切回观察模式不会降低调用量；A 的 `sub2api-ha-release-sync.timer` 不调用 Worker，不计入控制面请求基线。
- 不得仅通过延长租约降低请求量。请求量由心跳间隔决定，租约只决定失联后的 fail-closed 和接管等待时间。

#### 3.2 在线变更顺序

从 `5 秒心跳 / 30 秒租约` 在线调整到 `10 秒心跳 / 45 秒租约` 时，顺序固定为：

```text
部署 Worker 45 秒 TTL
-> 确认 owner/epoch/state/mode 不变且 A 新租约已接近 45 秒
-> 更新并重启 B Agent 为 10 秒
-> 确认 B 仍为 standby、容灾应用停止、B 原单机容器不变
-> 更新并快速重启 A Agent 为 10 秒
-> 确认 A 继续持有租约、应用容器 ID/StartedAt 不变
-> 核对公共健康、DNS、timer 和 Cloudflare 调用节奏
```

- Worker 必须先扩大租约余量，再降低 Agent 心跳频率；不得先把 A 调慢后继续使用旧的短租约。
- 非 owner 的 B 先更新，活动 owner A 最后更新，降低变更期间丢租约的风险。
- Worker 部署后不得重新执行 bootstrap。必须读取现有 Durable Object 状态，并确认 `owner`、`epoch`、`state`、`mode` 和入口没有被重置。
- Agent 安装和重启只影响 `sub2api-ha-agent.service`，不得重启 A 业务容器、数据库、Redis、Tunnel 或 B 原单机栈。
- 回滚到更短租约时顺序相反：先把 B、A Agent 恢复为更快心跳，确认稳定后，最后再缩短 Worker TTL。

#### 3.3 真实配置原子更新

- `/etc/sub2api-ha/agent.json` 必须使用 JSON 解析器更新单键，禁止用 `sed`、正则或字符串替换修改结构化配置。
- 更新时在 `/etc/sub2api-ha/` 同目录创建临时文件，完整解析并序列化 JSON，设置 `0600 root:root`，再使用原子 `rename`/`mv` 替换。
- 不能假设 A/B 都安装了 `jq`。有 `jq` 时可使用 `jq '.interval_seconds = 10'`；缺少时使用 Python 标准库 `json` + `tempfile` + `os.replace`，不得为了单次部署临时安装额外软件。
- 任一步在原子替换前失败时，原配置必须保持不变；产生的空临时文件应在核对路径和大小后清理。
- `install-agent.sh` 只安装传入的真实配置，不负责推导新的心跳参数。安装前后的配置文件都必须单独解析核验，不能只看到“安装完成”就认为运行参数已更新。

#### 3.4 管理员令牌与模式切换

- Cloudflare Worker Secret 写入后不可读取。创建或轮换 `ADMIN_TOKEN` 时，必须在同一操作中把相同值写入 A `/etc/sub2api-ha/admin-token`，权限为 `0600`；不得只写 Worker 后丢失本地副本。
- 轮换 `ADMIN_TOKEN` 只影响管理员控制命令，会使旧管理员令牌失效；不得改动 A/B 节点 HMAC 密钥或 Tunnel Token。
- 执行 `observe` 或 `automatic` 前先验证管理员令牌文件存在且权限正确。缺失时应轮换新令牌，而不是从日志、shell history 或版本库尝试恢复明文。
- `observe`/`automatic` 正常切换只改变 `mode`；`owner`、`epoch`、`state`、`entry_tunnel` 和当前租约持有者不得变化。
- `automatic` 开关本身不会立即把业务切到 B。只有后续租约过期且 B 全部门禁通过时，才允许进入故障接管状态机。

### 4. Validation & Error Matrix

| 条件 | 必须行为 |
|---|---|
| `request_timeout_seconds >= interval_seconds` | Agent 配置加载失败，不启动循环 |
| `lease_probe_timeout_seconds >= interval_seconds` | Agent 配置加载失败，避免单次本地探测耗尽整个心跳周期 |
| A 稳态租约探测命令包含 B SSH | 视为错误配置，不进入线上观察计时 |
| 详细探测超时 | 本轮 report 保持有效，只执行本地租约门禁并跳过所有状态机动作 |
| `LEASE_TTL_SECONDS` 不是当前固定值 `45` | Worker 返回 `INVALID_TTL`，不得用非预期 TTL 运行 |
| Worker 已部署但 A 续租后 TTL 仍接近旧值 | 停止 Agent 参数部署，先排查 Worker 版本、变量和 Durable Object 请求 |
| 先把 A 改为 10 秒、Worker 仍是 30 秒 | 视为错误部署顺序；恢复 A 快心跳或立即先完成 Worker TTL 部署 |
| B Agent 更新后数据库/Redis 角色改变或容灾应用启动 | 立即停止部署并按现场状态排查，不继续更新 A |
| A Agent 重启后 owner、epoch、state 或入口变化 | 停止部署，保持 `observe`，核对 Worker 事件和两端角色 |
| A 应用容器 ID、StartedAt 或 restart policy 因 Agent 更新变化 | 视为越界变更，停止并调查安装/重启命令 |
| JSON 更新工具不存在或解析失败 | 原配置保持不变；改用现有结构化解析器，不使用字符串替换 |
| 配置或管理员令牌权限不是 `0600` | 不重启 Agent，不执行模式切换 |
| Worker 中有 `ADMIN_TOKEN` 但 A 本地文件缺失 | 轮换新令牌并同时写入两端，不尝试读取 Worker Secret |
| 稳态调用量持续显著高于约 12 次/分钟 | 检查重复 Agent、额外 `status`/重试、错误循环和人工查询 |
| Cloudflare Analytics `errors > 0` | 检查 Worker 日志、节点认证、超时和控制面响应，不以请求总量正常掩盖错误 |

### 5. Good/Base/Bad Cases

- Good: Worker 先切到 45 秒 TTL，A 仍按 5 秒续租；随后 B、A 依次切到 10 秒，整个过程 owner=A、mode=observe、业务入口不变。
- Good: 两个 Agent 稳态约每分钟 12 次请求，Analytics 错误为 0，人工状态查询只造成少量可解释增量。
- Good: B 没有 `jq`，部署使用 Python JSON 解析器原子更新单键，最终配置仍是 `0600 root:root`。
- Good: 从 `automatic` 切回 `observe` 后 A 继续持有同一 epoch 和租约，B 保持 standby，DNS 仍指向 A。
- Base: 只重新部署 Worker 代码但不改变 TTL 时，仍要验证 Durable Object 状态和现有 Secrets 没有被重置或遗漏。
- Base: 只重启 B Agent 时允许短暂缺少 B 心跳，但不得启动 B 应用或改变复制角色。
- Bad: 为省调用量把心跳直接改为 30 秒并继续使用 45 秒租约，导致一次超时就接近租约边界。
- Bad: 先停止 A Agent，再慢慢修改配置和上传文件，导致活动 owner 在维护动作中主动丢租约。
- Bad: 用 `sed -i 's/5/10/'` 修改 JSON，误改其它数值或产生无效配置。
- Bad: 只在 Worker 中轮换管理员令牌，之后因为没有本地副本而无法执行 pause、observe 或 resume。

### 6. Tests Required

本地自动化至少执行：

```bash
cd .trellis/tasks/07-11-sub2api-automatic-failover-orchestration/artifacts/automation
python3 -m unittest discover -s test -v
python3 -m ruff check sub2api_ha test
python3 -m ruff format --check sub2api_ha test

cd ../cloudflare-worker
npm test
npm run check
npm run deploy:dry-run
```

断言点：

- Agent 未显式配置时默认 `interval_seconds=10`，`request_timeout_seconds=4`；超时不小于间隔时拒绝配置。
- Agent 未显式配置时默认租约探测预算 5 秒、详细探测预算 20 秒；租约探测预算不小于心跳间隔时拒绝配置。
- A 稳态心跳测试必须断言详细探测不会被调用；非稳态详细探测超时测试必须断言 report 已完成且未执行任何状态机动作。
- 缓存租约过期测试必须在不提供本地状态时断言：详细探测调用次数为零，应用停止动作执行一次。
- Worker 使用 `LEASE_TTL_SECONDS=45` 时，bootstrap、续租、B acquire、handoff 和 resume 都写入 45 秒租约；其它固定值返回 `INVALID_TTL`。
- 健康 owner 每轮只调用一次合并 report，不额外调用 renew。
- 两节点理论日请求数按公式得到 `17280`，`observe`/`automatic` 不改变稳态频率。
- A/B 示例配置、Python 默认值、Worker 变量、README、PRD/design 和本规范中的心跳/TTL 数值一致。

线上部署验证至少覆盖：

```text
Worker deploy dry-run 显示 LEASE_TTL_SECONDS=45
Worker 实际部署后 A TTL 大于 30 秒且持续刷新
B 配置解析为 interval=10、timeout=4，仍为 standby
A 配置解析为 interval=10、timeout=4，仍为唯一 owner
A 应用容器 ID、StartedAt、health 和 restart policy 不变
B 原单机容器 ID、StartedAt 和 restart policy 不变
A release-sync timer enabled/active，B 不存在该 timer
公共健康返回 200，DNS 仍指向当前 owner Tunnel
Cloudflare Analytics 呈现约 10 秒心跳节奏且 errors=0
```

### 7. Wrong vs Correct

#### Wrong

```text
A Agent 先从 5 秒改为 10 秒并重启
-> Worker 仍使用 30 秒 TTL
-> 再慢慢部署 Worker 和 B
```

问题：活动 owner 先降低续租频率，变更窗口内只保留很少的重试机会。

#### Correct

```text
Worker 先部署 45 秒 TTL并验证 A 新租约
-> B Agent 改为 10 秒并验证 standby
-> A Agent 改为 10 秒并快速重启
-> 验证 owner/epoch/入口、业务容器和请求节奏
```

#### Wrong

```bash
sed -i 's/"interval_seconds": 5/"interval_seconds": 10/' /etc/sub2api-ha/agent.json
chmod 644 /etc/sub2api-ha/agent.json
```

问题：字符串替换不能保证 JSON 结构正确，且权限放宽会暴露真实运行配置。

#### Correct

```text
用 jq 或 Python json 完整解析配置
-> 在同目录写临时文件
-> 设置 0600 root:root
-> 原子替换 agent.json
-> 重新解析并打印 interval/timeout 验证
```

#### Wrong

```text
wrangler secret put ADMIN_TOKEN
-> 不保存本地副本
-> 之后尝试从 Cloudflare 读取 Secret 明文
```

#### Correct

```text
生成新的随机 ADMIN_TOKEN
-> 同一次操作写入 Worker Secret
-> 同一次操作写入 A /etc/sub2api-ha/admin-token
-> 验证权限为 0600
-> 执行模式切换并确认只有 mode 改变
```
