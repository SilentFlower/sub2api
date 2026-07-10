# Release Operations

## Conclusion

存在发布操作：除 sub2api 代码发布外，还包含 `ai-fund` 远端 D1 的一次性监控 User-Agent 配置更新。

## Evidence Checked

- `task.json`
- `prd.md`
- `design.md`
- `implement.md`
- `implement.jsonl`
- `check.jsonl`
- 业务提交 `792c51ff`
- 任务快照提交 `c14c84d1`
- sub2api 当前 git 状态与变更文件
- `ai-fund` 远端 D1 中 monitor ID 1 及 `gpt-5.6-luna` 跟踪状态

## Drift Check

原任务缺少 `release.md`。联调阶段发现 `ai-fund` 监控未配置 Codex UA，导致其默认发送 `AI-Hub-Monitor/1.0`；该外部配置操作现已补充记录。

## SQL Changes

sub2api 无数据库 schema 或 migration 变更。

## Configuration Changes

- sub2api 无新增环境变量、配置键或 feature flag。
- 发布环境必须运行包含业务提交 `792c51ff` 的构建，并重启或滚动更新服务实例。

## Batch / Deployment Scripts / Data Repair

已于 2026-07-10 对 `ai-fund` 远端 D1 数据库 `ai-fund-db` 执行一次性配置更新，目标为 monitor ID 1（`gpt（官渠）`）：

```sql
UPDATE monitors
SET user_agent = 'codex_cli_rs/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color',
    updated_at = datetime('now')
WHERE id = 1
  AND COALESCE(user_agent, '') = '';
```

执行结果为 `rows_written=1`，回读值与目标 UA 一致。

## External Systems / Dependent Platforms

- `ai-fund` Cloudflare D1：monitor ID 1 的健康检测、单模型 TEST、批量测试和定时轮询均会使用上述 Codex UA。
- OpenAI OAuth 上游：使用同一 URL、API Key、Responses 请求体和新版 UA 实测 `gpt-5.6-luna` 返回 HTTP 200。
- `ai-fund.monitor_models`：`gpt-5.6-luna` 已于 `2026-07-10T02:46:32.828Z` 更新为 `available`。

## Release Order

1. 发布包含 `792c51ff` 的 sub2api 构建并重启服务。
2. 确认 `ai-fund` monitor ID 1 的 `user_agent` 保持为 Codex `0.144.1`。
3. 若此前 404 已触发模型级冷却，等待 30 分钟或通过现有管理能力清除冷却后再验证。
4. 对 `gpt-5.6-luna` 执行 Responses 或 `ai-fund` 单模型 TEST。

## Rollback Notes

- sub2api：回退业务提交 `792c51ff`。
- `ai-fund` 外部配置回滚：

```sql
UPDATE monitors
SET user_agent = '',
    updated_at = datetime('now')
WHERE id = 1;
```

回滚外部 UA 会恢复 `AI-Hub-Monitor/1.0` 默认值，并可能重新触发 Luna 的旧客户端兼容问题。

## Post-release Verification

- `go test -tags=unit ./internal/service -count=1` 通过。
- `go test -tags=unit ./... -count=1` 通过。
- CI 锁定的 `golangci-lint v2.9.0` 增量检查为 `0 issues`。
- 非测试生产 Go 文件不存在 `0.125.0` Codex 身份字面量。
- `git diff --check` 通过。
- `ai-fund` 同构请求调用 `gpt-5.6-luna` 返回 HTTP 200，响应模型为 `gpt-5.6-luna`。
- `ai-fund` 跟踪状态为 `available`。
