# Design: 隔离 build 私有功能以降低上游合并冲突

## 1. 设计目标

本任务在 `main 0.1.158 -> build` 真实合并之前，先重组 build 私有能力的接入形态。目标不是追求更多文件，而是让上游共享文件只承担稳定编排：业务判断、归一化、投影、文案和回归测试由功能域文件拥有。

完成后父任务重新执行 merge 预演。原先由 build 在上游热点内追加内容造成的冲突，应通过“迁出 build 内容”而自然消失；真实 merge 和 main 新行为集成仍由父任务负责。

## 2. 设计原则

### 2.1 功能所有权优先

一段代码满足任一条件时，应迁入功能域文件：

- 只服务于 build 独有配置、路由或展示。
- 由 build 独有提交引入，main 共享方法只负责调用。
- 在两个以上入口重复执行同一判断、归一化或 payload 更新。
- 回归测试只验证 build 能力，却被追加到 main 大型综合测试中。

以下情况不因本任务拆分：

- 文件本身已经是单一协议或业务主体。
- 仅有一个不可避免的中央字段、接口方法、路由注册或 ProviderSet 条目。
- 抽象后需要更多隐式状态、回调或重复类型才能工作。

### 2.2 薄接入点定义

共享入口可以保留：

1. 读取当前入口已经拥有的上下文或模型。
2. 调用一个功能域 helper/service/component。
3. 传播返回值、错误或明确的 handled 结果。

共享入口不得继续拥有：

- build 专属规则表、通配符判断或配置默认值。
- build 专属 JSON 字段的多处分支更新。
- build 专属日志格式和结果投影。
- 为单一 build 回归场景准备的大段测试数据和断言。

薄不是固定行数指标；判断标准是共享入口不再定义该功能的业务语义。

### 2.3 审计分类先于拆分

当前差异同时包含 15 个仍独有的产品/协议领域、5 类分支资产、main 已替代的旧实现和机械生成差异。实施前必须先按 `research/build-private-feature-inventory.md` 确认所有权：

- 当前仍独有且仍嵌在共享热点中的逻辑进入本轮隔离。
- 已经位于独立文件/目录的能力只薄化接入，不为形式一致继续拆碎。
- main 已覆盖的算法直接复用 main owner，不创建 build wrapper 保存提交痕迹。
- HA/DR、CI、README 和 Trellis 资产只做归属保护，不搬入产品包。
- DTO、route、构造器、共享类型、模型元数据和生成文件按中央契约处理。

## 3. 执行顺序

```text
当前 build（已含 main 0.1.157）
  -> 完整功能/资产清单与边界确认
  -> 仅重组 build 私有代码/测试/i18n
  -> 全量检查
  -> 子任务独立 commit + push
  -> 父任务重新 fetch / merge-tree
  -> 父任务合并 main 0.1.158
```

本子任务不启动真实 merge。这样每次回滚都能以一个普通 build 提交为边界，避免未提交重构与 merge index 混在一起。

## 4. 后端边界

### 4.1 账号策略方法

`Account` 类型仍定义在原中央模型文件，但仅属于 Codex 自定义客户端、JSON Schema/Web Search 或其它 build 能力的方法可迁到同 package 的领域文件。迁移只改变文件归属，不改变 receiver、签名、JSON 字段或调用方。

优先处理 Git 审计确认的 `account.go` 私有方法区块；已有 main 通用方法保持原位。

### 4.2 Codex reset 与 Grok Billing handler

Codex reset 的 repository/service/modal 已经独立，但三个管理端 handler 方法仍在通用 `account_handler.go`。这些方法迁入同 package 的 `account_handler_openai_codex_reset.go`；`AccountHandler` 的依赖字段、构造参数和 route 只保留注册，不复制 service 或 DTO。

Grok 独立套餐 Billing 同理：`QueryBillingQuota` 及其专属绑定/返回逻辑迁入 `grok_oauth_handler_billing_quota.go`，`GrokOAuthHandler` 只保留独立 service 字段和构造注入。该链路不得与 main `/quota`、reconciler 或 Responses probe 合并。

### 4.3 Gateway 与 WebSocket

Responses Lite 已由 `openai_responses_lite_policy.go` 持有决策，生图桥接已有独立主体。四条入口应只保留：

- 提供最终模型和入站 Lite 信号。
- 调用统一策略。
- 传播 body/header/metadata 结果和错误。

同样方式审查 reasoning effort、JSON Schema/Web Search、Grok force-chat 等 build 接入。已有领域 helper 时直接复用；没有且共享入口仍定义规则时，迁入按协议域命名的文件。不得创建统一的 `build` 总开关或万能 helper。

### 4.4 Raw Chat 调试快照

Raw Chat debug 的环境变量解析、文件打开/生命周期、快照格式和写入逻辑由明确命名的 debug snapshot 文件拥有。`GatewayService` 与 `OpenAIGatewayService` 只保留必要的文件引用、构造时初始化和转发前后一次记录调用。

该重组必须保持现有环境变量、默认文件名、header/body/extra 格式和关闭状态下零开销语义，不把调试能力塞进通用 logger helper。

### 4.5 AccountUsage 与 Grok Billing

独立 Grok Billing service、parser 和前端队列保持不动。`AccountUsageService` 只读取既有 `grok_usage_snapshot`、main quota/Billing 快照和独立套餐快照并完成投影，不注入 `GrokQuotaService`，也不主动调用 `ProbeBilling` 或 `/billing-quota`。

handler route、`GrokOAuthHandler` 构造器参数和 ProviderSet 属于中央装配，保留最小注册；`AccountUsageService` 构造器不保留跨链路依赖。独立 Billing 的前端/后端 DTO 与 main `GrokBillingSummary` 继续保持不同所有权，任一链路失败不得覆盖另一链路状态。

### 4.6 AnySearch provider

`pkg/websearch/anysearch.go` 继续拥有 MCP JSON-RPC 请求、鉴权和响应解析；manager/provider union 只保留 provider 枚举和一条构造注册。AnySearch 专属默认 endpoint、可选 API Key 说明和 UI 条件不进入通用 provider 分支表。

ChannelsView 中若仍有 AnySearch 专属输入/帮助文本，则迁入 provider 功能组件；父页面保留 provider 列表、最终 config 对象和保存动作。

### 4.7 Settings 与模型元数据

Go struct 字段和 handler DTO 无法跨文件扩展，继续留在中央定义。以下逻辑应按功能域迁出：

- build 设置的默认值构造。
- parse/normalize/validate。
- cache refresh 或保存后的功能专用刷新。
- 复杂 audit projection。

中央 Get/Update 流程只调用这些 helper，并保持现有 snake_case API 契约。

Responses Lite 阻止模型列表、生图主模型/思考预算和 Web Search provider 的功能规则分别由各自领域 owner 管理；不得在 Settings handler、service、SettingsView 三处重复 trim、默认值和通配符判断。

GPT-5.6 `supports_max_reasoning_effort` 与价格 JSON 是中央模型元数据，保留最小数据差异。模型感知 `max` 主算法已由 main 覆盖，本任务不再创建 build 版本。

### 4.8 协议主体例外

Anthropic Chat bridge、Responses Web Search/web.run、Codex reset、Alpha Search 等文件已经是功能主体。即使文件较长，也不为减少行数而拆成无所有权的小片段；只调整它们在共享入口的接入。

## 5. 前端边界

### 5.1 账号 Create/Edit/Bulk

只抽离 Git 证明为 build 私有的表单能力，不重构 main 自有 Header Override、Grok Base URL Presets 或其它无关 UI。

抽离单位按功能语义选择：

- 可复用纯转换继续放在现有 feature utility。
- Create/Edit 共享的单字段或小型表单块使用 typed props/emits 组件。
- Bulk 与单账号语义不同的场景使用独立 bulk 组件，不用大量 mode 分支强行复用。
- 多处共同维护同一 extra 字段时，使用一个 typed helper/composable 负责从现有 extra 复制后逐键更新。

父弹窗继续拥有最终提交动作和完整 payload，子组件不得直接调用 API。

### 5.2 功能 API 与共享类型

Codex reset 的 API 函数和 DTO 迁入独立功能模块，admin API index 或 `accounts.ts` 只做稳定 re-export。Grok Billing 的 API、DTO、请求队列和组件形成同一功能域模块，不继续把独立 Billing 细节堆入通用 `grok.ts` 和全局类型文件。

`AccountUsageInfo.grok_billing_quota`、公共 Account 类型和 provider union 是跨页面契约，可以保留一个最小引用。功能模块不得再声明第二份同名结构，也不得把 snake_case 转成组件私有 camelCase payload。

### 5.3 SettingsView 与 ChannelsView

SettingsView 继续拥有系统设置 form 和保存请求。build 私有的 OpenAI/Codex 面板拆成领域组件，使用 typed props 和 `update:*` 事件传递值；列表校验可由功能 utility 拥有。

不得在子组件复制一份完整 settings form，也不得让子组件直接持久化。

ChannelsView 继续拥有 Web Search config 与保存动作；AnySearch 专属字段和说明进入 provider 组件，公共 provider 选择与配置数组保持单一状态源。

### 5.4 I18n

build 私有消息按功能域放入 `{en,zh}/admin/` 下的扩展模块。由于 admin index 只做顶层浅展开，嵌套的 `accounts`/`settings` 消息必须由所属主模块在正确对象层级展开，不能在 index 中覆盖整个业务域。

中英文扩展导出结构必须同构，并由 locale 编译/冲突测试验证。

扩展模块至少按 custom client、Codex reset、Grok Billing、Responses Lite、Web Search/AnySearch 分域，避免所有 build 文案重新聚合成一个新的大文件。

## 6. 测试结构

- build 专属测试使用功能域文件名，测试函数不再插入 main 综合场景。
- 同 package Go 测试可复用现有 capture/stub，但不依赖测试执行顺序。
- Vue 组件迁移后，原父组件测试保留 payload 集成断言；新增子组件测试覆盖局部交互和 emits。
- 结构迁移前后的测试名称、输入和关键断言建立映射，避免“移动测试”时缩减覆盖。

### 6.1 当前 WS 冲突

当前 `KeepLeaseAcrossTurns` 恢复为共同基线的通用 lease 测试；Lite `gpt-5.6-terra -> gpt-5.5 -> gpt-5.6-terra` 场景迁入独立 Lite WS session test。这样 main 0.1.158 对原测试增加生图终态事件时可以直接应用。

### 6.2 当前 locale 冲突

自定义 UA 文案迁出 `accounts.ts` 的 Codex 图片文案邻接区。main 的图片桥接文案后续由父任务合并，并按 build 的模型阻止策略修正事实描述。

## 7. 分支级资产

- HA/DR、自动故障切换、镜像同步和 Worker/systemd artifacts 已位于独立任务与规范目录，本轮不改其运行内容，只保证审计和后续 merge 不遗漏。
- `.github/workflows/my-ci.yml` 保持独立手动 GHCR 构建；删除上游 `cla.yml` 是 fork 策略，不得被误判为缺失文件后恢复。
- `README_CN.md` 的 Claude Code Plan Mode 已知问题属于 build 文档增量，后续合并只做语义保留。
- `.agents`、`.trellis` 和 Flower manifest 是开发工作流资产，不纳入产品 bundle 拆分，也不因差异文件多而删除。

## 8. 中央契约与生成代码

允许保留修改但必须最小化：

- service/handler/repository interface 与构造器。
- DTO/SystemSettings 和前端 API 共享类型。
- routes、ProviderSet、API contract。
- Ent schema/migration 与生成结果。
- `UsageInfo` / `AccountUsageInfo`、共享 provider union、admin API re-export。
- GPT-5.6 capability/price JSON 等中央模型元数据。

`wire_gen.go` 在本子任务只在 ProviderSet 或构造器因隔离发生变化时重新生成；父任务完成 main 合并后还需再次按最终依赖图生成。

## 9. 兼容性不变量

- 所有 API 字段、默认值、错误 reason 和状态码不变。
- 账号 `extra` 和 `credentials` 必须从既有对象复制后只修改本功能键。
- Responses Lite 最终模型策略、Grok 独立 Billing、Codex reset、Web Search、reasoning effort 和自定义 UA 行为不变。
- Grok 独立 `/billing-quota` 与 main `/quota` 继续双链路隔离；Codex reset build API 与 main reset-quota API 继续独立。
- main 已覆盖的 GPT-5.6 模型感知规则不因隔离恢复旧 build 算法。
- 前端用户可见文案 key 不变；允许改变消息模块来源，不允许组件改用新路径。
- 隔离不得改变调度、缓存、计费或 failover 时机。

## 10. 风险与控制

| 风险 | 控制 |
| --- | --- |
| Vue 抽组件后 v-model 或 reset 时机变化 | 父组件保留最终状态源；子组件只发 typed update；保留父组件 payload 测试 |
| Go 方法迁移遗漏同名调用或测试 | 迁移后 `rg` receiver/函数名，运行 package 全测与 lint |
| 为薄化入口创造万能 helper | 每个 helper 必须有明确协议/业务所有者，不接受 branch 命名聚合 |
| i18n 浅合并覆盖业务域 | 只在 accounts/settings 内部展开；运行 locale compile/no-collision 测试 |
| 测试移动后覆盖缩水 | 建立旧测试到新测试映射，比较断言和定向测试数量 |
| API/type 拆分形成循环依赖或双定义 | 功能模块拥有 DTO；中央类型只 import type/引用一次；admin API 只 re-export |
| main 已覆盖算法被误判为 build 私有 | 逐项对照最新 main 和历史 merge 决策，只保留当前树仍独有的契约 |
| CI/CLA/Trellis 资产被产品重构误改 | 仅记录归属并在父任务 merge 复核，不搬动已独立目录 |
| 隔离提交后 main 再次更新 | 父任务重新 fetch 和 merge-tree，以新的 origin/main 为准 |

## 11. 回滚

- 本子任务不进入 merge 状态，失败时只涉及普通工作区改动。
- 通过 `trellis-push` 形成独立提交后，如需撤销使用普通 `git revert <isolation-commit>`；不改写 build 历史。
- 父任务只有在隔离提交成功并重新预演后才开始真实 merge。
