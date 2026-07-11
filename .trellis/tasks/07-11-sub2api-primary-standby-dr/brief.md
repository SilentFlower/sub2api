# Brief — Sub2API 主备容灾部署

## Goal

- 在不引入第三节点或自动 fencing 的条件下，建设可人工确认的“A 主、B 备”容灾闭环：B 可在 A 故障后提升接管，A 修复后可从 B 重建并受控回切，最终恢复 A 主、B 备。

## Scope

- 保留 B `/root/sub2api-dr/scripts/switch-mode.sh`，完成 `status`、`standby`、`enable`、`freeze`；`freeze` 只停止 B 容灾应用，不改变已提升数据库的主库角色。
- 在 A `/root/sub2api-ha-export` 增加恢复 Compose覆盖文件和统一 `switch-mode.sh`，提供 `status`、`prepare-from-b`、`cutback-to-a`、`restore-b-standby`。
- A 从 B 重建时使用新的明确命名卷，修正 PostgreSQL 18 父级卷布局，并同步 PostgreSQL、Redis及全部非临时应用数据；故障前旧卷默认保留。
- B 增加仅在 A 恢复期间启用的反向复制出口和辅助脚本，使 A 可从 B 初始化，并在 A 回切后从新 A 主库重新初始化 B。
- 两端所有变更命令支持 `--dry-run`、状态校验、幂等保护和独立确认口令；更新两端 README和操作顺序。
- 当前只实施脚本、Compose定义、无凭据模板及静态/只读验证，不执行实际 B 提升、A/B 数据卷重建、复制方向切换或公共入口切换。

## Non-Goals

- 不实现三节点仲裁、自动 fencing、全自动故障提升或 A/B 双活。
- 不修改、停止或重建 B 现有 `/root/sub2api` 单机部署及其 `8080` 服务。
- 不在 B 接管前重建 A 当前 PostgreSQL匿名卷，也不在本轮脚本准备中执行生产回切。
- 不把 SSH密码、数据库密码、Token或真实业务凭据写入任务文件和版本库；不实施通用 SSH、防火墙或账号安全加固。
- 公共入口方式未确认前，不自动切换域名或流量，也不把“节点已就绪”报告成完整回切成功。

## Key Context

- A→B 的 PostgreSQL异步流复制和 Redis主从复制已建立：B PostgreSQL WAL receiver 为 `streaming`，B Redis主链路为 `up`，最近验证 LSN和 offset均无积压。
- B 容灾 PostgreSQL和 Redis常驻运行，B 容灾应用停止且 `18080` 空闲；B 原 Sub2API栈保持不变。
- A 原 Compose 位于 `/root/sub2api/deploy/docker-compose.yml`，项目名 `deploy`；现有服务和容器名为 `sub2api`、`sub2api-postgres`、`sub2api-redis`。
- A PostgreSQL 18真实数据位于父级匿名卷。恢复覆盖文件必须使用 Compose `!override` 完全替换旧子目录挂载，将新命名卷挂载到 `/var/lib/postgresql`，并设置 `PGDATA=/var/lib/postgresql/data` 和 `volume.nocopy`。
- A 提升会产生新 PostgreSQL时间线，因此 B 重新入备必须执行新的物理基础备份，不能只修改连接配置或原地降级。
- A 脚本通过标准 SSH调用 B 辅助脚本，不保存密码或依赖 `sshpass`；远程检查失败必须发生在本地破坏性动作之前。
- B 当前未报告 NTP同步，实际提升前必须处理；复制追平以 PostgreSQL LSN和 Redis offset为硬判断。

## Acceptance

- B 脚本四个命令能正确识别并保护 `standby`、`active`、`active-stopped` 和异常状态；`freeze` 不改变数据库角色，已提升状态下 `standby` 拒绝原地降级。
- A 脚本四个命令能识别旧主隔离、A 从 B 追平、A 已回切为主、B 已恢复为备等状态，并在两端角色矛盾时停止。
- `prepare-from-b` 使用新恢复卷并保留旧卷；A PostgreSQL以正确 PostgreSQL 18卷布局作为 B 的备库运行，A Redis作为 B 从库运行，A 应用保持停止。
- `cutback-to-a` 只有在 B 写入冻结且 A PostgreSQL LSN、Redis offset追平后才允许提升 A和启动应用；入口未知时停在人工切换提示。
- `restore-b-standby` 从新的 A 主库重建 B PostgreSQL和 Redis，最终 B 为只读备库、Redis从库且应用停止。
- 所有变更命令的 `--dry-run` 不停止服务、不清理卷、不提升数据库、不改变复制方向、不启动写入应用；Compose、Bash、ShellCheck和状态验证通过。

## Next Step

- 进入 implement route，先完成 B `freeze` 和双端回切文件，再执行静态检查以及两台服务器上的 `status`、`--dry-run` 验证；实际生产切换留待单独维护窗口授权。
