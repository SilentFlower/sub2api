# Implement Plan: 同步 main 0.1.155 到 build

## 执行步骤

1. 记录当前分支、工作区、`build`、`origin/build`、`origin/main` 和远端 URL。
2. 执行 `git fetch --prune origin main build`，重新计算 merge base、双方独有提交、共同修改文件和 `git merge-tree`。引用或冲突矩阵变化时先更新任务材料。
3. 确认本地 `build` 与 `origin/build` 一致，创建并验证 `backup/build-before-main-0155-<build短提交号>`。
4. 检查两个未跟踪任务目录。必要时只对 `.trellis/tasks/07-14-deepseek-response-format-json-schema-compat/` 使用精确 pathspec stash；当前任务目录保持可见。
5. 执行 `git merge --no-commit --no-ff origin/main`，记录 `MERGE_HEAD`、冲突索引和自动合并文件。
6. 按 `design.md` 解决 16 个显式冲突，逐文件暂存；禁止整仓 add。
7. 完成 Grok 收敛：删除旧独立 Billing endpoint、快照类型、前端 API/组件和重复测试，确认 main 统一链路闭环。
8. 完成 Codex 图片组合：main 统一识别、namespace 三项保护、旧原生 `none→auto`、可配置主模型/思考预算。
9. 完成依赖注入组合：build 独立 Codex reset、main OpenAI quota reset、main Grok quota/import prober 同时可用；修改 Provider 源后运行 `cd backend && go generate ./cmd/server`。
10. 复核 37 个自动合并文件和连带新增/删除文件，重点覆盖 OpenAI gateway、Grok route/extra、Codex reset、设置、i18n、migration、Ent、HA/DR 和部署资产。
11. 运行 `cd backend && go generate ./ent`，核对生成结果只反映 main schema；对实际修改的 Go 文件运行 gofmt。
12. 搜索冲突标记、重复类型/函数、旧 Grok Billing 符号和错误的图片工具分支。
13. 运行后端、前端定向测试；根据失败证据修复后运行完整 unit、typecheck、lint 和 migration/生成一致性检查。
14. 运行 Git 完整性检查，确认 `git ls-files -u` 为空、其它任务目录未暂存、`git diff --cached --check` 通过。
15. 通过 `trellis-push` merge-aware 路径创建本地双父 merge commit：确认 cached 文件集合等于 exact planned files、无计划外 staged，并验证父提交分别为合并前 build 与固定 `origin/main`。
16. 恢复精确 stash，记录主线更新摘要、冲突处理结果、build 特性处置和残余风险，然后停止；不自动 push。

## 定向验证

```bash
cd backend
go test -tags=unit ./internal/pkg/xai ./internal/handler/admin ./internal/server/routes -run 'Grok|Billing|SSO|CodexReset|ResetQuota' -count=1
go test -tags=unit ./internal/service -run 'Grok|Billing|ImageGeneration|ImageGen|Namespace|ToolChoice|GPT56|ReasoningEffort|CodexReset|QuotaReset|Messages|ResponsesChatFallback|WSHTTPBridge' -count=1
go test -tags=unit ./internal/pkg/apicompat -count=1
go test -tags=unit ./internal/repository ./migrations -run 'Grok|Billing|SystemLog|LatestIP|LongContext|Migration|CodexReset' -count=1
```

```bash
cd frontend
pnpm vitest run \
  src/api/__tests__/admin.grok.spec.ts \
  src/components/account/__tests__/AccountUsageCell.spec.ts \
  src/components/account/__tests__/EditAccountModal.spec.ts \
  src/components/account/__tests__/OpenAIQuotaResetCell.spark_shadow.spec.ts \
  src/components/admin/account/__tests__/OpenAICodexResetModal.spec.ts \
  src/components/keys/__tests__/UseKeyModal.spec.ts \
  src/views/admin/__tests__/SettingsView.spec.ts
pnpm typecheck
pnpm lint:check
```

## 完整质量门槛

```bash
cd backend
go test -tags=unit ./... -count=1
GOTOOLCHAIN=go1.26.5 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.9.0 run --timeout=30m --build-tags=unit --new ./...
```

```bash
cd frontend
pnpm test:run
pnpm typecheck
pnpm lint:check
```

## Git 与生成检查

```bash
git status --short --branch
git ls-files -u
rg -n '^(<<<<<<<|=======|>>>>>>>)' . --glob '!frontend/node_modules/**'
rg -n 'queryBillingQuota|GrokBillingQuota|grok_billing_quota' backend frontend
git diff --cached --check
git diff --name-only --cached -- .trellis/tasks/07-14-deepseek-response-format-json-schema-compat
```

提交后验证：

```bash
git rev-list --parents -n 1 <merge-commit>
git merge-base --is-ancestor <target-main-commit> <merge-commit>
git merge-base --is-ancestor <pre-merge-build-commit> <merge-commit>
```

## 风险门禁

- 远端引用变化、冲突数量变化或新增 delete/modify 冲突：停止并更新规划。
- Grok 旧 Billing 符号仍有生产引用：不得提交。
- namespace 触发旧提示、旧工具注入或 `none→auto`：不得提交。
- build Codex reset 任一 API/弹窗丢失，或 main reset API/单元格丢失：不得提交。
- migration/Ent/Wire 生成结果不一致：不得用手工编辑生成文件掩盖。
- 定向测试或完整质量门槛失败：继续修复或明确环境阻断并重新取得用户决定。

## 回滚点

- 合并前：备份分支与精确 pathspec stash。
- 合并中：`git merge --abort` 后恢复 stash。
- merge commit 后：保留双父历史，使用后续修复提交；不自动 reset/amend。
- push 后回滚不属于本任务范围。
