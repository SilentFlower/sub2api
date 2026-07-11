# Release Operations

## Conclusion

任务提前终止。半自动主备容灾准备已部署，生产提升、入口切换、回切和重建演练未执行。

## Evidence Checked

- `task.json`
- `prd.md`
- `design.md`
- `implement.md`
- `implement.jsonl`
- `check.jsonl`
- 提交 `d5fef3ec`、`45561e0d`
- 当前工作区状态

## Drift Check

归档前缺少 `release.md`，本文件补记实际部署状态和未完成边界。

## SQL Changes

- A PostgreSQL 已在线配置物理复制用户、HBA、WAL 保留参数和 B 专用物理复制槽。
- B PostgreSQL 已建立为 A 的异步物理备库。

## Configuration Changes

- A 已部署 `/root/sub2api-ha-export` 复制出口、恢复覆盖配置和统一操作脚本。
- B 已部署 `/root/sub2api-dr` 隔离容灾栈；B 原 `/root/sub2api` 单机部署保持不变。
- A/B 已同步固定应用镜像 digest，归档时 `image_sync=ok`。
- B NTP 未同步，任何生产提升或回切演练前必须先处理或明确确认。

## Batch / Deployment Scripts / Data Repair

- 已部署 A、B 两端 `switch-mode.sh` 及辅助脚本。
- 未执行阶段 6：B 生产提升、公共入口切换、A 从 B 重建、自动或人工回切、B 从新 A 重建。

## External Systems / Dependent Platforms

- 公共入口的实际切换方式尚未接入本任务脚本。
- 自动故障切换所需的外部强一致租约服务不属于本任务，转入独立任务设计。

## Release Order

本任务不再执行新的生产发布。后续操作必须以服务器当前状态为基线，由独立自动切换任务重新规划和授权。

## Rollback Notes

不得把 B 已提升后的数据库原地降级为备库。任何未来切换都必须遵守单写节点、数据追平和全量重建边界。

## Post-release Verification

- A 应保持 `legacy-active`。
- B 应保持 `standby`，容灾应用停止。
- A/B 应保持 `image_sync=ok`。
- 阶段 6 未执行，不得将本任务归档解释为生产切换与回切已验证。
