# Release Operations

## Conclusion

存在发布操作。代码已推送并完成 CI/安全扫描/镜像构建，但尚无本任务对应的生产部署证据；生产发布前必须确认镜像身份、migration 174 和 Alpha Search 默认计费行为。

## Evidence Checked

- `task.json`、`prd.md`、`design.md`、`implement.md`
- `implement.jsonl`、`check.jsonl`、`research/merge-evidence.md`
- merge commit `80679de9` 与 snapshot commit `daca269f`
- `backend/migrations/174_group_web_search_price_per_call.sql`
- migration runner、GitHub Actions 和 GHCR 镜像元数据

## Drift Check

原任务缺少 `release.md`，本文件根据最终提交、CI 与镜像证据补齐。生产环境当前状态仍需发布前重新采集，不能只沿用上一任务记录。

## SQL Changes

- 服务启动时 migration runner 会自动执行 `174_group_web_search_price_per_call.sql`，为 `groups` 新增可空的 `web_search_price_per_call DECIMAL(20,8)`。
- migration 在事务中执行并受 PostgreSQL advisory lock 保护；数据库账号必须具备 `ALTER TABLE` 权限。
- 该变更不回填历史数据。历史分组保持 `NULL`，新版本会把 `NULL` 解释为默认 `0.01 USD/次`；`0` 才表示免费。
- `ADD COLUMN` 仍需获取表级锁。虽然无默认值、通常不会重写整表，仍应避开数据库高峰并观察启动超时。
- 发布后核对 `schema_migrations` 中存在 `174_group_web_search_price_per_call.sql`，并确认列类型、nullable 和 checksum 正常。

## Configuration Changes

- 无新增环境变量、权限配置、secret 或 feature flag。
- 发布前必须由业务侧确认未配置单价的分组采用默认 `0.01 USD/次` 符合预期；需要免费的分组应显式配置为 `0`。

## Batch / Deployment Scripts / Data Repair

- 无一次性数据修复或批处理脚本。
- `build` 的两次 push 均触发镜像构建：
  - 业务 merge revision `80679de9`：`ghcr.io/silentflower/sub2api@sha256:2b179d1d9217408e6e52c1e02f9f8c14ba56c9ccb488615ef38e3e2b922bf15e`
  - snapshot revision `daca269f`，也是当前 `latest`：`ghcr.io/silentflower/sub2api@sha256:2aa03f874a9795ff62e1299314ece5b62ecd4ac1ebb0e8747f7d944be2272a53`
- 两个 revision 的 CI、Security Scan 和 Build Image 均成功。推荐生产发布固定业务 merge digest；若使用 `latest`，必须先确认 OCI revision 为 `daca269f` 并接受该 bookkeeping revision 作为发布身份。

## External Systems / Dependent Platforms

- GHCR 的 `build-latest` 与 `latest` 已被 `daca269f` 构建覆盖；禁止把浮动标签直接写入容灾发布记录。
- 本任务未执行 A/B 生产发布。若沿用现有双节点发布流程，应只重建 A 应用，验证健康和数据库迁移后，再用固定 digest 执行 `sync-release --dry-run` 与 `sync-release`；B 应用保持停止。

## Release Order

1. 重新采集 A/B 模式、当前生产 digest、数据库/Redis 容器 ID 和健康基线。
2. 确定要发布的不可变 digest，并核对 OCI revision。
3. 确认 Alpha Search 默认单价策略和需要显式设为 `0` 的分组。
4. 仅更新 A 应用；启动阶段等待 migration 174 成功。
5. 验证健康、数据库、Alpha Search 成功/失败计费和 Grok 实际端点记录。
6. 如使用 A/B 容灾，按固定 digest 同步到 B，保持 B 应用停止并确认 `image_sync=ok`。

## Rollback Notes

- 发布前必须重新读取 A 当前运行 digest；上一任务已知生产 digest 为 `sha256:74f4f8c88729918b4ec21fbe9f236b740149e923a47f0ee57da7a93a1097e674`，只能作为候选基线，不能代替现场确认。
- 应用异常时回退到发布前固定 digest，只重建应用服务。
- migration 174 是 forward-only、nullable 的加列操作。应用回退时保留该列和 migration 记录，不在故障窗口删除列或改写 checksum。
- 若计费行为不符合预期，优先显式调整分组单价或停用 Alpha Search，再评估后续代码修复；不要直接删除 usage 或 migration 记录。

## Post-release Verification

- `/health` 正常，应用启动日志无 migration/checksum 错误。
- `schema_migrations` 已记录 migration 174，`groups.web_search_price_per_call` 类型和 nullable 正确。
- Alpha Search 仅成功 2xx 产生一次按次费用；非 2xx、重定向和 failover 不计费。
- 默认价、分组覆盖价、显式免费 `0` 和倍率计算均符合预期。
- Grok Chat/Responses、Messages fallback、Codex identity 和现有 Responses 流量无回退。
- 若执行 A/B 同步，A/B 固定 digest 一致、B 保持 standby 且应用未启动。
