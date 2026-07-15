# Brief — OpenAI 上游 Structured Outputs 与 Web Search 能力兼容

## Goal

- 升级 OpenAI Responses 与 Chat 回退的能力协商：显式兼容不支持 `json_schema` 的上游，并让 typed Web Search、DeepSeek 原生搜索和新版 Codex `web.run.search_query` 按各自协议真实执行，禁止静默丢失能力。

## Scope

- 为 OpenAI APIKey 账号提供显式 `json_schema -> json_object` 兼容，并保留原 Schema 的 best-effort 约束、未知字段和 function/tool Schema。
- 复用账号 `web_search_emulation=default|enabled|disabled`、渠道默认和全局 provider 能力开关；渠道未配置时默认关闭，全局开关不能隐式开启账号。
- typed `web_search` 仅在纯搜索或明确强制搜索时由本地模拟接管；复用 `websearch.Manager`、Brave/Tavily/AnySearch、账号代理、配额、失败回滚和按次计费。
- 保留 Responses -> Anthropic -> Responses 的 DeepSeek 原生 `web_search_20250305` 请求、搜索结果、失败状态、citations 和流式协议桥。
- 对 OpenAI APIKey Chat fallback 识别顶层 tools 与 Responses Lite `additional_tools` 中的 `namespace=web,name=run`。
- 模拟策略开启时，只向 Chat 上游暴露 `web.run.search_query`；天气问题由模型生成普通搜索词，不实现独立 `weather` 命令。
- 对合法 `search_query` 执行受控模型 -> provider -> tool result -> 模型续跑：每个请求最多 4 个查询、最多 2 轮，保持原 call ID，累计模型 usage 和真实成功搜索次数。
- 内部工具轮次不泄漏给 Codex；非流式返回 Responses JSON，流式返回完整 Responses SSE 生命周期。

## Non-Goals

- 不让同一 OpenAI 账号按请求动态切换 Chat 与 Anthropic 端点。
- 不实现 `web.run` 的 `weather`、`open`、`click`、`find`、`screenshot`、`image_query`、`finance`、`sports`，也不引入浏览器、网页抓取或页面会话。
- 不实现 `web.run` 之外的通用混合工具自动执行循环。
- 不支持 OpenAI Chat `web_search_options`、厂商私有搜索字段、strict JSON Schema 验证、基于 400 的自动重试或能力缓存。
- 不新增数据库列，不重构账号选择、HA、配额、代理或计费架构。

## Key Context

- OpenAI Responses 入口是 `OpenAIGatewayService.Forward`；Chat fallback 位于 `openai_gateway_responses_chat_fallback.go`，当前已用 `EffectiveResponsesTools`、`NamespaceToolNames` 和 `ResponsesToChatCompletionsRequest` 处理 `additional_tools` 与 namespace 摊平/还原。
- `web.run` 只在最终账号策略允许且实际走 Chat fallback 时接管；其它 namespace、function、custom、tool_search 和 typed Web Search 行为必须保持不变。
- 上游 Chat function Schema 收窄为 `search_query` 和可选 `response_length`，并关闭并行工具调用；`short|medium|long` 分别最多回灌 3、5、10 条结果。
- `search_query` 每项要求非空 `q`；可选 `recency` 只做兼容输入，当前 provider 不能严格过滤时必须在工具结果中明确告知。
- 内部轮次使用同一账号和映射后模型；若模型选择其它客户端工具，继续按现有 Responses 回程交给 Codex，网关不执行。
- 日志不得记录查询全文、结果正文、凭据或完整请求；参数错误、未支持命令和失败搜索不计 Web Search 次数。
- 高风险点是中间 function call 泄漏、call ID 漂移、流式生命周期不完整、跨轮次 usage 漏算，以及修改通用 namespace 行为。

## Acceptance

- JSON Schema 显式兼容只输出 `json_object` 并保留 best-effort 约束；配置关闭时保持原请求。
- typed 本地搜索和 DeepSeek 原生搜索继续满足现有 Responses、citation、错误、流式和计费契约。
- 新版 Codex `web.run.search_query` 会真实执行 provider 搜索、按原 call ID 回灌并得到最终模型回答，不再出现无 tool output 的重复调用。
- 模拟策略开启时，上游 Web function 不再声明 `weather/open/click/find`；天气类请求通过普通 `search_query` 完成，未支持命令不执行也不伪装成功。
- 单请求查询数、搜索轮次、provider 错误和非法参数均受控；模型 token 与真实成功查询次数正确累计。
- 账号、渠道、全局配置默认行为不变；相关后端测试、前端测试/typecheck/lint 和 `git diff --check` 全部通过。

## Next Step

- 进入 `trellis-route(implement)`，实现 `implement.md` 第 9 项 `web.run.search_query` 工具循环，再执行定向验证和全范围检查。
