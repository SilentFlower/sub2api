# Structured Outputs 与 Web Search 能力兼容设计

## Architecture

任务保留现有平台边界，在账号选定后执行能力决策：

```text
OpenAI Responses 入站
  -> OpenAIGatewayService.Forward
     -> 账号 JSON Schema 兼容预处理
     -> Web Search 能力决策
        -> emulation：本地 provider -> Responses writer
        -> Chat fallback + web.run：模型 -> search_query -> provider -> 模型续跑
        -> native Responses：保持现有上游
        -> Chat fallback 且含不可处理 Web Search：明确 reject
        -> 普通请求：保持现有路径

Anthropic 分组的 Responses 入站
  -> GatewayService.ForwardAsResponses
     -> ResponsesToAnthropicRequest
     -> DeepSeek /anthropic/v1/messages
     -> AnthropicToResponsesResponse / AnthropicEventToResponsesEvents
```

不把 `GatewayService` 注入 `OpenAIGatewayService`，也不让 OpenAI 账号临时调用 Anthropic URL。两个平台继续使用各自的 Handler、转发结果、usage 和 failover 链。

## Account Contracts

### JSON Schema

账号 extra 键：

```text
openai_json_schema_to_json_object: boolean
```

- 仅 OpenAI APIKey 账号读取。
- `true` 表示兼容转换；缺失、false、非法值与其它账号类型表示保持原请求。
- 使用 bool 而不是新增枚举：当前“native”和“未配置”的行为完全相同，枚举不能提供额外语义。

### Web Search 模拟

复用既有键：

```text
web_search_emulation: default | enabled | disabled
```

`Account.GetWebSearchEmulationMode` 的平台 guard 从“仅 Anthropic APIKey”扩展为“Anthropic/OpenAI APIKey”。`default` 继续读取渠道：

```text
features_config.web_search_emulation.anthropic
features_config.web_search_emulation.openai
```

全局搜索配置未开启、没有可用 provider 或 Manager 未初始化时，即使账号为 enabled 也不能执行；返回可诊断错误，不伪造成功。

## JSON Schema Compatibility

在 `internal/pkg/openai_compat` 定义键名，在 `service.Account` 提供带平台/类型 guard 的公开读取方法。

在 `internal/pkg/apicompat` 提供两个 raw JSON helper：

```go
func DowngradeResponsesJSONSchemaToJSONObject(body []byte) ([]byte, bool, error)
func DowngradeChatJSONSchemaToJSONObject(body []byte) ([]byte, bool, error)
```

转换使用 `map[string]json.RawMessage` 保存未知字段，不能把完整请求解码为精简 DTO 后重新序列化。

### Responses

- 命中 `text.format.type=json_schema` 且 `schema` 是 JSON object。
- `text.format` 改为 `{"type":"json_object"}`。
- 在字符串 instructions 后追加稳定约束；instructions 缺失时创建。
- instructions 类型不兼容或 Schema 非法时保持原请求，让现有入口负责协议错误。

### Chat

- 命中 `response_format.type=json_schema` 且 `json_schema.schema` 是 JSON object。
- `response_format` 改为 `{"type":"json_object"}`。
- 在连续 system/developer 前缀之后插入独立 system 消息，携带 best-effort Schema 约束。
- 保留原 messages 顺序及所有 tool/function Schema。

调用点：

- `OpenAIGatewayService.Forward` 在 Responses 路由分支和 passthrough 之前处理 Responses shape。
- `OpenAIGatewayService.ForwardAsChatCompletions` / raw Chat 路径在发送上游前处理 Chat shape。
- 同一个请求只能执行一次转换；helper 幂等，已是 json_object 时不再注入约束。

## Web Search Decision

增加轻量信号检查，只有 body 可能包含 Web Search 时才完整解析 `ResponsesRequest`。

```go
type responsesWebSearchDecision string

const (
    responsesWebSearchPass      responsesWebSearchDecision = "pass"
    responsesWebSearchEmulate   responsesWebSearchDecision = "emulation"
    responsesWebSearchReject    responsesWebSearchDecision = "reject"
)
```

决策输入：账号三态、渠道默认、全局配置、上游是否走 Chat fallback、有效工具列表和 tool_choice。

安全接管条件：

1. effective tools 只有一个 Web Search；或
2. tool_choice 对象明确为 `type=web_search`。

typed `web_search` 混合工具的 `auto|required|none`、空 choice 或具名其它工具不接管。若上游走原生 Responses 则 pass；若只能走 Chat 则 reject，防止现有桥静默删除 Web Search；`web.run.search_query` 按下文独立循环处理。

本地模拟与 `text.format=json_schema` 同时出现时 reject。模拟器只返回搜索摘要，没有模型生成阶段，不能履行任意 JSON Schema。

## Local Search Execution

在 `OpenAIGatewayService` 增加独立 Responses 模拟入口，复用 package 级 `websearch.Manager`：

```go
func (s *OpenAIGatewayService) handleResponsesWebSearchEmulation(...) (*OpenAIForwardResult, error)
```

执行流程：

1. 从最后一条 user input 的 string/input_text 提取查询。
2. 根据 `search_context_size` 决定本地 max results：low=3、缺失/medium=5、high=10。
3. 调用 `SearchWithBestProvider`，传入账号代理。
4. 对 `filters.allowed_domains` 做 URL host 后过滤；非法域名配置在搜索前拒绝。
5. 构造 Responses response/events 并写入客户端。
6. 返回 `OpenAIForwardResult{WebSearchCalls: 1}`；provider 或写响应失败时不返回成功结果。

查询全文、摘要和结果正文不写日志。日志只记录 provider、结果数量、耗时、账号和决策。

账号代理不可用沿用 `websearch.ErrProxyUnavailable -> UpstreamFailoverError`；其它 provider 全失败直接返回网关错误，因为换账号不会改变全局 provider 状态。

## Codex `web.run.search_query` Loop

该路径只处理 OpenAI APIKey 的 Chat fallback，不改变 typed `web_search` 本地模拟和原生 Responses/Anthropic 路径。

### 能力识别与工具收窄

1. 使用 `EffectiveResponsesTools` 同时读取顶层 tools 和 `input[].additional_tools`。
2. 从 `NamespaceToolNames` 找到映射为 `namespace=web,name=run` 的 Chat 摊平工具，不能把其它 namespace 的同名子工具误判为 Web Search。
3. 账号最终策略允许模拟时，在 `ResponsesToChatCompletionsRequest` 之后把该 Chat function 的描述和参数 Schema 收窄为 `search_query`，并保留可选 `response_length`；同时设置 `parallel_tool_calls=false`，避免服务端搜索与客户端工具出现在同一批并行调用中。
4. 收窄后的 Schema 不声明 `weather/open/click/find/screenshot/image_query/finance/sports`。天气问题由模型生成类似“杭州天气”的 `search_query[].q`，而不是由网关转换专用天气结构。

`search_query` 参数契约：

```json
{
  "search_query": [
    {"q": "杭州天气", "recency": 1}
  ],
  "response_length": "short"
}
```

- `search_query` 必须包含 1 至 4 项，每项 `q` 必须是非空字符串。
- `response_length=short|medium|long` 分别把每个查询回灌给模型的结果数量限制为 3、5、10 条；缺失时按 medium=5，非法值返回工具参数错误。
- `recency` 是兼容输入；当前 provider 接口没有严格时间过滤参数。存在该字段时，工具结果附带“provider 未保证严格 recency 过滤”的机器可读警告，不伪造过滤能力。
- 参数只包含未支持命令、JSON 非法、查询为空或超限时，不调用 provider，生成与原 call ID 对应的工具错误。

### 受控续跑

Chat fallback 的 `web.run` 请求使用内部非流式轮次，避免中间 function call 泄漏给 Codex 客户端：

1. 第一次调用同一上游账号与映射后模型。
2. 若返回的唯一工具调用是映射后的 `web.run`，解析 `search_query` 并逐项调用 `SearchWithBestProvider`。
3. 把 assistant tool call 和对应 `role=tool` 结果追加到 Chat messages；工具结果只含 query、title、url、snippet、provider 能力警告，不含内部凭据或完整 provider 响应。
4. 使用同一账号、模型和策略续跑；最多 2 轮搜索工具调用，整个请求最多执行 4 个查询。
5. 返回最终模型 message；若模型选择其它客户端工具，则按现有 Responses namespace 回程交给 Codex，不由网关执行。

客户端请求 `stream=true` 时，中间轮次仍在服务端缓冲；最终 message 使用现有 Responses 事件类型合成完整 SSE 生命周期。非流式请求返回普通 Responses JSON。两种路径都不向客户端暴露已由服务端消费的 `web.run` function call。

模型 usage 累加所有内部轮次；`WebSearchCalls` 等于真实成功完成的 provider 查询数。参数错误、未支持命令和 provider 失败不增加搜索次数。

## AnySearch Provider

在 `internal/pkg/websearch` 独立实现 AnySearch-compatible provider，不引用 new-api package 或复制源码。

请求：

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "search",
    "arguments": {
      "query": "...",
      "max_results": 5
    }
  }
}
```

- 默认 endpoint 为 `https://api.anysearch.com/mcp`。
- API Key 可选；非空时使用 `Authorization: Bearer`。
- 读取上限与 Brave/Tavily 一致，错误体截断。
- 归一化顺序：结构化 `results/items/data/list` -> MCP content text 内嵌 JSON -> 单个文本 fallback。
- `ProviderTypeAnySearch` 加入配置校验、Manager 创建和前端 provider 选项；现有配额/代理逻辑不分叉。

## Responses Wire Format

在 `apicompat` 扩展协议类型：

- `ResponsesAnnotation`：type、url、title、start_index、end_index。
- `ResponsesContentPart.Annotations`。
- `ResponsesOutput.Status` 已存在，Web Search item 使用 in_progress/completed/failed。
- 流事件增加 annotation/annotation_index，支持 `response.output_text.annotation.added`。

本地非流式输出顺序：

1. `web_search_call` completed，action 含真实 query。
2. assistant message completed，output_text 是稳定结果摘要。
3. 每个结果标题或 URL 在摘要中的字符区间对应一个 `url_citation`。

本地流式输出顺序：

```text
response.created
response.output_item.added(web_search_call)
response.output_item.done(web_search_call)
response.output_item.added(message)
response.content_part.added(output_text)
response.output_text.delta
response.output_text.annotation.added*
response.output_text.done
response.content_part.done
response.output_item.done(message)
response.completed
data: [DONE]
```

固定 ID 在单次响应内保持一致；sequence_number、output_index 和 content_index 单调且与事件引用一致。

## Native DeepSeek Anthropic Bridge

### Request

`ResponsesTool` 补充 Web Search 专有字段，`AnthropicTool` 补充 server tool 字段。基础映射：

| Responses | Anthropic/DeepSeek | 行为 |
|---|---|---|
| `web_search` | `web_search_20250305` | 支持 |
| `web_search_preview` | `web_search_20250305` | 支持 |
| `filters.allowed_domains` | `allowed_domains` | 结构化映射 |
| approximate `user_location` | 扁平 `user_location` | 结构化映射 |
| forced web_search choice | `tool/name=web_search` | 保持强制语义 |
| `search_context_size` | 无等价物 | 原生路径拒绝；模拟路径用于 max results |
| `external_web_access` | 无等价物 | 拒绝 |
| `return_token_budget` | 无等价物 | 拒绝 |

`convertResponsesToAnthropicTools` 改为返回 error；未知高级字段不能被 typed DTO 吞掉。普通 function/custom/namespace 行为保持不变。

### Response

扩展 Anthropic DTO：

- `server_tool_use` 的 id/name/input。
- `web_search_tool_result` 的结果数组或 error object。
- text block 的 `web_search_result_location` citations。
- usage 中的 `server_tool_use.web_search_requests` 仅保留解析，首版不改变 Anthropic 分组计费模型。

非流式：

- `server_tool_use(name=web_search)` -> Responses `web_search_call`。
- 查询从 input.query 解析。
- text citation -> output_text `url_citation`；citation 索引覆盖其所属 text block，保证范围合法。
- search result error -> search item failed；不能标记为 completed。

流式：

- server tool block start/input_json_delta/stop 聚合为一个 web_search_call item。
- result block记录来源或错误，不输出 function call。
- text/citation delta 转成 Responses text 与 annotation 事件。
- 现有 reasoning/function/message 收尾状态机保持完整。

## Billing And Usage

Responses 模拟成功设置 `WebSearchCalls=1`，复用 `OpenAIGatewayService.RecordUsage` 的现有按次分支：

```text
cost = calls * group.web_search_price_per_call * multiplier
```

分组未配置时沿用默认 `$0.01/次`。本地模拟不调用模型，token usage 记 0；搜索摘要长度不伪装成模型 token。响应写入失败或 provider 失败没有成功 result，不计费。

## Admin UI

- OpenAI APIKey 创建/编辑区域增加 JSON Schema toggle 和 Web Search 三态 selector。
- Anthropic APIKey 保留现有三态 selector；文案改为平台无关描述。
- 渠道 OpenAI section 增加 Web Search 默认开关。
- 全局 provider type 增加 AnySearch；API Key 显示/保存继续沿用脱敏契约。
- 保存 extra 时复制原对象，仅增删两个任务键。

## Error Matrix

| 场景 | 结果 |
|---|---|
| JSON 兼容关闭 | 原样转发 |
| JSON 兼容开启且合法 json_schema | 转 json_object + 注入 best-effort 约束 |
| JSON 兼容开启但格式/Schema 非法 | 保持原请求，由既有入口/上游报错 |
| 纯/强制 Web Search + 模拟可用 | 本地执行并输出 Responses |
| 混合 auto Web Search + Chat fallback | 400 capability error |
| Web Search + json_schema + 模拟路径 | 400 incompatible capabilities |
| 模拟开启但无 provider | 503 capability unavailable |
| 账号代理不可用 | failover 换号 |
| provider 全失败 | 502 upstream/search error |
| `web.run.search_query` 合法 | 执行 provider、回灌同 call ID、同账号续跑 |
| `web.run` 只有 weather/open/click/find 等命令 | 不调用 provider，回灌稳定的 unsupported tool error |
| `web.run.search_query` 超过 4 项或超过 2 轮 | 停止续跑并返回稳定的 limit error |
| 原生强制搜索被 DeepSeek 拒绝 | 原上游 400 经过脱敏映射返回，不改 auto |
| 原生 server tool result error | web_search_call failed，保留可诊断错误 |

## Compatibility And Rollback

- 所有新账号配置默认关闭/继承，存量行为不变。
- 删除 `openai_json_schema_to_json_object` 可立即停止 JSON 降级。
- 设置 `web_search_emulation=disabled` 可立即停止账号模拟接管。
- AnySearch 是 Manager 内独立 adapter，可单独移除，不影响 Brave/Tavily。
- OpenAI Responses writer 与 Anthropic native bridge 分文件实现，可分别回滚。
- `web.run` 循环只位于 Chat fallback，可独立移除；移除后 namespace 摊平与其它客户端工具行为恢复原样。
- 不修改数据库 schema、migration、账号选择算法或跨平台动态路由。
