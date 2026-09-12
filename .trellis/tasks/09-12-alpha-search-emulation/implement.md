# Implement: Codex Alpha Search 上游 Responses web_search 桥接与本地模拟兜底

## 前置

- 读 `.trellis/spec/backend/protocol-adapter-guidelines.md` L923-1047（Alpha Search 独立端点转发）、`quality-guidelines.md`、`.trellis/spec/guides/build-private-feature-isolation-guide.md`、`.trellis/spec/frontend/component-guidelines.md`。
- 参照实现：`openai_alpha_search.go:142-330`（PAT 翻译路径）、`openai_responses_websearch.go:370-445,478-495,542-600`（模拟执行、结果数映射、域名过滤、输出格式）、`codex_web_search_bridge.go:108-120`（资格判定）、`openai_responses_lite_downgrade.go` 与 `features/responsesLite/`（开关模式）。
- 提交拆分：① 后端领域文件 + 接线 + 单测；② 前端开关、文案与 spec；③ 规范文档。

## 步骤

### A. 后端领域文件
1. [ ] 新建 `openai_alpha_search_responses_bridge.go`：常量 `accountExtraKeyOpenAIAlphaSearchViaResponses`、访问器 `IsOpenAIAlphaSearchViaResponsesEnabled`、`buildOpenAIAlphaSearchAPIKeyResponsesRequest`、`forwardAlphaSearchViaUpstreamResponsesWebSearch`（含 3.4 分类表）。
2. [ ] 扩展 `parseOpenAIResponsesSSEForAlphaSearch` 返回 `searched`，更新 `openAIAlphaSearchResponseFromResponsesSSE` 调用点（PAT 路径忽略）。
3. [ ] 新建 `openai_alpha_search_emulation.go`：`alphaSearchEmulationEligible`、`parseOpenAIAlphaSearchCommands`、`emulateOpenAIAlphaSearch`、`buildOpenAIAlphaSearchEmulationOutput`、`writeOpenAIAlphaSearchFailed`（502 JSON）。
4. [ ] `ForwardAlphaSearch` 在 PAT 分支后加 `if account.IsOpenAIAlphaSearchViaResponsesEnabled() { return s.forwardAlphaSearchViaUpstreamResponsesWebSearch(...) }`。
5. [ ] 单测 `openai_alpha_search_responses_bridge_test.go` / `openai_alpha_search_emulation_test.go`：AC1-AC10。夹具复用 `httpUpstreamRecorder`、`alphaSearchResponsesSSE`、`alphaSearchAccountStateRepo`；模拟资格用 `codex_web_search_bridge_test.go` 里的 settingService/manager 夹具，执行器用 `openAIWebSearchExecutor` 注入。

### B. 前端
6. [ ] `features/alphaSearch/extra.ts` + `__tests__/extra.spec.ts`。
7. [ ] `features/alphaSearch/AlphaSearchViaResponsesToggle.vue` + `__tests__/AlphaSearchViaResponsesToggle.spec.ts`。
8. [ ] `EditAccountModal.vue`：import、`alphaSearchViaResponsesEnabled` ref、`canConfigureAlphaSearchViaResponses` computed、模板块紧随 Lite 块、`:3939` 处读取、`:5549` 处保存。
9. [ ] `CreateAccountModal.vue`：同上，`:4846` 平台切换重置、`:5462` 保存构造。
10. [ ] `locales/{zh,en}/admin/accountsAlphaSearch.ts`，spread 进 `accounts.ts` `openai` 段（zh `:706` / en `:623` 旁），`buildFeatureLocaleExtensions.spec.ts` 加配对与非空断言。
11. [ ] Edit/Create 弹窗 spec：显示条件、保存 payload、关闭删除键（AC11）。

### C. 文档与验证
12. [ ] `protocol-adapter-guidelines.md`：Alpha Search scenario contracts 加例外一句；L1047 后新增 "Scenario: Codex Alpha Search 经上游 Responses web_search 桥接与本地模拟兜底"（Scope/Signatures/Contracts/Matrix/Cases/Tests）。
13. [ ] 后端 `gofmt -l`、定向单测、全量 service 单测。
14. [ ] 前端定向 spec、`pnpm typecheck`、改动文件 eslint。

## 验证命令

```bash
cd backend
gofmt -l ./internal/service ./internal/handler
go test -tags=unit ./internal/service ./internal/handler -run 'AlphaSearch|WebSearch' -count=1
go test -tags=unit ./internal/service -count=1
cd ../frontend
pnpm vitest run src/features/alphaSearch src/components/account/__tests__/EditAccountModal.spec.ts src/components/account/__tests__/CreateAccountModal.spec.ts src/i18n/__tests__/buildFeatureLocaleExtensions.spec.ts
pnpm typecheck
```

## 风险文件 / 回滚点

- `openai_alpha_search.go` 为上游共享文件：只允许一次薄调用与 SSE 解析函数返回值扩展，提交前 `git diff` 复核。
- `EditAccountModal.vue` / `CreateAccountModal.vue` / `accounts.ts` 为上游共享文件：每处改动对齐 Lite 开关的既有行数模式。
- 回滚点：三个提交可独立 revert；开关关闭即恢复旧行为。

## 收尾检查（task.py start 前）

- [x] prd.md 已收敛，无 Open Questions。
- [x] design.md / implement.md 与 prd.md 的 R/AC 编号一致。
- [x] implement.jsonl / check.jsonl 已填真实 spec 条目。
