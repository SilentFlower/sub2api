# Release Operations

## Conclusion

Needs human review. 本任务把 `main` 0.1.173 合并到 `build`，包含 21 个数据库迁移、运行时配置与后台设置扩展，以及 migration 220 的数据清理。代码和测试证据完整，但当前没有生产数据库 `schema_migrations`、应用镜像或实际部署结果，归档不代表已经上线。

## Evidence Checked

- `task.json`、`prd.md`、`design.md`、`implement.md`
- `implement.jsonl`、`check.jsonl`
- 业务 merge commit `da830dd323777dd67fc065a9b7e2ab4fef7488bc`
- `backend/migrations/192_*.sql` 至 `220_*.sql` 中本次新增的 21 个文件
- `backend/internal/repository/migrations_runner.go` 与 `backend/internal/setup/setup.go`
- `backend/internal/config/config.go`、`deploy/config.example.yaml`
- 后端设置 DTO、SettingService 与前端后台设置类型

## Drift Check

原任务缺少 `release.md`。本文件根据最终 merge commit 和当前源码补齐。生产环境版本、数据库迁移状态、配置取值及外部服务凭据均缺少现场证据，因此保留 `Needs human review`。

## SQL Changes

- 服务启动时 migration runner 在 PostgreSQL advisory lock 下自动执行未应用迁移；数据库账号必须具备建表、改表、建索引、函数和授权所需权限。
- migration 192-193 为 `groups` 增加利润控制字段，并扩展认证缓存失效触发函数。
- migration 194 为 `usage_logs` 增加 `upstream_response_model`、`upstream_model_mismatch`，migration 195 的 `_notx` 文件并发创建部分索引。
- migration 194-206 创建 Channel Monitor V2 基础表、固定粒度汇总表、索引、配置、权限和默认设置；后台聚合器会渐进回填历史数据，应观察数据库负载和覆盖水位。
- migration 217-219 为分组增加视频模型、Grok Voice 和搜索工具定价字段。字段为可空值，`NULL` 与显式 `0` 的计费含义不同，发布后需核对默认价和免费配置。
- migration 220 会先创建 `groups_video_price_backup_220` 快照表，再清空非 Grok、非 composite 分组的历史视频价格。该数据更新不可仅靠应用回滚恢复。
- 本批次同时存在 `194_*`、`195_*` 的多个不同文件；迁移记录以完整文件名为键。发布后必须逐个核对 21 个文件的记录与 checksum，不能只按数字前缀判断完成。

## Configuration Changes

- 新增 `gateway.disable_codex_identity_enforcement`，默认 `false`；旧键 `gateway.disable_codex_originator_normalization` 继续兼容。只有上游身份策略发生反向变化时才应启用回滚开关。
- 新增 `gateway.grok.password_auth_enabled`，默认 `false`；启用后开放管理员密码换取 SSO 的流程，生产环境应优先使用 SSO cookie、浏览器 OAuth 或 refresh token。
- Grok 免费额度软门控默认开启：500000 token、95% 阈值、24 小时窗口、60 秒统计缓存。发布前需确认这些默认值符合当前运营策略。
- Codex 客户端版本自动同步默认开启，每 6 小时访问 GitHub `openai/codex` release API；受限网络环境需允许出站访问，或在后台关闭自动同步并设置固定版本。
- Channel Monitor 默认模式为 `v1`，`v2` 需要管理员显式启用；用户侧吞吐量默认隐藏。不得在未评估历史回填负载时直接切换全部环境到 V2。
- 新增腾讯云与阿里云验证码后台设置、Grok 默认模型/上游模式、跨客户端模型映射和账号自动停调阈值。默认关闭的功能不要求凭据；启用前必须完整配置并实测。
- CSP 已加入腾讯云、阿里云验证码所需域名。使用额外代理或自定义域名时需另行核对 CSP 与网络出口。

## Batch / Deployment Scripts / Data Repair

- 没有必须手工运行的一次性脚本；迁移由应用启动自动执行。
- `backend/cmd/profit-preview` 是本地预览工具，不属于生产部署步骤。
- Channel Monitor V2 的历史数据由后台聚合器有界回填，不应另行启动无审计的全量 SQL 回填。
- migration 220 的备份表应保留到新版本计费配置稳定后再考虑清理，本任务不执行 DROP。

## External Systems / Dependent Platforms

- 启用腾讯云验证码前，需要腾讯 Captcha AppID/AppSecret 及云 API SecretID/SecretKey，并确认选择 `cn` 或 `intl` 区域。
- 启用阿里云验证码前，需要 AccessKey、SceneID、Prefix 和区域配置，并确认账号权限和服务端验证接口可达。
- Codex 自动同步依赖 GitHub Releases API；调用失败会保留现有同步值或回退编译期版本，不应阻止应用启动。
- 本任务没有生产部署、镜像 digest、DMS 工单或外部平台配置完成证据。

## Release Order

1. 记录发布前应用固定 digest、数据库备份、`schema_migrations` 和关键分组定价快照。
2. 确认数据库账号具备迁移权限，并在低峰期滚动发布单个应用实例。
3. 等待启动迁移完成，逐个核对本任务 21 个 migration 文件及 checksum；确认 `_notx` 索引有效。
4. 检查 `groups_video_price_backup_220` 已建立，并确认仅非 Grok、非 composite 分组的视频价格被清空。
5. 验证健康检查、核心 OpenAI/Grok 请求、计费、账号调度和后台设置读写。
6. 观察 Channel Monitor 聚合水位、数据库负载和错误日志；确认后再按需启用 V2、验证码或 Grok 密码授权。
7. 单实例稳定后再继续其余实例，避免多个实例同时放大迁移后回填与版本同步流量。

## Rollback Notes

- 应用异常时回退到发布前固定 digest；保留已经应用的 additive schema 和 `schema_migrations` 记录，不删除列、表或迁移记录，也不修改已发布 SQL checksum。
- migration 220 的数据需要从备份表恢复：按 `group_id` 将 `video_price_480p`、`video_price_720p`、`video_price_1080p` 和 `video_model_prices` 写回 `groups`。执行恢复前必须人工核对备份行数与目标分组，走受控数据变更流程。
- Codex 身份问题可临时设置 `gateway.disable_codex_identity_enforcement=true`；自动同步问题可关闭后台自动同步并固定客户端版本。
- Channel Monitor V2 异常时切回 `channel_monitor_mode=v1`；保留 V2 表和水位，不在故障窗口删除汇总数据。
- 新验证码链路异常时先关闭对应 provider，恢复原认证流程；不要删除已保存 secret 以外的无关 OAuth 设置。

## Post-release Verification

- `/health` 正常，版本为 `0.1.173`，启动日志无 migration、checksum 或 Wire 初始化错误。
- `schema_migrations` 已记录本任务 21 个文件；并发索引有效，Channel Monitor V2 表和权限完整。
- migration 220 备份表行数与被清理分组一致，Grok/composite 分组视频价格未被清空。
- OpenAI Responses、Chat Completions、passthrough、WebSocket 与模型目录使用一致的 Codex 身份版本。
- Grok 免费/付费额度、独立 Billing Quota、视频/语音/搜索计费和账号停调阈值符合配置。
- Channel Monitor V1 默认行为不回退；启用 V2 时聚合水位推进且数据库负载可接受。
- 若启用腾讯云或阿里云验证码，注册、登录、找回密码和 OAuth 起始流程分别通过真实 provider 验证。
- 生产数据库、镜像和外部服务现场证据完成复核后，方可关闭本文件的 `Needs human review`。
