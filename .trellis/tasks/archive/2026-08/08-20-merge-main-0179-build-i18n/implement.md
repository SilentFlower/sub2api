# 合并 main 0.1.179 到 build 并保护现有功能与 i18n - 实施计划

## 1. 合并前固定证据

- [x] 确认当前分支为 `build`，HEAD 为 `45ca348e8`，upstream 为 `origin/build`。
- [x] 重新 fetch 并确认 `origin/main` 仍为 `2bc139ab5`、VERSION 为 `0.1.179`；若目标漂移，
  回到 planning 更新冲突矩阵和 Brief。
- [x] 确认除当前任务产物外无 dirty、无未完成 merge/rebase/cherry-pick。
- [x] 重新运行 `git merge-tree --write-tree --messages --name-only HEAD origin/main`，确认
  冲突集合仍为 5 个文件、7 个 hunk。

## 2. 真实合并与硬冲突

- [x] 执行 `git merge --no-commit --no-ff origin/main`。
- [x] 按 `design.md` 合并 `openai_gateway_chat_completions.go` 的 imports 与 raw Chat 分流。
- [x] 按 `design.md` 合并 `openai_gateway_messages.go`，保留 native/adaptive 优先和 build
  force-chat helper。
- [x] 合并 Create Account 的 build 领域 imports 与 main adaptive/long-context imports。
- [x] 合并 Grok placeholder 断言和 Create Account spec 的 Web Search/simple-mode mocks。
- [x] 运行 gofmt/前端 formatter 的现有受控入口，清理冲突标记；确认 `git ls-files -u` 为空。

## 3. 自动合并语义复核

- [x] 用共同基线重新计算 build-only、main-only、overlap 集合，验证单侧路径 blob 无漂移。
- [x] 逐项复核 47 个 overlap，重点检查 Responses fallback、WebSocket bridge、Grok、
  account forms、billing、channels、Wire 和 migration。
- [x] 核对 build 领域 owner 与共享薄接入：Responses Lite、Web Search、DeepSeek downgrade、
  Codex custom clients、OpenAI image、Grok force-chat、Antigravity GIF。
- [x] 核对 main 新能力：adaptive protocol、CN header overrides、channel multipliers、
  long-context billing、monitor quota、Responses/client tools/tool-search/failover。
- [x] 对任何“0 冲突但最终语义变化”的热点补充或更新回归测试。

## 4. i18n 与 migration

- [x] 检查 6 个 main locale 文件中英文新增/修改项成对保留。
- [x] 检查 build locale extension 的中英文 import、深层 spread、key path 和最终运行时值。
- [x] 运行全部 i18n 专项测试，并对账号自适应协议、渠道倍率、长上下文说明和 build
  关键弹窗分别断言 en/zh 最终可见文案。
- [x] 保留 main 的 226-228 migration 与已有 `226_channel_monitor_quota_mode.sql`；确认
  migration runner 仍以完整 filename 排序和记录，运行相关 unit/integration 测试。

## 5. 聚焦验证

- [x] 后端 OpenAI/协议聚焦：

```bash
cd backend
go test -tags=unit ./internal/pkg/apicompat
go test -tags=unit ./internal/service -run 'Test.*(Messages|Chat|Responses|WebSocket|Grok|DeepSeek|Adaptive|HeaderOverride|LongContext|Pricing|Antigravity.*GIF|WebSearch)'
go test -tags=unit ./internal/handler/... -run 'Test.*(OpenAI|Grok|CN|Adaptive|Responses|WebSearch|Antigravity.*GIF|Channel)'
go test -tags=unit ./internal/repository/... ./migrations/...
```

- [x] 前端账号、渠道和 build feature 聚焦：

```bash
cd frontend
pnpm exec vitest run \
  src/components/account/__tests__ \
  src/components/admin/channel/__tests__ \
  src/features/codexCustomClients/__tests__ \
  src/features/openAICompatibility/__tests__ \
  src/i18n/__tests__ \
  src/views/admin/__tests__/channelPlatformOptions.spec.ts \
  src/views/admin/__tests__/platformFilterCatalogUsage.spec.ts
```

## 6. 全量质量门禁

- [x] 后端：

```bash
cd backend
go test -tags=unit ./...
go test -tags=integration ./...
golangci-lint run ./...
```

- [x] 前端：

```bash
cd frontend
pnpm typecheck
pnpm lint:check
pnpm test:run
pnpm build
```

- [x] 仓库级：

```bash
git diff --check
git ls-files -u
```

## 7. 最终复核与提交

- [ ] 对合并索引重新执行三方文件集合核对，记录 build-only、main-only、overlap 结果。
- [ ] 运行 Trellis Check-All；修复所有未接受的 CHK/FBK findings。
- [ ] 评估是否需要更新 build 隔离、协议适配或 i18n spec；需要时通过
  `trellis-update-spec` 写入，事实未变化则记录 no-op。
- [ ] 通过 `trellis-push` 展示精确 merge commit 计划并等待确认。
- [ ] 创建双父 merge commit，验证第一父为合并前 build、第二父为
  `2bc139ab5`，且 `origin/main` 为祖先。
- [ ] 经用户确认后普通推送 `build -> origin/build`；不得 force push。

## 8. 回滚点

- merge commit 前：使用 `git merge --abort` 恢复到合并前 HEAD，不使用 destructive reset。
- merge commit 后未推送：保留 commit 现场，按用户决定继续验证或显式 revert，不 amend 历史。
- 推送后：如需回滚，创建明确 revert/修复提交，不重写远端 `build`。
