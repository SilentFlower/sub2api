# 合并证据记录

## 分支状态

- 记录日期：2026-07-13。
- 当前分支：`build`。
- 当前本地 `build`：`f59991b5`，与 `origin/build` 一致。
- 当前本地 `main`：`e316ebf5`。
- 当前 `origin/main`：`a1930ea6`，版本 0.1.152；本地 `main` 可纯快进 40 个提交。
- merge base：`e316ebf52838a89d57fc790981cce7520f819ac8`。
- 独有提交：`build` 100，`origin/main` 40。
- 上一轮 main 0.1.151 已通过 merge commit `0712a147` 进入 build，因此本次基点正好是上一轮目标 main。

## 历史经验

- 相关历史任务：`.trellis/tasks/07-10-sync-main-0151-into-build`。
- `trellis mem` 会话：`019f4c9b-694e-70b0-b74f-8c46232dc0d4`。
- 上次确认的有效做法：合并前创建备份分支；逐文件组合语义；审查自动合并文件；Codex 版本单点定义；验证通过后使用双父 merge commit。
- 本次用户确认：创建本地 merge commit 后暂停，等待单独确认再推送。

## Git 预演

规划阶段执行：

```bash
git merge-tree --write-tree --messages build origin/main
```

- 虚拟合并树：`62c6d7bdb64a74cd5d67be64d1d76862d26afc4f`。
- 合并结果相对 build：118 个文件变化、5973 行新增、382 行删除。
- 双方共同修改 43 个文件。
- 文本冲突 12 个，自动合并但需语义复核 31 个。

## 文本冲突

1. `backend/internal/handler/endpoint.go`
2. `backend/internal/handler/openai_alpha_search.go`
3. `backend/internal/handler/openai_chat_completions.go`
4. `backend/internal/service/openai_alpha_search.go`
5. `backend/internal/service/openai_alpha_search_test.go`
6. `backend/internal/service/openai_gateway_chat_completions_raw.go`
7. `backend/internal/service/openai_gateway_grok.go`
8. `backend/internal/service/openai_gateway_grok_test.go`
9. `backend/internal/service/openai_gateway_messages_chat_fallback.go`
10. `backend/internal/service/openai_gateway_messages_chat_fallback_test.go`
11. `backend/internal/service/openai_gateway_service.go`
12. `frontend/src/components/account/__tests__/EditAccountModal.spec.ts`

详细双方意图与处理方案见 `design.md` 的文本冲突解决矩阵。

## 自动合并复核文件

1. `backend/internal/handler/admin/grok_oauth_handler_test.go`
2. `backend/internal/handler/dto/types.go`
3. `backend/internal/handler/endpoint_test.go`
4. `backend/internal/handler/openai_gateway_handler.go`
5. `backend/internal/pkg/apicompat/types.go`
6. `backend/internal/repository/account_repo.go`
7. `backend/internal/server/api_contract_test.go`
8. `backend/internal/server/routes/gateway.go`
9. `backend/internal/server/routes/gateway_test.go`
10. `backend/internal/service/account.go`
11. `backend/internal/service/grok_quota_service.go`
12. `backend/internal/service/grok_quota_service_test.go`
13. `backend/internal/service/openai_codex_transform.go`
14. `backend/internal/service/openai_compat_model_test.go`
15. `backend/internal/service/openai_gateway_cc_pipeline.go`
16. `backend/internal/service/openai_gateway_chat_completions.go`
17. `backend/internal/service/openai_gateway_forward.go`
18. `backend/internal/service/openai_gateway_messages.go`
19. `backend/internal/service/openai_gateway_responses_chat_fallback.go`
20. `backend/internal/service/openai_oauth_passthrough_test.go`
21. `backend/internal/service/openai_ws_http_bridge.go`
22. `backend/internal/service/openai_ws_http_bridge_test.go`
23. `frontend/src/components/account/AccountUsageCell.vue`
24. `frontend/src/components/account/CreateAccountModal.vue`
25. `frontend/src/components/account/EditAccountModal.vue`
26. `frontend/src/components/account/__tests__/AccountUsageCell.spec.ts`
27. `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`
28. `frontend/src/i18n/locales/en/admin/settings.ts`
29. `frontend/src/i18n/locales/zh/admin/settings.ts`
30. `frontend/src/types/index.ts`
31. `frontend/src/views/admin/SettingsView.vue`

## main 0.1.152 重点增量

- Alpha Search 按次计费：`7cbb36f2`，复审修复 `64a2a317`，契约夹具补齐 `e5af699d`。
- migration：`174_group_web_search_price_per_call.sql`，新增 `groups.web_search_price_per_call DECIMAL(20,8)`，NULL 使用默认 0.01 USD/次。
- Alpha Search 计费结果：成功 2xx 返回 `OpenAIForwardResult.WebSearchCalls=1`；普通错误、重定向和 failover 不产生按次费用。
- Grok/xAI：API Key 账号支持 `d9e466ad`、cache/路由集成修复 `038b25c0`、Composer reasoning 清理 `aeb34d20`、按平台诊断不可用模型 `8a22dc73`。
- 其它重点：Codex alpha/search 上游修复、remote compaction、compact writer、message item id、OpenAI Messages identity 和 Fast/Flex 用户范围。

## 隐藏风险证据

- 虚拟合并后的 `endpoint_test.go` 同时包含两参数与三参数 `resolveOpenAIUpstreamEndpoint` 调用，Git 不会报告，但采用 main 三参数签名后旧调用无法编译。
- main 允许 Grok `/v1/chat/completions` 动态走 raw Chat 或 Responses bridge；build 旧版仅凭入站 Chat 路径回退为 Chat 会误记桥接请求。最终端点必须优先来自 result/runtime，缺失时默认 Responses。
- build 的 `openai_codex_client_identity.go` 已声明 `codexCLIVersion=openAICodexClientVersion`；若接受 main 在 `openai_gateway_service.go` 中的字面量声明会重复定义并破坏单点版本契约。
- 当前协议规范仍记录 `ForwardAlphaSearch(... ) error` 和旧端点判定签名，main 已引入 result/实际端点契约，必须同步规范。
- `openai_gateway_messages_chat_fallback.go` 的 `true` 表示强制上游 SSE，不等于客户端 `stream` 偏好；直接选择 main 的 `clientStream` 会回退 build 的非流式折叠设计。
- Grok 4.5 usage 必须从 provider 归一化后的最终 body 提取；采用 main 冲突块的改写前值会把 `xhigh -> high` 记录成旧值。

## 工作区证据

规划开始前已有：

- 修改：`.trellis/spec/backend/disaster-recovery-guidelines.md`。
- 未跟踪：`.trellis/tasks/07-10-sync-main-0151-into-build/` 下 7 类规划/研究材料。

当前任务目录是本轮新增，不属于应隔离的旧内容。虚拟合并结果不会修改上述旧路径；实际操作仍采用选择性 stash，避免用户内容进入 merge commit，同时保持当前任务可见。

## 实施记录

- 实施前重新 fetch 后远端引用未变化：`origin/main=a1930ea6`、`origin/build=f59991b5`。
- 本地 `main` 已纯快进到 `a1930ea6`，与 `origin/main` 完全一致。
- 已创建备份分支 `backup/build-before-main-0152-f59991b5`，准确指向合并前 `build=f59991b5`。
- 用户已有 Trellis 内容已选择性暂存到 `stash@{0}`（`pre-merge-main-0152-existing-trellis`）；当前任务目录保持可见且未进入合并索引。
- 已执行 `git merge --no-commit --no-ff main`，实际 `MERGE_HEAD=a1930ea6`。
- 12 个文本冲突已按 `design.md` 矩阵逐项组合解决；31 个自动合并文件已完成协议、计费、身份、前端和迁移链路复核。
- 额外修复了三个 Git 未报告的连带问题：端点解析旧调用签名、WebSocket HTTP bridge 新增 cache identity 参数、Grok usage snapshot 的账号参数类型。
- 目标测试发现并修正 Grok `/v1/chat/completions` 动态选择 Responses bridge 时的端点误记：最终优先使用转发结果，其次使用运行时端点，缺失时 Grok 默认 Responses；显式 Messages/API Key raw Chat 仍回退 Chat Completions。
- Ent 生成器执行成功，生成后工作区相对索引无差异；migration 目录仅新增 `174_group_web_search_price_per_call.sql`，未修改既有 migration。

## 验证结果

- 后端冲突相关定向测试：通过。
- `internal/pkg/apicompat`、`internal/repository`、`internal/server`、`migrations` 扩展测试：通过。
- 后端全量 `go test -tags=unit ./... -count=1`：通过。
- 后端 golangci-lint（unit build tags，`--new`）：通过，`0 issues`。
- 前端相关 5 个测试文件：66 个用例通过。
- 前端全量 Vitest：146 个测试文件、952 个用例通过。
- 前端 `pnpm typecheck`：通过。
- 前端 `pnpm lint:check`：通过。
- `git diff --check`、`git diff --cached --check`、`git ls-files -u`：通过；冲突标记扫描仅命中一处原有测试字符串。
- `go generate ./ent`：通过，未产生生成代码漂移。

## Check All 结论

- 三件套实现：提交前阶段无偏差；本地 merge commit、stash 恢复和提交拓扑验证按流程在最终检查通过后执行。
- 假设验证：API 字段、nullable 历史值、跨层数据流、Grok 动态端点和组件 extra 保留均由源码与自动化测试确认。
- 跨层完整性与规范：migration、Ent、DTO、cache、计费、usage、前端类型和管理界面闭环完整；双端 lint、类型检查、测试和格式检查通过。

## 本地提交与恢复

- 本地 merge commit：`80679de907706c8b639942cddac1b3081cbded42`（`merge(main): 同步 0.1.152 并保留 build 特性`）。
- 第一父提交：`f59991b562933b8eb05f1e756dfc2d0badbba136`（合并前 `build`）。
- 第二父提交：`a1930ea6f29fc5f17ae0020f4e2d38e789c49d73`（固定目标 `main`）。
- 两个父提交的祖先检查均通过；提交创建前最后一次 fetch 后 `origin/main` 和 `origin/build` 未变化。
- 选择性 stash 已成功 pop 并删除，用户容灾规范 blob 哈希恢复为 `ab15a4190e1acc2afe82deec76e08e97412175b2`。
- 旧任务目录聚合哈希恢复为 `f145d1ee9f6d0d525c17173965a4cf575a636f9ea15f0a01cf39d14f7820df7c`。
- 当前 `build` 相对 `origin/build` ahead 41；`origin/build` 仍为 `f59991b5`，本轮未执行 push。
