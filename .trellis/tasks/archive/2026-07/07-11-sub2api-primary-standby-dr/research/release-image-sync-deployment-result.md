# 容灾应用发布镜像同步部署结果

执行时间：2026-07-11。

## 部署内容

- A `/root/sub2api-ha-export/scripts/switch-mode.sh` 已增加 `sync-release [--dry-run]`，并扩展 `status` 与 `status --machine` 的发布镜像状态。
- B `/root/sub2api-dr/scripts/set-release-image.sh` 已部署，`switch-mode.sh enable` 和 `promote.sh` 已在数据库提升前增加固定 digest、缓存和最近同步记录门禁。
- A、B 的脚本库已增加镜像 digest 校验、缓存检查、`.env` 单键原子替换和 `state/release-image.env` 原子写入。
- A `sync-release` 与 B 内部设置脚本都会在写配置前调用现有只读复制验证，明确要求 PostgreSQL WAL receiver 为 `streaming`、Redis 主链路为 `up` 且不在同步过程中。
- A、B 的 README 已增加日常更新后的版本同步步骤，并明确同一标签也必须重新同步。
- 正式文件通过 staging SHA256 校验后安装；README 权限为 `0644`，脚本权限为 `0755`，双端全部脚本通过远端 `bash -n`。
- 最终检查移除了 B `prepare-runtime-env.sh` 与 `lib.sh` 的重复 `env_value` 实现；修正版单文件重新部署后 SHA256 与本地一致，B 原栈及容灾数据库容器 ID、启动时间和端口基线未变化。

## 首次同步结果

- A 当前来源引用为 `ghcr.io/silentflower/sub2api:build-latest`。
- A 当前实际运行镜像为 `ghcr.io/silentflower/sub2api@sha256:ff1bd8e6494963642570706d0ab55e04a1a15d9fd79707fc555d0f844f357d52`。
- 同步前 A、B 的 `SUB2API_IMAGE` 已指向该 digest，B 本地也已缓存，但 B 尚无最近同步记录，因此 A 状态为 `image_sync=unknown`。
- `sync-release --dry-run` 成功，只输出同步计划，没有拉取镜像、修改环境文件、写状态、重启服务或启动 B 应用。
- 补强复制健康门禁后再次执行 `sync-release --dry-run` 成功，状态为 `sync=ok`，证明线上 PostgreSQL 与 Redis 复制健康检查可以通过。
- 实际 `sync-release` 先确认 B 已缓存精确 digest，再写 B 发布记录和配置，随后写 A 发布记录和配置。
- 同步后 A `status --machine` 显示 `image_sync=ok`；A 当前运行、A 恢复配置、B 容灾配置、B 缓存和双端发布记录均为同一 digest。
- A、B 的 `.env` 均只有一条 `SUB2API_IMAGE`，发布状态文件只包含 digest、上一 digest、来源引用和同步时间。

## 无重启与隔离验证

- A 原 `sub2api`、`sub2api-postgres`、`sub2api-redis` 容器 ID 和启动时间在部署及同步前后保持不变。
- A PostgreSQL 当前进程启动时间仍为 `2026-07-08 14:45:03.71956+08`，A `8080/health` 返回正常。
- B 原单机 `sub2api`、`sub2api-postgres`、`sub2api-redis` 容器 ID、启动时间和端口保持不变，原 `8080/health` 返回正常。
- B 容灾 PostgreSQL、Redis 容器 ID 和启动时间保持不变；PostgreSQL 仍在恢复且接收/回放 LSN 一致，Redis 仍为从库且主从 offset 一致。
- B 容灾应用仍不存在，`18080` 未监听。
- B `enable --dry-run` 通过发布镜像门禁；执行前后 B `.env` 与全部状态文件 SHA256 完全一致，没有提升数据库、重建容器或启动应用。

## 保留事项

- 新增任务级隔离回归 `tests/release-image-sync-test.sh`，使用临时 `.env`、状态文件以及 `docker`、`ssh`、`curl`、`ss` mock，不连接服务器或本机 Docker 服务。
- 隔离回归共 15 项并全部通过：同标签新 digest、缺失/错仓库/错镜像 ID/多匹配 RepoDigest、B 单键更新、B 运行环境重复键清理、B 非 standby、复制不健康、缓存缺失、记录漂移、B enable 成功门禁、A 健康失败、SSH 失败、状态漂移、B 接管后 A 恢复 digest、A/B 机器状态字段契约，以及关键操作顺序。
- 其中一项使用真实 A/B 脚本配合 mock SSH 跑完实际 `sync-release`，确认同一来源标签对应新 digest 时先更新 B、再更新 A，双端 `.env` 和发布记录最终收敛到新 digest，前一 digest 保留为旧值。
- 测试脚本及 mock 全部通过 `bash -n` 和 ShellCheck warning 级别检查；生产环境只执行同 digest 幂等和 dry-run 验证，没有通过生产数据库提升制造异常分支。
- B 仍未报告 NTP 同步。实际故障提升前必须处理或人工确认系统时间，复制追平继续以 PostgreSQL LSN 和 Redis offset 为准。
- 保留上一 digest 只用于诊断；数据库迁移可能不向后兼容，不能据此承诺旧应用镜像可直接回滚。
