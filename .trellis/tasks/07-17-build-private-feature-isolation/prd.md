# 隔离 build 私有功能以降低上游合并冲突

## Goal

在不改变现有业务行为的前提下，审计并重组 build 相对 main 的私有能力：功能逻辑优先放入按领域命名的独立文件或前端模块，main 共享入口只保留稳定、薄的调用点，从而降低后续 `main -> build` 同步时的内容冲突和语义回退风险。

## Background

- 当前父任务为 `07-16-merge-main-responses-lite-header-policy`，负责把最新 `origin/main` 合入 `build`。
- 本轮已同步到 `origin/main=bc2244c8`，版本 `0.1.158`；相对上次 main 基线新增 19 个提交、100 个文件。
- `git merge-tree` 预演发现 3 个硬冲突：
  - `backend/internal/service/openai_ws_forwarder_ingress_session_test.go`
  - `frontend/src/i18n/locales/en/admin/accounts.ts`
  - `frontend/src/i18n/locales/zh/admin/accounts.ts`
- `backend/cmd/server/wire_gen.go` 自动合并成功，不属于上述硬冲突；它必须在最终 ProviderSet 稳定后重新生成。
- Git 审计显示 build 有 136 个仅本分支可达的非 merge 提交；排除任务日志等 bookkeeping 后有 45 个代码或运维候选提交。
- 假想合并树相对最新 main 共 533 个差异文件：`.trellis` 269、`backend` 159、`.agents` 60、`frontend` 42、`.github` 2、`README_CN.md` 1。
- backend/frontend 共 201 个差异文件，其中修改 160 个、新增 41 个；生产或配置文件 111 个、测试文件 90 个。
- 当前仍独有的产品与协议能力覆盖 15 个领域：Codex 自定义客户端、Codex reset、生图设置、Codex 身份、Alpha Search、图片桥接、Responses Lite Header 策略、Anthropic 直连桥、provider-specific reasoning/GPT-5.6 元数据、Grok force-chat、Grok 独立 Billing、JSON Schema 降级、Web Search/web.run、AnySearch 和 Raw Chat 调试快照。
- build 另有 5 类分支资产：HA/DR、手动 GHCR 构建、fork CLA 策略、README_CN 定制和 Trellis/agents 工作流。它们必须纳入审计，但已天然隔离时不做无收益拆分。

## Requirements

### R1. Git 证据驱动的功能清单

- 以 main 共同基线、build 独有提交、假想合并树和文件历史为证据，建立 build 私有能力清单。
- 清单必须覆盖产品/协议能力、运维资产、文档和开发工作流，不得只统计 backend/frontend 的显眼功能。
- 每项能力必须归类为：当前仍独有、main 已替代、已经独立、需要抽离、中央契约无法独立或生成文件。
- main 已等价覆盖的算法、0.1.158 新增能力、纯维护/撤销链和机械生成差异必须明确排除，不能虚增 build 功能数量。
- 不得仅凭文件名或当前 diff 猜测归属。

### R2. 后端隔离原则

- build 私有业务逻辑优先放入按功能域命名的同 package 文件，例如现有 `openai_responses_lite_policy.go`；禁止创建含义模糊的 `build_helpers.go`。
- main 共享 handler/service/repository 方法只保留必要的数据准备、一次薄调用和错误传播，不继续内嵌大段 build 分支逻辑。
- Go 类型的方法可以迁移到同 package 的功能文件，但不得改变公开签名、依赖方向或序列化契约。
- Codex reset handler、Grok 独立 Billing handler 和 Raw Chat debug snapshot 必须从共享大文件迁入各自功能文件；route、构造器字段和 ProviderSet 只保留最小注册。
- AnySearch provider 主体保持独立，websearch manager/provider union 只保留不可避免的一条中央注册。
- 已经是单一功能域主体的协议适配文件不为追求文件数量而继续拆碎。

### R3. 前端隔离原则

- Create/Edit/Bulk 账号表单中的 build 私有 UI、状态转换和校验优先抽到功能组件或 composable，原页面只负责装配和提交编排。
- SettingsView 中 build 私有设置面板和规则编辑器优先抽到功能组件；中央表单只保留字段装配与 API 提交。
- Codex reset 与 Grok 独立 Billing 的 API/DTO 进入各自功能模块，中央 admin API 只做稳定 re-export；全局共享类型只保留不可避免的字段引用。
- ChannelsView 中 AnySearch 专属配置与说明按 provider 功能模块隔离，公共 provider union 和最终提交仍由中央层拥有。
- build 私有中英文文案进入独立 locale 扩展模块，main locale 文件只保留稳定的一次合并入口。
- 不为了隔离制造卡片嵌套、重复状态源或跨组件隐式副作用。

### R4. 测试隔离原则

- build 私有回归场景放入按功能命名的独立 `_test.go` / `.spec.ts` 文件。
- 不再把 build 场景追加到 main 已有的大型综合测试函数中；必要时复用同 package 测试工具，但不得改变测试语义。
- 结构迁移前后必须保持原断言覆盖，并补充薄接入点的回归断言。

### R5. 三个硬冲突的结构化解决

- WS 测试冲突：保留 main 对生图终态 `generating -> completed` 的原测试；把 build 的 Responses Lite `allow -> block -> allow` 多轮模型切换场景迁入独立测试文件。
- 英文和中文账号 locale 冲突：保留 main 对 hosted `image_generation` 与客户端 `image_gen` 的概念区分；不得照搬“仅非 Lite 注入”的描述，因为 build 的 Lite 标记传播由阻止模型列表决定。
- build 自定义 UA 等私有文案迁入 locale 扩展模块，避免继续与 main 的 Codex 图片文案占用同一修改区块。

### R6. 中央契约与生成文件

- DTO/SystemSettings 字段、路由注册、ProviderSet、API contract、Ent schema/migration 等中央契约允许保留最小必要改动。
- `UsageInfo` / `AccountUsageInfo`、共享 provider union、admin API re-export 和 GPT-5.6 capability/price JSON 属于中央契约或中央数据，只要求最小差异。
- `wire_gen.go`、Ent 等生成文件不得手工塑形；依赖图或 schema 稳定后使用仓库既有命令重新生成并核对 diff。
- 生成结果必须与源定义一致，不能以减少冲突为由绕过生成流程。

### R7. 行为兼容

- 隔离重构不得改变 build 现有功能、配置默认值、错误 reason、API 字段、模型策略或前端交互语义。
- GPT-5.6 `max` 主算法、main Grok `/quota` 和其它已被 main 覆盖的共享能力继续复用 main 实现，不恢复 build 的重复旧算法或双数据流。
- 合并 main 0.1.158 时必须同时保留 main 的 Grok 端点/WS、生图终态、Codex 模型发现、用户批量限额和分组复制能力。
- 自动合并但双方都修改的 27 个文件必须做语义复核，不能只依赖 Git 无冲突结论。

## Non-Goals

- 不把所有大文件机械拆成小文件。
- 不重写已经按单一功能域组织良好的协议适配实现。
- 不改变数据库结构、公开 API 或产品行为来迁就文件隔离。
- 不重写已独立的 HA/DR、CI、Trellis 或 README 资产；只记录归属并防止后续 main 合并覆盖。
- 不在本子任务中执行真实 `main -> build` merge；本子任务先独立完成隔离、检查和 `trellis-push`，父任务随后重新预演并执行合并。

## Acceptance Criteria

- [ ] build 私有能力清单覆盖 15 个产品/协议领域和 5 类分支资产，包含 Git/文件证据，并为每项给出“保持、抽离、main 替代或中央契约保留”的决定。
- [ ] 高冲突共享入口中的 build 大段逻辑已迁入功能文件，入口只保留薄调用与必要错误处理。
- [ ] Codex reset handler、Grok Billing handler、Raw Chat debug 和 AnySearch 接入按明确功能所有权隔离，route/constructor/manager 只保留薄注册。
- [ ] Create/Edit/Bulk、SettingsView、ChannelsView、功能 API/DTO 和 locale 的 build 私有部分按功能模块隔离，行为与提交 payload 不变。
- [ ] 三个预演硬冲突按 R5 解决，真实 merge 后 `git ls-files -u` 为空且无 conflict marker。
- [ ] 27 个双边修改文件完成语义复核，main 与 build 的能力均有对应测试证据。
- [ ] Wire/Ent 等生成文件由源定义重新生成，生成 diff 可解释且无手工残留。
- [ ] 后端定向与完整 unit、lint，前端定向 Vitest、完整测试、typecheck、lint、build 以及 `git diff --check` 全部通过；无法运行项有明确记录。
- [ ] 隔离提交完成后重新运行 `git merge-tree HEAD origin/main`；三个原始硬冲突应被消除或显著缩小，剩余冲突必须回到父任务记录和处理。
