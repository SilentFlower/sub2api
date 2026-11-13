# 实施计划

## 顺序

- [x] 审阅完整 Brief，授权后 task.py start；经 trellis-route 选择实施方式，读取 trellis-before-dev 及相关规范、实体、DTO 和调用定义。
- [x] 记录 build/main 完整哈希与工作区基线；核对 Flower 19 文件与上游写入路径交集，保存补丁；确认合并父提交与预演一致。
- [x] 准备 Go 1.27.3 与 pnpm 工具链，执行 git merge --no-commit --no-ff 530c5e3a4。
- [x] 按 cyber、配置部署、协议模型、网关计费、服务设置、前端顺序处理 32 个冲突；每个领域先读取双方完整定义和调用关系。
- [x] 逐项复核 research/merge-preview.md 的 68 个自动合并交集，补充单边新增实现的接线、数据库迁移、Ent/Wire、locale 与依赖锁检查。
- [x] 保留双方有效测试并添加交互行为回归；独立领域 owner 承接 build 规则。
- [x] 执行下列验证，修复本轮引入的失败。记录既有失败、环境阻塞与外部依赖边界。
- [x] 进入 trellis-route(target=check) 与 Check-All，完成全面验收。规范更新和精确 Git 计划由完成链继续。

## 验证命令

在 backend 执行：

```bash
go version
go test -tags=unit ./internal/config ./internal/pkg/apicompat ./internal/pkg/openai ./internal/handler ./internal/service ./internal/repository
go test -tags=unit ./...
go build ./cmd/server
golangci-lint run ./...
```

涉及 Ent/Wire 源定义合并时按仓库 go generate 入口生成并比较实际输出。迁移测试按现有 harness 和运行条件执行，检查新增 SQL 与迁移器排序/校验。

在 frontend 执行：

```bash
pnpm install --frozen-lockfile
pnpm lint:check
pnpm typecheck
pnpm exec vitest run src/i18n/__tests__/localeKeyCompleteness.spec.ts src/i18n/__tests__/localesMessageCompile.spec.ts src/i18n/__tests__/localesNoKeyCollision.spec.ts src/i18n/__tests__/buildFeatureLocaleExtensions.spec.ts
pnpm test:run
pnpm build
```

仓库检查：git diff --check、冲突标记定向扫描、最终索引未解冲突检查；Compose 按环境支持执行 config 校验。完整双方功能矩阵至少覆盖 cyber 明确身份/白名单/长历史/WS、流式错误停调、Responses Lite、Fast、搜索引用、Sol/Terra/Claude 新模型、cache TTL/图片计费、渠道平台维度和账号 extra。

## 审阅与回退点

- 初次 Brief 确认后进入实施。业务互斥且规范未裁决的点单独提交决定。
- 通过 Check-All 和规范更新后展示实际精确 Git 计划；提交与推送等待独立授权。
- 需要取消或迁移合并现场时，先保全 Flower 及本轮补丁，核对原始基线后恢复。

## 本轮真实验证结果（2026-11-13）

证据日志保存在 gitignored `.trellis/.runtime/merge-main-build/`。所有后端命令使用 `GOTOOLCHAIN=go1.27.3`。

| 验证 | 命令与覆盖 | 结果 / 日志 |
| --- | --- | --- |
| 后端全量 unit | backend：`go test -tags=unit ./...` | exit 0；backend-unit-complete.log。service 239 秒全量结果复用 Go cache；repository 修复本地代理 fixture 后重新通过。 |
| 后端生产构建 | backend：`go build ./cmd/server` | exit 0；backend-build-final.log。随后仅调整测试。 |
| 后端 lint | backend：golangci-lint v2.14.0（Go 1.27.3）`run ./...` | exit 0，0 issues；backend-lint-complete.log。 |
| 协议交互定向回归 | Messages effort 分组映射/拒绝、关闭推理、流式停调、结构化搜索、disabled 模型归一 | exit 0；backend-policy-recheck.log。 |
| 隔离 DB 集成 | `go test -tags=integration ./internal/repository -run 'TestMigrationsRunner_|TestUsageLogRepositoryCacheWriteTTLRoundTrip|TestChannelCacheWriteTTLPricesRoundTrip|TestMCPOAuthIntegration|TestBatchDisableUsers_RevokesMCPGrantsPersistently' -count=1 -timeout=5m` | exit 0，73.073 秒；backend-db-integration.log。Testcontainers Postgres/Redis；迁移、TTL 读写、MCP grant/revoke。 |
| 前端依赖 | frontend：`pnpm install --frozen-lockfile` | exit 0。 |
| 前端 lint | frontend：`pnpm lint:check` | exit 0；frontend-lint-complete.log。 |
| 前端类型 | frontend：`pnpm typecheck` | exit 0。 |
| 前端全量 Vitest | frontend：`pnpm test:run --maxWorkers=4 --minWorkers=1` | exit 0，404 files / 3211 tests；frontend-tests-complete.log。包含四项 locale 契约与账号/渠道/设置行为。 |
| 前端构建 | frontend：`pnpm build` | exit 0；frontend-build.log。 |
| 生成物 | `go generate ./ent`、`go generate ./cmd/server` | exit 0；Wire 与 main 一致，Ent 相对 main 仅两处 build 中文注释差异。 |
| Compose | 四套 Compose：`docker compose -f <file> config --quiet` | 全部 exit 0；standalone 使用本地占位环境变量。 |

验证中发现并修复：分组 none 策略被过早清除；annotations 的 nil 被输出为 null；ChannelsView 测试对内联实现的旧假设；SettingsView 测试挂载未清理；进程级内存断言被并发后台任务干扰；验证码 SDK 测试受大小写代理变量干扰。所有修复保持原有行为断言或资源阈值。

本机旧 golangci-lint v2.13.0 的 export data 版本失败通过匹配 CI 的 v2.14.0 解决。早期失败日志保留为诊断材料；最终成功日志以上表为准。

迁移 `250_openai_cache_write_ttl.sql` 保留 main 原始 blob，`diff --check` 的末尾空行属于上游既有格式。保持已发布迁移 checksum 优先。真实上游调用、生产配置与生产数据库属于发布验收边界。
