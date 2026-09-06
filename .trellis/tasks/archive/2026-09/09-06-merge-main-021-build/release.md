# Release Operations

## Conclusion

Release operations exist. 本任务包含数据库迁移及可选配置核对事项。以下全部事项来源于 [09-06-merge-main-021-build]，由部署负责人在授权上线时执行；本次归档只维护记录。

## Evidence Checked

- `task.json`、`prd.md`、`design.md`、`implement.md`、`implement.jsonl`、`check.jsonl`、`check-report.md`。
- 合并提交 `6884f7234` 的 274 个文件；任务完成记录提交 `71e30a704` 已同步到 `origin/build`。
- 四个新增迁移、`backend/internal/repository/migrations_runner.go`、`backend/migrations/README.md`、`deploy/config.example.yaml`。
- 上游请求 ID、Codex 目录分组配置、Fast 策略的当前源码；`research/validation-results.json`。

## Drift Check

Missing release.md. 已补齐本任务上线记录。部分实施和检查文档保留提交前快照，其“尚未提交”是当时状态；当前生命周期以 `task.json` 的 completed 和已推送的 Git 提交为准。

迁移 232 的注释声称未配置头名时使用默认识别链，但当前 `upstream_request_id.go` 明确在未配置时不记录。上线验收按当前代码核对，不改写已经合入的迁移文件。

## SQL Changes

[09-06-merge-main-021-build] 部署负责人通过项目既有迁移流程执行并核对以下文件：

| 迁移 | 变化与验收 |
| --- | --- |
| `232_add_usage_log_upstream_request_id.sql` | `usage_logs.upstream_request_id VARCHAR(128)`，允许 NULL，旧记录可以继续读取。 |
| `233_add_usage_log_upstream_request_id_index_notx.sql` | 为非 NULL 请求 ID 创建 `idx_usage_logs_upstream_request_id`；必须按非事务方式执行并确认索引有效。 |
| `234_channel_max_reasoning_effort_multiplier.sql` | `channel_model_pricing.max_reasoning_effort_multiplier NUMERIC(10,4)`，允许 NULL 或大于 0。 |
| `234_group_codex_models_manifest_config.sql` | `groups.codex_models_manifest_config JSONB NOT NULL DEFAULT '{}'`。 |

迁移器按完整文件名排序及记录校验和，两个 `234_` 文件必须都包含在制品中，不能按数字前缀去重。核对 `schema_migrations` 的文件名与校验和，不手改历史迁移记录。

## Configuration Changes

[09-06-merge-main-021-build] 没有新增必填环境变量；部署负责人保留现有 build 配置，并按需要核对：

- 账号 `extra.upstream_request_id_header`：填写实际上游响应头名才记录请求 ID；未配置、头缺失或 WS 轮次应保持空值。
- OpenAI 账号图片 URL 转 base64 开关与原有 JSON Schema、搜索、Lite、生图设置共同保存和回读；不批量自动开启新开关。
- 分组 Codex 目录配置 `enabled/account_ids/fallback_to_scheduler`：新字段默认空对象；启用固定账号时核对账号顺序和失败回退策略。
- 渠道 `max_reasoning_effort_multiplier`：由定价负责人按需要配置，保持 NULL 或正数。
- Fast 策略增加 `ultrafast` 匹配值；GPT-6 本地目录仍只声明 `priority`，不能据此为 GPT-6 添加 Ultrafast 能力。
- 定价 `fallback_file`、`override_file` 在下一次哈希检查时热更新；示例 `hash_check_interval_minutes=10`。删除本地文件会移除其条目，非 JSON 内容保留旧数据并重试，运维编辑后应核对实际生效价格。

## Batch / Deployment Scripts / Data Repair

[09-06-merge-main-021-build] 未新增需单独执行的一次性数据修复或批处理。新增 SQL 由既有迁移流程处理，构建、Trellis/Flower 和容灾资产按保留方案保留。

## External Systems / Dependent Platforms

[09-06-merge-main-021-build] 每次推送 build 都会触发现有 GHCR 镜像构建，更新 build-latest 和 latest；归档/日志推送也会触发同一流程。发布负责人核对最终成功构建的提交及镜像摘要，再决定是否部署。CI 构建不是生产实例已经升级的证据。

没有证据要求本轮修改外部配置中心或第三方平台；真实 provider 验收使用部署环境已有配置，由上线负责人执行。

## Release Order

[09-06-merge-main-021-build]

1. 部署负责人确认数据库备份、当前应用版本和可回退的镜像摘要，核对目标制品包含四个迁移。
2. 按现有服务启动/迁移流程升级；确认四条迁移记录和非事务并发索引成功后开放升级实例流量。
3. 按需要设置请求 ID、图片转换、分组目录和 max 倍率，执行下列验收。

## Rollback Notes

[09-06-merge-main-021-build] 应用问题由部署负责人回退到升级前已验证的镜像摘要；代码参考为备份分支 `backup/build-before-main-0201-4e9829519`。数据库迁移为向前变更，应用回退不等同数据库回退；保留新增列、索引及迁移账本。若确需数据库修复，另行评估并使用新的迁移，不删除历史记录或把 Down SQL 填入既有文件。

## Post-release Verification

[09-06-merge-main-021-build]

- 核对四条迁移、请求 ID 索引有效性、旧 usage 记录读取及新请求 ID 展示；搜索多轮回答应记录最后一次上游响应头。
- 核对 GPT-6 上下文 1050000、最高 max、priority 和 main 提示词选择；GPT-5.6 专用提示词继续有效。
- 使用已有测试账号验证 GLM-5.3 low 与 Anthropic thinking 优先级，并检查日志 effort 与最终上游请求一致。
- 验证账号新增/编辑/重开时，兼容设置和图片转换开关共同保存；按需验证生图、搜索失败/超限后继续回答、Lite、Grok 和 GIF 保留行为。
- 本地必需验证已 7/7 通过，前端 271 个文件、1936 个用例通过。真实数据库迁移和外部 provider 行为尚未执行，不将本地测试当成上线验收。
