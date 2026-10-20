# 双边功能验证矩阵

本矩阵已按最终本地合并树回填。通过表示本地单元/组件、模拟上游或隔离 PostgreSQL/Redis 的验证；真实供应商、支付、生产并发和滚动升级另列上线后验证。命令、退出码与日志索引见 `execution.md`、`execution-state.json`，没有把静态保留当作行为测试。

| 分组 | 双边功能 | 必须覆盖的可观察结果 | 已存在的验证入口/需补的组合场景 | 当前状态 |
| --- | --- | --- | --- | --- |
| V01 | cyber 透传、会话隔离、白名单 | 超过 256/600 条普通工具历史的独立会话不误拦；同 Key 不同 ID、不同 Key 同 ID 隔离；相同实际封禁 ID 仍拒绝；无 ID 不推断封禁；缓存 key/旧 v1 键不命中 v2；存储失败按原约定放行；原始上游拒绝留痕；白名单不写新封禁。 | `openai_cyber_session_block_test.go`、`openai_gateway_cyber_test.go`、`openai_cyber_allowlist_test.go`、WS passthrough cyber、`gateway_cache_cyber_test.go`。补同一个 WS 明确 ID 已预置封禁时白名单仍可建立连接的断言，不能用两个不同 ID 代替。 | 通过（本地，见下方证据） |
| V02 | Chat/Responses/Anthropic 协议语义 | refusal、developer、旧 function_call/结果配对、工具参数首事件/delta、数组/文件/图片及非连续索引均保留；负索引不 panic；原生 Anthropic 直通；安全回退不重放已输出/计费请求；SSE 心跳和 created_at 正确。 | `internal/pkg/apicompat` 全套，`TestConvertedResponsesInputStringRejection*`、Responses-shaped Chat、Responses fallback timing、`openai_responses_created_at_test.go`；补 refusal+annotations 同时存在和新路由下转换组合。 | 通过（本地，见下方证据） |
| V03 | 平台、模型目录、调度与映射 | profile 新平台和 OpenCode/国产三协议可达；Grok 特例不丢；合成分组/OAuth 歧义不误路由；公共模型映射后判断 retirement；OAuth GPT-5.5 被过滤、API Key 仍可用；compact/目录/实际转发一致。 | `upstream_protocol_routing_test.go`、`upstream_protocol_routing_matrix_test.go`、`provider_profile_test.go`、`openai_model_retirement_test.go`、Codex model service、composite tests；在 main golden 场景外补 build Grok 强制 Chat、固定 Chat 的 Responses 形状。 | 通过（本地，见下方证据） |
| V04 | 搜索、引用和混合工具 | main Messages 纯搜索模拟仅对合法 API Key/配置生效；OAuth 不被本地模拟；Alpha Search→Responses 必须有成功终态和搜索证据；AnySearch/web.run、混合工具、无 URL、预算耗尽和客户端写失败保持约定；引用不丢、不重复搜索/计费。 | `gateway_websearch_emulation_test.go`、`account_websearch_test.go`、`openai_alpha_search*test.go`、`openai_responses_websearch_test.go`、Responses fallback tests；补统一路由后的 raw Chat 不静默丢请求搜索工具。 | 通过（本地，见下方证据） |
| V05 | 推理、模型能力、Schema、输出上限 | build disabled 优先级和派生 effort、DeepSeek 回注、Schema 降级、GPT-5.6/Astra 提示词保持；main Haiku 5.5 采样/工具限制、新 token 下限生效；Ollama 最终上限不被重新抬高；日志 effort 来自实际出站。 | compat reasoning、provider reasoning、Schema gateway、Ollama、`apicompat_config` 和可配置 floor tests；以实际请求体测试自定义 floor+disabled+映射+Ollama 组合。 | 通过（本地，见下方证据） |
| V06 | Antigravity/Gemini 新能力与 GIF | GIF 开关、抽帧上限、包装大小和错误处理保持；functionCall/thoughtSignature、tool_result 图片、新模型映射及图片限流隔离仍正确；Gemini 思考档位和摘要在兼容流中可见。 | `antigravity_gif_compat_test.go`、`antigravity_gif_gateway_test.go`、Gemini compat thinking、Antigravity native tests；补带 GIF 的 native Gemini 工具轮次 actual body 断言。 | 通过（本地，见下方证据） |
| V07 | 费用、超时、并发与长连接 | 每 turn 使用当时分组倍率/图片 input token；删除 Key 后继续结算；cyber 不重复扣费；预占最终释放；stream/non-stream/image profile 选对；最终 effort/tier、长上下文和渠道时间价正确。 | billing/usage/async-reservation、HTTP upstream timeout、WS group pricing/image usage、pricing/retirement tests；断言 fallback 保留 startTime，串联搜索多轮与 usage task 释放。 | 通过（本地，见下方证据） |
| V08 | Lite、生图和 hosted 工具边界 | main web_search 历史在 Lite 的 additional_tools 声明；build blocked-model、GPT-5.5 兼容剥离、namespace 降级生效；有客户端 image_gen 不重复注入；入站 Lite 禁止桥接；non-OpenAI 原生 Responses 不注入 hosted 图片；合法桥接 none→auto。 | `openai_responses_lite*test.go`、`openai_lite_mapped_gpt55_test.go`、`openai_image_generation_controls_test.go`、HTTP/WS/WS→HTTP；旧日志和 v2 cyber 失败事件仍满足严格客户端结构。 | 通过（本地，见下方证据） |
| V09 | Gemini 显式缓存与模型/账号绑定 | AI Studio/Vertex/第三方上游适用范围正确；cache 只落到持有账号；按创建时模型/渠道计算存储费用；TTL 预留、并发修改租约、失败回滚/恢复和已删除账号处理正确；断开时按已收集 usage 计费。 | `gemini_cached_content_test.go`、`gemini_cached_content_hardening_test.go`、`gemini_cached_content_not_found_test.go`、`gemini_cached_content_repo_integration_test.go`，Gemini client disconnect/negative input tests。 | 通过（本地，见下方证据） |
| V10 | 账号创建、编辑、批量和模型导入 | 新平台/Vertex API Key/显式缓存/scope 可创建保存；原 Lite/Schema/Alpha Search/Codex 搜索桥/UA/映射/超时字段回显且不丢未知 extra；workspace-aware Codex 导入不覆盖其他工作区。 | Create/Edit/BatchEditAccount、credentials builder、UseKeyModal 全套；新增组合保存断言，而非仅验证开关存在。 | 通过（本地，见下方证据） |
| V11 | 系统/渠道设置跨层往返 | 新充值字段及平台配额类型与既有 Grok/timeout/GIF/搜索字段省略、清空、修改、重读一致；临时停调 scope 与后端默认相同；频道追加模型保留显式空价。 | SettingsView、ChannelsView、admin setting contract/service、scope tests；检查 JSON tag/TS/API/控件同名契约和设置缓存刷新。 | 通过（本地，见下方证据） |
| V12 | 国际化与最终覆盖 | en/zh 最终键集合一致；静态调用两侧存在、动态枚举完整；新旧 UI 都有中文；后置 override 不挡住 main 新语义；cyber 新身份说明真实显示。 | locale key completeness/compiler/no collision/build extensions、关键账号和设置组件 en/zh 渲染。预演已得 en/zh 各 8534 叶子键且 main 有效键无丢失，此结果不替代组件测试。 | 通过（本地，见下方证据） |
| V13 | 支付、认证与界面状态 | 新充值赠送/折扣、退款中扣减、支付回调验签与幂等不回退；邮箱验证码和密码 reset 单次性、并发 step-up、过时请求/导航取消正确；保留 build 品牌与原交互。 | main 支付/refund/promotion service 与相应 frontend tests、auth/email/reset/step-up tests；常规全量测试使用 fixture，不做真实交易。 | 通过（本地，见下方证据） |
| V14 | DB/迁移/生成物与生命周期 | 7 个 SQL 从旧 schema 升级和空库启动成功，旧 checksum 不变；新增平台合法值、group i18n、bonus、cache/租约结构可读写；Ent/Wire 一致；恢复循环可停止。 | migration/schema/checksum tests、repository integration、Ent/Wire generator；使用隔离 Postgres/Redis，不接业务库；具备条件时执行完整 integration。 | 通过（本地，见下方证据） |
| V15 | 全量构建、依赖、静态资源 | Go 1.27.2/lint v2.14 与 main CI 一致；后端全量 unit/lint/build、前端 frozen lock/lint/typecheck/full Vitest/build 通过；内嵌 SVG/API alias 的实际产物正确。 | `implement.md` 的全量命令，`backend/internal/web` embed tests，main frontend critical target。 | 通过（本地，见下方证据） |
| V16 | 部署和分支资产保留 | build CI/镜像 tag、品牌、Trellis/容灾资料保留；四个 Compose 新管理员默认与原资源/Redis 选项兼容；不因本地验证启动业务迁移。 | 部署脚本、Compose config/隔离 Redis 实际 argv、main/branch assets blob 复核；生产部署另行安排。 | 通过（本地，见下方证据） |

## 通过和失败记录

- 每组登记执行命令、固定输入、退出码、断言范围和日志；没有匹配测试或被跳过不能记为通过。
- 失败在原 build/main 上以相同工具链、fixture 和定向命令对照，分别记录新回归、原有失败、main 明确行为变化或环境缺口。
- 定向通过后只跑一次必要的全量检查；后续只因新改动、失败或未解决疑点重跑相关部分。
- 静态保持、编译、单元/组件、真实数据库集成、实际供应商调用分别标记；不能相互冒充。
- 任何未覆盖行为在交付中明确归属和验收方式，不作“全部线上功能已保证”的结论。
## 最终实际证据

| 组 | 证据与覆盖层级 |
| --- | --- |
| V01 | cyber/HTTP/WS/白名单定向及完整 unit；同 ID 预封禁白名单补充断言；实际生产代码已无 transcript/超限/旧 scope 引用。 |
| V02 | apicompat 和 service 全量；refusal、引用、稀疏/负索引、created_at、安全回退、写失败与原 startTime 回归。 |
| V03 | 完整协议矩阵保留原 golden，精确校验 1296 项形状修正及 48 项 Grok 修正；目录/退役/原生透传/替换映射定向与全量通过。 |
| V04 | WebSearch/AlphaSearch/WebRun 全量；固定 Chat 搜索拒绝与推理缓存组合、DeepSeek 与 Claude 5.5 三型号引用实际转换。 |
| V05 | 真实出站组合断言：自定义 floor=512、disabled、映射与 Ollama cap=256；完整 reasoning/Schema/能力测试。 |
| V06 | GIF 全量和 native Gemini 实际请求体同时断言 PNG 与 functionCall thoughtSignature。 |
| V07 | handler/service/repository 计费、预占、每 turn 倍率、图片 input token、timeout profile 和 fallback startTime 回归；双边定价结构比较零偏差。 |
| V08 | Lite/图片 HTTP、WS、WS→HTTP 全量；新增降级开关×APIKey/OAuth×普通/Lite 请求实际出站，降级后不注入 hosted 图片且不改 tool_choice。 |
| V09 | Gemini cache service 与 repository integration；迁移探针 cache/channel/lease 真实读写；Start/Stop 与 wire 清理接线静态核对。 |
| V10 | Create/Edit/BatchEdit、credentials builder、UseKeyModal 组件回归；额外组合保存旧兼容字段与未知 extra。 |
| V11 | settings API contract/handler/service 与 SettingsView/ChannelsView 全量；新旧字段的类型、JSON tag、默认/空值和保存链路已核对。 |
| V12 | 当前聚合对象 en/zh 各 8534 叶子键、差集零；compiler/completeness/collision 与真实 vue-i18n 渲染均通过。 |
| V13 | 支付/退款/赠送/认证/step-up/导航取消等服务和组件全量 fixture 回归；没有执行真实交易。 |
| V14 | 289→296 迁移及重复运行、历史订单和分组默认值、新平台插入、cache/lease 读写全部通过；空库 repository integration、独立审计 Postgres/Redis 214 项通过；Ent/Wire 一致。 |
| V15 | Go 1.27.2 unit 全量退出 0（58 包、23985 pass 事件）；lint、mod verify、普通/内嵌构建通过；Node20/pnpm9 前端 391 文件/2969 测试及 lint/typecheck/build 通过。 |
| V16 | 双边独立文件无意外丢失；部署 5 脚本与 4 Compose×2 密码条件配置共 8 项通过；3 份含 Redis 的配置实际启动并核对 argv/PONG，容器均移除。Compose 原文件未声明 CPU/内存限制，资源脚本检查的是随包定价文件。 |

## 跳过与阶段边界

- 最终 unit 有 17 个显式 skip；其中 9 个审计 opt-in 场景已在专用 PostgreSQL/Redis 单独覆盖，214 个 pass 事件、0 skip。其他为既有 sentinel、历史 NULL SQLite fixture 限制、TLS 外部捕获/网络、真实 TypeSafe/OpenAI 凭据与本地签名插件包缺失，不计作已通过。精确名单与原因见 `execution-state.json`。
- 完整 integration 首次仅 service 的目录断言失败；修正后 service integration 全包退出 0（8533 pass 事件）。其余 51 个包源码未变并已通过，复用该完整运行的有效证据。ConcurrencyCache 有一项上游硬编码 skip，未改断言。
- 裸错误流的 3 秒时序测试在固定原 main 重复 20 次也出现 1 次超时；最终完整 unit 已退出 0。缓存清理用例独立重复 20 次通过。中间失败记录保留，未放宽产品超时或测试断言。
- 前端最终完整运行启用了单次 retry，已如实记录；不是宣称所有中间尝试都成功。
- 上线后由发布方核对下游会话头稳定传递、全部旧实例替换、真实供应商响应与生产业务数据/计费；本轮未执行这些外部操作。
