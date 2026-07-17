# Release Operations

## Conclusion

Release operations exist.

## Evidence Checked

- `task.json`、`prd.md`、`design.md`、`implement.md`
- `implement.jsonl`、`check.jsonl`
- 业务提交 `d3988a03`、`1dee2569`、`dac223fb`、`bb4eed40`
- `backend/migrations/177_add_subscription_plan_currency.sql`
- `backend/migrations/178_channel_image_input_price.sql`
- `backend/migrations/179_usage_log_image_input_tokens.sql`
- `backend/migrations/180_audit_logs.sql`
- `backend/migrations/181_group_duplicate_operation_id.sql`
- 当前 Git 工作区与 `origin/build` 同步状态

## Drift Check

Missing release.md. 本文件根据任务材料和已推送提交补齐。

## SQL Changes

- 按项目既有 migration 流程依次应用 `177` 至 `181`：
  - `subscription_plans.currency`：非空字符串，默认 `''`。
  - `channel_model_pricing.image_input_price`：可空图片输入单价。
  - `usage_logs.image_input_tokens`、`usage_logs.image_input_cost`：默认分别为 `0`。
  - 新建 `audit_logs` 及查询索引。
  - `groups.duplicate_operation_id`：可空字段和仅针对未删除记录的部分唯一索引。
- migration 均为向前兼容的新增字段、表或索引；上线前必须确认 migration 成功完成。

## Configuration Changes

- 新增系统设置 `openai_responses_lite_header_blocked_models`，通过后台设置页维护，无需新增环境变量或重启服务。
- 设置键缺失时默认使用 `gpt-5.4`、`gpt-5.4-mini`、`gpt-5.5`；管理员显式保存 `[]` 表示不阻止任何模型。
- 存量环境无需预置该 setting；如需自定义模型范围，应在部署后通过后台保存。

## Batch / Deployment Scripts / Data Repair

None.

## External Systems / Dependent Platforms

None.

## Release Order

1. 备份数据库并按既有流程应用 `177` 至 `181` migration。
2. 确认 migration 成功后部署本次 backend 与 frontend 产物。
3. 按下方项目执行上线后验证；需要自定义 Lite 阻止模型时再保存系统设置。

## Rollback Notes

- 应用代码回滚时保留本轮新增的可空字段、默认字段、表和索引，不将删表或删列作为常规回滚动作。
- Responses Lite 设置属于现有 settings 表中的新增 key；旧版本会忽略该 key。需要行为回退时可恢复此前模型列表或删除该 key 以使用默认值。
- 已生成的审计记录、图片输入费用字段和群组复制 operation id 不需要在代码回滚时清除。

## Post-release Verification

- 后台设置接口和页面能够读取、保存 `openai_responses_lite_header_blocked_models`，显式空数组可往返。
- `gpt-5.6-terra` 的 Lite HTTP/WS 请求保留 Header/metadata 和 `reasoning.context=all_turns`。
- 默认阻止的 `gpt-5.5` 或 `gpt-5.4-mini` 不向上游发送 Lite Header/metadata，且不强制补齐 context。
- Grok 请求不携带 OpenAI Lite Header，Grok 快捷端点和 WS v2 可正常使用。
- 群组复制、用户批量限额和审计日志相关接口可正常访问，新增数据库字段与索引存在。
- 客户端 `image_gen` 与 hosted `image_generation` 不发生重复注入或执行域混淆。
