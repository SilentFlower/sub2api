# 共同修改文件的实施核对清单

范围从 `files.json` 生成：77 项，13 项文本冲突、64 项自动合并。以下保留规划的逐文件要求，并按最终源码、保留核对与实际回归回填状态。验证组的命令及证据见 `validation-matrix.md`、`execution.md` 和 `execution-state.json`；冲突方案见 `merge-preview.md`。

| 编号 | 文件 | 合并类型 | 核对与处理要求 | 验证组 | 状态 |
| --- | --- | --- | --- | --- | --- |
| F01 | `README_CN.md` | 自动合并 | 部署说明和现有定制说明均保留；确认图片、平台、启动说明与实际配置一致。 | V15、V16 | 部署文档和 build 补充说明均保留，静态核对通过；见 V15、V16 |
| F02 | `backend/internal/config/config.go` | 自动合并 | 新输出 token 下限、Gemini 缓存 TTL 与 build 非流式/图片响应头超时共存，校验默认值及环境变量。 | V05、V07、V09、V15 | 源码与对应回归通过；见 V05、V07、V09、V15 |
| F03 | `backend/internal/config/config_test.go` | 自动合并 | 保留新配置边界测试和旧超时回归；fixture 不能因新增最低 token 验证而失效。 | V05、V07 | 源码与对应回归通过；见 V05、V07 |
| F04 | `backend/internal/handler/admin/account_handler.go` | 自动合并 | 保留 main 平台/Vertex 模型同步及 probe 口径，保留 build 用量服务及同步接线。 | V03、V09、V10 | 源码与对应回归通过；见 V03、V09、V10 |
| F05 | `backend/internal/handler/admin/setting_handler.go` | 自动合并 | GET 同时返回充值阶梯与 build 网关/Grok/GIF 等字段。 | V11、V13 | 源码与对应回归通过；见 V11、V13 |
| F06 | `backend/internal/handler/admin/setting_handler_update.go` | 自动合并 | 省略与清空语义、充值字段检测、原兼容字段保存及即时缓存刷新都要保留。 | V11、V13 | 源码与对应回归通过；见 V11、V13 |
| F07 | `backend/internal/handler/dto/settings.go` | 自动合并 | 系统设置 DTO 保留新充值字段、旧定制字段及精确 JSON tag。 | V11、V13 | 源码与对应回归通过；见 V11、V13 |
| F08 | `backend/internal/handler/dto/types.go` | 自动合并 | Group i18n DTO 与 UsageLog 的 build 字段同时保留；协议内容类型另见 C02。 | V11、V12 | 源码与对应回归通过；见 V11、V12 |
| F09 | `backend/internal/handler/endpoint.go` | 文本冲突 | 按 C01 逐块整合双方行为并保留两套测试；禁止整文件选边。 | V03、V09 | 源码与对应回归通过；见 V03、V09 |
| F10 | `backend/internal/handler/openai_gateway_handler.go` | 自动合并 | 逐 turn 最新 API Key/分组倍率、图片 token、预占释放与稳定 Messages 会话身份并存；叠加 cyber 时保留白名单。 | V01、V03、V07 | 源码与对应回归通过；见 V01、V03、V07 |
| F11 | `backend/internal/handler/openai_gateway_handler_test.go` | 自动合并 | 同时保留最新分组倍率/image usage 与稳定会话身份入口回归。 | V01、V07 | 源码与对应回归通过；见 V01、V07 |
| F12 | `backend/internal/pkg/apicompat/anthropic_to_responses.go` | 自动合并 | 带配置 token 下限的新包装器和 build 的 billing-header 识别不能互相覆盖。 | V02、V05 | 源码与对应回归通过；见 V02、V05 |
| F13 | `backend/internal/pkg/apicompat/anthropic_to_responses_response.go` | 自动合并 | Haiku 5.5 思考签名与 build 搜索引用、工具输入首事件/delta 状态隔离共同验收。 | V02、V04 | 补齐DeepSeek 与 Claude 5.5 三型号文本项引用，实际转换回归通过；见 V02、V04 |
| F14 | `backend/internal/pkg/apicompat/chatcompletions_anthropic_bridge.go` | 自动合并 | 保留 build 直连桥接、工具/推理/缓存语义，使用 main 新增可配置 token 下限。 | V02、V05 | 源码与对应回归通过；见 V02、V05 |
| F15 | `backend/internal/pkg/apicompat/chatcompletions_anthropic_bridge_test.go` | 自动合并 | 可配置下限及旧直连转换、思考关闭、数组内容和工具参数测试分别保留。 | V02、V05 | 源码与对应回归通过；见 V02、V05 |
| F16 | `backend/internal/pkg/apicompat/responses_to_anthropic_request.go` | 自动合并 | Haiku 5.5 工具选择/采样规则与 build 混合工具、工具结果、空命名空间保护并存。 | V02、V04、V05 | 源码与对应回归通过；见 V02、V04、V05 |
| F17 | `backend/internal/pkg/apicompat/types.go` | 文本冲突 | 按 C02 逐块整合双方行为并保留两套测试；禁止整文件选边。 | V02、V04 | Refusal/Annotations 字段并集与转换回归通过；见 V02、V04 |
| F18 | `backend/internal/repository/http_upstream.go` | 自动合并 | 保留新 Grok CLI 身份头及官方回退清理，保留 build 非流式/图片 profile、连接池与 HTTP2 回退。 | V05、V07 | 源码与对应回归通过；见 V05、V07 |
| F19 | `backend/internal/repository/http_upstream_test.go` | 自动合并 | 同时验证 Grok 新身份头和 build 三类请求超时/HTTP2 连接池隔离。 | V05、V07 | 源码与对应回归通过；见 V05、V07 |
| F20 | `backend/internal/server/api_contract_test.go` | 自动合并 | 新充值 DTO 合同与旧网关设置合同同时验收。 | V11、V13 | 源码与对应回归通过；见 V11、V13 |
| F21 | `backend/internal/service/account.go` | 文本冲突 | 按 C03 逐块整合双方行为并保留两套测试；禁止整文件选边。 | V01、V03、V04 | 退役、调度、透传和目录映射组合回归通过；见 V01、V03、V04 |
| F22 | `backend/internal/service/account_header_override.go` | 自动合并 | 新平台 eligible 判断生效，同时保留 build 对内部 Lite 标记的保存拒绝和运行时防御。 | V03、V08、V11 | 源码与对应回归通过；见 V03、V08、V11 |
| F23 | `backend/internal/service/account_websearch_test.go` | 文本冲突 | 按 C04 逐块整合双方行为并保留两套测试；禁止整文件选边。 | V04 | 源码与对应回归通过；见 V04 |
| F24 | `backend/internal/service/antigravity_gateway_gemini.go` | 文本冲突 | 按 C05 逐块整合双方行为并保留两套测试；禁止整文件选边。 | V06 | GIF 抽帧与 functionCall 签名实际出站回归通过；见 V06 |
| F25 | `backend/internal/service/billing_service.go` | 自动合并 | Haiku/Argon/订阅长上下文免附加费与 build reasoning/image 定价规则并存。 | V07、V08 | 源码与对应回归通过；见 V07、V08 |
| F26 | `backend/internal/service/domain_constants.go` | 自动合并 | 新平台清单/额度/停调平台与 build 的所有设置键保留。 | V03、V11 | 源码与对应回归通过；见 V03、V11 |
| F27 | `backend/internal/service/gateway_forward_as_chat_completions.go` | 自动合并 | 新 token 配置、负索引防护、SSE 注释心跳与 build 既有协议桥接一致。 | V02、V05 | 源码与对应回归通过；见 V02、V05 |
| F28 | `backend/internal/service/gateway_forward_as_responses.go` | 文本冲突 | 按 C06 逐块整合双方行为并保留两套测试；禁止整文件选边。 | V02、V04 | 稀疏/负索引与引用聚合回归通过；见 V02、V04 |
| F29 | `backend/internal/service/gateway_service.go` | 自动合并 | 新 composite 归属/拒绝歧义路由和缓存存储量字段，保留 build 独立 owner 薄调用及 OAuth token 路径。 | V03、V07、V09 | 源码与对应回归通过；见 V03、V07、V09 |
| F30 | `backend/internal/service/gateway_websearch_emulation.go` | 自动合并 | main 共用的搜索模拟入口、OAuth 排除与 build 无 URL 结果过滤/真实搜索约定并存。 | V04 | 源码与对应回归通过；见 V04 |
| F31 | `backend/internal/service/openai_codex_models_service.go` | 自动合并 | main 退役过滤、500k 目录默认、Haiku 与 build GPT-5.6/Astra 描述和映射按最终公共模型生效。 | V03、V05 | 源码与对应回归通过；见 V03、V05 |
| F32 | `backend/internal/service/openai_codex_models_service_test.go` | 自动合并 | 新模型退役/能力目录与 build 模型提示词和显式映射回归共同运行。 | V03、V05 | 源码与对应回归通过；见 V03、V05 |
| F33 | `backend/internal/service/openai_codex_transform.go` | 自动合并 | main 按历史补 web_search 声明和 build Lite/图片桥接工具边界采用同一入站/最终模型语义。 | V04、V08 | 源码与对应回归通过；见 V04、V08 |
| F34 | `backend/internal/service/openai_gateway_cc_pipeline.go` | 自动合并 | 完整错误体识别用于安全重试，同时保留 build 原生 Chat 的 header/usage/流处理 helper。 | V02、V07 | 源码与对应回归通过；见 V02、V07 |
| F35 | `backend/internal/service/openai_gateway_chat_completions.go` | 文本冲突 | 按 C07 逐块整合双方行为并保留两套测试；禁止整文件选边。 | V02、V03、V04、V05 | 统一路由、固定 Chat 形状和搜索保护回归通过；见 V02、V03、V04、V05 |
| F36 | `backend/internal/service/openai_gateway_chat_completions_raw.go` | 自动合并 | 新结构化 input 回退、created_at 和 build Grok、推理、Lite、Schema、搜索边界共同验证。 | V02、V04、V05、V07 | 源码与对应回归通过；见 V02、V04、V05、V07 |
| F37 | `backend/internal/service/openai_gateway_chat_completions_test.go` | 自动合并 | 保留上游输入类型安全回退/终态测试以及 build Responses 形状和混合工具测试。 | V02、V04 | 源码与对应回归通过；见 V02、V04 |
| F38 | `backend/internal/service/openai_gateway_forward.go` | 自动合并 | 新统一路由、非 OpenAI 生图限制、最小输出配置与 build Lite、映射、Schema、实际超时 profile 接入一致。 | V03、V05、V07、V08 | Lite 降级不重开图片桥，实际请求体回归通过；见 V03、V05、V07、V08 |
| F39 | `backend/internal/service/openai_gateway_grok.go` | 自动合并 | main 解密 reasoning 拒绝恢复与 build 缓存身份/Free 工具路由并存；禁止重复回放有语义输出请求。 | V02、V05、V07 | 源码与对应回归通过；见 V02、V05、V07 |
| F40 | `backend/internal/service/openai_gateway_messages.go` | 文本冲突 | 按 C08 逐块整合双方行为并保留两套测试；禁止整文件选边。 | V03、V04、V05 | 新平台与 Grok 默认/强制路由回归通过；见 V03、V04、V05 |
| F41 | `backend/internal/service/openai_gateway_messages_chat_fallback.go` | 文本冲突 | 按 C09 逐块整合双方行为并保留两套测试；禁止整文件选边。 | V02、V05、V07 | 配置下限、disabled 与 Ollama 最终上限回归通过；见 V02、V05、V07 |
| F42 | `backend/internal/service/openai_gateway_passthrough.go` | 自动合并 | 新 compatibility options、web_search 历史和 created_at 追踪不能打破 build Lite 规则及模型映射透传。 | V02、V04、V08 | 源码与对应回归通过；见 V02、V04、V08 |
| F43 | `backend/internal/service/openai_gateway_request_body.go` | 自动合并 | profile 端点与 min-output 配置、工具历史修复与 build namespace/schema/Lite helper 顺序一致。 | V02、V03、V04、V08 | 源码与对应回归通过；见 V02、V03、V04、V08 |
| F44 | `backend/internal/service/openai_gateway_responses_chat_fallback.go` | 自动合并 | 保留原始 startTime 新参数，同时保留 build web.run、混合工具、搜索预算、reasoning cache、Ollama 上限。 | V02、V04、V05、V07 | 源码与对应回归通过；见 V02、V04、V05、V07 |
| F45 | `backend/internal/service/openai_gateway_responses_chat_fallback_test.go` | 文本冲突 | 按 C10 逐块整合双方行为并保留两套测试；禁止整文件选边。 | V02、V04、V07 | 源码与对应回归通过；见 V02、V04、V07 |
| F46 | `backend/internal/service/openai_gateway_scheduling.go` | 自动合并 | 退役模型在调度/compact 按最终映射判定；build 稳定会话身份和重试规则不能绕过它。 | V03、V07 | 源码与对应回归通过；见 V03、V07 |
| F47 | `backend/internal/service/openai_gateway_service.go` | 自动合并 | 新 created_at 补齐、模型退役/规范化和 build Lite/Schema/工具边界、延迟 profile 共同验证。 | V02、V03、V07、V08 | 源码与对应回归通过；见 V02、V03、V07、V08 |
| F48 | `backend/internal/service/openai_gateway_service_hotpath_test.go` | 自动合并 | 兼容性选项/热路径 fixture 跟随 main；保留 build 无 timeout 和 Lite 相关用例。 | V07、V08 | 源码与对应回归通过；见 V07、V08 |
| F49 | `backend/internal/service/openai_image_generation_controls_test.go` | 文本冲突 | 按 C11 逐块整合双方行为并保留两套测试；禁止整文件选边。 | V08 | 源码与对应回归通过；见 V08 |
| F50 | `backend/internal/service/openai_ws_forwarder_ingress.go` | 自动合并 | WS 首帧归一化新增 cfg，保留入站 Lite 原始状态和 bridge 禁用语义。 | V01、V02、V08 | 源码与对应回归通过；见 V01、V02、V08 |
| F51 | `backend/internal/service/openai_ws_http_bridge.go` | 自动合并 | 失败帧 created_at/sequence 与 build hosted 图片、入站 Lite、cyber 透传及记账边界一致。 | V01、V02、V07、V08 | 源码与对应回归通过；见 V01、V02、V07、V08 |
| F52 | `backend/internal/service/openai_ws_v2_passthrough_adapter.go` | 自动合并 | 首/后续 turn 配置归一化、图片 token 与 build Lite/图片规则及会话封禁接线一起验收。 | V01、V07、V08 | 源码与对应回归通过；见 V01、V07、V08 |
| F53 | `backend/internal/service/wire.go` | 自动合并 | Gemini cache provider/恢复循环和 build resolver 注入均保留，与生成构造器及关闭生命周期核对。 | V09、V14 | Ent/Wire 生成一致，恢复服务 Start/Stop 清理接线已核对；见 V09、V14 |
| F54 | `backend/internal/web/embed_test.go` | 自动合并 | 保留 build SVG 缓存测试及 main 裸 API alias 不被 SPA 吞掉的测试。 | V15 | 真实前端产物的 SVG 和 API alias 内嵌测试通过；见 V15 |
| F55 | `backend/resources/model-pricing/model_prices_and_context_window.json` | 自动合并 | main 新/改模型与 build 三项 GPT-5.6 价格均已结构化保留；仍需实际计费边界回归。 | V05、V07 | 双边定价结构零丢失，计费回归通过；见 V05、V07 |
| F56 | `deploy/.env.example` | 自动合并 | 新默认管理员不再固定密码，保留 build 非流式/图片超时变量。 | V07、V16 | 环境变量和管理员缺省策略的脚本/配置检查通过；见 V07、V16 |
| F57 | `deploy/config.example.yaml` | 自动合并 | main 输出 token、显式缓存等配置与 build 三类 timeout 说明和值均保留。 | V05、V07、V09、V16 | 配置字段和缺省值与源码、配置回归一致；见 V05、V07、V09、V16 |
| F58 | `deploy/docker-compose.dev.yml` | 自动合并 | 新默认管理员策略与 build 内存/CPU、Redis 密码和持久化参数共同验收。 | V16 | Compose/Redis 实际 argv 通过；原文件无 CPU/内存限额声明；见 V16 |
| F59 | `deploy/docker-compose.local.yml` | 自动合并 | 新默认管理员策略与 build 内存/CPU、Redis 密码和持久化参数共同验收。 | V16 | Compose/Redis 实际 argv 通过；原文件无 CPU/内存限额声明；见 V16 |
| F60 | `deploy/docker-compose.standalone.yml` | 自动合并 | 新默认管理员策略与 build 内存/CPU、Redis 密码和持久化参数共同验收。 | V16 | 外部 DB/Redis 参数下配置解析通过；原文件无 CPU/内存限额声明；见 V16 |
| F61 | `deploy/docker-compose.yml` | 自动合并 | 新默认管理员策略与 build 内存/CPU、Redis 密码和持久化参数共同验收。 | V16 | Compose/Redis 实际 argv 通过；原文件无 CPU/内存限额声明；见 V16 |
| F62 | `frontend/src/api/admin/settings.ts` | 自动合并 | 前端设置 API 新平台/充值类型与 build Grok/timeout 字段对应后端；locale settings 另按最终聚合验收。 | V11、V12、V13 | 源码与对应回归通过；见 V11、V12、V13 |
| F63 | `frontend/src/components/account/CreateAccountModal.vue` | 自动合并 | 平台清单、新平台、Vertex/API Key、显式缓存和 scope 与 build Schema/Lite/Alpha/搜索字段共存。 | V10、V11 | 源码与对应回归通过；见 V10、V11 |
| F64 | `frontend/src/components/account/EditAccountModal.vue` | 文本冲突 | 按 C12 逐块整合双方行为并保留两套测试；禁止整文件选边。 | V10、V11、V12 | 新旧字段组合保存、未知 extra 保留与回显回归通过；见 V10、V11、V12 |
| F65 | `frontend/src/components/account/__tests__/CreateAccountModal.spec.ts` | 自动合并 | 保留新 Gemini cache、scope、平台创建测试和 build 兼容开关测试。 | V10、V11 | 源码与对应回归通过；见 V10、V11 |
| F66 | `frontend/src/components/account/__tests__/EditAccountModal.spec.ts` | 文本冲突 | 按 C13 逐块整合双方行为并保留两套测试；禁止整文件选边。 | V10、V11 | 源码与对应回归通过；见 V10、V11 |
| F67 | `frontend/src/components/account/__tests__/credentialsBuilder.spec.ts` | 自动合并 | main 新凭据构造平台与 build 既有凭据保存回归一起运行。 | V10 | 源码与对应回归通过；见 V10 |
| F68 | `frontend/src/components/account/credentialsBuilder.ts` | 自动合并 | 新 profile 和协议基址/缓存凭据与 build extra owner 分工明确，保存不丢未知字段。 | V03、V10 | 源码与对应回归通过；见 V03、V10 |
| F69 | `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts` | 自动合并 | main workspace-aware Codex 导入与 build 远程/文件模型目录和所有限制回归共同保留。 | V10、V15 | 源码与对应回归通过；见 V10、V15 |
| F70 | `frontend/src/i18n/locales/en/admin/accounts.ts` | 自动合并 | 新平台/Vertex/缓存/scope 文案与 build 深层 locale 扩展并存，检查最终聚合值。 | V10、V12 | 最终 en/zh 8534 键对等和实际翻译渲染通过；见 V10、V12 |
| F71 | `frontend/src/i18n/locales/en/admin/settings.ts` | 自动合并 | 新充值及 cyber 身份说明与 build 深层 locale 扩展并存，复核最终 en/zh 键和值。 | V11、V12、V13 | 最终 en/zh 8534 键对等及 cyber 身份说明一致；见 V11、V12、V13 |
| F72 | `frontend/src/i18n/locales/zh/admin/accounts.ts` | 自动合并 | 新平台/Vertex/缓存/scope 文案与 build 深层 locale 扩展并存，检查最终聚合值。 | V10、V12 | 最终 en/zh 8534 键对等和实际翻译渲染通过；见 V10、V12 |
| F73 | `frontend/src/i18n/locales/zh/admin/settings.ts` | 自动合并 | 新充值及 cyber 身份说明与 build 深层 locale 扩展并存，复核最终 en/zh 键和值。 | V11、V12、V13 | 最终 en/zh 8534 键对等及 cyber 身份说明一致；见 V11、V12、V13 |
| F74 | `frontend/src/utils/format.ts` | 自动合并 | 保留 main 零倍率/价格显示修复及 build 小数舍入行为。 | V07、V13 | 源码与对应回归通过；见 V07、V13 |
| F75 | `frontend/src/views/admin/ChannelsView.vue` | 自动合并 | main 追加模型不改写显式空价的行为与 build 渠道/模型兼容配置并存。 | V10、V11 | 源码与对应回归通过；见 V10、V11 |
| F76 | `frontend/src/views/admin/SettingsView.vue` | 自动合并 | main 新平台和充值活动表单与 build 已抽离的独立设置组件、所有保存字段并存。 | V11、V13 | 源码与对应回归通过；见 V11、V13 |
| F77 | `frontend/src/views/admin/__tests__/SettingsView.spec.ts` | 自动合并 | 同时覆盖上游设置保存/回显和 build 白名单、Grok、超时、兼容配置，不删除旧断言。 | V11、V13 | 源码与对应回归通过；见 V11、V13 |
