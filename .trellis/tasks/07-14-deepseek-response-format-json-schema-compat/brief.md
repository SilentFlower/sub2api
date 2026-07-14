# Brief — OpenAI 上游 Structured Outputs 与 Web Search 能力兼容

## 目标

- 升级 OpenAI Responses 与 Chat 回退的能力协商：为显式配置的账号提供 `json_schema -> json_object` 兼容，并让 DeepSeek Web Search 通过独立原生账号或账号级本地模拟真实执行，禁止把结构化输出或搜索工具静默丢掉。

## 范围

- 为 OpenAI APIKey 账号增加 `openai_json_schema_to_json_object` 布尔配置；开启后将 Responses/Chat 的合法 `json_schema` 转为 `json_object`，并把原 Schema 注入 best-effort instructions/system 约束，保留未知字段和 function/tool Schema。
- 复用 `web_search_emulation=default|enabled|disabled`，扩展到 OpenAI APIKey 账号及渠道 OpenAI 默认开关；在 Responses 转 Chat 前完成 pass、emulation 或 reject 决策。
- 本地模拟仅接管纯 Web Search 或明确强制 Web Search；复用现有 `websearch.Manager`、Brave/Tavily、账号代理、Redis 配额、失败回滚和按次计费。
- 独立实现 AnySearch-compatible MCP provider，接入现有全局 provider 配置和管理页，不复制 new-api 的 AGPLv3 代码。
- 为本地模拟生成完整的 Responses 非流式/流式 `web_search_call`、搜索摘要和 `url_citation`。
- 扩展现有 Responses -> Anthropic -> Responses 桥，支持 DeepSeek 原生 `web_search_20250305` 的请求字段、强制选择、`server_tool_use`、搜索结果错误和 citations。
- 更新 OpenAI APIKey 账号页、渠道配置页、全局 provider 配置、类型与测试；不新增数据库列或 migration。

## 非目标

- 不让同一个 OpenAI 账号按请求在 Chat 与 Anthropic 端点间动态切换；DeepSeek 原生搜索使用独立 Anthropic APIKey 账号和 `https://api.deepseek.com/anthropic`。
- 不实现混合工具 auto/required 场景的完整模型工具循环；Chat 回退不能等价处理时明确拒绝。
- 不支持 OpenAI Chat `web_search_options`、厂商私有 Chat 搜索字段或没有公开契约的第三方 adapter。
- 不实现 strict JSON Schema 验证器、基于 400 的自动重试或能力缓存，也不把 Schema 降成 string 或静默删除。
- 不实现 Anthropic `pause_turn` 自动续跑、动态代码过滤和更新版搜索工具能力。
- 不重构现有账号选择、HA、配额、代理或计费架构。

## 关键上下文

- OpenAI Responses 热路径位于 `OpenAIGatewayService.Forward`；Web Search 决策必须发生在 `force_chat_completions` 丢弃服务端工具之前，且配置关闭时不改变存量行为。
- JSON 兼容 helper 使用 raw JSON 保留未知字段并保持幂等；仅合法 Schema object 才转换，非法输入沿现有错误链处理。
- 本地模拟与 `text.format=json_schema` 冲突时返回 400；混合 auto Web Search 走 Chat fallback 时返回能力错误，不伪造成功。
- 原生 DeepSeek 搜索固定走现有 `GatewayService.ForwardAsResponses` 和 `/v1/messages`；没有等价物的 `search_context_size`、`external_web_access`、`return_token_budget` 在原生桥中明确拒绝。
- Responses SSE 需要保持 ID、sequence、output/content index 和结束事件一致；原生回程必须将 `server_tool_use` 映射为 `web_search_call`，将 text citations 映射为合法 `url_citation`。
- 本地搜索成功返回 `WebSearchCalls=1`，复用 `web_search_price_per_call`，默认 `$0.01/次`；provider、校验或响应写入失败不计费。
- 日志不得记录 Schema、查询全文、摘要、结果正文、凭据或完整请求体；AnySearch 响应限长且错误内容脱敏。
- 高风险文件包括 OpenAI 转发/Chat fallback、Anthropic->Responses 流状态机、共享协议 DTO、Web Search Manager，以及账号/渠道/全局设置页面。

## 验收标准

- 开启兼容的 OpenAI APIKey 账号只向不支持 Schema 的上游发送 `json_object`，同时保留原 Schema 的 best-effort 约束；关闭或缺失配置时请求保持原样。
- 纯/强制 Web Search 能由本地模拟真实执行，混合自动请求不会误接管，也不会进入静默丢工具路径；强制搜索失败时返回明确错误而不是改成 auto。
- 独立 Anthropic APIKey 账号可通过 DeepSeek 官方 Anthropic-compatible 端点执行原生搜索；请求和非流式/流式回程保留 search call、失败状态与 citations。
- 本地 Responses 响应包含真实 `web_search_call`、索引正确的 URL citations 和完整 SSE 生命周期。
- AnySearch 通过现有 Manager 参与 provider 选择、配额、代理和失败回滚；成功模拟搜索计费一次，失败不计费。
- 账号、渠道和全局配置默认不改变存量行为，保存时保留无关 `extra` 与旧配置。
- JSON Schema、路由矩阵、AnySearch、DeepSeek 原生桥、Responses writer、计费和管理端测试通过；后端单测、前端测试/typecheck/lint 及 `git diff --check` 通过。

## 下一步

- 用户确认本 brief 和规划三件套后，运行 `task.py start .trellis/tasks/07-14-deepseek-response-format-json-schema-compat` 激活任务；进入实现阶段先执行 `trellis-route(implement)`，再按 `implement.md` 分阶段编码与验证。
