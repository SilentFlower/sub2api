# Brief — 兼容 Responses Lite Web Search 桥接

## Goal

- 让 Codex 在不关闭 Responses Lite 的前提下，通过 OpenAI APIKey Chat fallback 由模型按需调用现有 Web Search provider，并保持 Responses 回程、引用、usage 与计费语义。

## Scope

- 新增独立 `codex_web_search_bridge` 账号/渠道开关：账号布尔覆盖渠道 OpenAI 平台值，缺失默认关闭。
- 仅在官方 Codex 或 `ForceCodexCLI`、Lite Header、普通 HTTP `/v1/responses`、OpenAI APIKey Chat fallback、桥接策略和现有搜索策略均允许、全局 provider 可用时启用隐式桥。
- 不修改 Lite 原始 body；在 Responses -> Chat 转换后注入现有固定内部 function `sub2api_web_search`。
- 复用现有内部 Web 工具循环、同账号续跑、provider/代理/配额、Responses `web_search_call`、来源引用、usage 聚合和真实成功查询计费。
- 保持 function/custom/namespace/tool_search 与 `tool_choice` 语义；已显式声明 typed `web_search` 或 `web.run` 的请求继续走现有路径，不重复注入。
- 在 OpenAI 渠道提供桥接默认开关，在 OpenAI APIKey 账号编辑页提供跟随渠道、开启、关闭覆盖；前端规则和组件归属 `features/webSearch`，共享页面只做薄装配。
- 增加后端策略、fallback、流式/非流式、错误、计费与回归测试，以及前端配置读写、组件和页面装配测试。

## Non-Goals

- 不修改 Codex CLI、模型清单或 `use_responses_lite`，不要求用户关闭 Lite。
- 不向 Lite Responses body 注入 hosted `web_search`，不修改通用 Responses -> Chat 转换器。
- 不实现新搜索 provider、浏览器 open/click/find/screenshot、图片搜索、天气或金融专用工具。
- 不扩展 Responses WebSocket、`/responses/compact` 或 `/v1/alpha/search` standalone 协议。
- 不修改数据库 Schema、Ent、migration，也不重构进行中的 `07-18-websearch-settings-thin-layer` provider 设置页。
- 不部署、更新镜像、重启容器或修改生产配置/数据。

## Key Decisions

- 使用独立 `codex_web_search_bridge`，并与现有 `web_search_emulation`、全局 Web Search 配置和可用 manager 做合取；桥接开关不扩大已有搜索授权边界。
- 账号覆盖优先于渠道，账号和渠道均缺失时关闭；不新增全局桥接层，避免第四层优先级。
- Lite Header 只作为入站资格信号；转换为 Chat Completions 后不承诺向 Chat 上游发送 Responses 专用 Header。
- 缺失或 `auto` 可注入；`required` 仅在已有客户端可执行工具时注入；`none` 或明确选择其它工具时不注入。
- 显式搜索存在性独立扫描 `EffectiveResponsesTools`，不能用 nullable typed config 代替，避免纯 typed 搜索或 `tool_choice=none` 被隐式桥覆盖。
- 全局 provider 在注入前不可用时失败关闭；注入后临时失败沿现有稳定 tool-result 语义处理。
- 渠道桥开关不因渠道 Web Search 默认值关闭而禁用，允许账号 `web_search_emulation=enabled` 与渠道桥接组合生效。

## Key Context

- 抓包确认 Codex CLI `0.147.0` 的 Lite `/v1/responses` 请求没有 typed `web_search` 或 `web.run`，现有搜索入口因此不会接管。
- `backend/internal/service/openai_gateway_responses_chat_fallback.go` 是 Responses -> Chat 和内部 Web 工具循环的接线点。
- `backend/internal/service/openai_responses_web_run.go` 已拥有 `sub2api_web_search`、最多 4 查询/2 轮、来源、流式事件、usage 与计费实现。
- 新增 `backend/internal/service/codex_web_search_bridge.go` 作为 build 私有策略 owner；共享 gateway 只调用一次资格 helper。
- 前端使用 `frontend/src/features/webSearch/` 作为配置读写、账号字段和渠道 Toggle 的 owner；`ChannelsView.vue`、`EditAccountModal.vue` 只做回填、保存和组件装配。
- 固定代理名冲突继续返回 `400 invalid_request_error`、`param=tools`；启用循环时保持 `parallel_tool_calls=false`。
- 日志不得记录查询全文、搜索结果、请求 body 或凭据。

## Risks / Deferred

- 模型看到搜索工具后仍可能选择不调用；本任务保证能力可见、可执行和回程正确，不根据提示词强制搜索。
- 启用桥接后首轮 Chat 需要在网关内缓冲以消费潜在内部工具调用；模型不搜索时仍只发送一次 Chat 请求且不产生搜索费用。
- Responses WebSocket 和 compact 需要独立协议设计，本轮明确延后。
- 前端会触碰共享 ChannelsView、EditAccountModal 和 locale 装配点，实施时需保持薄接线并避开正在进行的 SettingsView/provider 重构文件。

## Acceptance

- Codex `gpt-5.6-sol` 保持 Lite，且请求无显式搜索工具时，满足策略即可让 Chat 上游看到 `sub2api_web_search`。
- 模型未搜索时只有一次 Chat 转发，provider 调用和 `WebSearchCalls` 都为 0，其它客户端工具正常回程。
- 模型搜索时复用现有 provider、同 call ID 回灌和同账号续跑，输出标准 `web_search_call`、最终文本、最多 5 条来源和有效 `url_citation`。
- 流式与非流式的搜索项、事件索引、来源、annotation、usage 和计费保持现有 mixed typed Web Search 契约。
- 默认关闭、桥关闭、搜索策略关闭、provider 不可用、非 Lite、非 Codex、原生 Responses、compact、WebSocket 或显式搜索请求均不被隐式接管。
- 管理端渠道和账号配置可正确回填、保存并保持未知配置字段；账号继承/开启/关闭和渠道默认关闭语义有测试。
- typed Web Search、`web.run`、直接模拟、Anthropic 搜索、生图桥、Lite tools、alpha/search 和 apicompat 不回归。
- 后端定向与完整 unit tests、lint，前端 Vitest、typecheck、lint:check、build，以及 `git diff --check` 全部通过。

## Next Step

- provider readiness 的 fail-closed 缺口和默认构建标签 lint 阻塞均已修复；最终 full Check-All strict pass，下一步等待确认后进入 `trellis-update-spec`。
