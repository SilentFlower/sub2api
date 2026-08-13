# 兼容 Responses Lite Web Search 桥接

## Goal

让 Codex 保持 Responses Lite 时，在 OpenAI APIKey Chat fallback 链路中仍可由模型按需调用现有 Web Search provider，并保持 Responses 回程、引用与计费语义。

## Background

- Codex CLI `0.147.0` 在 `gpt-5.6-sol` 模型清单标记 `use_responses_lite=true` 时，会发送 `X-OpenAI-Internal-Codex-Responses-Lite: true`，但抓包确认请求体不包含顶层 typed `web_search`，也不包含 `namespace=web,name=run`。
- 当前 typed Web Search 入口首先检查原始 body 是否包含 `web_search`，因此上述 Lite 请求不会进入能力决策，见 `backend/internal/service/openai_responses_websearch.go:69`。
- 当前 Chat fallback 只在转换结果中发现 typed Web Search 或 `web.run` 时才启用内部 Web 工具循环，见 `backend/internal/service/openai_gateway_responses_chat_fallback.go:54` 和 `backend/internal/service/openai_gateway_responses_chat_fallback.go:71`。
- 现有 `sub2api_web_search` 代理、多轮 Chat 工具循环、同账号续跑、provider 调用、Responses `web_search_call`、来源引用、usage 累计和按真实成功查询计费已经实现，不应建立第二套搜索执行链。
- Responses Lite 顶层工具只允许 `function`、`custom`、`tool_search`，namespace 会迁入 `input.additional_tools`；直接注入 hosted `web_search` 会违反 Lite 契约，见 `backend/internal/service/openai_responses_lite_tools.go:48`。
- 生图桥已经提供独立的 Codex capability bridge 开关与账号/渠道覆盖模式，可作为策略装配参考；搜索桥只能复用其配置模式，不能复制 hosted `image_generation` 注入方式。

## Requirements

- R1. 只在 Codex Responses Lite 请求最终路由到 OpenAI APIKey Chat fallback，且搜索桥策略和全局 Web Search provider 均允许时，向转换后的 Chat 请求注入现有固定内部工具 `sub2api_web_search`。
- R2. 不向 Lite 原始请求注入 hosted `web_search`，不要求客户端关闭 Responses Lite，不修改模型清单的 `use_responses_lite`；Lite Header 只作为入站能力判定信号，转成 Chat Completions 后不承诺向 Chat 上游发送该 Responses 专用 Header。
- R3. 注入后由上游模型按需选择是否搜索；不得根据用户提示词预执行搜索，也不得让每个 Lite 请求强制产生搜索费用。
- R4. 复用现有 `openAIResponsesInternalWebToolConfig`、`forwardResponsesViaWebRunChatCompletions` 和 Web Search provider/配额/代理/计费链，禁止复制第二套工具循环。
- R5. 内部工具调用必须由网关消费，不向 Codex 泄漏 `sub2api_web_search` function call；最终仍返回合法 Responses 输出。
- R6. 模型未选择搜索时，不调用 provider、不产生 `web_search_call`、不增加 Web Search 费用，并正常回传其它 function/custom/namespace/tool_search 调用。
- R7. 模型选择搜索时，保持现有查询/轮次上限、`parallel_tool_calls=false`、同账号续跑、usage 累计、真实成功查询计费、最多 5 条来源及 `url_citation` 语义。
- R8. 全局 provider 不可用、账号/渠道策略关闭、非 Lite、非 Codex、原生 Responses 上游、非 OpenAI 平台、compact 请求和 WebSocket 非目标链路不得被本桥隐式接管。
- R9. 内部代理名与客户端已声明工具冲突时返回 OpenAI 兼容 `400 invalid_request_error`，参数指向 `tools`，不得动态改名。
- R10. 日志只记录账号 ID、模型、Lite/bridge 决策、轮次、成功查询数和安全 provider 标识；不得记录查询全文、结果正文、请求体或凭据。
- R11. 新增独立 `codex_web_search_bridge` 账号/渠道开关，缺失时默认关闭；账号布尔覆盖渠道值，账号未配置时跟随渠道，渠道未配置时关闭。桥接实际生效还必须同时满足现有 `web_search_emulation` 账号/渠道策略、全局 Web Search 配置和可用 provider。
- R12. 隐式桥只处理未显式声明 typed `web_search` 或 `web.run` 的 Lite 请求；已有显式搜索工具继续走现有能力分类和工具循环，禁止重复注入或改变其 `tool_choice` 语义。

## Acceptance Criteria

- [ ] Codex `gpt-5.6-sol` 保持 `use_responses_lite=true`，Lite 请求不声明 typed `web_search`/`web.run` 时，满足桥接条件即可让 Chat 上游看到 `sub2api_web_search`。
- [ ] 上游不选择搜索时，只执行一次普通 Chat 转发，provider 调用数和 `WebSearchCalls` 均为 0。
- [ ] 上游选择搜索时，网关真实执行现有 provider、回灌同一 call ID、使用同一账号和模型续跑，并输出最终文本及标准 `web_search_call`。
- [ ] 流式与非流式回程的搜索项、最终文本、来源、annotation、usage 和计费保持现有 mixed typed Web Search 契约。
- [ ] Codex 客户端无需关闭 Lite，模型清单和 Lite 入站策略不变；原始 Lite body 不出现 hosted `web_search`，Chat fallback 仅在转换后的 Chat tools 中增加内部 function。
- [ ] 桥接关闭、provider 不可用、账号策略关闭或不满足目标路由时不注入内部工具，且不改变既有请求行为。
- [ ] 管理端可在 OpenAI 渠道设置桥接默认值，并可在 OpenAI APIKey 账号编辑页选择跟随渠道、开启或关闭；缺失配置展示和保存均保持默认关闭/继承语义。
- [ ] 原有 typed Web Search、`web.run`、直接模拟、原生 Responses、Anthropic 搜索、生图桥和 Responses Lite 工具归一化测试不回归。
- [ ] HTTP 请求的生产复现形态、工具名冲突、其它客户端工具、模型不搜索、单轮/多轮搜索、provider 失败、查询上限、结构化输出和计费均有定向测试。
- [ ] 相关 service/apicompat 单测、后端完整 unit tests、前端 Vitest/typecheck/lint:check/build 和 `git diff --check` 通过。

## Out of Scope

- 不修改 Codex CLI 或模型清单，不要求用户关闭 Responses Lite。
- 不实现新的搜索 provider、浏览器打开/点击/截图、图片搜索、天气或金融专用接口。
- 不重写 `/v1/alpha/search` standalone 搜索协议；该端点继续作为 Codex 客户端独立搜索路径。
- 不修改进行中的 `07-18-websearch-settings-thin-layer` 前端重构任务；若本任务需要新增 UI，只做桥接开关的最小装配，并与该任务协调文件边界。
- 不部署、更新镜像、重启容器或修改生产配置/数据。

## Decisions

- [x] 使用独立 `codex_web_search_bridge` 账号/渠道开关，默认关闭；同时要求现有 `web_search_emulation` 与全局 provider 可用，避免桥接开关扩大已有搜索授权边界。
- [x] 只覆盖 HTTP `/v1/responses` -> Chat Completions fallback；Responses WebSocket、`/responses/compact` 和 `/v1/alpha/search` 不在本轮范围。

## Notes

- 这是跨 Responses Lite、Responses -> Chat、内部工具循环、配置和计费回程的复杂任务，需要 `design.md` 与 `implement.md` 后才能进入实现。
- 历史任务 `.trellis/tasks/archive/2026-07/07-16-deepseek-mixed-tool-websearch/` 是搜索执行主体的设计和验收基线。
