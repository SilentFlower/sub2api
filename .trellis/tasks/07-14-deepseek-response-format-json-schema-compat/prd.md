# OpenAI 上游 Structured Outputs 与 Web Search 能力兼容

## Goal

升级 `sub2api` 的 OpenAI Responses 与 Chat 回退能力协商：

1. 对不支持 `json_schema` 的 OpenAI-compatible APIKey 账号提供显式 `json_schema -> json_object` 兼容配置，不降级为 string，也不静默删除 Schema。
2. 让 Web Search 根据账号协议稳定执行：DeepSeek 原生搜索使用独立 Anthropic-compatible 账号；现有 OpenAI/Chat 账号可按账号配置由本地模拟器接管，不能在桥接中无提示丢弃工具。

## Background

- 线上请求从 `POST /v1/responses` 入站，账号 `deepseek官渠` 使用 `openai_responses_mode=force_chat_completions`，模型 `deepseek-v4-pro`。
- Responses `text.format.type=json_schema` 当前会正确转换为 Chat `response_format.type=json_schema`，但目标上游不支持该类型，返回 `400 This response_format type is unavailable now`。
- CPA 的 Responses->Chat 转换没有处理 `text.format`，表现为请求成功但结构化输出约束被静默删除，不作为兼容方案。
- 当前 Responses->Chat 桥会丢弃 `web_search` 等服务端工具；这会让模型输出权限提示文本，而不是真正执行搜索。
- DeepSeek 原生 Web Search 已通过官方 Anthropic-compatible 端点 `https://api.deepseek.com/anthropic/v1/messages` 暴露，普通 OpenAI Chat 工具协议没有等价入口。
- 项目已有 Brave/Tavily 搜索 Manager、Redis 配额、代理故障处理、全局/渠道/账号开关和 Anthropic 模拟输出，但尚未接入 OpenAI Responses 路径。
- new-api 的增量主要是 AnySearch MCP provider；其模拟器仍只处理纯 Web Search 并只输出 Anthropic。new-api 为 AGPLv3，本项目为 LGPLv3，不能直接复制实现。
- 2026-07-15 生产验证发现新版 Codex 在 `web_search="live"` 下不再只声明 typed `web_search`，而是通过 Responses Lite `input[].type=additional_tools` 下发 `namespace=web,name=run`，随后产生 `weather` 或 `search_query` 参数的 function call。两者在 `web.run` 协议中是不同命令；现有 typed Web Search 模拟器不会命中任一形态。
- 同次生产验证中，运行镜像已包含本任务提交，AnySearch 配置为 enabled 且从宿主机、应用容器都可真实搜索成功；`deepseek官渠` 账号为 `web_search_emulation=enabled`。对应五次 `/v1/responses` 请求全部返回 200，但没有模拟器决策/执行日志，Codex 会话也没有对应 `function_call_output`，因此问题属于协议执行缺口，不是 provider 或服务器网络故障。

## Requirements

### R1. 账号与渠道配置

- JSON Schema 配置使用 `accounts.extra.openai_json_schema_to_json_object=true`，仅对 `platform=openai && type=apikey` 生效；缺失、false、类型错误或其它账号保持原行为。
- Web Search 复用现有 `accounts.extra.web_search_emulation=default|enabled|disabled`：
  - `enabled`：满足安全接管条件时本地模拟优先。
  - `disabled`：绝不本地模拟。
  - 缺失/`default`：跟随渠道 `features_config.web_search_emulation.<platform>`；渠道字段缺失或不是严格布尔值 `true` 时按关闭处理。
- 全局 Web Search 开关只控制 provider 能力是否可用，不改变账号策略；单独开启全局设置不会自动开启任何账号。
- Web Search 三态扩展到 OpenAI APIKey 账号；渠道配置扩展 `openai` 平台，不创建含义重复的新键。
- 配置默认不改变任何存量账号行为，保存时只增删任务拥有的键并保留其它 `extra`。
- 不新增数据库列或 migration。

### R2. JSON Schema 兼容

- 配置关闭时完整保留 Responses `text.format` 和 Chat `response_format`。
- 配置开启且格式为 `json_schema` 时：
  - Responses 请求改为 `text.format={"type":"json_object"}`。
  - Chat 请求改为 `response_format={"type":"json_object"}`。
  - 原 Schema 作为不可信数据附加到 instructions/system 约束中，要求只输出一个 JSON object，并明确不保证 strict schema。
- 覆盖 `/v1/responses` 原生/透传/强制 Chat 回退，以及直接 `/v1/chat/completions` 两类入站。
- 已是 `json_object`、text、格式缺失、Schema 缺失/非法或未知格式时不伪装成已支持类型。
- 不修改 function/tool 参数 Schema，不实现 CPA 式静默删除。
- 不匹配 400 文本自动重试，不缓存账号或模型能力推断；未配置账号的上游错误沿原错误链返回。
- 日志只记录账号 ID、模型、端点和 `json_schema -> json_object` 决策，不记录 Schema、请求体或凭据。

### R3. Web Search 路由边界

- 不在同一个 OpenAI DeepSeek 账号内动态切换 `/chat/completions` 与 `/anthropic/v1/messages`。
- DeepSeek 原生搜索通过独立 Anthropic APIKey 账号接入，管理员显式配置 `base_url=https://api.deepseek.com/anthropic`；整条账号链固定使用现有 Responses->Anthropic->Responses 路径。
- OpenAI DeepSeek 账号保持现有 Responses/Chat 路径；符合条件的 Web Search 在进入 `force_chat_completions` 分支前由本地模拟器接管。
- typed `web_search` 本地模拟只接管：
  - 工具列表中只有一个 Web Search；或
  - `tool_choice={"type":"web_search"}` 明确要求必须搜索。
- typed `web_search` 与其它工具混合且 tool choice 为 auto、required、缺失或指向其它工具时不接管。若实际目标只能走 Chat，则返回明确能力错误，不继续进入会丢弃 Web Search 的桥接；R9 的 `web.run.search_query` 受控循环是独立例外。
- `tool_choice={"type":"web_search"}` 保持强制语义：模拟模式真实执行搜索；原生 Anthropic 路径转换为 `{"type":"tool","name":"web_search"}`；无法执行时返回错误，不改成 `auto`。
- 本地模拟与 `text.format=json_schema` 同时出现时返回明确冲突错误，因为本地搜索摘要无法满足任意输出 Schema。
- `web_search` 与 `tool_search` 完全独立，不互相转换。

### R4. 搜索 Provider

- 复用现有 `websearch.Manager`、Brave/Tavily provider、Redis 配额、provider 选择、代理和失败回滚，不建立平行体系。
- 在现有 Manager 内独立实现 AnySearch-compatible provider：使用 MCP JSON-RPC `tools/call`、工具名 `search`，支持 query 与 max_results。
- AnySearch 响应兼容结构化 results 和 MCP text block 内嵌 JSON；响应体有限长，错误内容截断且不得包含 API Key。
- AnySearch 配置进入现有全局 provider 配置与管理页；账号只决定是否接管并复用账号代理，不保存 provider 凭证。
- 不复制 new-api 的 AGPLv3 源码。

### R5. Responses 请求与回程

- 本地模拟从最后一条用户输入提取查询；无法提取时返回 OpenAI-compatible `invalid_request_error`。
- 本地模拟非流式响应包含：
  - `web_search_call`，记录真实查询与 completed 状态。
  - assistant message/output_text 搜索摘要。
  - 每个来源对应可点击的 `url_citation`，索引与最终文本一致。
- 本地模拟流式响应输出合法 Responses SSE 生命周期：response created、搜索 item、message/content、text delta/done、completed 和终止标记；不得伪装成 function call。
- 原生 Responses->Anthropic 请求支持 `web_search`、`web_search_preview` 和强制 tool choice，统一映射为基础 `web_search_20250305`。
- 原生回程把 `server_tool_use` 转为 `web_search_call`，把 Anthropic text citations 转为 Responses `url_citation`；非流式与流式都必须保留搜索事件和引用。
- 对有明确等价物的 domain filter、approximate user location 做结构化映射；`search_context_size`、`external_web_access`、`return_token_budget` 等无等价物字段在原生桥接中明确拒绝，不静默丢失。
- 搜索工具结果错误必须转换为可诊断的 Responses 错误/状态，不把失败搜索伪装成 completed。

### R6. 计费、错误与可观测性

- OpenAI Responses 本地模拟搜索成功返回 `OpenAIForwardResult.WebSearchCalls=1`，复用分组 `web_search_price_per_call`；未配置单价时使用现有默认 `$0.01/次`。
- 搜索 provider 失败、请求校验失败或响应未成功写完时不计费。
- 账号代理不可用继续使用现有 `UpstreamFailoverError` 触发换号；全局 provider 都失败时返回稳定的 OpenAI-compatible 错误。
- 记录 `native|compat|emulation|reject` 决策、账号 ID、模型、provider 与结果数量，不记录查询全文、Schema、搜索摘要、凭据或完整请求体。
- 不改变现有 error envelope、failover 和 usage 记录入口。

### R7. 管理端

- OpenAI APIKey 创建/编辑页展示“JSON Schema 兼容降级”和 Web Search 模拟三态。
- JSON Schema 文案明确：兼容模式只保证 JSON object，不保证 strict schema。
- Web Search 文案明确：账号 `enabled` 会在安全条件下本地接管；原生 DeepSeek 搜索应创建 Anthropic-compatible 账号。
- 渠道 OpenAI 平台展示 Web Search 模拟默认开关。
- 全局搜索 provider 支持 AnySearch 选项，并保持 API Key 脱敏与已有配置非破坏性更新。

### R8. 测试

- JSON Schema：Responses/Chat 原生、兼容、非法输入、未知类型、直接 Chat、Responses->Chat、passthrough、工具 Schema 不变。
- 模拟搜索：账号/渠道三态、纯搜索、强制搜索、混合自动拒绝、结构化输出冲突、查询提取、provider 失败、代理换号。
- Responses 回程：非流式/流式 `web_search_call`、SSE 顺序、citation 索引、终止事件、失败不计费、成功按次计费。
- 原生 DeepSeek：工具/choice 转换、server tool/result/citation 回程、无等价字段拒绝、现有 function/custom/namespace/tool_search 回归。
- AnySearch：请求格式、可选鉴权、结构化响应、text JSON、超大响应、脱敏错误、Manager 配额回滚。
- 前端：账号可见性、三态/布尔键保存、`extra` 保留、渠道 OpenAI 开关、AnySearch 配置与类型检查。

### R9. 新版 Codex `web.run` 兼容

- 对 OpenAI APIKey 账号识别顶层 tools 与 `input[].additional_tools` 中的 `namespace=web`、子工具 `name=run`；普通 namespace、function、custom、tool_search 和 typed Web Search 行为保持不变。
- 只有账号最终 Web Search 策略允许时才执行 `web.run`；全局 Web Search 仅提供 provider 能力，不能单独把任何账号从关闭状态变成开启状态。
- 进入 Chat fallback 后，只向上游模型暴露 `web.run.search_query` 搜索能力；不暴露 `weather`、`open`、`click`、`find`、`screenshot`、`image_query`、`finance` 或 `sports`。天气类用户问题由模型组织为普通 `search_query[].q`，网关不维护独立天气规则。
- 当 Chat fallback 模型返回 `namespace=web,name=run` 且参数包含 `search_query` 时，网关执行受控工具循环：校验查询、调用现有 `websearch.Manager`、构造与原 call ID 对应的工具结果、回灌同一上游账号并继续到最终模型回答。
- `search_query` 是非空数组，每项必须包含非空 `q`；兼容生产已出现的可选 `recency` 和顶层 `response_length`。`response_length` 只控制返回给模型的结果预算；provider 无法严格执行 `recency` 时必须在工具结果中明确说明，不能宣称已经精确过滤。
- 单个请求最多执行 4 个查询、最多 2 轮搜索工具调用；超过限制或只有未支持命令时返回稳定工具错误，不调用 provider，也不能形成无限循环。
- provider、参数或续跑失败必须产生可诊断错误，不能让模型在缺少 tool output 的情况下反复猜测“网络中断”；日志仍不得记录查询全文、结果正文、凭据或完整请求体。
- 成功计费必须累计工具循环内各模型请求的 token，并按实际成功执行的查询数累计 Web Search 调用次数；没有真实执行搜索的 function call 不得收取 Web Search 按次费用。
- 流式客户端仍只看到合法 Responses 生命周期；namespace/name、call ID、output index 和最终文本顺序必须保持 Codex 可消费。
- 系统设置、渠道和账号管理端文案必须明确实际优先级与未配置时的安全默认；账号默认选项显示为“跟随渠道（渠道未配置时关闭）”，避免把“全局 provider 已启用”误解为“所有账号已启用”。

## Acceptance Criteria

- [ ] 开启 JSON Schema 兼容的 OpenAI APIKey 账号只发送 `json_object`，并保留 Schema 的 best-effort 约束；从不降级成 string。
- [ ] 配置关闭或缺失时保持原请求，不自动重试或写入能力缓存。
- [ ] OpenAI DeepSeek 账号不会动态切换 Anthropic 端点；独立 Anthropic APIKey 账号可使用 DeepSeek 原生 Web Search。
- [ ] 纯/强制 Web Search 可本地接管；混合自动请求不会被误接管或进入静默丢工具路径。
- [ ] 本地模拟的非流式和流式响应包含真实 `web_search_call`、可用 URL citations 和完整 Responses 生命周期。
- [ ] 强制 Web Search 不会被改成 auto；成功表示实际搜索，失败返回明确错误。
- [ ] AnySearch 通过现有 Manager 参与配额、代理、失败回滚和脱敏配置。
- [ ] typed Web Search 成功模拟按分组单次价格计费一次；`web.run.search_query` 按真实成功查询数计费；失败调用不计费。
- [ ] 原生 DeepSeek 回程保留 server search call 与 citations；无等价能力字段不被静默丢弃。
- [ ] 账号/渠道/全局管理配置默认不改变存量行为，保存不覆盖无关字段。
- [ ] 新版 Codex `web.run.search_query` 会真实执行搜索、按原 call ID 回灌工具结果并得到最终模型回答；天气类问题通过普通搜索词完成，不再出现无 tool output 的重复调用。
- [ ] 模拟策略开启时，`web.run` 的上游 Chat 工具定义只声明 `search_query`；`weather/open/click/find` 等未支持命令不会被执行或伪装成成功。
- [ ] 开启全局 provider 不会隐式开启任一账号；账号、渠道均未显式启用时 Web Search 保持关闭。
- [ ] 定向测试、后端完整单测、前端测试/typecheck/lint 和 `git diff --check` 通过。

## Out of Scope

- 同一 OpenAI 账号按请求动态切换 Chat 与 Anthropic 上游。
- 除 `namespace=web,name=run` 外的通用混合工具自动执行循环。
- `web.run` 的 `weather`、`open`、`click`、`find`、`screenshot`、`image_query`、`finance`、`sports` 等非 `search_query` 命令，以及浏览器、网页抓取和页面会话能力。
- OpenAI Chat `web_search_options`、厂商私有 Chat 搜索字段或没有公开契约的第三方 adapter。
- 与 OpenAI 等价的 strict JSON Schema 验证器，或根据 400 自动探测/缓存能力。
- Anthropic `pause_turn` 自动续跑、动态代码过滤和 `web_search_20260209+` 新能力。
- 新建平行搜索 provider、配额、代理或计费系统。
- 与本任务无关的 HA、调度、故障切换和计费架构重构。

## Evidence

- OpenAI Tools：<https://developers.openai.com/api/docs/guides/tools>
- OpenAI Web Search：<https://developers.openai.com/api/docs/guides/tools-web-search>
- OpenAI Structured Outputs：<https://developers.openai.com/api/docs/guides/structured-outputs#function-calling-vs-response-format>
- DeepSeek Anthropic API：<https://api-docs.deepseek.com/zh-cn/guides/anthropic_api>
- DeepSeek V4 Pro 强制搜索问题：<https://github.com/deepseek-ai/awesome-deepseek-agent/issues/70>
- DeepSeek 原生搜索实际调用：<https://github.com/deepseek-ai/DeepSeek-V3/issues/1487>
- Anthropic Web Search：<https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/web-search-tool>
- 代码与 new-api 对照见同目录 `research.md`。
