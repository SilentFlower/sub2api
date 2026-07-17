# Brief — 隔离 build 私有功能以降低上游合并冲突

## Goal

- 在不改变业务行为的前提下，完整审计 build 相对最新 main 的私有能力，并把仍嵌在上游共享热点中的 build 逻辑迁入按领域命名的独立文件或前端模块，使共享入口只保留稳定薄调用，降低后续 `main -> build` 合并冲突和语义回退风险。

## Scope

- 以 build 独有提交、first-parent merge 决策、假想合并树和文件历史为证据，覆盖 15 个当前仍独有的产品/协议领域与 5 类分支资产；同时标明 main 已替代、中央契约、生成文件和非功能差异。
- 后端隔离 `account.go` 私有方法、Codex reset handler、Grok 独立 Billing handler、Raw Chat debug snapshot，以及 Responses Lite、生图、reasoning、JSON Schema/Web Search、Grok force-chat 等共享入口接入。
- 保持 AnySearch provider、Anthropic 直连桥、Codex reset service/repository、Grok Billing service/parser、Alpha Search、Responses web.run 等已有功能主体，不做机械拆分。
- 前端抽离 Create/Edit/Bulk 的 build 私有表单块，拆分 Codex reset 与 Grok Billing API/DTO，拆出 SettingsView 生图/Responses Lite 面板和 ChannelsView AnySearch 配置，按功能域拆分 accounts/settings/channels locale 与测试。
- 解决三个预演硬冲突：WS 通用测试留在原文件、Lite 三轮模型切换迁入独立测试；中英文 accounts 私有文案迁入扩展模块，main 图片桥接文案由父任务合并。
- DTO、共享类型、route、构造器、ProviderSet、API contract、模型元数据和 Ent/Wire 只保留最小中央接入；Wire 在依赖图稳定后重新生成。
- 只读保护 HA/DR、手动 GHCR workflow、fork CLA 策略、README_CN 和 Trellis/agents 资产，不重写已独立内容。

## Non-Goals

- 不把所有大文件机械拆小，不创建 `build_helpers.go` 或统一 build 总开关。
- 不重写已按单一功能域组织的协议状态机、HA/DR、CI、README 或 Trellis 资产。
- 不改变数据库结构、公开 API、默认值、错误 reason、模型策略、调度、计费、缓存或前端交互。
- 不恢复已被 main 覆盖的 GPT-5.6 `max` 重复算法，也不合并 Grok 独立 `/billing-quota` 与 main `/quota`。
- 本子任务不执行真实 `main -> build` merge；隔离提交完成后由父任务重新 fetch、merge-tree 和合并。

## Key Context

- 当前 build 为 `c7335534`；最新 main 为 `bc2244c8`（0.1.158）；共同基线为 `b960ec19`；预演树为 `853a87babb7e30987baef4b0dbf5ecd20a6b8baa`。
- 假想合并树相对 main 有 533 个差异文件；backend/frontend 共 201 个，其中生产或配置 111、测试 90，修改 160、新增 41。
- Git 侧有 136 个仅 build 可达的非 merge 提交，过滤 bookkeeping 后有 45 个代码/运维候选提交；merge-only 决策还包含 Responses Lite Header 模型策略等当前契约。
- 三个硬冲突是 `openai_ws_forwarder_ingress_session_test.go`、英文 `accounts.ts`、中文 `accounts.ts`；`wire_gen.go` 不是硬冲突。
- build 当前独有领域包括 custom client、Codex reset、生图设置、Codex 身份、Alpha Search、图片桥接、Responses Lite、Anthropic 直连、provider reasoning/GPT-5.6 元数据、Grok force-chat、Grok Billing、JSON Schema、Web Search/web.run、AnySearch 和 Raw Chat debug。
- main 0.1.158 的 Grok 端点/WS、生图终态、Codex 模型发现、用户批量限额和分组复制属于上游增量，不得误算为 build 私有或在后续合并中丢失。
- 详细证据见 `research/build-private-feature-inventory.md` 与 `research/build-commit-classification.md`；技术边界和顺序见 `design.md`、`implement.md`。

## Acceptance

- 完整清单逐项给出 Git/文件证据和保持、抽离、main 替代、中央保留或生成决定。
- Codex reset handler、Grok Billing handler、Raw Chat debug、AnySearch 接入及其它高冲突入口完成领域隔离，共享入口不再定义 build 业务语义。
- Create/Edit/Bulk、SettingsView、ChannelsView、功能 API/DTO 和 locale 完成模块隔离，现有 credentials/extra/settings payload 与 i18n key 不变。
- 三个原始硬冲突被消除或显著缩小，build 专属断言未减少；27 个双方修改文件在父任务合并时完成语义复核。
- Wire/Ent 等生成结果来自源定义；后端定向/完整 unit/lint、前端定向/完整测试/typecheck/lint/build 和 `git diff --check` 通过。
- 子任务提交后重新运行 merge-tree，剩余冲突写回父任务；真实 merge 后无冲突索引、conflict marker 或手工生成残留。

## Next Step

- 用户确认最新三件套和本 brief 后运行 `task.py start`，随后进入 `trellis-route(implement)`；实现、检查和子任务独立提交完成后恢复父任务，重新拉取 main 并预演真实合并。
