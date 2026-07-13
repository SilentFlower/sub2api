# Implement Plan: 同步 main 0.1.152 到 build 并保护定制特性

## 执行清单

1. 记录当前分支、`build` HEAD、`origin/build`、本地 `main`、`origin/main`、工作区和远端 URL。
2. 执行 `git fetch --prune origin main build`，重新计算 merge base、独有提交、双方共同修改文件和 `git merge-tree`；若与规划证据不同，先更新 PRD、设计和研究记录。
3. 将本地 `main` 纯快进到固定的 `origin/main`，验证二者提交完全一致；不得 checkout 到 `main` 破坏当前工作区。
4. 创建并验证 `backup/build-before-main-0152-<build短提交号>`。
5. 选择性 stash 用户/历史已有的 8 个 Trellis 路径，保留当前任务目录可见；记录 stash 引用并确认当前任务未被暂存。
6. 执行 `git merge --no-commit --no-ff main`，记录实际 `MERGE_HEAD`、冲突索引和自动合并状态。
7. 按 `design.md` 的矩阵解决 12 个文本冲突；只使用显式路径 `git add`，禁止整仓 `git add -A`。
8. 修复 Git 未报告的连带问题：`endpoint_test.go` 旧签名、Codex 版本重复定义风险和协议规范签名漂移。
9. 逐项复核 31 个自动合并文件，重点检查：
   - Alpha Search migration/Ent/DTO/cache/billing/frontend 跨层闭环；
   - Grok APIKey/OAuth、cache、effort、quota 和错误诊断；
   - Anthropic Messages 直连桥接和 OpenAI fallback；
   - Codex identity、compact、image generation 和 WebSocket；
   - Grok 前端设置、Fast/Flex、UseKeyModal、SettingsView 和 i18n。
10. 更新 `.trellis/spec/backend/protocol-adapter-guidelines.md`：Alpha Search 返回签名、成功按次计费、实际上游端点判定新签名及优先级。
11. 对实际修改的 Go 文件运行 gofmt，搜索冲突标记、重复常量、重复函数和旧调用签名。
12. 运行冲突相关定向测试；按失败证据修复后，运行后端全量 unit、lint、migration/Ent 检查和前端质量门槛。
13. 运行 `git diff --cached --check`、`git ls-files -u`、冲突标记扫描、merge tree 祖先检查和最终状态复核。
14. 创建只包含 merge 结果和必要规范同步的本地双父 merge commit；验证两个父提交分别为合并前 `build` 和固定 `main`。
15. 恢复选择性 stash，确认用户已有规范改动与旧任务材料内容不丢失且未进入 merge commit。
16. 更新当前任务的研究/进度材料；如需提交任务材料，使用独立 `chore(task)` 提交，不重写 merge commit，也不混入用户已有路径。
17. 展示本地 merge commit、验证结果、备份分支、恢复后的工作区和最新远端状态，然后停止；未经用户再次确认不执行 push。

## 验证命令

Git 与格式检查：

```bash
git status --short --branch
git ls-files -u
rg -n '^(<<<<<<<|=======|>>>>>>>)' . --glob '!frontend/node_modules/**'
git diff --cached --check
git grep -n 'resolveOpenAIUpstreamEndpoint(c, [^,)]*)' -- 'backend/**/*.go'
```

后端定向测试：

```bash
cd backend && go test -tags=unit ./internal/handler ./internal/server/routes ./internal/service \
  -run 'AlphaSearch|NormalizeInboundEndpoint|DeriveUpstreamEndpoint|ResolveOpenAIUpstreamEndpoint|WebSearch|Grok|Messages|ChatCompletions|ReasoningEffort|CodexIdentity' -count=1
cd backend && go test -tags=unit ./internal/pkg/apicompat -count=1
cd backend && go test -tags=unit ./internal/repository ./internal/server -count=1
cd backend && go test -tags=unit ./migrations -count=1
```

后端完整质量门槛：

```bash
cd backend && go test -tags=unit ./... -count=1
cd backend && GOTOOLCHAIN=go1.26.5 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.9.0 run --timeout=30m --build-tags=unit --new ./...
```

前端验证：

```bash
cd frontend && pnpm vitest run \
  src/components/account/__tests__/EditAccountModal.spec.ts \
  src/components/account/__tests__/AccountUsageCell.spec.ts \
  src/components/keys/__tests__/UseKeyModal.spec.ts
cd frontend && pnpm test:run
cd frontend && pnpm typecheck
cd frontend && pnpm lint:check
```

提交拓扑验证：

```bash
git rev-list --parents -n 1 <merge-commit>
git merge-base --is-ancestor <target-main-commit> <merge-commit>
git merge-base --is-ancestor <pre-merge-build-commit> <merge-commit>
git fetch origin build main
git rev-parse origin/build origin/main
```

## 风险与门禁

- 远端引用变化：立即停止，重新规划，不能沿用旧冲突方案。
- 选择性 stash 失败或当前任务目录被包含：不开始 merge，先恢复任务可见性。
- 发现新增冲突或自动合并语义不明确：更新研究记录并回到规划讨论，不凭猜测选边。
- migration/Ent 生成不一致：不得手改生成代码掩盖问题，先核对 schema 与 migration，再运行生成/测试。
- 测试或 lint 未通过：不得创建 merge commit，除非明确记录无法执行的环境限制并取得用户确认。
- merge commit 创建后：不 amend、不 reset、不 push；先向用户展示结果。

## 回滚点

- 合并前：备份分支和选择性 stash。
- 合并中：`git merge --abort`，随后恢复 stash。
- merge commit 后：保留历史，使用后续修复提交；任何删除/重做本地提交需用户授权。
- push 后：不属于当前授权范围。
