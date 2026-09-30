# 共同修改文件逐项复核

## 输入与状态

- 输入固定为 PRD 中的共同基线、原 build 和 main；列表由两个 `git diff --name-only` 的交集生成。
- 共 71 项：10 项文本冲突、61 项自动合并。下表已完成逐项源码复核；行为证据按链接区分 unit、组件、构建和隔离运行。
- 本次复核 71/71；10 项文本冲突均逐块适配，61 项自动合并均完成契约与语义检查。
- 冲突详细策略由 `design.md` 和 `approved-preview.md` 拥有；跨领域场景以原预览第 4 节为准。

## 逐项记录

| 编号 | 文件 | 合并类型 | 关注点 | 验收映射 | 状态 | 结论与证据 |
| --- | --- | --- | --- | --- | --- | --- |
| F01 | `backend/internal/config/config.go` | 自动合并 | 配置默认值、定制项和工程兼容 | AC1、AC2、AC6、AC7 | 通过 | 默认活动 Key 200、每小时创建 60、预占启用及 TTL 900；build 配置均保留。[V2](validation.md#v2) |
| F02 | `backend/internal/handler/admin/account_handler.go` | 自动合并 | 账号创建/编辑/批量字段和类型 | AC8 | 通过 | Claude reset 查询字段通过现有账号 DTO 暴露，创建/编辑类型未变。[V2](validation.md#v2) |
| F03 | `backend/internal/handler/admin/setting_handler.go` | 自动合并 | 新旧字段往返、DTO/解析/保存/cache | AC8、AC9 | 通过 | GET 返回白名单及旧字段；设置服务往返回归通过。[V8](validation.md#v8) |
| F04 | `backend/internal/handler/admin/setting_handler_audit.go` | 自动合并 | 新旧字段往返、DTO/解析/保存/cache | AC8、AC9 | 通过 | 新白名单纳入审计字段，旧审计范围保留。[V2](validation.md#v2) |
| F05 | `backend/internal/handler/admin/setting_handler_update.go` | 自动合并 | 新旧字段往返、DTO/解析/保存/cache | AC8、AC9 | 通过 | 指针入参允许省略新字段，更新校验和旧字段保存顺序保留。[V8](validation.md#v8) |
| F06 | `backend/internal/handler/dto/settings.go` | 自动合并 | 新旧字段往返、DTO/解析/保存/cache | AC8、AC9 | 通过 | 新增白名单 string 字段与 GET/更新 DTO 一致，未移除旧字段。[V2](validation.md#v2) |
| F07 | `backend/internal/handler/openai_alpha_search.go` | 自动合并 | 搜索证据、完成状态、失败与回退 | AC3 | 通过 | 搜索入口预占先于转发，保留搜索路由与任务释放。[V2](validation.md#v2) |
| F08 | `backend/internal/handler/openai_chat_completions.go` | 自动合并 | 预占/异步释放、协议、模型/Lite 和定制入口 | AC3、AC4、AC5、AC6、AC7 | 通过 | 上游预占与 build 粘性/模型策略共同生效，任务丢弃由 abandon 释放。[V2](validation.md#v2) |
| F09 | `backend/internal/handler/openai_gateway_handler.go` | 文本冲突 | 预占/异步释放、协议、模型/Lite 和定制入口 | AC3、AC4、AC5、AC6、AC7 | 已适配并通过 | 逐块整合预占、粘性和 cyber；异步 wrapper 两返回值及所有调用一致。[V2](validation.md#v2) |
| F10 | `backend/internal/handler/openai_gateway_handler_test.go` | 自动合并 | 预占/异步释放、协议、模型/Lite 和定制入口 | AC3、AC4、AC5、AC6、AC7 | 通过 | 保留双方 mock 与入口回归，全量 handler unit 通过。[V2](validation.md#v2) |
| F11 | `backend/internal/pkg/apicompat/anthropic_to_responses.go` | 自动合并 | 协议类型、工具输入、推理关闭与 usage | AC4、AC5 | 通过 | 新增模型和工具策略保留，disabled 由公共转换器输出标准 none。[V2](validation.md#v2) |
| F12 | `backend/internal/pkg/apicompat/anthropic_to_responses_response.go` | 文本冲突 | 协议类型、工具输入、推理关闭与 usage | AC4、AC5 | 已适配并通过 | 搜索输入独立返回，真实工具 delta 清理首事件 seed；跨状态回归通过。[V2](validation.md#v2) |
| F13 | `backend/internal/pkg/apicompat/chatcompletions_anthropic_bridge.go` | 文本冲突 | 协议类型、工具输入、推理关闭与 usage | AC4、AC5 | 已适配并通过 | 公共桥接采用上游 disabled=none；兼容 thinking 策略归属 service 私有 helper。[V2](validation.md#v2) |
| F14 | `backend/internal/pkg/apicompat/chatcompletions_anthropic_bridge_test.go` | 自动合并 | 协议类型、工具输入、推理关闭与 usage | AC4、AC5 | 通过 | 保留流式和非流式最终 usage 断言，全量 apicompat unit 通过。[V2](validation.md#v2) |
| F15 | `backend/internal/pkg/apicompat/responses_to_anthropic_request.go` | 自动合并 | 协议类型、工具输入、推理关闭与 usage | AC4、AC5 | 通过 | 请求转换保留工具、推理档位与 build 字段，实际类型匹配调用者。[V2](validation.md#v2) |
| F16 | `backend/internal/pkg/apicompat/types.go` | 自动合并 | 协议类型、工具输入、推理关闭与 usage | AC4、AC5 | 通过 | 新增工具/PDF 类型保留，未覆盖现有兼容请求字段。[V2](validation.md#v2) |
| F17 | `backend/internal/pkg/openai/constants.go` | 文本冲突 | 模型目录、提示词、实际出站策略 | AC5、AC7 | 已适配并通过 | GPT-6.1 Sol 提示词/元信息加入目录，build 5.6/Astra 提示词及 fallback 保留。[V2](validation.md#v2) |
| F18 | `backend/internal/server/api_contract_test.go` | 自动合并 | 路由、构造器、Wire 和接口完整性 | AC8、AC11 | 通过 | 新增 reset 路由与原路由契约并存，API contract 测试通过。[V2](validation.md#v2) |
| F19 | `backend/internal/server/routes/admin.go` | 自动合并 | 路由、构造器、Wire 和接口完整性 | AC8、AC11 | 通过 | 重置 handler 依赖注入及权限路由完整，构建通过。[V4](validation.md#v4) |
| F20 | `backend/internal/service/account_usage_service.go` | 自动合并 | metric 入参、调用与 stub/mock | AC11 | 通过 | 趋势查询新 metric 参数与实现、调用、stub 一致，编译和 unit 通过。[V2](validation.md#v2) |
| F21 | `backend/internal/service/antigravity_gateway_claude.go` | 自动合并 | 预占/异步释放、协议、模型/Lite 和定制入口 | AC3、AC4、AC5、AC6、AC7 | 通过 | 客户端断开按 499 记录；保留协议及计费收尾，新增状态测试通过。[V2](validation.md#v2) |
| F22 | `backend/internal/service/antigravity_gateway_gemini.go` | 自动合并 | 预占/异步释放、协议、模型/Lite 和定制入口 | AC3、AC4、AC5、AC6、AC7 | 通过 | Gemini 取消路径保留 build 转换与上游 499 状态语义。[V2](validation.md#v2) |
| F23 | `backend/internal/service/billing_service.go` | 自动合并 | 定价、缓存、Ultrafast、最终 effort/tier | AC5、AC6 | 通过 | 渠道图片价格继承/零价、长上下文缓存及 build Fast/Ultrafast 定价均有 unit 证据。[V2](validation.md#v2) |
| F24 | `backend/internal/service/domain_constants.go` | 自动合并 | 定价、缓存、Ultrafast、最终 effort/tier | AC5、AC6 | 通过 | 新增风控白名单 key 与读取、保存、缓存常量一致。[V8](validation.md#v8) |
| F25 | `backend/internal/service/gateway_forward_as_chat_completions.go` | 自动合并 | 预占/异步释放、协议、模型/Lite 和定制入口 | AC3、AC4、AC5、AC6、AC7 | 通过 | 接入统一 Claude usage 提取，同时保留模型映射、工具与流式处理。[V2](validation.md#v2) |
| F26 | `backend/internal/service/gateway_forward_as_chat_completions_test.go` | 自动合并 | 预占/异步释放、协议、模型/Lite 和定制入口 | AC3、AC4、AC5、AC6、AC7 | 通过 | Chat usage、客户端断开及流式收尾测试共同保留并通过。[V2](validation.md#v2) |
| F27 | `backend/internal/service/gateway_forward_as_responses.go` | 自动合并 | 预占/异步释放、协议、模型/Lite 和定制入口 | AC3、AC4、AC5、AC6、AC7 | 通过 | Responses 终止事件 usage 和 build 工具恢复链并存。[V2](validation.md#v2) |
| F28 | `backend/internal/service/gateway_forward_as_responses_test.go` | 文本冲突 | 预占/异步释放、协议、模型/Lite 和定制入口 | AC3、AC4、AC5、AC6、AC7 | 已适配并通过 | 冲突中同时保留 build 搜索 fixture 与上游终止事件 usage 测试。[V2](validation.md#v2) |
| F29 | `backend/internal/service/gateway_service.go` | 自动合并 | 预占/异步释放、协议、模型/Lite 和定制入口 | AC3、AC4、AC5、AC6、AC7 | 通过 | passthrough 默认与普通账号映射分别保留，公开路由回归通过。[V2](validation.md#v2) |
| F30 | `backend/internal/service/openai_alpha_search.go` | 文本冲突 | 搜索证据、完成状态、失败与回退 | AC3 | 已适配并通过 | 解析器四返回值保留 searched/error；失败、截断不得计为成功，适用时本地回退。[V2](validation.md#v2) |
| F31 | `backend/internal/service/openai_codex_models_service.go` | 自动合并 | 模型目录、提示词、实际出站策略 | AC5、AC7 | 通过 | 新增 GPT-6.1 目录、动态目录权限与 build Lite/blocked 元信息共存。[V2](validation.md#v2) |
| F32 | `backend/internal/service/openai_codex_models_service_test.go` | 自动合并 | 模型目录、提示词、实际出站策略 | AC5、AC7 | 通过 | 新增模型目录测试及原 build 断言共同通过。[V2](validation.md#v2) |
| F33 | `backend/internal/service/openai_codex_transform.go` | 自动合并 | 模型目录、提示词、实际出站策略 | AC5、AC7 | 通过 | Lite 拒绝策略保留，动态元信息与 build 模型提示词未丢失。[V2](validation.md#v2) |
| F34 | `backend/internal/service/openai_compat_model.go` | 文本冲突 | 归一化返回契约、模型后缀和 disabled | AC5 | 已适配并通过 | 保留归一化返回 effort 契约；显式 disabled 优先于后缀，显式 output effort 优先于派生值。[V2](validation.md#v2) |
| F35 | `backend/internal/service/openai_gateway_chat_completions.go` | 自动合并 | 预占/异步释放、协议、模型/Lite 和定制入口 | AC3、AC4、AC5、AC6、AC7 | 通过 | 在最终映射后应用 GPT 校验，保留原 Chat 路由和失败处理。[V2](validation.md#v2) |
| F36 | `backend/internal/service/openai_gateway_chat_completions_raw.go` | 文本冲突 | 预占/异步释放、协议、模型/Lite 和定制入口 | AC3、AC4、AC5、AC6、AC7 | 已适配并通过 | 新 GPT-6.1 工具/采样校验与旧 GPT-5.5 处理同时保留。[V2](validation.md#v2) |
| F37 | `backend/internal/service/openai_gateway_chat_completions_test.go` | 自动合并 | 预占/异步释放、协议、模型/Lite 和定制入口 | AC3、AC4、AC5、AC6、AC7 | 通过 | GPT-6.1 映射、非法 none/minimal、disabled 及工具校验回归通过。[V2](validation.md#v2) |
| F38 | `backend/internal/service/openai_gateway_forward.go` | 自动合并 | 预占/异步释放、协议、模型/Lite 和定制入口 | AC3、AC4、AC5、AC6、AC7 | 通过 | 新旧 GPT 校验、beta/header 及 build 原生 Responses 路径共同保留。[V2](validation.md#v2) |
| F39 | `backend/internal/service/openai_gateway_messages.go` | 自动合并 | 预占/异步释放、协议、模型/Lite 和定制入口 | AC3、AC4、AC5、AC6、AC7 | 通过 | Messages 路由保留 build fallback/Lite 策略及上游模型约束。[V2](validation.md#v2) |
| F40 | `backend/internal/service/openai_gateway_messages_chat_fallback.go` | 文本冲突 | 直连 Chat、新模型校验、Ollama 实际出站 | AC4、AC5、AC10 | 已适配并通过 | 按实际供应商投影 disabled；在最终 usage 前裁剪 Ollama token，真实出站回归通过。[V2](validation.md#v2) |
| F41 | `backend/internal/service/openai_gateway_messages_chat_fallback_test.go` | 自动合并 | 直连 Chat、新模型校验、Ollama 实际出站 | AC4、AC5、AC10 | 通过 | 原 fallback 回归与新增真实 disabled/Ollama 请求测试共同通过。[V2](validation.md#v2) |
| F42 | `backend/internal/service/openai_gateway_record_usage_test.go` | 自动合并 | 预占/异步释放、协议、模型/Lite 和定制入口 | AC3、AC4、AC5、AC6、AC7 | 通过 | 上游免费 Fast 缺价记录逻辑与原 usage/tier 回归共同保留。[V2](validation.md#v2) |
| F43 | `backend/internal/service/openai_gateway_request_body.go` | 自动合并 | 预占/异步释放、协议、模型/Lite 和定制入口 | AC3、AC4、AC5、AC6、AC7 | 通过 | 新增模型采样清理与限制在实际请求体执行，保留 build 请求定制。[V2](validation.md#v2) |
| F44 | `backend/internal/service/openai_gateway_responses_chat_fallback.go` | 文本冲突 | 最终 effort、混合工具/搜索/缓存/Ollama | AC3、AC5、AC10 | 已适配并通过 | 保留混合工具、搜索、缓存、Ollama；最终改写后再提取 usage effort，避免后缀复活。[V2](validation.md#v2) |
| F45 | `backend/internal/service/openai_gateway_scheduling.go` | 自动合并 | 预占/异步释放、协议、模型/Lite 和定制入口 | AC3、AC4、AC5、AC6、AC7 | 通过 | legacy/高级调度归属清晰，窗口过期与 reset 通知调用者一致。[V2](validation.md#v2) |
| F46 | `backend/internal/service/openai_gateway_service.go` | 自动合并 | 预占/异步释放、协议、模型/Lite 和定制入口 | AC3、AC4、AC5、AC6、AC7 | 通过 | openai-beta 允许头及原 build profile 保留，构造器依赖完整。[V4](validation.md#v4) |
| F47 | `backend/internal/service/openai_ws_forwarder_ingress.go` | 自动合并 | WS 首帧/后续帧、映射、准入与窗口 | AC6、AC7 | 通过 | WS 准入保留公开模型校验与原 raw hash；窗口切换重置 previous 链。[V2](validation.md#v2) |
| F48 | `backend/internal/service/openai_ws_forwarder_ingress_test.go` | 自动合并 | WS 首帧/后续帧、映射、准入与窗口 | AC6、AC7 | 通过 | 保留首帧/后续帧映射和窗口切换回归，全量 service unit 通过。[V2](validation.md#v2) |
| F49 | `backend/internal/service/setting_parse.go` | 自动合并 | 新旧字段往返、DTO/解析/保存/cache | AC8、AC9 | 通过 | 白名单解析为正整数集合，错误与重复值处理由已有回归覆盖。[V8](validation.md#v8) |
| F50 | `backend/internal/service/setting_service.go` | 自动合并 | 新旧字段往返、DTO/解析/保存/cache | AC8、AC9 | 通过 | 新增白名单参与立即刷新，旧设置/缓存逻辑未移除。[V8](validation.md#v8) |
| F51 | `backend/internal/service/setting_update.go` | 自动合并 | 新旧字段往返、DTO/解析/保存/cache | AC8、AC9 | 通过 | 保存支持显式清空及省略保留，刷新失败行为有回归证据。[V8](validation.md#v8) |
| F52 | `backend/internal/service/settings_view.go` | 自动合并 | 新旧字段往返、DTO/解析/保存/cache | AC8、AC9 | 通过 | 完整设置视图暴露新白名单与 build 字段，API/前端 string 契约一致。[V8](validation.md#v8) |
| F53 | `backend/internal/service/wire.go` | 自动合并 | 路由、构造器、Wire 和接口完整性 | AC8、AC11 | 通过 | 构造器新增依赖与 wire_gen 一致；完整 unit 与二进制构建通过，无需重生成。[V4](validation.md#v4) |
| F54 | `backend/resources/model-pricing/model_prices_and_context_window.json` | 自动合并 | 定价、缓存、Ultrafast、最终 effort/tier | AC5、AC6 | 通过 | 结构化比较仅新增 gpt-6.1-sol/claude-sonnet-5-5 对象；旧模型未删除，新值与 main 一致。[V1](validation.md#v1) |
| F55 | `deploy/config.example.yaml` | 自动合并 | 配置默认值、定制项和工程兼容 | AC1、AC2、AC6、AC7 | 通过 | 新增默认预占和 Key 限额示例与真实配置一致，build 部署项保留。[V7](validation.md#v7) |
| F56 | `deploy/docker-compose.dev.yml` | 自动合并 | Redis argv、密码传递和 build 资产 | AC2、AC6 | 通过 | Redis 改为 argv 数组；空/复杂密码解析及隔离容器 PONG，原持久化选项保留。[V7](validation.md#v7) |
| F57 | `deploy/docker-compose.local.yml` | 自动合并 | Redis argv、密码传递和 build 资产 | AC2、AC6 | 通过 | Redis argv、空间/引号/dollar 密码及持久化开关经隔离运行通过。[V7](validation.md#v7) |
| F58 | `deploy/docker-compose.yml` | 自动合并 | Redis argv、密码传递和 build 资产 | AC2、AC6 | 通过 | Redis argv 隔离运行及部署安全/环境/资源四项资产检查通过。[V7](validation.md#v7) |
| F59 | `frontend/src/api/admin/settings.ts` | 自动合并 | 新旧字段往返、DTO/解析/保存/cache | AC8、AC9 | 通过 | GET/更新类型新增白名单 string，现有 build 设置字段保留。[V5](validation.md#v5) |
| F60 | `frontend/src/components/account/BulkEditAccountModal.vue` | 自动合并 | 账号创建/编辑/批量字段和类型 | AC8 | 通过 | 批量 modal 新旧字段/映射配置并存；56 项回归通过。[V5](validation.md#v5) |
| F61 | `frontend/src/components/account/CreateAccountModal.vue` | 自动合并 | 账号创建/编辑/批量字段和类型 | AC8 | 通过 | 创建 modal 新旧字段与映射/订阅配置并存；41 项回归通过。[V5](validation.md#v5) |
| F62 | `frontend/src/components/account/EditAccountModal.vue` | 自动合并 | 账号创建/编辑/批量字段和类型 | AC8 | 通过 | 编辑 modal 新映射选项与 build 凭据字段共存，保存/回显回归通过。[V5](validation.md#v5) |
| F63 | `frontend/src/components/account/__tests__/EditAccountModal.spec.ts` | 自动合并 | 账号创建/编辑/批量字段和类型 | AC8 | 通过 | 编辑回归同时保留上游新场景和 build 场景。[V5](validation.md#v5) |
| F64 | `frontend/src/components/account/__tests__/credentialsBuilder.spec.ts` | 自动合并 | 账号创建/编辑/批量字段和类型 | AC8 | 通过 | 新增计划 SKU 与原 identity alias 分开处理，70 项 builder 回归通过。[V5](validation.md#v5) |
| F65 | `frontend/src/components/account/credentialsBuilder.ts` | 自动合并 | 账号创建/编辑/批量字段和类型 | AC8 | 通过 | 新 SKU 命名与旧凭据/未知值保留相互兼容；集中 helper 未重复实现。[V5](validation.md#v5) |
| F66 | `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts` | 自动合并 | Key 界面及原有行为 | AC6、AC11 | 通过 | Key 弹窗保留多 tab 与目录导入功能；30 项含远程/文件目录、1 MiB 边界回归通过。[V5](validation.md#v5) |
| F67 | `frontend/src/i18n/locales/en/admin/accounts.ts` | 自动合并 | 最终语言对象、override 与新增 key | AC9 | 通过 | 最终 en accounts 新 reset keys 与 build 后置扩展并存；真实英文确认框测试通过。[V6](validation.md#v6) |
| F68 | `frontend/src/i18n/locales/en/admin/settings.ts` | 自动合并 | 最终语言对象、override 与新增 key | AC9 | 通过 | 最终 en settings 白名单文案与 build 旧扩展并存，未遮蔽上游有效新文案。[V6](validation.md#v6) |
| F69 | `frontend/src/i18n/locales/zh/admin/accounts.ts` | 自动合并 | 最终语言对象、override 与新增 key | AC9 | 通过 | 最终 zh accounts 新 reset keys 保留；真实中文确认框和插值通过。[V6](validation.md#v6) |
| F70 | `frontend/src/i18n/locales/zh/admin/settings.ts` | 自动合并 | 最终语言对象、override 与新增 key | AC9 | 通过 | 最终 zh settings 新白名单 key 与旧扩展并存，最终语言集合一致。[V6](validation.md#v6) |
| F71 | `frontend/src/views/admin/SettingsView.vue` | 自动合并 | 新旧设置表单、字段保存往返及双语界面 | AC8、AC9 | 通过 | selector 正整数去重、API string 保存和重载/清空通过；build Grok 字段保存不丢失。[V5](validation.md#v5) |

## 独立文件及已执行预览

- main 独立变更 197 项、build 独立变更 716 项；最终文件模式和 blob 核对及必要适配详见 [V1](validation.md#v1)。
- 预览树 `f7cdacc5bb71f384f0434df112ef5dd5e2f93ba2` 有冲突标记，不能作为构建结果。
- 已执行的 locale 静态检查：en/zh 各 8453 个叶子键，键集合一致、main 有效 key 未丢失；详情见原预览第 6 节。该结果不替代实际合并后的组件测试。
- 已实际合并且仍未提交；执行证据与真实上游的未覆盖范围见 [validation.md](validation.md)。原预览树及预览结论保留为历史记录。
