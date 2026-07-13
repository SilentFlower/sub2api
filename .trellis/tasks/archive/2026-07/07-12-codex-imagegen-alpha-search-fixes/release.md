# Release Operations

## Conclusion

存在发布操作，且任务记录显示发布已完成。

## Evidence Checked

- `task.json`、`prd.md`、`design.md`、`implement.md`
- `implement.jsonl`、`check.jsonl`
- `research/issue-pr-action-evidence.md`
- 最终生产 revision `53fea533` 的 CI、Security Scan、Build Image 与 A/B 验证记录

## Drift Check

原任务缺少 `release.md`。本文件按最终生产证据补齐，未发现任务规格与发布结果漂移。

## SQL Changes

无数据库 migration、数据修复或 migration 记录变更。发布验证确认 PostgreSQL 未重启。

## Configuration Changes

- A 主部署继续使用 `latest` 作为日常拉取来源。
- A 恢复配置和 B 容灾发布记录使用固定 digest，不写入浮动标签。
- 未修改 HA owner、租约、复制拓扑、Tunnel、DNS 或 secret。

## Batch / Deployment Scripts / Data Repair

- GitHub Actions 为 revision `53fea5336ad7cf9f35fea817d7084168ddceaf28` 构建镜像。
- A 拉取目标 `latest` 后只使用 `--no-deps --force-recreate` 重建应用服务。
- A 执行 `sync-release --dry-run` 后执行 `sync-release`，把固定 digest 同步到 B。
- 无数据修复或后台任务重跑。

## External Systems / Dependent Platforms

- GHCR 固定 digest：`ghcr.io/silentflower/sub2api@sha256:74f4f8c88729918b4ec21fbe9f236b740149e923a47f0ee57da7a93a1097e674`。
- A 已运行该 digest 并通过 Docker health、HTTP、Responses、Alpha Search 鉴权和生图桥接验证。
- B 已缓存同一 digest，保持 standby/streaming，容灾应用未启动；最终 `image_sync=ok`。

## Release Order

已按以下顺序完成：CI/镜像成功 -> 核对 revision/digest -> 仅更新 A 应用 -> 验证 A -> `sync-release --dry-run` -> 同步 B -> 验证 `image_sync=ok`。

## Rollback Notes

- 旧生产 digest：`ghcr.io/silentflower/sub2api@sha256:e30acd8618135cc5598b91590c0d27365fe35a94fa0c5f2eb48b78e76bc0b0fa`。
- A Compose 备份：`/root/sub2api/deploy/docker-compose.yml.pre-53fea53-20260712`。
- 回滚时只重建 A 应用，并重新用固定 digest 同步容灾发布记录；不重启 PostgreSQL/Redis，不启动 B 应用。

## Post-release Verification

- CI、Security Scan、Build Image 均成功。
- A revision/digest、健康检查和业务流量验证通过，PostgreSQL/Redis 容器未重启。
- B PostgreSQL/Redis 复制正常，应用保持停止，A/B digest 一致且 `image_sync=ok`。
