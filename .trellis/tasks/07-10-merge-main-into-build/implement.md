# Implement Plan: 合并 main 到 build 并保留现有 feature

## Checklist

1. 记录当前 `build` HEAD、工作区状态和远端跟踪状态。
2. 执行 `git fetch origin main build`，确认 `origin/main` 和 `origin/build`，重新计算分叉与冲突列表；若冲突面变化，先更新本任务材料。
3. 创建 `backup/build-before-main-merge-<build短提交号>`，验证其指向合并前 HEAD。
4. 执行 `git merge --no-commit --no-ff origin/main`，保留冲突现场并记录实际冲突列表。
5. 解决 Codex identity 三处冲突：
   - `account_usage_service.go`
   - `openai_gateway_service.go`
   - `setting_gateway_runtime.go`
   统一引用 `openai_codex_client_identity.go`，不重复定义版本或 UA。
6. 解决 effort 四处冲突：
   - raw Chat、Messages fallback、Responses fallback 保留最终上游 body 提取；
   - WS HTTP bridge 保留 Grok 4.5 最终值；
   - 扩展组合 helper，使非 provider-specific 路径同时使用 upstream/billing/original 模型候选。
7. 确认 Messages fallback 仍使用直连 `AnthropicToChatCompletions`，保留 Beta Fast `service_tier=priority`、Grok token/URL/quota 和缓存稳定契约。
8. 复核 8 个自动合并文件的最终差异，逐项对照相关 build 提交和 main 提交，修复文本无冲突但语义不完整的问题。
   - `normalizeOpenAIReasoningEffortForModel` 仅为 GPT-5.6 保留 `max`，其它 GPT/Codex 折叠为 `xhigh`；
   - pricing JSON 使用 main 的 GPT-5.6 分档价格/cache write 成本，并保留 build 的 max effort capability 标记。
9. 搜索重复定义、冲突标记和旧调用签名；对修改的 Go 文件运行 gofmt。
10. 运行定向测试：Codex identity、reasoning effort candidates、Grok 4.5、raw Chat、Messages/Responses fallback、WS bridge、Anthropic apicompat、Grok messages 路由和 quota。
11. 运行 package 级验证：backend apicompat/service/handler/repository，frontend typecheck 与相关 Vitest；按失败范围补跑全量 unit tests。
12. 检查 `git diff --check`、merge parents、`origin/main` 祖先关系和未解决状态。
13. 验证全部通过后创建双父 merge commit，以 normal 模式普通推送当前 `build` 到 `origin/build`；不 force push，不额外合并到其它分支，随后写入并推送任务 snapshot。

## Validation Commands

```bash
git diff --check
rg -n '^(<<<<<<<|=======|>>>>>>>)' . --glob '!frontend/node_modules/**'
cd backend && go test -tags=unit ./internal/pkg/apicompat
cd backend && go test -tags=unit ./internal/service
cd backend && go test -tags=unit ./internal/handler ./internal/repository
cd frontend && pnpm typecheck
```

定向回归至少覆盖：

```bash
cd backend && go test -tags=unit ./internal/service -run 'TestOpenAICodexClientIdentity|TestExtractOpenAIReasoningEffortFromBodyModelCandidates|TestExtractOpenAIUpstreamReasoningEffort|TestForwardAsRawChatCompletions|TestForwardAsAnthropic|TestForwardResponses|TestProxyOpenAIWSHTTPBridge|TestShouldForwardAnthropicMessagesViaRawChatCompletions'
cd backend && go test -tags=unit ./internal/handler -run 'TestResolveOpenAIUpstreamEndpointForGrokMessagesForceChat'
cd frontend && pnpm vitest run src/components/account/__tests__/EditAccountModal.spec.ts src/components/account/__tests__/CreateAccountModal.spec.ts
```

## Risk Points

- Git 自动合并不代表语义正确，尤其是 helper 签名变化与 build 后续调用组合。
- effort 在“客户端原始值”“provider 归一化值”“usage 元数据”之间语义不同，提取时机错误会产生账单维度漂移。
- `main` 与 `build` 都更新了 GPT-5.6/Codex 版本相关常量，重复定义会直接编译失败或未来版本漂移。
- model pricing JSON 可自动合并但字段值可能互相覆盖，需要结合对应提交和测试核验。
- 当前 `build-bak` 已过期，必须使用本次新备份分支。

## Rollback Points

- merge commit 前：`git merge --abort`。
- merge commit 后：从 `backup/build-before-main-merge-<build短提交号>` 恢复，或明确对 merge commit 做 revert。
- 任一验证阶段发现 feature 缺失时，先定位对应 build 独有提交，不用整文件 `ours` 覆盖 main 修复。

## Execution Results

- 远端主分支未发生变化，实际冲突与规划一致。
- 7 个冲突已解决，8 个自动合并文件已完成语义复核。
- 后端全量 unit、固定版本 golangci-lint、前端 typecheck/lint 和相关 7 个 Vitest 文件共 94 项测试已通过。
- Check All 的三件套实现、假设验证、跨层完整性与规范性三个维度均通过，未发现待修复问题。
- Phase 3.3 已更新后端协议规范，固化 `upstream -> billing -> original` effort 候选顺序、GPT-5.6 `max` 边界和 OAuth compact 例外。
- 当前停留在 merge 未提交状态，等待 Phase 3.4 按 normal 模式创建 merge commit 并自动推送 `origin/build`。
