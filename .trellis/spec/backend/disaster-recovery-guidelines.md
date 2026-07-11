# Disaster Recovery Guidelines

> 本项目双节点 Sub2API 主备容灾的部署、切换和恢复契约。

---

## Scenario: 双节点人工主备容灾

### 1. Scope / Trigger

- Trigger: 修改 A/B 容灾目录、Compose 文件、切换脚本、复制参数、端口、卷、网络、SSH 编排或操作手册时，必须按本节检查。
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
/root/sub2api-ha-export/scripts/switch-mode.sh prepare-from-b [--dry-run]
/root/sub2api-ha-export/scripts/switch-mode.sh cutback-to-a [--dry-run]
/root/sub2api-ha-export/scripts/switch-mode.sh restore-b-standby [--dry-run]
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
```

- 每行格式固定为 `key=value`，不得把说明文字写入标准输出。
- A 的回切脚本至少依赖 B 的 `mode`、`postgres_lsn` 和 `redis_master_offset`。
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
| A `prepare-from-b` | A 为 `legacy-active` 或 `offline`，且 B 为 `active` 或 `active-stopped`；A 已是 `standby-from-b` 时直接成功返回 |
| A `cutback-to-a` | A 为 `standby-from-b`、`cutback-postgres-promoted` 或 `active-recovered-stopped`，且 B 为 `active` 或 `active-stopped`；A 已是 `active-recovered` 时直接成功返回 |
| A `restore-b-standby` | A 必须为 `active-recovered`；B 为 `active-stopped` 时全量重建，B 已是 `standby` 时只同步非日志应用数据 |
| B `standby` | PostgreSQL 必须仍在恢复，Redis 必须是已追平的从库；若应用意外运行，只停止应用后重新验证 |
| B `enable` | 容灾 PostgreSQL、Redis 容器必须存在；实际提升始终要求人工 fencing 口令，已完成阶段按状态标记和真实角色幂等复核 |
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
| 来源受限出口只能从本机监听、另一节点协议验证失败 | 不停止旧服务、不删除卷、不开始基础备份 |
| B 未报告 NTP 同步 | `enable` 明确警告；真实演练前必须人工处理或确认时间同步，追平判断仍以 LSN/offset 为准 |
| 健康检查失败或确认口令不匹配 | 当前阶段立即停止，不自动越过或切换入口 |

### 5. Good/Base/Bad Cases

- Good: A 正常提供服务，B 显示 `standby`，PostgreSQL 为 `streaming`，Redis 链路为 `up` 且同步完成，B 容灾应用和 `18080` 均未运行。
- Good: A 故障时先在控制台确认 A 已停止，再执行 B `enable --dry-run` 和 `enable`，最后人工切换公共入口。
- Good: A 修复后使用新恢复卷从 B 重建，冻结 B 后按 LSN/offset 等待完全追平，再提升 A 并从新 A 全量重建 B。
- Base: 只修改 SSH 目标或外部端口时，同步修改两端环境文件并重新验证 Compose、SSH、协议连接和 `status --machine`。
- Base: `status` 可以长期执行；B 容灾数据库可以常开复制，但备用应用必须停止。
- Bad: 因为 A ping 不通或 SSH 超时就直接提升 B，这不构成 fencing，可能造成双主写入。
- Bad: 只改 `B_DR_ROOT` 后移动 B 目录，却不更新 README、远程命令、预检和服务器部署文件。
- Bad: 手工运行 `docker compose down`、删除卷、启动 B 容灾应用或绕过统一脚本修改数据库角色。
- Bad: 在 B 本机用 `127.0.0.1` 自测只允许 A 来源的恢复出口，并把被拒绝误判为出口故障。
- Bad: A 提升后直接让 B 使用旧数据目录连接新时间线，或在创建新物理槽前先执行 B 恢复源预检。

### 6. Tests Required

静态修改至少执行：

```bash
bash -n /root/sub2api-ha-export/scripts/*.sh
bash -n /root/sub2api-dr/scripts/*.sh
shellcheck -S warning /root/sub2api-ha-export/scripts/*.sh
shellcheck -S warning /root/sub2api-dr/scripts/*.sh
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
/root/sub2api-dr/scripts/switch-mode.sh status
```

断言点：

- `status --machine` 的字段名、`key=value` 格式和退出码保持稳定。
- 正常态为 A `legacy-active` 或 `active-recovered`，B `standby`。
- B PostgreSQL 为恢复状态且 WAL receiver 为 `streaming`；Redis 为从库、链路 `up`、同步状态 `0`。
- B 容灾应用停止，`18080` 不监听；临时恢复阶段以外 `25432/26379` 不监听。
- B 原部署容器 ID、启动时间、端口、网络和卷基线不变；A 在线准备阶段原容器和 PostgreSQL 进程启动时间不变。
- 所有状态允许的变更命令先跑对应 `--dry-run`，并确认容器角色、卷列表、监听端口和状态文件没有变化。
- 非法状态的 `--dry-run` 必须在角色保护处失败，不能为了展示计划而绕过状态机。
- 用包含反斜杠和冒号的测试密码验证 `.pgpass` 结果符合 PostgreSQL 转义规则，但不得把真实密码写入测试输出。
- 来源限制出口的 PostgreSQL 和 Redis 协议测试必须从真实允许的另一节点执行。
- 修改部署产物后，对比本地受管文件与服务器文件 SHA256，确保两端部署内容没有漂移。

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
