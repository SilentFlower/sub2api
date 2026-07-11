# 双端模式控制与回切准备结果

执行时间：2026-07-11。

## 已完成

- B `/root/sub2api-dr/scripts/switch-mode.sh` 已支持 `status`、`standby`、`enable`、`freeze`。
- A `/root/sub2api-ha-export/scripts/switch-mode.sh` 已支持 `status`、`prepare-from-b`、`cutback-to-a`、`restore-b-standby`。
- A、B 的 `README.md` 已统一改为简明操作手册，按日常检查、A 故障切 B、A 修复回切、状态含义和禁止事项组织，并已同步部署到两台服务器。
- B 已部署临时恢复出口定义 `compose.recovery-export.yaml`，但恢复出口容器未启动，宿主机 `25432`、`26379` 未监听。
- B 已部署 `prepare-recovery-source.sh` 和 `restore-standby-from-a.sh`，分别用于 B 作为 A 恢复源，以及 A 回切后从新 A 主库重新初始化 B。
- A 已部署 `compose.recovery.yaml` 和 `compose.recovery-promoted.yaml`。Compose渲染确认 PostgreSQL只挂载新的父级命名卷 `/var/lib/postgresql`，设置 `PGDATA=/var/lib/postgresql/data` 和 `volume.nocopy`，旧子目录挂载未进入恢复配置。
- A 已部署 PostgreSQL基础备份、应用数据双向同步、角色验证和统一编排脚本。应用数据同步排除 `logs/`，并验证 `.installed`、`config.yaml`。
- A 创建了仅用于调用 B 容灾辅助脚本的专用 SSH私钥；私钥只保存在 A，B 仅安装对应公钥，任务目录和版本库未保存私钥或密码。
- 两次物理重建前都增加复制协议检查：执行 `IDENTIFY_SYSTEM` 并读取目标物理复制槽，确认复制账号、密码和槽可用后才允许停止旧容器或删除目标卷。

## 当前真实状态验证

- A 统一脚本识别 A 为 `legacy-active`，PostgreSQL为主库、Redis为主库、Sub2API运行中。
- B 统一脚本识别 B 为 `standby`，PostgreSQL处于恢复状态且 WAL receiver 为 `streaming`，Redis为从库、主链路为 `up`、同步过程为 `0`。
- A 脚本可通过专用 SSH密钥读取 B 状态，不需要在脚本或环境文件中保存 SSH密码。
- B `standby --dry-run` 和 `enable --dry-run` 均成功，未提升数据库或启动应用。
- B 当前为备库时，`freeze --dry-run`、`prepare-recovery-source.sh --dry-run`、`restore-standby-from-a.sh --dry-run` 均按角色保护返回拒绝。
- A 当前仍是旧主且 B 未提升时，`prepare-from-b --dry-run`、`cutback-to-a --dry-run`、`restore-b-standby --dry-run` 均按状态机返回拒绝。
- B 原部署基线与容灾备库基线检查通过；A 原容器和 PostgreSQL进程基线检查通过，未发生重启或重建。
- B 再次执行复制验证通过，PostgreSQL接收与回放 LSN一致，Redis主从 offset一致。
- A 未创建 `sub2api-ha-app-data`、`sub2api-ha-postgres-data`、`sub2api-ha-redis-data` 恢复卷。
- B 未创建或启动临时恢复出口容器，`18080`、`25432`、`26379` 均未监听。

## 静态检查

- 全部 A/B Bash脚本通过 `bash -n`。
- 全部 A/B Bash脚本通过 ShellCheck warning级别检查。
- B 备用、提升和临时恢复出口 Compose定义通过 `docker compose config`。
- A 恢复和提升覆盖文件通过本地及 A 服务器真实环境的 `docker compose config`。
- 本地受管文件与两台服务器对应文件的 SHA256一致。

## 未执行

- 未提升 B PostgreSQL或 Redis。
- 未启动 B 容灾应用，未占用 `18080`。
- 未启动 B 临时恢复出口，未开放 `25432`、`26379`。
- 未停止、移除或重建 A 原业务容器。
- 未创建或清理 A、B 恢复数据卷。
- 未冻结生产写入、改变复制方向或切换公共入口。

## 保留事项

- B 仍未报告 NTP同步，实际提升前必须处理或确认时间同步；复制追平继续以 PostgreSQL LSN和 Redis offset为硬判断。
- 公共入口方式仍未确定，脚本只在节点就绪后提示人工切换。
- 实际故障提升、A 重建、回切和 B 重新入备必须在独立维护窗口中逐阶段确认。
- 最后校验期间，当前工作机直连 A 的 SSH端口临时返回拒绝，但 A `8080/health` 正常，且从 B 到 A 的 SSH连接正常；通过 B 跳板完成文件校验，未修改 A 的 SSH服务配置。

## Check-all 修复

- 初版 B 恢复源准备脚本会从 B 本机 `127.0.0.1` 探测只允许 A 固定来源的 socat端口，本机连接会被来源限制拒绝。
- 已移除该错误协议探测。B 只验证恢复出口容器和宿主端口已就绪；A 随后从真实允许来源执行 PostgreSQL `IDENTIFY_SYSTEM`、`READ_REPLICATION_SLOT` 和 Redis `PING`，全部通过后才允许停止 A 旧容器。
- 初版 A PostgreSQL恢复初始化脚本在生成 `.pgpass` 时少了一层 shell转义，复制密码包含反斜杠或冒号时可能写出无效条目，导致 A 从 B 基础备份后无法建立 WAL receiver。
- 已将 A 的密码转义实现与 B 侧已验证实现对齐。使用 PostgreSQL 18 Alpine镜像实测 `a\b:c` 会生成 `a\\b\:c`；全部 20 个 A/B Bash脚本重新通过 `bash -n` 和 ShellCheck warning级别检查，本地文件与 A 已部署文件的 SHA256一致。
- 初版 `restore-b-standby` 会先调用 B 的恢复脚本校验 A 上的 B 专用物理复制槽，之后才在新 A 主库运行 `configure-primary.sh` 创建该槽。隔离的 PostgreSQL 18 实验确认 `pg_basebackup` 不会复制源库物理复制槽，因此正常回切后该预检必然提前失败。
- 已将 `restore-b-standby --dry-run` 保持为纯只读角色、脚本存在性和计划检查；实际执行在独立确认后先启动 A 复制出口并重建复制用户、HBA和物理槽，再调用 B 的源端协议预检和全量重建。全部脚本重新通过语法与 ShellCheck检查，修正后的 A 脚本和 README已部署且 SHA256一致，A/B 当前角色和原业务容器未变化。
