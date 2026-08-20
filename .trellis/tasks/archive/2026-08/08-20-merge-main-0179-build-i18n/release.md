# Release Operations

## Conclusion

Release operations exist. 本次合并包含 3 个新增数据库迁移；服务启动时会由 migration runner 自动执行。

## Evidence Checked

- `task.json`
- `prd.md`
- `design.md`
- `implement.md`
- `implement.jsonl`
- `check.jsonl`
- merge commit `2befdb21bf11eb7430e3debef6d243f21a95229b`
- task record commit `6e0228827d9542f336cb01fdd1ce100c22662021`
- 当前 Git 状态与迁移执行器

## Drift Check

原任务缺少 `release.md`。本文件根据任务材料、合并提交和最终代码补充。

## SQL Changes

- `226_add_usage_log_effective_model_indexes_notx.sql`：使用 `CREATE INDEX CONCURRENTLY IF NOT EXISTS` 为 usage logs 的有效请求模型和有效上游模型增加查询索引。
- `227_composite_routes_add_cn_providers.sql`：扩展组合路由平台约束，允许 `kimi`、`zhipu` 和 `deepseek`。
- `228_channel_pricing_multipliers.sql`：为渠道模型和价格区间增加 fast、flex、input、output、cache write、cache read 倍率字段及正数约束。
- 迁移在服务初始化时按完整文件名排序执行，并由 PostgreSQL advisory lock 串行化；无需人工执行 SQL。

## Configuration Changes

- 新增可选配置 `security.proxy_probe.urls`，支持按顺序配置 `ip-api`、`ipify`、`chatgpt-trace` 探测端点。
- 默认值为空，继续使用内置探测地址；现有部署不需要强制修改配置。

## Batch / Deployment Scripts / Data Repair

None.

## External Systems / Dependent Platforms

None.

## Release Order

1. 正常部署新版本服务。
2. 等待首个实例完成启动迁移；多实例会通过 advisory lock 串行等待。
3. 确认所有实例启动正常后恢复或继续流量。

## Rollback Notes

- 应用代码可以回滚到合并前版本。
- 新增索引、约束和可空字段均为向后兼容变更，普通代码回滚时保留数据库对象，不删除 `schema_migrations` 记录，也不修改已发布迁移文件。

## Post-release Verification

- 确认 `schema_migrations` 已记录 3 个新增迁移，服务启动日志无 migration 错误。
- 验证组合路由可选择 Kimi、Zhipu、DeepSeek，渠道倍率和长上下文计费行为正常。
- 验证 OpenAI、Grok、CN adaptive、Responses、Web Search 和中英文 i18n 关键路径。
- 使用真实第三方凭证执行任务中延期的 OpenAI、xAI、Kimi、Zhipu、DeepSeek、Antigravity 上游冒烟测试。
