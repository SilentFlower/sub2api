# 技术设计：Chat fallback 混合工具 Web Search

## 1. 设计目标

在不改变原生 Responses、Anthropic 原生搜索和纯/强制 Web Search 直接模拟行为的前提下，为 OpenAI APIKey Chat fallback 增加模型驱动的 typed `web_search` 内部工具循环。该循环必须保持 `tool_choice` 语义、其它客户端工具协议、Responses 流式事件、usage、计费与安全日志。

## 2. 基本事实与约束

- 普通 Chat Completions 上游不能直接执行 Responses 服务端 `web_search`，通用转换器当前会丢弃该工具。
- 混合工具 `auto` 不能使用现有直接摘要模拟器，因为后者会在模型选择工具之前提前搜索，改变 `auto` 语义。
- 项目已有 `web.run` 内部工具循环，已经解决同账号续跑、provider、查询上限、usage 聚合、Responses `web_search_call` 和流式缓冲问题，应扩展为共享内部 Web 工具循环。
- 现有账号/渠道 Web Search 三态和全局 provider 是唯一能力开关，不新增配置字段。
- 不按域名、账号名或模型判断 DeepSeek；协议条件相同的 Chat fallback 账号行为一致。

## 3. 决策矩阵

| 上游与请求条件 | 行为 |
|---|---|
| 原生 Responses 上游 | 保持 `native_pass`，原样转发 typed `web_search` |
| Chat fallback，纯 Web Search，策略允许 | 保持现有直接模拟 |
| Chat fallback，明确强制 Web Search，策略允许 | 保持现有强制直接模拟，不降级成 auto |
| Chat fallback，混合工具，`auto`/`required`/缺失，策略允许 | 启用内部 typed Web Search function 代理和模型驱动循环 |
| Chat fallback，`tool_choice=none` 或明确选择其它工具 | 不执行搜索，按现有桥接处理其它工具 |
| Chat fallback，可选择 Web Search，但策略关闭 | 保持明确能力 400，禁止静默丢工具 |
| 多个 typed Web Search 声明或不支持的高级字段 | 400 `invalid_request_error`，不猜测配置合并规则 |

## 4. 请求数据流

```text
Responses request
  -> Web Search 能力分类
  -> 纯/强制：现有 direct emulation
  -> 混合可选：Responses -> Chat 转换
  -> 注入内部 typed Web Search function 代理
  -> Chat 上游模型选择工具
     -> 选择其它客户端工具：按现有 Responses 回程
     -> 选择内部搜索代理：调用 websearch.Manager
        -> assistant tool_call + 同 call ID tool result
        -> 同账号、同模型继续 Chat
        -> 最终文本或客户端工具调用
  -> 标准 web_search_call + 最终 Responses 输出
```

## 5. 能力分类

扩展 `backend/internal/service/openai_responses_websearch.go` 的决策结果，区分：

- `native_pass`：上游能执行原生 Responses。
- `direct_emulation`：唯一或强制 Web Search，沿用当前直接模拟。
- `chat_tool_loop`：Chat fallback、策略允许、混合可选工具。
- `reject`：Chat fallback 无法安全保持搜索语义。

分类继续使用 `EffectiveResponsesTools`，同时读取顶层 `tools` 与 `input[].additional_tools`。typed 工具别名 `web_search`、`web_search_preview`、`web_search_20250305` 使用同一规则。

`chat_tool_loop` 只允许恰好一个 typed Web Search 声明。进入 Chat 上游前校验当前本地实现支持的配置：

- `search_context_size` 映射为每个查询的 3/5/10 条结果预算。
- `filters.allowed_domains` 与 `blocked_domains` 复用现有规范化和结果过滤。
- `max_uses` 必须大于零，并与现有最多两轮搜索限制取较小值。
- `user_location`、`external_web_access`、`return_token_budget` 等当前无等价物字段继续显式拒绝。

## 6. Chat 内部代理

在 service 层完成代理注入，不修改通用 `ResponsesToChatCompletionsRequest` 对服务端工具的默认丢弃规则，避免影响其它调用方。

- 使用固定保留名 `sub2api_web_search`。
- 注入前扫描转换后的 Chat tools；同名冲突返回明确 400，不动态改名，保证请求重放稳定。
- 参数 Schema 只包含 `search_query` 数组，每项包含非空 `q`；允许单轮 1 至 4 个查询。
- typed Web Search 的结果预算和域名策略来自原始工具配置，不允许模型覆盖客户端约束。
- 启用循环时设置 `parallel_tool_calls=false`。若上游仍同时返回内部搜索与客户端工具，沿用现有 502 防护，不能部分执行。
- `tool_choice=auto`、`required` 与缺失保持字符串/默认语义；强制 typed Web Search 已在直接模拟入口处理。

## 7. 共享搜索循环

将 `openai_responses_web_run.go` 中只与 `web.run` 名称绑定的部分抽象为内部 Web 工具循环参数：

- 内部工具名与模式：`web.run` 或 typed Web Search 代理。
- 参数解析器：`web.run` 保持现有 `search_query/response_length/recency`；typed 代理只接受 `search_query`。
- 每查询结果预算、域名过滤、最大轮次和查询总数。
- 客户端可见 `web_search_call` 构造、usage 累计、真实成功调用数和最终 writer 共用。

provider 普通失败继续作为稳定 tool result 回灌，让模型生成可诊断回答；账号代理不可用继续返回 `UpstreamFailoverError`。参数错误、空结果和失败查询不增加 `WebSearchCalls`。

不把内部代理加入 custom/namespace/tool_search 映射；模型调用内部代理时由循环消费。模型选择其它工具时，现有 Chat -> Responses 转换按客户端原始工具类型回传。

## 8. 来源与引用

搜索循环保留每次成功查询返回的已脱敏结果，构建共享来源投影：

- 按规范化 URL 首次出现顺序去重。
- 全局最多追加 5 个来源，标题和 URL 使用现有长度限制。
- 最终普通文本追加稳定后缀：`Sources:`，每行一个 `- <title>: <url>`。
- `ResponsesAnnotation` 的 rune 索引只覆盖后缀中的 URL；不解析或猜测模型原文引用。
- 非流式 writer 修改最终 assistant `output_text` 并附加 annotations。
- 流式 writer 在 `output_text.done` 前增加来源 delta 和 `response.output_text.annotation.added`，同步更新 content part、message item 和 `response.completed` 中的最终文本与 annotations。
- 没有最终文本、没有成功结果、客户端工具回程或结构化文本输出时不追加来源后缀。

来源投影和 annotation 计算只实现一次，非流式与流式 writer 共享，防止字符索引漂移。

## 9. 错误与日志

- 客户端校验错误继续使用 OpenAI 兼容 `error` 对象，参数指向 `tools` 或 `tool_choice`。
- 上游、provider、代理 failover 和客户端断连沿用现有错误链与提交状态处理。
- 能力决策日志记录 `decision`、`account_id`、`model`、`tool_choice`、`tool_count`。
- 循环完成日志记录模式、轮次、成功查询数和安全 provider 标识；不记录查询、结果、请求体或凭据。

## 10. 兼容与回滚

- 无数据库、配置、Ent、前端或 migration 变更。
- 现有 `web_search_emulation` 开关仍是即时运行时回滚手段；关闭后恢复明确 reject，不会静默丢工具。
- 代码回滚只涉及 service/apicompat 测试范围，不需要数据回滚。
- 本任务不执行生产发布；生产更新必须另行确认。

## 11. 测试矩阵

- 10 至 15 个混合工具、typed Web Search、`auto` 不再入口 400。
- 模型不选择搜索、选择客户端 function/custom/namespace/tool_search。
- 模型选择 typed 搜索，单轮/多轮、流式/非流式、缺失 call ID。
- `required`、缺失、`none`、强制其它工具、纯/强制 Web Search。
- 固定代理名冲突、重复 typed Web Search、上游违反并行约束。
- search context、domain filters、max uses、不支持高级字段。
- provider 成功/失败/空结果、代理 failover、轮次与查询上限、usage 与计费。
- 来源去重、5 条上限、Unicode rune 索引、流式 annotation 顺序、结构化输出不追加来源。
- 原生 Responses、Anthropic 原生搜索、现有 `web.run` 全量回归。
