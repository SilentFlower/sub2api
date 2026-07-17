# main 0.1.158 双边修改文件语义复核

## 1. 复核基线

- merge-base：`b960ec19`
- build HEAD：`c7335534`
- origin/main：`bc2244c8`
- 最终临时预演提交：`72c4a2e2aa0fe50b3a1ff3804e7d4b61e0f187be`
- 最终合并树：`22f5394911aea175c0ee45ef0d37bccd63aadeb3`，`MERGE_STATUS=0`
- 取集方式：分别计算 `merge-base..HEAD` 与 `merge-base..origin/main` 的修改文件并取交集，共 27 个文件。
- 复核原则：不能以 Git 自动合并成功代替语义检查；每个文件都要同时保留 main 0.1.158 增量和 build 私有能力，build 专项逻辑优先迁出共享热点。
- 本子任务不执行真实 merge；最终结论以包含当前工作区隔离改动的临时索引 merge-tree 为准。

## 2. 后端文件

| 文件 | main 改动 | build 改动 | 最终语义与处理 | 测试证据 | Wire 处理 |
| --- | --- | --- | --- | --- | --- |
| `backend/cmd/server/wire_gen.go` | 吸收重复分组初始化安全性和最新 Grok/管理端依赖图。 | Codex reset、独立 Grok Billing、Responses Lite 设置链路等 build 依赖。 | 以源 ProviderSet/构造器为准重新生成；`AccountUsageService` 不再注入 `GrokQuotaService`，但 handler 的 main `/quota` 与 build `/billing-quota` 依赖均保留。 | `go generate ./cmd/server`；`go test -tags=unit ./cmd/server ./internal/handler/...`。 | 已重新生成，禁止手工塑形。 |
| `backend/internal/handler/admin/grok_oauth_handler.go` | Grok 上游端点手动切换、快捷端点和 SSO 自定义地址保留。 | 独立套餐 Billing handler 原先内嵌在共享文件。 | main Grok OAuth/端点逻辑留在原文件；build `QueryBillingQuota` 迁到 `grok_oauth_handler_billing_quota.go`，构造器只保留注入。 | `grok_oauth_handler_test.go`、`grok_oauth_handler_billing_quota_test.go`。 | 不涉及生成代码；构造器变化由 server Wire 覆盖。 |
| `backend/internal/handler/admin/grok_oauth_handler_test.go` | 覆盖 main Grok endpoint、SSO、quota/reset 行为。 | 独立 Billing 绑定、脱敏和响应断言。 | 原文件保留 main 综合回归；build Billing 场景迁到独立测试文件，避免后续相邻冲突。 | 两个 Grok handler 测试文件定向执行。 | 不涉及 Wire。 |
| `backend/internal/repository/wire.go` | 注册 main 新增的分组/管理端 repository provider。 | 注册 build Codex reset、Grok Billing 等 repository provider。 | ProviderSet 同时保留双方 provider，不创建重复 constructor。 | `go test -tags=unit ./internal/repository/...`，并由 server Wire 生成成功证明依赖可解析。 | 源 ProviderSet 保留；生成结果体现在 `cmd/server/wire_gen.go`。 |
| `backend/internal/server/api_contract_test.go` | 增加用户批量限额等 main API 契约。 | 增加 Codex reset、Grok Billing 和 build 设置字段契约。 | 合并后的 contract 同时暴露双方 endpoint/字段；不重命名 snake_case。 | `go test -tags=unit ./internal/server -run Test.*Contract`。 | 不涉及 Wire。 |
| `backend/internal/server/routes/admin.go` | 注册 main 用户批量限额和 Grok 新端点。 | 注册 Codex reset 与独立 `/billing-quota`。 | route 同时保留；main `/quota` 与 build `/billing-quota` 不复用 handler 方法或 DTO。 | API contract、Codex reset handler、Grok Billing handler 测试。 | 不涉及 Wire；handler 实例由 Wire 提供。 |
| `backend/internal/service/account.go` | 保留 Grok base URL/endpoint 规则及 main 账号行为。 | Codex 自定义客户端、JSON Schema、Web Search 等 build 方法。 | main 通用账号主体留在原文件；build 方法迁到 `account_codex_cli_only_allowed_clients.go`、`account_openai_json_schema.go`、`account_websearch.go`。 | `account_base_url_test.go`、功能文件对应 service/前端 extra 测试、完整 service unit。 | 不涉及 Wire。 |
| `backend/internal/service/openai_codex_models_service_test.go` | 覆盖无效模型清单不得破坏 Codex 能力发现。 | 保留 build Codex 身份和兼容模型相关断言。 | 双方断言并存，模型发现继续复用 main 实现，不恢复 build 旧算法。 | 该测试文件完整执行。 | 不涉及 Wire。 |
| `backend/internal/service/openai_gateway_grok_test.go` | 保留 main Grok OAuth 路由、endpoint 与 WS 修复。 | Grok Messages 强制 Chat 回归原先混入共享测试。 | main Grok gateway 测试留在原文件；build 强制 Chat 路由/转发迁到 `openai_gateway_grok_force_chat_test.go` 和 endpoint 专项测试。 | `openai_gateway_grok_test.go`、`openai_gateway_grok_force_chat_test.go`、`openai_grok_force_chat_endpoint_test.go`。 | 不涉及 Wire。 |
| `backend/internal/service/openai_image_generation_controls_test.go` | 保留 WS 生图终态 `generating -> completed` 修复覆盖。 | 保留 build Codex 生图 bridge、tool policy 和 Lite 交互覆盖。 | 两类行为同时存在；build Lite 多轮状态不再改写 main 通用 session 测试。 | 该文件、`openai_responses_lite_ws_ingress_session_test.go`、图片 intent/bridge 测试。 | 不涉及 Wire。 |
| `backend/internal/service/openai_ws_forwarder_ingress.go` | 保留 main WS 生图终态事件处理。 | 按最终模型应用 Responses Lite metadata/body 策略。 | 入口只准备最终模型并调用 Lite owner；终态处理继续由 main 流程负责。 | ingress、session、Lite WS 专项测试。 | 不涉及 Wire。 |
| `backend/internal/service/openai_ws_forwarder_ingress_session_test.go` | main 增加生图最终完成事件与通用 lease/session 断言。 | build 原先加入 Lite allow/block/allow 三轮切换。 | 原文件恢复 main 通用两轮语义；build 三轮模型策略迁到 `openai_responses_lite_ws_ingress_session_test.go`。 | 两个 session 测试文件分别执行。 | 不涉及 Wire。 |
| `backend/internal/service/openai_ws_http_bridge.go` | 保留 main `UpstreamTerminalEvent` 和调度成功语义。 | 保留 provider reasoning 最终值和 Responses Lite Header 重建策略。 | bridge 只调用统一 reasoning/Lite helper；Header 是否重建由最终模型阻止列表决定。 | WS HTTP bridge、Responses Lite tools、provider reasoning 测试。 | 不涉及 Wire。 |
| `backend/internal/service/openai_ws_v2_passthrough_adapter.go` | 保留 main WS v2 passthrough 生图终态处理。 | 保留 build Responses Lite metadata 最终过滤。 | passthrough adapter 只传最终模型给 Lite 策略，终态字段不丢失。 | WS passthrough、Lite WS session、完整 service unit。 | 不涉及 Wire。 |

## 3. 前端文件

| 文件 | main 改动 | build 改动 | 最终语义与处理 | 测试证据 | Wire 处理 |
| --- | --- | --- | --- | --- | --- |
| `frontend/src/components/account/BulkEditAccountModal.vue` | Grok endpoint/base URL 批量配置与清理。 | Codex 自定义 UA、兼容模式、Web Search 等 build 批量字段。 | main 表单状态保留；build 字段使用功能组件/helper 写入 existing extra 副本，父组件只组装 payload。 | `BulkEditAccountModal.spec.ts`、各 feature extra 测试。 | 不涉及 Wire。 |
| `frontend/src/components/account/CreateAccountModal.vue` | Grok endpoint preset、自定义 URL 和切平台清理。 | Codex 自定义客户端、JSON Schema/Web Search、Grok force-chat。 | main credentials 构建顺序不变；build UI/extra 迁入 feature 组件和 helper，最终 payload 保持单一来源。 | `CreateAccountModal.spec.ts`、`credentialsBuilder.spec.ts`、feature 测试。 | 不涉及 Wire。 |
| `frontend/src/components/account/EditAccountModal.vue` | Codex 图片桥接与本地执行器边界、main 上游配置。 | build 自定义 UA、兼容/Web Search、Grok force-chat 和 Responses Lite 相关接入。 | main 图片工具主体保留；build 字段抽离后父组件只复制并逐键更新 extra，图片文案由独立 locale override 覆盖最终模型策略。 | `EditAccountModal.spec.ts`、feature extra 测试、locale extension 测试。 | 不涉及 Wire。 |
| `frontend/src/components/account/__tests__/BulkEditAccountModal.spec.ts` | 覆盖 main Grok 批量 endpoint 字段。 | 覆盖 build 批量 custom UA/compatibility/Web Search payload。 | 双方关键 payload 断言保留；build 局部转换由功能测试承接。 | 该测试文件及 feature tests。 | 不涉及 Wire。 |
| `frontend/src/components/account/__tests__/EditAccountModal.spec.ts` | 覆盖 main 图片工具 policy 与说明边界。 | 覆盖 build extra 字段和迁移兼容。 | main 图片 mode 断言保留；build extra 测试不再复制组件内部规则。 | 该测试文件、`buildFeatureLocaleExtensions.spec.ts`。 | 不涉及 Wire。 |
| `frontend/src/components/account/__tests__/credentialsBuilder.spec.ts` | Grok endpoint preset、header 和 credentials 清理规则。 | build 账号 feature extra/credentials 兼容断言。 | 继续以 `credentialsBuilder.ts` 为 main 单一 credentials owner；build 仅通过 feature helper 扩展 extra。 | 该测试文件。 | 不涉及 Wire。 |
| `frontend/src/components/account/credentialsBuilder.ts` | main Grok base URL preset 与 credentials 构造规则。 | build 需要在创建/编辑流程保留私有字段。 | 不把 build extra 规则塞入 builder；builder 维持 main credentials owner，feature helper 在其后薄接入。 | `credentialsBuilder.spec.ts`、Create/Edit 集成测试。 | 不涉及 Wire。 |
| `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts` | 澄清 hosted `image_generation` 与本地 `image_gen` 的配置输出边界。 | build Codex 图片桥接/Responses Lite 行为不得污染 key 配置。 | 保留 main 文案和输出断言，继续确保配置中不错误生成 `image_generation`。 | 该测试文件。 | 不涉及 Wire。 |
| `frontend/src/i18n/locales/en/admin/accounts.ts` | main 修改 Codex 图片桥接说明。 | build custom UA、Grok Billing、Codex reset、兼容/Web Search 文案。 | build 文案按领域模块迁出；main 图片文案原键保留在原位以承接上游修改，`accountsOpenAIImageGenerationOverrides.ts` 在 `accounts.openai` 末尾覆盖 build 最终策略文案。 | `buildFeatureLocaleExtensions.spec.ts`、locale compile/no-collision。 | 不涉及 Wire。 |
| `frontend/src/i18n/locales/en/admin/channels.ts` | main 澄清渠道图片桥接与本地执行器边界。 | build Web Search/AnySearch 与图片 bridge 提示。 | 主文件只在稳定对象边界展开两个 override，最终英文文案同时保留 main 概念区分和 build 最终模型策略。 | locale extension、compile、no-collision。 | 不涉及 Wire。 |
| `frontend/src/i18n/locales/zh/admin/accounts.ts` | main 修改 Codex 图片桥接中文说明。 | build 中英文同构的私有账号文案。 | 与英文同构：保留 main 原键并在稳定末尾展开 build override；最终文案明确 Lite 图片注入按最终模型阻止列表决定，不写成“仅非 Lite”。 | locale extension、compile、no-collision。 | 不涉及 Wire。 |
| `frontend/src/i18n/locales/zh/admin/channels.ts` | main 澄清渠道图片桥接中文边界。 | build Web Search/AnySearch 和图片 bridge 提示。 | 与英文保持完全同构，主文件只保留稳定展开。 | locale extension、compile、no-collision。 | 不涉及 Wire。 |
| `frontend/src/types/index.ts` | main 增加用户批量限额及相关共享类型字段。 | build Grok Billing、Codex reset、设置和账号 feature 契约。 | main 类型字段保留；build 专属 DTO 迁入 feature 模块，中央类型只保留 `AccountUsageInfo.grok_billing_quota` 等不可避免引用。 | `pnpm typecheck`、Grok Billing usage/API/component tests。 | 不涉及 Wire。 |

## 4. 结论

- 27 个双边修改文件均已给出 main/build 所有权和最终语义；自动合并文件不再按“无冲突即正确”处理。
- build 私有测试、文案和业务判断已优先迁出共享热点；中央文件只保留 route、类型、ProviderSet、构造器或稳定展开等最小接入。
- `wire_gen.go` 是唯一需要生成器重建的双边文件；本轮因 `AccountUsageService` 构造器变化已重新生成，其余文件不通过 Wire 处理。
- `CHK-009` 证明“删除或包裹 main 正在修改的 locale 键”仍会形成同区块冲突；最终让 main 原键相对 HEAD 保持零 diff，并在稳定末尾把 build override 收窄为 `Record<string, string>` 后展开，既避免 TypeScript 重复直接属性告警，也兼顾上游可合并性与 build 文案所有权。
- 最终 merge-tree 中原 WS、中英文 accounts 三个硬冲突均不再出现；真实工作区仍无冲突索引和 `MERGE_HEAD`，本子任务没有执行真实 merge。
