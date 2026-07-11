# A 在线复制出口阶段执行结果

执行时间：2026-07-11。

## 已完成

- 在 A 创建独立目录 `/root/sub2api-ha-export` 和 Compose 项目 `sub2api-ha-export`，未编辑 A 原 `/root/sub2api/deploy/docker-compose.yml`。
- 使用固定摘要的 `alpine/socat:1.8.0.3` 镜像启动两个转发容器：
  - `sub2api-ha-postgres-export`：宿主机 `15432` 转发至现有 `postgres:5432`。
  - `sub2api-ha-redis-export`：宿主机 `16379` 转发至现有 `redis:6379`。
- 两个 socat监听器只接受 B 当前固定公网 `/32` 来源；实际地址仅保存在 A 的 `.env`，未写入任务文件。
- 在 A PostgreSQL在线创建专用 replication 用户和物理复制槽 `sub2api_b_standby`。
- 为专用复制用户增加 A Docker网络来源的 HBA规则，并执行 `pg_reload_conf()`。
- 在线设置 `wal_keep_size=2GB`、`max_slot_wal_keep_size=8GB`。
- 复制密码由 A 本机生成，只保存在 `/root/sub2api-ha-export/secrets.env`，权限为 `0600`。

## 验证结果

- A PostgreSQL复制用户具备 `LOGIN` 和 `REPLICATION` 权限。
- 物理复制槽存在，当前尚未建立持续复制，因此状态为 inactive。
- HBA规则解析无错误，认证方式为 `scram-sha-256`。
- `wal_keep_size` 实际值为 `2048 MB`，`max_slot_wal_keep_size` 实际值为 `8192 MB`，两项均为 `sighup` 上下文且 `pending_restart=false`。
- B 通过 A `15432` 执行 `pg_isready`，返回 accepting connections。
- B 通过 A `16379` 执行 Redis `PING`，返回 `PONG`。
- socat日志确认两条协议检查均来自配置的 B 固定公网来源，并成功连接 A 内部 PostgreSQL、Redis服务。
- 使用专用复制用户从 A Docker网络建立 replication协议连接并执行 `IDENTIFY_SYSTEM` 成功，证明 HBA、SCRAM密码和 replication权限可用；该测试未启动持续复制，也未激活物理复制槽。
- A 本地部署包与服务器上的全部受管文件 SHA256逐项一致。

## 零重启证据

- A 原 Sub2API、PostgreSQL、Redis三个容器的 ID 和容器启动时间未变化。
- PostgreSQL `pg_postmaster_start_time()` 未变化。
- A 原 Compose 文件和原 `.env` 的 SHA256未变化。
- A 原 `http://127.0.0.1:8080/health` 在阶段前后均正常。
- 基线脚本初版因 Docker挂载数组顺序不稳定产生误报；已保留原始变更前基线，并在校验容器身份、配置哈希和 PostgreSQL进程启动时间后迁移为排序后的稳定格式，最终比对通过。

## B 状态

- B 原有 Sub2API三个容器基线再次比对通过，`8080/health` 正常，`18080` 仍空闲。
- B 尚未创建任何 `sub2api-dr-*` 容器、卷或网络。

## 现有状态备注

- A 原 `sub2api` 容器在本阶段开始前 Docker health 状态已是 `unhealthy`，但宿主机 `8080/health` 正常；本阶段未重启、重建或修改该容器。
- A Redis当前未配置密码；阶段 2 通过 socat来源 `/32` 限制避免其他来源使用转发端口，阶段 3 的 Redis从库配置必须保持空 `masterauth`。

## 下一阶段边界

- 当前 `pg_stat_replication` 为空，尚未执行 `pg_basebackup`，不代表持续复制已经建立。
- 下一步是阶段 3：把复制凭据和 A 应用运行配置安全同步到 B，初始化 PostgreSQL备库和 Redis从库，并保持备用应用停止；开始前需要再次取得用户确认。
