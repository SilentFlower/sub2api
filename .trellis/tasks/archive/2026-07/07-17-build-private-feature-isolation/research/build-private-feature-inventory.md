# build 私有功能 Git 审计

## 1. 审计基线

- 当前 build：`c7335534`
- 最新 main：`bc2244c8`（0.1.158）
- 上次共同 main 基线：`b960ec19`
- 合并预演树：`853a87babb7e30987baef4b0dbf5ecd20a6b8baa`
- 仅 build 可达的非 merge 提交：136 个。
- 排除 task/journal/Trellis 常规 bookkeeping 后的代码或运维候选提交：45 个。
- 预演树相对最新 main 共 533 个差异文件：`.trellis` 269、`backend` 159、`.agents` 60、`frontend` 42、`.github` 2、`README_CN.md` 1。
- backend/frontend 共 201 个差异文件，其中修改 160 个、新增 41 个；生产或配置文件 111 个、测试文件 90 个。

主要取证命令：

```bash
git log --right-only --cherry-pick --no-merges origin/main...HEAD
git log --first-parent --merges b960ec19..HEAD
git diff --name-status origin/main 853a87babb7e30987baef4b0dbf5ecd20a6b8baa
git log --right-only --cherry-pick --no-merges --pretty=format: --name-only origin/main...HEAD
git merge-tree --write-tree HEAD origin/main
```

## 2. 归类规则

- **当前仍独有**：最新 main 没有等价行为，或 build 明确维持不同的产品边界。
- **main 已替代**：能力主体已由 main 覆盖，build 只剩经确认的兼容契约、元数据或调用时机，不能继续维护重复算法。
- **中央契约**：DTO、共享类型、路由、构造器、ProviderSet 等必须保留一个最小接入点，不能为了零 diff 复制定义。
- **生成文件**：Wire、Ent 等只由源定义生成，不把生成结果当作 build 功能主体。
- **分支资产**：CI、README、HA/DR、Trellis 等属于 build 分支定制，但已天然位于独立目录时不做机械拆分。
- **非功能差异**：lint 修复、测试构造参数修复、撤销链、注释/格式、合并适配和 task/journal 记录不单独计为产品能力。

## 3. 当前仍独有的产品与协议能力

| 功能域 | Git / 文件证据 | 当前边界 | 本任务决定 |
| --- | --- | --- | --- |
| Codex 自定义客户端放行 | `416943fe`；`internal/pkg/openai/custom_allowed_client.go`、`components/account/codexClientAllowlist.ts` | matcher/utility 已独立；`account.go`、client detector/gateway、Create/Edit/Bulk 和 accounts locale 仍有接入 | 保留独立主体；迁移 Account 方法、表单块、测试和文案，gateway 只调用检测结果 |
| Codex reset / 邀请重置 | `e1a089e4`、`c084180c`、`d53e8b6c`、`c9d52416`；独立 repository/service/modal | `account_handler.go`、route、Wire、`api/admin/accounts.ts`、AccountsView 仍承载接入 | handler 方法迁到 `account_handler_openai_codex_reset.go`；前端 API/DTO 迁到 Codex reset 功能模块；route、构造器和 modal 装配保持薄注册 |
| OpenAI OAuth 生图主模型与思考预算 | `524b9b7a`；setting、runtime cache、SettingsView、Codex transform | 设置字段是中央契约，缓存/归一化和 UI 面板仍散在共享文件 | 设置 cache/normalize 保持领域 owner；SettingsView 拆功能面板；DTO 和最终 payload 字段保留 |
| Codex 客户端身份版本 | `792c51ff`；`openai_codex_client_identity.go` | 独立文件已是版本、UA、probe identity 的单一来源 | 保持现状；usage/gateway/settings 消费者只调用，不恢复重复常量 |
| Codex Alpha Search 独立端点 | `d390f057`；独立 handler/service/test | route、endpoint 枚举和计费结果接入属于中央注册 | 保持功能主体；只保留最小 route/endpoint 注册，不拆服务实现 |
| Codex 图片桥接与工具选择边界 | `b5a51fd0`、`f59991b5`、`a02dca33`；image bridge/intent/transform | hosted `image_generation`、客户端 `image_gen` namespace、`tool_choice none -> auto` 和 Lite 生图桥接跨多个入口 | 保留现有领域主体；HTTP/WS/bridge 入口只准备模型/工具上下文并调用统一判断；专项测试独立 |
| Responses Lite Header 模型策略 | merge `d3988a03`；`openai_responses_lite_policy.go`、setting/API/UI | 按最终上游模型决定 Header/WS metadata 透传；Header Override 禁止用户伪造内部头；五条入口仍需统一调用 | 策略文件继续唯一拥有默认阻止列表、通配符、缓存和最终模型判断；设置面板、locale 和 Lite WS 测试独立，入口薄化 |
| Anthropic Messages ↔ Chat Completions 直连桥 | `e086ca5d`、`5dba3180`、`24316df9`、`014d69de`、`d6d3f1bf`、`8f070522` | 直连转换、缓存前缀、tool_result 顺序、确定性 tool id、thinking 互斥和 metadata 粘性已有协议主体 | 不机械拆碎协议状态机；只迁出 messages handler 中的 sticky/session 判断和薄接入 |
| Provider-specific reasoning 与 GPT-5.6 元数据 | `05918460`、`60d36274`；reasoning helper、usage、pricing JSON、前端格式化 | main 已拥有模型感知 `max` 主算法；build 仍保留 Grok 4.5/GLM 最终上游 effort 语义、GPT-5.6 `supports_max_reasoning_effort` 和少量价格/展示元数据 | 复用 main 算法，不恢复旧重复解析；只隔离 provider-specific 归一化/usage 投影，中央模型元数据保留最小数据差异 |
| Grok Messages 强制 Chat | `421df83b`；Grok chat bridge/fallback | messages/cc pipeline、实际上游 endpoint、Create/Edit extra 仍有判断 | 保留桥接主体；抽离账号 extra 编辑和路由决定，通用调度/计费/failover 留在共享入口 |
| Grok 独立套餐 Billing | `57e409da`、`cbd34d3a`、`a328495d`、`62661ccb`；`grok_billing_quota_service.go`、`pkg/xai/billingquota`、`GrokBillingQuotaCell.vue`、队列 | 独立 `/billing-quota` 是账号列表套餐额度队列的唯一远端来源；`AccountUsageService` 只读快照，不能跨入 main `ProbeBilling` | handler 方法迁到 `grok_oauth_handler_billing_quota.go`；前端 API/DTO 迁到 Billing 功能模块；AccountUsage 仅投影既有快照，不保留主 quota service 构造依赖 |
| JSON Schema 降级 | `831eaa4a`；`pkg/apicompat/json_schema_downgrade.go`、`service/openai_json_schema_downgrade.go` | 协议转换主体已独立；account method、gateway forward/fallback 和表单开关仍有接入 | 保持降级主体；迁出 Account 方法、表单状态和入口判断，不拆 schema walker |
| Web Search / `web.run` 混合工具循环 | `831eaa4a`、`d7e97a71`、`2f460f5e`、`a0e2aaaf`；responses websearch/web.run 文件 | 搜索循环和事件转换已独立；gateway fallback、settings 和账号表单仍装配 | 保持循环主体；共享入口只传 executor/config 并消费结果，前端设置/账号块按功能拆分 |
| AnySearch provider | `831eaa4a`；`pkg/websearch/anysearch.go`、manager/provider/types、ChannelsView | provider 实现已独立，provider 枚举、manager 注册、设置类型和渠道 UI 是中央接入 | 保持独立 provider；manager 只做一条注册；AnySearch 专属 UI/说明迁入 provider 功能组件或 locale 模块，公共 provider union 保留 |
| Raw Chat 调试快照 | `89e05bdc`；`gateway_service.go`、`openai_gateway_service.go`、messages fallback | 环境变量、文件生命周期、headers/body snapshot 与调用点仍分散在共享 gateway 文件 | 新建明确命名的 debug snapshot 文件持有解析、文件和写入逻辑；两个 gateway 只保留字段、初始化和一次记录调用 |

## 4. build 分支级运维与工作流资产

| 资产域 | 证据 | 归属决定 |
| --- | --- | --- |
| 双节点 DR/HA 与自动故障切换 | `d5fef3ec`、`45561e0d`、`771765f9`、`0f1c4a6b`、`1fdfe9dc`；归档任务 artifacts、自动化、Worker、systemd 和 `disaster-recovery-guidelines.md` | 已位于独立 Trellis artifacts/spec，不进入产品代码重构；清单和验证边界必须保留 |
| 手动 GHCR 镜像构建 | `4e75bfce`；`.github/workflows/my-ci.yml` | 独立 workflow，保留 build 分支或 `[build]` 提交触发与 GHCR tags；不塞回 main CI |
| fork CLA 策略 | `4e75bfce`；相对 main 删除 `.github/workflows/cla.yml` | 这是 fork 仓库策略，不是遗漏文件；后续合并不得无意恢复上游专用 CLA workflow |
| README_CN 定制说明 | `README_CN.md` 的 Claude Code Plan Mode 已知问题 | 保持独立文档增量；不与产品运行时代码混算，但合并时需语义保留 |
| Trellis / agents / Flower 工作流 | `.agents` 60 个差异文件、`.trellis` 269 个差异文件，含 `59d4c7e7`、`c3e7a6a0` 等 | 属于 build 开发工作流资产，已按目录隔离；不为减少统计而删除，也不纳入产品代码拆分 |

## 5. main 已覆盖或不属于 build 私有能力的差异

- GPT-5.6 `max` 的模型感知归一化、候选模型提取、compact 例外和官方分档价格主体已由 main 覆盖；build 不保留第二套算法。
- main 的 Grok 统一 `/quota`、SSO/reconcile、免费 24 小时估算和手动探测继续存在；build 私有的是与其隔离的账号列表 `/billing-quota` 链路。
- Beta Fast/Flex、xAI 通用请求 ID、Codex 模型发现等若最新 main 已有等价实现，只作为共享能力使用，不列为当前 build 私有功能。
- main 0.1.158 的 Grok 端点/WS、生图终态、Codex 模型发现、用户批量限额和分组复制能力属于本次父任务需要吸收的上游增量，不进入 build 功能清单。
- `wire_gen.go`、Ent 生成代码、API contract 快照、构造参数适配和注释/格式变化不是独立功能；它们按源定义或双方契约处理。
- `53fea533`、`91348728`、`74d2b819` 等 lint/测试基础设施修复，以及 `62091bfa`/`a9ad55b3` 等撤销链，不单独形成产品功能域。

## 6. 高冲突热点

历史修改频次最高的共享文件：

- `frontend/src/i18n/locales/{en,zh}/admin/accounts.ts`：各被 8 个 build 独有提交修改。
- `backend/internal/service/openai_gateway_service.go`：7 个 build 独有提交。
- `backend/internal/service/openai_gateway_responses_chat_fallback.go`：6 个 build 独有提交。
- `frontend/src/components/account/{CreateAccountModal,EditAccountModal}.vue`：各 5 个 build 独有提交。
- `backend/internal/service/openai_codex_transform.go`：5 个 build 独有提交。
- `frontend/src/views/admin/SettingsView.vue`：build 净新增 209 行，继续承载多个设置域。

本轮 main 0.1.158 与 build 同时修改 27 个文件，其中风险最高的是：

- OpenAI WS ingress、passthrough、HTTP bridge 及 session tests。
- Grok OAuth handler、账号 Create/Edit/Bulk 表单和 credentials builder。
- accounts/channels 中英文 locale。
- Wire、API contract 和共享类型。

## 7. 必须抽离的边界

### 7.1 后端

- 将 `account.go` 中只属于 custom client、JSON Schema/Web Search 等 build 能力的方法迁入同 package 的领域文件。
- 将 Codex reset 的 handler 方法迁出 `account_handler.go`；route 和 `AccountHandler` 依赖字段只保留最小注册。
- 将 Grok 独立 Billing 的 handler 方法迁出 `grok_oauth_handler.go`；构造器参数保留，独立 Billing 不与 main `/quota` 合并。
- 将 Raw Chat debug 的环境解析、文件生命周期和 snapshot 写入迁入明确的 debug 领域文件，gateway 结构体只持必要字段。
- 将 HTTP/WS 入口中的 Responses Lite、reasoning effort、JSON Schema/Web Search 等 build 编排压缩为领域 helper 调用。
- AnySearch 保持 provider 独立文件；manager/provider union 只保留不可避免的一条中央注册。
- 将 build 专属日志、投影或判断从 `openai_gateway_service.go`、AccountUsage 等共享主体迁出；结构体字段和构造器参数仍留在中央定义。
- 不拆分已经是单一协议主体的长文件，只处理它们在其它共享入口中的接入。

### 7.2 前端

- Create/Edit/Bulk 只抽离 build 私有的自定义 UA、兼容模式、Grok 强制 Chat、Web Search/JSON Schema 等块；不重构 main 自有 Header Override 或其它无关 UI。
- Codex reset 的 API、DTO 和调用函数迁入独立功能模块；`accounts.ts` 或 admin API index 只做稳定 re-export，AccountsView 只挂 modal 与事件。
- Grok 独立 Billing 的 API、DTO、队列和组件形成完整功能模块；全局 `AccountUsageInfo` 只保留可选字段引用，AccountUsageCell 只装配组件和合并更新。
- SettingsView 抽离生图设置、Responses Lite 模型列表和 AnySearch/Web Search 等 build 私有面板；中央 form/API payload 仍维持单一数据源。
- locale 按 custom client、Codex reset、Grok Billing、Responses Lite、Web Search/AnySearch 等功能域导出消息片段，由 `accounts.ts` / `settings.ts` / `channels.ts` 在所属嵌套对象中做一次稳定展开。
- build 测试从 main 大型 spec 中迁出，但复用现有组件和 API mock 工具。

## 8. 中央契约保留项

以下文件无法通过独立文件完全消除改动，只要求最小、可生成或可审计：

- DTO、SystemSettings、`UsageInfo` / `AccountUsageInfo` 和共享 TypeScript provider union。
- route 注册、handler/service 构造器、ProviderSet 和 admin API re-export。
- API contract 快照。
- Ent schema、migration 与生成代码。
- GPT-5.6 capability/price JSON 等中央模型元数据。
- `wire_gen.go`。

## 9. 三个预演硬冲突

1. `backend/internal/service/openai_ws_forwarder_ingress_session_test.go`
   - main：首轮生图事件终态归一化。
   - build：Lite 模型 allow/block/allow 三轮切换。
   - 处理：main 测试留在原文件，build 场景迁入独立 Lite WS session test。
2. `frontend/src/i18n/locales/en/admin/accounts.ts`
   - build 自定义 UA 文案与 main Codex 图片桥接文案相邻修改。
   - 处理：build 文案迁入功能 locale 模块，图片桥接文案按实际 Lite 模型策略修正。
3. `frontend/src/i18n/locales/zh/admin/accounts.ts`
   - 与英文冲突同构，保持中英文 key 完全一致。

`backend/cmd/server/wire_gen.go` 不是硬冲突；它在依赖图稳定后重新生成。

## 10. 2026-07-17 隔离实施复核

### 10.1 产品与协议能力处置结果

| 功能域 | 最终处置 | 薄接入或领域 owner |
| --- | --- | --- |
| Codex 自定义客户端放行 | 已抽离 | 后端账号读取位于 `account_codex_cli_only_allowed_clients.go`，拒绝响应位于 `openai_client_restriction_response.go`；前端字段、extra 转换和测试位于 `features/codexCustomClients/`，Create/Edit/Bulk 只装配组件和 helper。 |
| Codex reset / 邀请重置 | 已抽离 | handler 位于 `account_handler_openai_codex_reset.go`；前端 API、DTO、modal 位于 `features/openAICodexReset/`；route、构造器字段和 admin API re-export 保持中央最小接入。 |
| OpenAI OAuth 生图主模型与思考预算 | 已抽离 | 后端缓存、默认值和归一化位于 `setting_openai_image_generation.go`；前端面板位于 `features/openAIImageGeneration/OpenAIImageGenerationSettings.vue`；Settings DTO、SettingService 字段和最终保存 payload 保持中央契约。 |
| Codex 客户端身份版本 | 保持现状 | `openai_codex_client_identity.go` 已是唯一版本与 UA 来源；共享入口只消费派生常量，不创建 build wrapper。 |
| Codex Alpha Search | 保持现状 | 独立 handler/service/test 已是功能主体；endpoint、route、usage 计费字段只保留中央注册。 |
| Codex 图片桥接与工具选择 | 部分薄化、main owner 保留 | HTTP/WS/Lite 入口调用现有桥接与策略 helper；Channels 接入位于 `features/openAIImageGeneration/`。EditAccountModal 的图片工具主体也存在于 main 0.1.158，因此不误迁为 build 私有 UI。 |
| Responses Lite Header 模型策略 | 已抽离 | 后端统一位于 `openai_responses_lite_policy.go`，HTTP/WS/bridge 只传最终模型和入站信号；前端规则与面板位于 `features/responsesLite/`；专项 WS 三轮测试位于 `openai_responses_lite_ws_ingress_session_test.go`。 |
| Anthropic Messages ↔ Chat Completions 直连桥 | 保持协议主体、入口薄化 | 协议状态机继续由 apicompat/fallback 文件拥有；handler 会话粘性位于 `openai_gateway_messages_session.go`，共享 Messages 入口只调用一次。 |
| Provider-specific reasoning 与 GPT-5.6 元数据 | 已抽离运行时策略 | Grok/GLM 归一化和最终发送值投影位于 `openai_provider_reasoning_effort.go`；各协议入口只在最终 body 上调用；GPT-5.6 capability/price 继续作为中央模型元数据。 |
| Grok Messages 强制 Chat | 已抽离 | Grok 判断位于 `openai_gateway_grok_force_chat.go`，通用路由入口位于 `openai_gateway_messages_route.go`；前端读取、支持条件和 extra 写入位于 `features/grokForceChat/`；service/handler 专项测试已迁到功能命名文件。 |
| Grok 独立套餐 Billing | 已抽离 | handler、service、usage 投影分别位于 `grok_oauth_handler_billing_quota.go`、`grok_billing_quota_service.go`、`grok_billing_quota_usage.go`；前端 API、DTO、队列、组件、套餐优先级和 Free/付费判断位于 `features/grokBillingQuota/`。`AccountUsageService` 不注入 `GrokQuotaService`，main `/quota` 与独立 `/billing-quota` 保持双链路。 |
| JSON Schema 降级 | 已抽离接入 | Account 方法位于 `account_openai_json_schema.go`，协议 walker 保持在 apicompat/service 专项文件；前端字段和 extra 读写位于 `features/openAICompatibility/`。 |
| Web Search / `web.run` | 保持协议主体、前端薄化 | websearch/web.run 循环继续由独立 service 文件拥有；账号 UI、渠道 feature config 和类型位于 `features/webSearch/`，共享页面只装配和提交。 |
| AnySearch provider | 已独立 | `pkg/websearch/anysearch.go` 持有请求和解析；manager/provider union 只保留注册；前端 provider 规则、可选 API Key 和提示位于 `features/webSearch/anySearch.ts` 与组件。 |
| Raw Chat 调试快照 | 已抽离 | 环境解析、文件生命周期和快照写入位于 `gateway_debug_snapshot.go`；两个 gateway service 只保留文件指针、初始化和一次写入调用。 |

### 10.2 分支资产处置结果

| 资产域 | 最终处置 |
| --- | --- |
| 双节点 DR/HA 与自动故障切换 | 已天然隔离在独立自动化、归档任务和 `disaster-recovery-guidelines.md`；本任务不改运行内容。 |
| 手动 GHCR 镜像构建 | 保留 `.github/workflows/my-ci.yml`，不并入 main CI。 |
| fork CLA 策略 | 保持相对 main 删除 `.github/workflows/cla.yml`，父任务不得误恢复。 |
| README_CN 定制说明 | 保留 Claude Code Plan Mode 已知问题说明，父任务做语义合并。 |
| Trellis / agents / Flower 工作流 | 保持 `.agents`、`.trellis` 和 manifest 的独立目录所有权，不纳入产品代码拆分。 |

### 10.3 测试、中央契约与生成文件

- Responses Lite、Grok force-chat、Grok Billing handler/usage、SettingsView 生图、Responses Lite、AnySearch 等 build 场景已迁入功能命名测试文件；共享综合测试只保留 main 基线或必要的最终 payload 集成断言。
- `UsageInfo` / `AccountUsageInfo`、SystemSettings/DTO、route、构造器字段、ProviderSet、API contract、共享 provider union 和 admin API re-export 继续作为中央契约保留最小差异。
- 本子任务没有修改 Ent schema 或 migration；由于移除了 `AccountUsageService` 的 `GrokQuotaService` 构造依赖，已执行 `go generate ./cmd/server` 重新生成 `wire_gen.go`。父任务真实合并 main 后仍需按最终依赖图再次复核 Wire。
- EditAccountModal 的 Codex 图片工具主体属于 main 0.1.158 共同 owner，本任务只保留 build 策略接入，不为了文件形式迁走 main 行为。
