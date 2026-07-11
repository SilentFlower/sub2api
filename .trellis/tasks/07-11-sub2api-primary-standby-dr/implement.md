# Implement Plan: Sub2API 主备容灾部署

## 阶段 1：B-only 准备

1. 记录 B 现有 Compose 项目、容器 ID、启动时间、端口、网络和卷，形成不可触碰基线。
2. 创建 `/root/sub2api-dr`，不得修改 `/root/sub2api`。
3. 写入隔离的 `compose.yaml`、`.env.example`、Redis备用配置和脚本目录。
4. 将新容器、网络和卷统一命名为 `sub2api-dr-*`。
5. 将备用应用镜像固定为 A 当前运行的可拉取 `仓库@sha256` 引用；固定 PostgreSQL 18 和 Redis 8 镜像。
6. PostgreSQL、Redis和备用应用均使用显式 profile，保证不带 profile 的默认 Compose 操作不会启动任何容灾服务。
7. PostgreSQL、Redis不发布宿主机端口；备用应用提升后使用 `18080`。
8. PostgreSQL显式设置 `PGDATA=/var/lib/postgresql/data`，所有容器、网络和卷使用明确的 `sub2api-dr-*` 名称。
9. `.env.example` 只写占位值，不写 A 或 B 的真实密码、Token 或其他凭据。
10. 拉取所需镜像，运行 `docker compose --env-file .env.example config` 和脚本语法检查。
11. 可创建隔离网络和空卷，但不初始化数据库、不启动任何容灾服务。
12. 对比阶段前后的 B 现有容器 ID、启动时间、端口和健康状态，确认没有变化。

## 阶段 2：A 在线复制出口

> 本阶段开始前再次取得用户确认。

1. 记录 A 现有三个容器 ID、启动时间和网络。
2. 创建独立的复制转发 Compose 项目，加入现有 `deploy_sub2api-network`。
3. 固定 socat 镜像摘要，监听 `15432`、`16379`，并仅接受 B 固定公网 `/32` 来源。
4. 新增 PostgreSQL、Redis转发容器，不编辑 A 原 Compose，不重建现有容器。
5. 创建 PostgreSQL replication 用户、A Docker网络来源规则和物理复制槽。
6. 在线设置 `wal_keep_size=2GB`、`max_slot_wal_keep_size=8GB` 并 reload，不重启 PostgreSQL。
7. 从 B 对 PostgreSQL、Redis转发端口执行协议级连通检查，确认 A 原业务健康状态不变。
8. 再次核对 A 现有容器 ID、启动时间和 PostgreSQL进程启动时间未变化。

## 阶段 3：建立复制

1. 在 B 的空 PostgreSQL卷执行在线 `pg_basebackup`。
2. 启动 B PostgreSQL，验证恢复状态、时间线和复制槽连接。
3. 启动 B Redis从库，验证初次全量同步和持续复制。
4. 盘点并同步 A `/app/data` 中全部非临时业务文件到 B 容灾应用卷，排除日志。
5. 以 A 当前 Compose 环境为源生成 B 容灾 `.env`，保留业务密钥和参数，只覆盖内部服务地址、资源名及备用端口。
6. 验证备用应用容器定义存在但保持停止。
7. 连续观察复制状态和延迟，确认无明显 WAL/Redis积压。

## 阶段 4：入口与提升脚本

1. 完成 B 统一模式控制脚本，提供 `status`、`standby`、`enable`、`freeze`，并明确已提升主库不能原地切回备库。
2. `standby` 仅在 PostgreSQL和 Redis仍保持复制角色时停止备用应用、检查 `18080` 并运行复制验证。
3. `enable` 复用提升脚本的人工确认、状态记录、幂等检查和失败停止逻辑，不新增绕过确认的参数。
4. `freeze` 仅在 B 已启用时停止容灾应用并验证 `18080` 已停止监听，不改变 PostgreSQL和 Redis主库角色。
5. 执行 `status`、`standby --dry-run`、`enable --dry-run` 和 `freeze --dry-run`，确认脚本不会在未确认时提升数据库、改变复制方向或启动应用。

## 阶段 5：A 恢复与回切脚本

1. 在 A 的 `/root/sub2api-ha-export` 部署恢复 Compose覆盖文件，使用新命名卷替换原 PostgreSQL、Redis和应用卷，不删除故障前旧卷。
2. PostgreSQL恢复卷挂载到 `/var/lib/postgresql`，设置 `PGDATA=/var/lib/postgresql/data` 和 `volume.nocopy`；使用 Compose `!override` 避免旧子目录挂载残留。
3. 在 B 增加临时恢复出口定义，只在 A 回切准备期间启动 PostgreSQL `25432` 和 Redis `26379` 转发，并限制为 A 固定公网来源。
4. 在 B 增加恢复源准备脚本，在线准备 A 专用复制用户访问、HBA和物理复制槽，不停止 B 主库。
5. 在 A 增加统一 `switch-mode.sh`，提供 `status`、`prepare-from-b`、`cutback-to-a`、`restore-b-standby`。
6. `prepare-from-b` 验证 B 为当前启用节点，停止 A 旧容器，创建新的恢复卷，从 B 完成 PostgreSQL基础备份、Redis同步和非临时应用数据同步，并保持 A 应用停止。
7. `cutback-to-a` 调用 B `freeze`，等待 A PostgreSQL LSN与 Redis offset追平，提升 A 数据库，使用持久化主库配置重建 A Redis，启动 A 应用并输出人工入口切换提示。
8. `restore-b-standby` 在 A 已稳定为主后重新创建 B 复制槽，远程调用 B 恢复脚本重建 `sub2api-dr-*` 数据卷，并验证 B 恢复为只读备库、Redis从库且应用停止。
9. 两端所有变更命令均支持 `--dry-run`；清理卷、提升数据库、冻结写入和改变复制方向使用独立确认口令。
10. 更新 A、B README，写明正常故障切换、A 修复准备、回切和恢复 B 为备库的命令顺序。

## 阶段 6：计划演练与生产回切

> 本阶段需要单独维护窗口和用户确认。

1. 停止 A 应用写入并等待 B 追平。
2. 提升 B PostgreSQL、Redis并启动 B 应用。
3. 切换入口并完成业务验证。
4. 使用正确卷布局重建 A PostgreSQL，并从 B 初始化为备库。
5. 将 A Redis设为 B 的从库，验证追平。
6. 冻结 B 写入并确认 A 追平，提升 A 并切回公共入口。
7. 从新的 A 主节点重新初始化 B，使 A 恢复为主、B 恢复为备。

## 验证命令

B 资源隔离：

```bash
docker compose ls
docker ps --format 'table {{.ID}}\t{{.Names}}\t{{.Status}}\t{{.Ports}}'
docker volume ls
docker network ls
cd /root/sub2api-dr && docker compose --env-file .env.example config
bash -n scripts/*.sh
ss -lntp | grep -E ':8080|:18080'
docker ps --filter 'name=^/sub2api-dr-' --format '{{.Names}} {{.Status}}'
./scripts/switch-mode.sh status
./scripts/switch-mode.sh standby --dry-run
./scripts/switch-mode.sh enable --dry-run
./scripts/switch-mode.sh freeze --dry-run
```

A 回切脚本：

```bash
cd /root/sub2api-ha-export
docker compose --env-file .env.example -f /root/sub2api/deploy/docker-compose.yml -f compose.recovery.yaml config
bash -n scripts/*.sh
./scripts/switch-mode.sh status
./scripts/switch-mode.sh prepare-from-b --dry-run
./scripts/switch-mode.sh cutback-to-a --dry-run
./scripts/switch-mode.sh restore-b-standby --dry-run
```

PostgreSQL复制：

```sql
-- A
SELECT application_name, client_addr, state, sync_state,
       write_lag, flush_lag, replay_lag
FROM pg_stat_replication;

-- B
SELECT pg_is_in_recovery();
SELECT status, receive_start_lsn, written_lsn, flushed_lsn
FROM pg_stat_wal_receiver;
```

Redis复制：

```bash
redis-cli INFO replication
redis-cli INFO persistence
```

应用提升验证：

```bash
curl -fsS http://127.0.0.1:18080/health
docker compose --profile promoted ps
```

## 风险点

- B 已有同名 Sub2API栈，任何遗漏 `sub2api-dr` 前缀的资源都可能误操作现有服务。
- A PostgreSQL真实数据位于匿名卷，阶段 2 不能触发其容器重建。
- 物理复制槽断连过久会保留 WAL并消耗 A 磁盘，需要监控槽位积压。
- B PostgreSQL卷必须为空才能执行首次 `pg_basebackup`。
- Redis仅执行运行时 `REPLICAOF NO ONE` 不足以保证重启后主库身份，提升必须同步切换持久化配置。
- B 应用不得在 PostgreSQL仍为备库时启动。
- 公共入口方式尚未确认，提升脚本的最后一步暂不能定稿。
- A 恢复覆盖文件必须完全替换旧 PostgreSQL子目录挂载，否则 PostgreSQL 18 父级卷仍会遮蔽新恢复数据。
- A 与 B 的远程编排依赖标准 SSH可用；脚本不得记录密码，SSH失败时必须在任何本地破坏性动作前停止。
- B 回切前被冻结后仍是权威数据库主节点；若 A 提升失败，不得重新开启 B 应用和继续 A 提升两个方向同时操作。
- A 提升产生新时间线后，B 必须重新基础备份，不能仅修改 `primary_conninfo` 假装重新入备。

## 回滚点

- 阶段 1：删除新增目录和空的 `sub2api-dr-*` 资源。
- 阶段 2：停止并删除独立转发项目，不影响 A 现有 Compose。
- 阶段 3：停止 B 容灾数据库并清理新容灾卷后重新初始化。
- 阶段 4：未切换入口前保持 A 为唯一主节点。
- 阶段 5：脚本和覆盖文件准备只执行静态检查与 `--dry-run`，不触发实际提升、数据卷重建或入口切换。
- 阶段 6：一旦 B 已提升，不自动回退；A 重建失败时保留 B 为主，A 回切成功但 B 重新入备失败时保留 A 为唯一主节点并保持 B 应用停止。
