# Research: Structured Outputs 与 Web Search 上游能力兼容

## 官方协议

### Structured Outputs

- Responses 使用 `text.format.type=json_schema` 表达结构化输出。
- Chat Completions 使用 `response_format.type=json_schema`，schema 放在 `json_schema` 对象内。
- Structured Outputs 与 function calling 用途不同：前者约束模型面向用户的输出，后者连接应用工具。
- `json_schema → json_object` 只保留 JSON Mode，不再保证 schema/strict。

来源：<https://developers.openai.com/api/docs/guides/structured-outputs#function-calling-vs-response-format>

### Web Search

- 新 Responses 集成使用 `tools:[{"type":"web_search"}]`；`web_search_preview` 是兼容旧类型。
- 搜索输出包含独立 `web_search_call`，文本引用放在 `message.content[].annotations[].url_citation`。
- `search_context_size`、filters、location、sources、外网访问和返回预算属于真实能力字段，不能通过当前简化 DTO 丢弃。
- OpenAI 官方 Chat Completions 搜索使用专用 Search API 模型；它不是把 Responses 工具原样塞进普通 Chat 请求。

来源：<https://developers.openai.com/api/docs/guides/tools-web-search>

## sub2api 当前实现

### JSON Schema

- `ResponsesToChatCompletionsRequest` 调用 `responsesTextFormatToChatResponseFormat`，正确生成 Chat `response_format.json_schema`。
- `deepseek-v4-pro` 等不支持该类型的上游会返回 400。
- 之前实现过账号级 `json_schema → json_object`，随后按用户要求整体回滚；当前没有账号兼容配置。

### Web Search

- Responses→Chat 的 `responsesToolsToChatTools` 明确只保留 function/custom/tool_search/namespace，其它服务端工具全部丢弃。
- `ResponsesTool` 未保存 Web Search 专有配置字段。
- Responses 输出类型虽然有 `web_search_call.action`，但文本内容 DTO 尚未完整表达 URL annotations。
- 项目已有 `websearch.Manager` 与配置/UI/计费，但入口在 Anthropic `GatewayService.Forward`，限制为 Anthropic API Key 且仅唯一 Web Search 工具，输出也是 Anthropic 协议。

## CPA 对照

- CPA 的 OpenAI Responses→Chat 转换同样不保留 `text.format`。
- CPA 新版支持 function/custom/namespace/additional_tools，但通用转换器仍丢弃 Web Search。
- CPA 的 xAI 专用 executor 另有 `web_search` / `x_search` 原生适配，说明原生搜索必须按 provider 契约实现，不能靠通用 Chat 桥猜测。

## 设计结论

1. 能力模型至少区分 Structured Outputs、Responses 原生 Web Search、Chat 厂商原生搜索和网关模拟。
2. 通用桥只负责有标准等价物的字段；厂商私有搜索应放 provider adapter。
3. Web Search 模拟扩展 OpenAI 输出时复用现有 manager，不复制 provider/配额逻辑。
4. 不支持能力时应明确 reject 或显式 compat，禁止静默删除。

## 用户范围决定

- Web Search 第一版指定支持 DeepSeek 上游的原生搜索能力。
- DeepSeek 官方原生搜索通过 Anthropic 兼容协议暴露，不通过普通 OpenAI Chat tools 暴露。

## DeepSeek 官方 API 核对

- 官方 OpenAI Chat 文档仍只公开 `function` 工具；该端点不能直接接收 Responses `web_search`。
- 官方 Anthropic API 指南给出的 `base_url` 是 `https://api.deepseek.com/anthropic`，模型示例直接使用 `deepseek-v4-pro`。
- 官方兼容表声明 `tool_choice` 的 `none`、`auto`、`any`、`tool` 均受支持；消息内容支持 `server_tool_use` 与 `web_search_tool_result`。
- DeepSeek 官方 GitHub 组织的 issue #70 给出可复现请求：`POST https://api.deepseek.com/anthropic/v1/messages`，工具为 `{"type":"web_search_20250305","name":"web_search","max_uses":8}`。问题只在强制 `tool_choice.type=tool` 时触发，说明搜索工具本身可执行。
- DeepSeek-V3 issue #1487 使用官方 Anthropic 端点与 `deepseek-v4-flash` 成功执行 `WebSearchTool20260209`；其问题是 `allowedDomains` 未被遵守，而不是搜索不可用。
- awesome-deepseek-agent issue #245 仍将 OpenAI `/v1/responses` 支持列为功能请求。因此正确桥接是 Responses→Anthropic Messages→DeepSeek，而不是 Responses→Chat。
- 若目标账号不是 `api.deepseek.com`，仍需显式声明其是否兼容 DeepSeek Anthropic Web Search 契约，不能按模型名或域名自动假设。

来源：

- <https://api-docs.deepseek.com/sitemap.xml>
- <https://api-docs.deepseek.com/api/create-chat-completion>
- <https://api-docs.deepseek.com/guides/tool_calls>
- <https://api-docs.deepseek.com/zh-cn/guides/anthropic_api>
- <https://github.com/deepseek-ai/awesome-deepseek-agent/issues/70>
- <https://github.com/deepseek-ai/DeepSeek-V3/issues/1487>
- <https://github.com/deepseek-ai/awesome-deepseek-agent/issues/245>

## sub2api 可复用路径与缺口

- `GatewayService.ForwardAsResponses` 已实现 Responses→Anthropic Messages→Responses，并调用账号 `base_url + /v1/messages?beta=true`。
- `convertResponsesToAnthropicTools` 已把 `web_search`、`google_search` 和 `web_search_20250305` 映射为 `web_search_20250305`，但尚未识别 OpenAI 旧别名 `web_search_preview`；当前 `ResponsesTool` 也不保存 `max_uses`、域名过滤、位置等字段，映射不是无损的。
- OpenAI 平台账号当前由 `OpenAIGatewayService.Forward` 处理；当 `openai_responses_mode=force_chat_completions` 时直接进入 Responses→Chat，因此需要在该分支前完成本地模拟接管或明确拒绝。任务明确不让同一账号按请求切到 DeepSeek Anthropic 端点；原生搜索使用独立 Anthropic APIKey 账号。
- Anthropic→Responses 流转换已有基础事件状态机，但现有 typed content 只覆盖文本、图片和普通工具。需要用 DeepSeek 实际 SSE 样例补全 `server_tool_use`、`web_search_tool_result`、引用信息到 `web_search_call` 与 `url_citation` 的映射。
- 官方文档与 issue 对强制 `tool_choice.type=tool` 的行为不一致，不能仅凭兼容表认定可用；实施前需固定测试矩阵，并为强制 Web Search 选择定义显式策略。

## new-api Web Search 对照

- new-api 的本地 Web Search 入口位于 Anthropic Messages handler：渠道配置启用后，在请求只有一个 Web Search 工具时短路上游并本地执行搜索。
- 它支持 Tavily 和 AnySearch。AnySearch 使用 MCP JSON-RPC `tools/call`，工具名为 `search`，可传 `query`、`max_results`、`freshness`、`content_types`；响应兼容结构化 results 和 MCP text block 内嵌 JSON。
- 它构造的仍是 Anthropic `server_tool_use`、`web_search_tool_result` 与文本摘要，流式也只生成 Anthropic SSE；没有实现 OpenAI Responses 的 `web_search_call`、Responses SSE 或 `url_citation`。
- 它同样拒绝混合工具接管，因此不能直接解决任意 Responses 工具集合；强行照搬仍会遇到当前路径缺口。
- new-api 将 provider 凭证放在渠道设置中；sub2api 已有更完整的全局 `websearch.Manager`、Brave/Tavily provider、Redis 配额、provider 选择、代理故障标记和账号代理复用，不应退回到每账号/渠道即时创建 provider 的结构。
- sub2api 已有 `accounts.extra.web_search_emulation=default|enabled|disabled`，但 `GetWebSearchEmulationMode` 当前只允许 Anthropic APIKey 账号，管理端也只在 Anthropic 账号展示。扩展 OpenAI APIKey 即可满足账号维度接管，无需新增重复字段。
- 许可证边界：new-api 为 AGPLv3，sub2api 为 LGPLv3。不能直接复制 new-api 实现到当前项目而不引入许可证影响；AnySearch provider 应基于公开协议和测试样例独立实现。

### 推荐复用边界

1. 保留 sub2api 的 Manager、全局 provider 配置、配额与代理实现。
2. 独立新增 AnySearch-compatible provider，不搬运 new-api 代码。
3. 扩展账号三态和渠道 OpenAI 平台开关，决定模拟器是否接管。
4. 在 OpenAI Responses 分流到 Chat 之前判断接管；输出使用专门的 Responses writer/事件适配器。
5. 原生 DeepSeek Web Search 与本地模拟在账号层隔离：OpenAI DeepSeek 账号使用本地模拟，原生搜索使用独立 Anthropic APIKey 账号；不在同一账号内动态切换端点。
6. 模拟接管仅覆盖纯 Web Search 或明确强制 Web Search；混合工具自动选择需要完整工具循环，第一版明确排除。

## 本地模拟计费链

- `OpenAIForwardResult` 已有 `WebSearchCalls` 字段；成功的 `/alpha/search` 请求以 `WebSearchCalls=1` 进入统一 OpenAI usage 记录。
- `OpenAIGatewayService.RecordUsage` 在 `WebSearchCalls>0` 时调用 `CalculateWebSearchCost`，使用分组 `web_search_price_per_call` 与倍率；未配置单价时默认 `$0.01/次`。
- Responses 本地模拟成功可以复用同一字段和计费路径，无需新增账单模型；搜索失败时不生成成功结果，因此不得计费。
- 现有 Anthropic 模拟器返回空 token usage，尚未接入这条 OpenAI 按次计费链；本任务的 OpenAI Responses writer 必须显式返回 `WebSearchCalls=1`。
