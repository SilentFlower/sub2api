# 支持 DeepSeek 混合工具 Web Search

## Goal

让只能通过 OpenAI Chat Completions 上游转发的账号，在 Responses 请求同时声明 `web_search` 与其它客户端工具、且 `tool_choice=auto` 时，仍能由模型按需选择并真实执行 Web Search，而不是在网关入口返回 400。

## Background

- 2026-07-16 生产只读排查确认：`deepseek官渠` 账号 `3806` 为 `platform=openai,type=apikey`，配置 `openai_responses_mode=force_chat_completions`、`openai_responses_supported=false`、`web_search_emulation=enabled`。
- 全局 Web Search 已启用，provider 为 AnySearch；DeepSeek 通道同期存在大量正常 200，请求失败不是上游或 provider 故障。
- 最近 24 小时出现 36 次同类 400。失败请求均为 `deepseek-v4-pro`、`tool_choice=auto`，有效工具数为 10、11 或 15；最新失败在 2026-07-16 13:20:08，网关决策为 `reject`。
- 当前入口在 `backend/internal/service/openai_responses_websearch.go` 中只允许“唯一 Web Search 工具”或“明确强制 Web Search”进入本地模拟。混合工具 `auto` 在 Chat fallback 账号上会被明确拒绝，避免现有 Responses -> Chat 转换静默删除服务端工具。
- 当前 Responses -> Chat 转换在 `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go` 中丢弃 `web_search` 等服务端工具，并同步丢弃指向已删除工具的 `tool_choice`。
- 项目已有 `backend/internal/service/openai_responses_web_run.go` 受控搜索循环：把 `namespace=web,name=run` 暴露为 Chat function，由模型选择后调用现有 `websearch.Manager`，回灌同账号继续生成最终回答，并生成 Responses `web_search_call`、聚合 usage 与按实际成功查询数计费。
- 已归档任务 `07-14-deepseek-response-format-json-schema-compat` 明确禁止混合 `typed web_search + auto` 被旧的“直接摘要”模拟器抢先接管，但没有禁止通过模型驱动工具循环保持 `auto` 语义。

## Requirements

### R1. 路由与语义

- 仅当账号最终 Web Search 策略允许、账号实际走 Chat fallback、请求声明有效 `web_search`，且选择语义允许模型选择时，启用混合工具兼容。
- `tool_choice=auto`、`required` 或缺失时必须由上游模型按原选择语义决定是否调用搜索；网关不得仅因声明了 `web_search` 就提前执行搜索。
- `tool_choice=none` 与明确选择其它工具时不得执行 Web Search，并保持其它工具的现有转发与回程语义。
- 明确强制 `web_search` 继续保持强制语义；不得降级成 `auto`，也不得允许模型跳过搜索。
- 原生 Responses 上游、Responses -> Anthropic 原生 Web Search、纯/强制 Web Search 直接模拟及现有 `web.run` 行为不得回归。

### R2. Chat 工具代理

- 为 Chat fallback 内部生成一个无歧义、稳定的 Web Search function 代理；不得与客户端 function、custom、namespace 摊平名或 `tool_search` 代理撞名。
- 代理只声明本地 provider 能真实支持的查询参数，不伪造网页打开、点击、截图或浏览器会话能力。
- 混合请求中的其它 function、custom、namespace 和 `tool_search` 必须继续完整传给 Chat 上游；不能为支持搜索而删掉或改写其它工具 Schema。
- 上游模型未选择搜索而选择其它客户端工具时，网关必须按现有 Responses 类型回传该调用，不得吞掉或错误映射为 Web Search。

### R3. 受控搜索循环

- 优先复用现有 `web.run` 的查询校验、provider 调用、同账号续跑、轮次/查询数上限、代理 failover、usage 聚合、Responses 搜索事件和计费能力，避免建立第二套循环。
- 模型选择内部 Web Search 代理时，网关执行真实 provider 查询，把结果作为同一 call ID 的 tool result 回灌，并继续调用同一账号和模型直到得到最终文本或客户端工具调用。
- 内部搜索 function call 与 tool result 不得泄漏给客户端；客户端只能看到标准 Responses `web_search_call` 与最终 message/其它客户端工具项。
- 参数错误、provider 失败、代理不可用、超过轮次或查询上限时沿用现有可诊断错误和 failover 语义；失败搜索不得计费。

### R4. Responses 回程与流式

- 非流式与流式请求都必须保留合法 Responses 生命周期，并在真实搜索发生时输出 `web_search_call`。
- 流式客户端不得看到内部 Chat 工具轮次；搜索项必须出现在最终文本或其它客户端工具项之前，`output_index` 和 `response.completed.output` 保持一致。
- usage 必须累计所有内部模型轮次；`WebSearchCalls` 只统计真实成功完成的 provider 查询。

### R5. 可观测性与安全

- 决策日志应能区分 `direct_emulation`、`chat_tool_loop`、`native_pass` 与 `reject`，记录安全字段如账号 ID、模型、工具数量、选择类型、搜索轮次和成功查询数。
- 不记录查询全文、搜索结果正文、请求体、API Key、Authorization、Cookie 或 provider 原始响应。
- 不通过上游 400 文本自动重试，不动态修改账号能力缓存，不按 DeepSeek host/model 建立静态白名单。

### R6. 兼容范围

- 能力适用于所有满足相同协议条件的 OpenAI APIKey Chat fallback 账号，不按 DeepSeek 域名、账号名或模型建立白名单。
- 只有账号或所属渠道按现有优先级允许 Web Search 时才启用内部搜索代理；全局 provider 可用不代表自动为所有账号开放搜索。
- 非 OpenAI APIKey、非 Chat fallback、原生 Responses 和 Web Search 策略关闭的账号保持现有行为。

### R7. 来源引用

- 模型完成真实搜索并返回最终文本时，网关在模型原文末尾追加确定性来源列表，并为来源链接生成 `url_citation`。
- 来源只能取自本次成功 provider 查询的实际结果；按规范化 URL 去重并设置合理上限，不能引用 provider 失败、空结果或未执行查询。
- annotation 的字符范围只覆盖网关自己追加的来源链接，不猜测或改写模型原文中的引用位置，确保 `item_id`、`start_index` 与 `end_index` 可验证。
- 模型最终返回客户端工具调用而没有文本时，不追加来源文本或伪造 annotation；已执行搜索仍通过标准 `web_search_call` 暴露。
- 请求使用 `text.format=json_schema` 或 `json_object` 等结构化文本格式时，不追加来源文本或 annotation，避免破坏结构化输出合同；真实搜索仍通过标准 `web_search_call` 暴露。

### R8. 交付范围

- 本任务只包含代码、自动化测试、任务文档和发布说明，不部署到 `www.havefun.eu.cc`，不更新镜像，不重启容器，也不修改生产配置或数据。
- 完成后如需生产发布，必须另行展示发布步骤、影响范围、验证与回滚方案并取得确认。

## Acceptance Criteria

- [ ] 生产复现形态（10 至 15 个混合工具、`tool_choice=auto`、声明一个 `web_search`）不再在入口返回能力 400。
- [ ] 模型未选择搜索时不调用 provider、不产生 `web_search_call`、不收取 Web Search 费用，并能正常回传其它客户端工具。
- [ ] 模型选择搜索时真实调用现有 provider、按同一 call ID 回灌、续跑同一 DeepSeek 账号并返回最终回答。
- [ ] 非流式与流式回程都包含标准 `web_search_call`，不泄漏内部 function 代理，事件顺序和索引合法。
- [ ] 最终文本末尾包含去重后的真实来源列表和可点击 `url_citation`；annotation 索引精确覆盖所追加 URL，失败/未执行搜索不产生来源。
- [ ] 结构化文本输出不被来源后缀破坏；搜索成功时仍保留 `web_search_call`，但不追加普通文本引用。
- [ ] `tool_choice=none`、明确选择其它工具、明确强制 Web Search、纯 Web Search、原生 Responses、Anthropic 原生搜索和现有 `web.run` 均保持既有语义。
- [ ] 多轮模型 usage 正确累加，成功查询按实际次数计费，未执行或失败查询不计费。
- [ ] 工具名冲突、并行混合调用、查询/轮次上限、provider 失败、代理 failover 和客户端断连均有测试覆盖。
- [ ] 所有符合协议条件的 OpenAI APIKey Chat fallback 账号行为一致，不存在 DeepSeek host/model 特判；策略关闭的账号不启用搜索代理。
- [ ] 定向后端单测、相关 package 完整单测、`git diff --check` 通过。

## Out of Scope

- 让 DeepSeek OpenAI 账号按请求动态切换到 Anthropic 端点。
- 实现网页打开、点击、抓取、截图、浏览器会话、图片搜索或专用天气 API。
- 新建搜索 provider、配额、代理或计费体系。
- 修改客户端 Codex 的工具声明方式。
- 与本任务无关的 Responses/Chat 协议重构。
- 生产部署、容器重启、远端配置修改和生产数据变更。
