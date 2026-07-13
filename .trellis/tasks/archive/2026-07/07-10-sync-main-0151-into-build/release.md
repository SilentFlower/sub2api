# Release Operations

## Conclusion

需要人工复核。任务包含 migration 173，且后续生产应用更新理论上会自动执行该迁移，但现有任务证据没有直接记录生产库的 `schema_migrations` 和约束状态。

## Evidence Checked

- `task.json`、`prd.md`、`design.md`、`implement.md`
- `implement.jsonl`、`check.jsonl`、`research/merge-evidence.md`
- merge commit `0712a147`
- `backend/migrations/173_allow_cyber_blocked_usage_request_type.sql`
- 后续 `53fea533` 生产发布记录

## Drift Check

原任务缺少 `release.md`。代码和镜像发布证据完整，但 migration 173 的生产数据库落地证据缺失，因此保留人工复核风险。

## SQL Changes

- migration 173 会删除并重建 `usage_logs_request_type_check`，允许 `request_type` 取值 `0..4`。
- 新约束使用 `NOT VALID`：新写入会受约束保护，但 PostgreSQL 不会在迁移时扫描验证全部历史行。
- `DROP CONSTRAINT` / `ADD CONSTRAINT` 需要表锁，发布时可能短暂阻塞高并发 usage 写入。
- 需要在生产库确认 `schema_migrations` 已记录 migration 173，并核对约束定义包含 `4`；`convalidated=false` 是当前 SQL 的预期状态，不应误判为迁移失败。

## Configuration Changes

无新增环境变量、secret、权限配置或外部 endpoint。

## Batch / Deployment Scripts / Data Repair

无数据回填脚本。若未来需要把约束转为 validated，必须另行评估历史数据并使用新的受控 migration，不能修改 migration 173。

## External Systems / Dependent Platforms

代码已随 `build` 镜像进入后续生产发布；没有独立外部平台配置操作。

## Release Order

1. 发布应用并等待启动迁移完成。
2. 核对 migration 173 记录与约束定义。
3. 验证 cyber-policy blocked usage 可以写入 `request_type=4`，普通 usage 不受影响。

## Rollback Notes

- 应用可回退到发布前固定 digest。
- 回退应用时保留扩展后的约束和 migration 记录；允许值集合扩大对旧版本兼容，不在故障窗口恢复旧约束。

## Post-release Verification

- `schema_migrations` 存在 migration 173 且 checksum 正常。
- `usage_logs_request_type_check` 包含 `0,1,2,3,4`。
- cyber-policy blocked usage 可见且不与 legacy `request_type=0` 混淆。
- 对生产库缺失的直接证据进行一次人工核验后，方可关闭本 release 风险。
