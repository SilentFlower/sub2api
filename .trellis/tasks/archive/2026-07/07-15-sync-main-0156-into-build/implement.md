# 实施计划：同步 main 0.1.156 到 build

## 执行步骤

1. 记录工作区、分支、远端和固定引用；fetch `origin main build` 后重新计算 merge
   base、双方提交数、共同修改文件、merge-tree 冲突。引用变化则先更新任务材料。
2. 确认 `build == origin/build` 且工作区仅含当前任务规划文件，创建并验证
   `backup/build-before-main-0156-<短SHA>`。
3. 执行 `git merge --no-commit --no-ff origin/main`，记录 `MERGE_HEAD`、13 个
   conflict path 和所有自动合并文件，不创建 merge commit。
4. 先处理协议实现：Anthropic terminal state、Codex 图片工具分类、新旧
   Chat->Anthropic 桥迁移、Messages fallback；对相关 Go 文件运行 gofmt 和定向
   测试。
5. 处理 Grok 双链路：handler 同时注入独立 Billing 与 reconciler；usage/provider
   保持 snapshots-only；保留 main 手动 quota、OAuth refresh 和 reconcile。
6. 处理账号 handler：合并 Grok OAuth/Codex reset 构造签名，全仓修复
   `NewAccountHandler`、`ProvideAccountHandler`、`NewGrokOAuthHandler` 调用点和
   中文 Javadoc。
7. 处理前端账号操作：菜单 emits 与 AccountsView 绑定取并集，保留 duplicate、
   Codex reset、Spark shadow 三套状态和测试。
8. 处理 `service/wire.go` provider，运行 `cd backend && go generate ./cmd/server`，
   以生成结果解决 `wire_gen.go`，核对无计划外生成文件。
9. 迁移/合并双方测试：终态、Grok snapshots-only、Lite hosted fallback、namespace、
   flat/nested function、WS 四轮、handler 构造和前端三类账号操作。
10. 扫描并复核双方共同修改的 61 个自动合并文件，进行 PRD 正向检查和旧符号/
    漏调用点反向检查；发现新语义冲突先更新 design，不直接猜测。
11. 运行全部定向和完整质量门槛；失败按根因修复后重跑受影响层和最终全量检查。
12. 检查 `git ls-files -u`、冲突标记、重复符号、生成一致性、staged 文件集合和
    `git diff --cached --check`，形成冲突决策与 build 特性保留报告。
13. 回到 Phase 2.2 完成最终 check-all。merge commit 与 push 仅在用户通过
    `trellis-push` 确认精确文件范围和提交信息后执行。

## 最终检查修复

1. 跳过 finalize 时仍缺 function name 的 pending tool call；仅在合法工具块
   实际开始后记录 `HasToolCall`，没有合法工具块时把 `tool_calls` 终态降为
   `end_turn`。
2. 保留 main 的 cache read/cache creation 完整 token 拆分并更新协议规范。
3. 更新 Messages 默认 `reasoning_effort=medium` 与 Responses Lite 内部标头不
   透传的旧测试断言。
4. 清理 check-all 报告的 Go lint 问题，重跑协议/service 定向测试、后端全量
   unit、golangci-lint 和 Git 完整性检查。

## 定向验证

```bash
cd backend
go test -tags=unit ./internal/pkg/apicompat -count=1
go test -tags=unit ./internal/service -run 'Test.*(Messages|Chat|Anthropic|ImageGeneration|ImageGen|ResponsesLite|Grok|Billing|Codex)' -count=1
go test -tags=unit ./internal/handler/admin ./internal/server/routes -run 'Test.*(Account|Grok|Billing|Codex|Duplicate)' -count=1
```

```bash
cd frontend
pnpm vitest run \
  src/api/__tests__/admin.accounts.duplicate.spec.ts \
  src/components/admin/account/__tests__/AccountActionMenu.spark_shadow.spec.ts \
  src/views/admin/__tests__/AccountsView.sparkShadow.spec.ts \
  src/components/account/__tests__/GrokBillingQuotaCell.spec.ts
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
pnpm build
```

## Git 与静默冲突检查

```bash
git status --short --branch
git ls-files -u
rg -n '^(<<<<<<<|=======|>>>>>>>)' . --glob '!frontend/node_modules/**'
rg -n 'anthropic_chatcompletions|ChatCompletionsToAnthropicStreamState|NewAccountHandler\(|NewGrokOAuthHandler\(' backend
rg -n 'grokQuotaService' backend/internal/service/account_usage_service.go backend/internal/service/wire.go
rg -n 'X-OpenAI-Internal-Codex-Responses-Lite' backend/internal/service
git diff --cached --check
```

## 风险门禁

- fetch 后固定引用、冲突数或文件集合变化：停止并刷新 PRD/design/implement。
- 新旧 Chat->Anthropic 状态机仍同时存在或 build 稳定性测试未迁移：不得提交。
- Grok account usage 自动发起 Billing/Responses 请求：不得提交。
- Lite header 进入上游，或 namespace/function 与 hosted tool 同时存在：不得提交。
- duplicate、Codex reset、Spark shadow 任一菜单/handler 丢失：不得提交。
- Wire 只能通过手改生成文件才能编译：回到 provider 源修复，不得提交。
- 任一定向测试、全量 unit、typecheck 或 lint 失败：继续修复或明确环境阻断并取得
  用户决定。

## 回滚点

- 合并前：备份分支。
- 合并中：`git merge --abort`，确认工作区恢复到 pre-merge build。
- merge commit 后：保留双父历史，只用后续修复提交；不自动 reset/amend。
- push、部署和 migration 均不在本任务自动授权范围。
