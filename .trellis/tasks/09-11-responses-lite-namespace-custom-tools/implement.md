# Implement: Responses Lite 兼容（chat 桥接修复 + 原生降级开关）

## 前置

- 读 `.trellis/spec/backend/protocol-adapter-guidelines.md` L1053-1134（Responses 模型映射/chat 回退）与 L1224-1312（Lite 工具边界）、`quality-guidelines.md`、`.trellis/spec/guides/build-private-feature-isolation-guide.md`、`.trellis/spec/frontend/component-guidelines.md`。
- 提交拆分建议：① apicompat 共享修复（chat 桥 + 原生）；② service 开关与接线；③ 前端开关与文案。

## 步骤

### A. 共享契约与 chat 桥接（apicompat）
1. [x] `NamespacedToolName` 增加 `Custom`；`NamespaceToolNames` 记录 custom 子工具。
2. [x] `namespaceChildrenToChatTools` 接受 custom 子工具（`customToolInputSchema`）。
3. [x] 历史 `custom_tool_call` 分支按 namespace 摊平。
4. [x] 以 `resolveCustomToolCall` / `customIdentityForStreamTool` 替代 `customToolCallName` / `customNameForStreamTool`，新增精确 namespace custom 与唯一裸名别名；更新调用点；wire 序列化补 `custom_tool_call.namespace`。
5. [x] 非流式、`announceChatToolItem`、收尾、最终 output 四处补 `Namespace`。
6. [x] 单测：AC1-AC5（`chatcompletions_responses_bridge_custom_tools_test.go`）。

### B. 原生路径共享修复（apicompat）
7. [x] `FlattenResponsesNamespacesExcept` 接受 custom 子工具并降级、标记 `Custom`。
8. [x] `rewriteNamespaceQualifiedCall` 处理带 namespace 的 `custom_tool_call`。
9. [x] `restoreResponsesOutputClientTools`、`restoreClientToolValue`、`recordItem` 与流式 custom 事件发射输出带 namespace 的 `custom_tool_call`。
10. [x] 单测：AC7（`responses_client_tools_test.go` / `responses_namespace_test.go`）。

### C. 开关与接线（service）
11. [x] 新建 `openai_responses_lite_downgrade.go`：常量、访问器、`applyOpenAIResponsesLiteDowngrade`、上下文标记读写。
12. [x] `Forward` 在 Lite 段最前接线；扩展 `:151` 原生适配条件。
13. [x] 单测：AC6、AC8、AC9、AC10、AC11（全部放在新建的 `openai_responses_lite_downgrade_test.go`，与 Lite 用例集中维护）。

### D. 前端
14. [x] `features/responsesLite/extra.ts` + `__tests__/extra.spec.ts`。
15. [x] `features/responsesLite/ResponsesLiteDowngradeToggle.vue` + 组件 spec。
16. [x] `EditAccountModal.vue` / `CreateAccountModal.vue` 接入与提交写入；补弹窗 spec 断言（AC12）。
17. [x] `locales/{zh,en}/admin/accountsResponsesLite.ts` 并 spread 进 `accounts.ts`；更新 locale 扩展 spec。

### E. 验证
18. [x] 后端：`gofmt -l`、apicompat 与 service 单测全量。
19. [x] 前端：相关 spec、`pnpm typecheck`、改动文件 eslint。

## 验证命令

```bash
cd backend
gofmt -l ./internal/pkg/apicompat ./internal/service
go test -tags=unit ./internal/pkg/apicompat -count=1
go test -tags=unit ./internal/service -count=1
cd ../frontend
pnpm test -- --run src/features/responsesLite src/components/account/__tests__ src/i18n
pnpm type-check
```

## 风险文件 / 回滚点

- `chatcompletions_responses_bridge.go`、`responses_client_tools.go`、`responses_namespace.go`、`openai_gateway_forward.go` 为上游共享文件；改动限定在设计第 3-5 节列出的函数，提交前 `git diff --stat` 复核。
- 回滚点：三个提交可独立 revert；开关关闭恢复原生路径旧行为。

## 收尾检查（task.py start 前）

- [x] prd.md 已收敛，无 Open Questions。
- [x] design.md / implement.md 与 prd.md 的 R/AC 编号一致。
- [x] implement.jsonl / check.jsonl 已填真实 spec 条目。

## Phase 3 提醒

- 3.3 更新 `protocol-adapter-guidelines.md`：新增 scenario "Responses Lite 降级与 namespace custom 子工具"，覆盖开关语义、chat/原生两条路径契约与测试要求；frontend spec 如有账号弹窗字段清单一并补充。
- 可选：在 Wei-Shaw/sub2api#6653 留言说明真实 Lite 形态。

## 检查记录（2026-09-12 Check-All full）

- 结论：通过·已接受风险。验证 9/9 通过（gofmt、go vet、golangci-lint v2.13.0 0 issues、apicompat 与 service 全量单测、vitest 功能/i18n/弹窗 spec、`pnpm typecheck`、改动文件 eslint）。
- 已接受风险（用户 2026-09-12 明确接受当前报告全部风险）：
  - `FBK-001` P1：降级删除入站 Lite 头后，handler 级账号 failover 的后续尝试读不到 Lite 头，DeepSeek 账号跳过降级、OAuth 账号按非 Lite 处理。后续修法：领域文件内记录入站 Lite 标记并按需回填头。
  - `CHK-001` P2：`openai_gateway_forward.go:367-371` 图片桥接门禁在降级后读不到 Lite 头，可能对 Lite 请求重新开启 hosted 工具注入，与 spec L1305 契约不一致。后续修法：门禁同时判断 `openAIResponsesLiteDowngraded(c)`。
- 上线后验证：Kimi / MiniMax 原生 Responses 对降级后顶层 function 工具的接受度；DeepSeek 抽样确认。
