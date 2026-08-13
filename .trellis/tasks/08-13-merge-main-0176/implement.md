# 合并 main 0.1.176 到 build - 实施计划

## 1. 合并准备

- [x] 复核 `build` 工作区干净、`main` 指向 `fbfdcef81`、版本为 `0.1.176`。
- [x] 创建 `backup/build-before-main-0176-<timestamp>-<sha>` 保护引用。
- [x] 记录 merge-base、双方独有提交数、双边修改文件和 build 私有能力清单。

## 2. 执行合并与处理冲突

- [x] 执行 `git merge --no-ff main`，保留真实 merge 状态。
- [x] Grok 冲突采用 main 行为；只删除独立套餐额度查询/展示链路，保留 force-chat、
  provider reasoning 和登录脚本。
- [x] Anthropic 桥组合 reasoning alias 与 build 流式/工具稳定性契约。
- [x] Create/Edit/Bulk 同时保留自定义 UA 和 fingerprint mode 的 UI、状态、回填、
  reset、校验及 payload。
- [x] `AccountsView.vue` 仅替换 Grok 解析，保留 Codex reset 和其它 build 装配。
- [x] 清理全部冲突标记，检查未合并索引为空。

## 3. 删除 Grok 独立套餐额度链路

- [x] 删除独立 parser/service/handler/route/ProviderSet 和专项后端测试。
- [x] 从 UsageInfo、DTO/API contract、admin API 中移除 `grok_billing_quota`。
- [x] 删除前端 `features/grokBillingQuota`、组件、队列、类型、locale 和专项测试。
- [x] 清理 AccountUsageCell、AccountsView、locale extension 和其它 fixture 的引用。
- [x] 保留历史 extra key 的 scheduler-neutral 兼容，不新增数据迁移。
- [x] 重新生成 Wire，并用 `rg` 确认生产代码无独立链路引用。

## 4. build 私有能力回归审计

- [x] 按 `research/build-feature-regression-audit.md` 检查每项 owner、入口、数据往返和测试。
- [x] 单独验证 Codex 统一身份与 main fingerprint mode 的组合顺序和 HTTP/WS 一致性。
- [x] 单独验证 Responses Lite 与 main x_search、reasoning alias、图片流 failover 的组合。
- [x] 单独验证 Web Search/web.run/AnySearch 与原生 `/x_search` 的分流及 sources 保留。
- [x] 单独验证 Anthropic 直连桥、provider reasoning、Grok force-chat 和 Raw Chat 调试。
- [x] 验证 Codex reset、Alpha Search、Antigravity GIF、设置面板、locale、CI/README/
  Trellis/HA 资产没有静默消失。

## 5. 验证

- [x] 后端格式化并运行受影响包单元测试：apicompat、handler、repository、server、service。
- [x] 运行完整 `go test -tags=unit ./...` 和项目 lint/构建门禁。
- [x] 前端运行 `pnpm lint:check`、`pnpm typecheck`、功能矩阵定向 Vitest、
  `pnpm test:run` 和 `pnpm build`。
- [x] 执行冲突标记、失效 import、孤立 route/provider、旧 Grok Billing 引用和
  locale 最终值扫描。
- [x] 重新计算 main/build 双边文件交集，逐项记录自动合并文件的最终语义。
- [x] 输出实际丢失/修复风险清单；通过 Check-All 后再等待提交确认。
