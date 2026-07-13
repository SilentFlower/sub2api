# Implement Plan: 同步 main 0.1.151 到 build 并保护定制特性

## 执行清单

1. 记录当前 `build` HEAD、工作区、跟踪分支和远端 URL。
2. 执行 `git fetch origin main build`；确认本地 `build` 未落后 `origin/build`，固定目标 `origin/main` 提交。
3. 重新计算 merge base、独有提交、双方共同修改文件和 `git merge-tree` 冲突列表；若与规划不同，先更新 `research/merge-evidence.md`、`prd.md` 和 `design.md`。
4. 创建并验证 `backup/build-before-main-0151-<build短提交号>`。
5. 执行 `git merge --no-commit --no-ff origin/main`，记录实际 `MERGE_HEAD` 和未解决索引项。
6. 解决 `openai_gateway_grok.go`：使用 provider 归一化后的最终请求体提取 effort，不保留多余的原始模型提取变量。
7. 解决 `openai_gateway_grok_test.go`：覆盖嵌套别名与扁平兼容字段，并验证上游 body、result metadata 和 usage 一致。
8. 解决 `openai_gateway_messages_chat_fallback.go`：保留直连 Anthropic/Chat 桥接、custom UA、Beta Fast、错误/failover、quota 与缓存稳定契约；拒绝旧 Responses 中转代码错位进入 header builder。
9. 解决 `openai_oauth_passthrough_test.go`：保留双方身份测试并确保生产身份配对逻辑复用统一版本常量。
10. 按 `design.md` 逐项复核 21 个自动合并文件，重点验证：
    - apicompat custom/tool_search/namespace 与 cache creation；
    - Codex identity 与 image_gen HTTP/raw/WS；
    - Fast/Flex `user_ids` 跨层链路；
    - GPT-5.6 `max`、长上下文定价与 OpenCode 配置；
    - setup-token refresh、writer nil guard 和 migration 173。
11. 搜索重复函数、重复常量、旧调用签名、未解决索引项和冲突标记；对实际修改的 Go 文件运行 gofmt。
12. 运行定向回归测试并修复失败，随后运行后端 package 级、全量 unit、lint 和前端质量门槛。
13. 运行 `git diff --cached --check`、`git ls-files -u`、冲突标记扫描和最终状态检查。
14. 创建 merge commit，提交信息使用中文并明确同步 0.1.151 与保留 build 特性。
15. 验证 merge commit 有两个父提交，目标 `origin/main` 是当前 `build` 的祖先，工作区干净。
16. 再次确认远端 `origin/build` 未发生并发前进；执行普通 `git push origin build`，禁止 force push。
17. 使用远端引用核对 push 结果，记录最终 merge commit、备份分支、冲突处理和验证结果。

## 验证命令

基础 Git 检查：

```bash
git status --short --branch
git ls-files -u
rg -n '^(<<<<<<<|=======|>>>>>>>)' . --glob '!frontend/node_modules/**'
git diff --cached --check
```

后端定向测试：

```bash
cd backend && go test -tags=unit ./internal/pkg/apicompat -count=1
cd backend && go test -tags=unit ./internal/service -run 'Test.*(Grok|ReasoningEffort|Messages|ChatCompletions|Responses|CodexIdentity|ImageGeneration|OpenAIFast|TokenRefresh)' -count=1
cd backend && go test -tags=unit ./internal/handler ./internal/server/middleware ./internal/repository -count=1
cd backend && go test -tags=unit ./migrations -count=1
```

后端完整质量门槛：

```bash
cd backend && go test -tags=unit ./... -count=1
cd backend && GOTOOLCHAIN=go1.26.5 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.9.0 run --timeout=30m --build-tags=unit --new ./...
```

前端验证：

```bash
cd frontend && pnpm vitest run src/components/keys/__tests__/UseKeyModal.spec.ts src/views/admin/__tests__/SettingsView.spec.ts src/composables/__tests__/useModelWhitelist.spec.ts
cd frontend && pnpm typecheck
cd frontend && pnpm lint:check
```

提交与推送验证：

```bash
git rev-list --parents -n 1 HEAD
git merge-base --is-ancestor <target-main-commit> HEAD
git push origin build
git ls-remote --heads origin build
```

## 风险点

- Git 自动合并不会报告跨文件语义回退，21 个共同修改文件必须逐个复核。
- Messages fallback 的文本冲突来自结构错位，误收 main 冲突块会把响应转换代码放进 header builder。
- Grok effort 必须记录最终上游值；从原始模型重新解析会产生 usage 漂移。
- Codex identity 新旧文件职责互补，但版本字面量必须继续单点维护。
- 直接推送存在远端并发更新风险；普通 push 被拒绝时必须停止，不能 force push。
- migration 历史存在重复数字前缀，但已发布文件不可整理或重命名。

## 回滚点

- 合并前：备份分支。
- merge commit 前：`git merge --abort`。
- push 前：停止并报告，不执行历史改写。
- push 后：`git revert -m 1 <merge-commit>` 后普通推送。
