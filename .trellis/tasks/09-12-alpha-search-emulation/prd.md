# Codex Alpha Search 上游 web_search 桥接：账号级开关让 /v1/alpha/search 改经上游 Responses 执行，未真正搜索时自动改走本地模拟

## Goal

Codex（Responses Lite + code mode，如 `gpt-6-astra`）经 sub2api 使用 DeepSeek 等 OpenAI 兼容上游时，模型在 `exec` 里调用的 `tools.web__run` 联网搜索能够拿到真实结果，而不是像现在一样 `/v1/alpha/search` 全部 404/502。

用户价值：给 API Key 账号一个"Alpha Search 经上游 Responses 执行"的开关；开启后 sub2api 把 Codex 的独立搜索翻译成带 `web_search` 工具的 Responses 请求交给上游托管搜索执行，上游不真正搜索时自动改用本地 Web Search Emulation 供应商兜底。配合既有"Responses Lite 降级"开关，Lite 请求可以走 Responses 到上游并保留搜索能力。

## Background / Confirmed Facts

### Codex 侧机制（源码 0.154，已核对）
- `gpt-6-astra` 目录元数据：`use_responses_lite: true`、`tool_mode: code_mode_only`、`web_search_tool_type: text_and_image`。
- 工具规划器对 Lite 模型跳过全部托管工具（`hosted_model_tool_specs`：Lite 只接受客户端执行工具），因此 `{"type":"web_search"}` 永远不会出现在 `/v1/responses` 请求里；服务器日志 `explicit_web_tool=false` 已证实。
- 独立搜索启用条件：命名空间工具可用 且 供应商 `web_search` 能力（默认 true） 且（模型是 Lite 或开启 `standalone_web_search` 特性）。Lite 模型下用户配置里的 `standalone_web_search = true` 是多余的。
- 独立搜索扩展注册条件：供应商 `is_openai()`（name 为 `OpenAI`）或配置了 `x-openai-actor-authorization` 头或 `supports_standalone_web_search`；且 `web_search != disabled`。用户 provider 名为 `OpenAI` 且配了该头。
- code mode 下 `web.run` 暴露为 `tools.web__run`，模型调用后 Codex `SearchClient` 向 `{base_url}/alpha/search` 发 POST。`web_search = "disabled"` 只会移除该工具，不会退回托管搜索。
- 请求体（`codex-rs/codex-api/src/search.rs`）：`id`、`model`、`reasoning?`、`input?`（最近对话）、`commands`（`search_query[{q,recency?,domains?}]`、`image_query`、`open[{ref_id,lineno?}]`、`click`、`find`、`screenshot`、`finance`、`weather`、`sports`、`time`、`response_length?`）、`settings`（`user_location`、`search_context_size`、`filters{allowed_domains,blocked_domains}`、`external_web_access`…）、`max_output_tokens`。
- 响应体：`{"output": string, "results"?: [opaque JSON], "encrypted_output"?: string}`。Codex 只把 `output` 作为纯文本 `function_call_output` 回给模型；`results` 用于 UI/历史，保持不透明。
- 会话 `01a0917d-65de-7ef3-8a4a-ab697abf184b` 实录：三次 `web__run`（含 `search_query` 与 `open`）均收到 `http 502 Bad Gateway`，模型改用 curl 硬抓。

### sub2api 侧现状
- `/v1/alpha/search` 由 `backend/internal/handler/openai_alpha_search.go` 调度（仅 OpenAI/Composite 分组，调度平台 `PlatformOpenAI`，国产平台账号不会被选中）、`backend/internal/service/openai_alpha_search.go` `ForwardAlphaSearch` 转发；API Key 账号目标为 `{base_url}/v1/alpha/search`（`openAIAlphaSearchURL`，`:678`）。
- 上游 404/405 时 `isOpenAIAlphaSearchEndpointUnsupported`（`:538`）触发换号；单账号场景换号耗尽后对外 502。服务器日志：01:03–01:07 共 105 次、01:21–01:23 共 60 次换号失败。
- PAT 账号已有 `forwardAlphaSearchViaResponsesWebSearch`（`:142`）：`buildOpenAIAlphaSearchResponsesWebSearchBody`（`:292`）把 `commands`/`settings`/`input` 拼入提示词并挂 `web_search` 工具，`buildOpenAIAlphaSearchResponsesWebSearchRequest`（`:232`）固定发往 `chatgptCodexURL` 并带 ChatGPT 专属头，`openAIAlphaSearchResponseFromResponsesSSE`（`:557`）从 SSE 收集 `output_text.delta` 与 `url_citation` 产出 `{"output","results"}`，`results` 元素 `{type:"text_result", ref_id:"turn0searchN", url, title}`。
- DeepSeek `/responses` 实测（账号 Key，`tools:[{"type":"web_search"}]`）：输出仅 `reasoning` + `message`，无 `web_search_call`/`url_citation`，模型自述无法获取实时信息；官方文档中英文四页均写"内置工具忽略、tools 仅 function"。
- Web Search Emulation 现成能力：`doWebSearchWithMaxResults(ctx, account, query, maxResults)`（`gateway_websearch_emulation.go:188`）经 `websearch.Manager.SearchWithBestProvider` 调用 Brave/Tavily/AnySearch，返回 `[]SearchResult{URL,Title,Snippet,PageAge}`；`s.openAIWebSearchExecutor`（`openai_gateway_service.go:499`）可注入替代执行器；`filterOpenAIResponsesSearchResults(results, allowed, blocked)`（`openai_responses_websearch.go:542`）做域名过滤；`search_context_size` low/medium/high → 3/5/10（`:478`）。
- 模拟资格判定现成模式（`codex_web_search_bridge.go:108-120`）：账号 `GetWebSearchEmulationMode()` enabled/disabled，default 跟随渠道 `IsWebSearchEmulationEnabled(PlatformOpenAI)`；再要求系统设置 `IsWebSearchEmulationEnabled` 且 `manager.HasAvailableProvider(proxy)`。
- 计费：alpha/search 成功按 `WebSearchCalls=1` 走分组 `web_search_price_per_call` 按次计费（`openai_gateway_usage.go:565`）；Responses 模拟搜索路径同样返回 `WebSearchCalls: 1`（`openai_responses_websearch.go:441`）。
- 账号级开关既有模式：`accounts.extra` 布尔键 + `Account` 访问器（`openai_responses_lite_downgrade.go`）；前端 `features/responsesLite/extra.ts`（key 常量、`supports*`、`read*`、`apply*Extra`）+ Toggle 组件，接入 `EditAccountModal.vue`（`:1868` 模板、`:3620` computed、`:3939` 读取、`:5549` 保存）与 `CreateAccountModal.vue`（`:3318` 模板、`:4418` computed、`:4846` 平台切换重置、`:5462` 保存构造）；文案 `locales/{zh,en}/admin/accountsResponsesLite.ts` spread 进 `accounts.ts`（zh `:706` / en `:623`），`buildFeatureLocaleExtensions.spec.ts:49` 做中英文配对。
- 规范：`protocol-adapter-guidelines.md` L923-1047 "Scenario: Codex Alpha Search 独立端点转发"要求 alpha 请求作为不透明 JSON 转发、仅 2xx 计费、failover 先于写响应；`build-private-feature-isolation-guide.md` 要求 build 私有逻辑放独立领域文件，共享入口只做薄调用。
- 用户账号现状：id=1 `deepseek`，`openai` 平台 API Key，base_url `https://api.deepseek.com`，`web_search_emulation: enabled`，`openai_responses_mode: force_chat_completions`。

## Requirements

- R1 账号级开关：`accounts.extra` 布尔键 `openai_alpha_search_via_responses`，默认关闭；访问器 `IsOpenAIAlphaSearchViaResponsesEnabled()` 仅对 `platform=openai` 且 `type=apikey` 返回真（国产平台账号进不了 alpha 调度，OAuth/PAT 保持既有行为）。缺失、非布尔、false 视为关闭。
- R2 开关开启时的主路径：`ForwardAlphaSearch` 对 API Key 账号不再请求上游 `alpha/search`，而是复用 PAT 路径的请求体构造与 SSE 解析，把 alpha 请求翻译成带 `web_search` 工具的流式 Responses 请求，发往账号 base_url 派生的 Responses 端点（`buildOpenAIResponsesURLForPlatform`，无 base_url 时用官方端点），Bearer 鉴权、账号 header 覆盖与代理，不带 ChatGPT 专属头与 Lite 头。
- R3 真实搜索判定：上游 2xx 且 SSE 中出现 `web_search_call` 输出项，或收集到至少一条 `url_citation`，视为上游已搜索：写回 `{"output","results"}`，`WebSearchCalls=1`，`UpstreamEndpoint=/v1/responses`。
- R4 本地模拟兜底：上游 2xx 但无真实搜索证据，或上游非 2xx 时，若账号具备模拟资格（R6），改用本地供应商执行并写回；模拟不可用时，2xx-未搜索返回 502 `web_search_failed` 不计费，非 2xx 沿用 PAT 路径既有的 failover / 透传分类。
- R5 本地模拟执行：只执行 `commands.search_query`（`image_query` 视同文本搜索），最多 4 条查询，每条按 `search_context_size` low/medium/high → 3/5/10（默认 5）取结果；`search_query[].domains` 与 `settings.filters.allowed_domains` 作为允许域名、`settings.filters.blocked_domains` 作为拒绝域名过滤；跨查询按 URL 去重；`output` 为模型可读纯文本（每条含 `turn0searchN` 标记、标题、URL、摘要、日期），`results` 为 `{type:"text_result", ref_id, url, title}` 数组；`max_output_tokens` 存在时按 4 字符/token 截断 `output`。`open`/`click`/`find`/`screenshot`/`finance`/`weather`/`sports`/`time` 不执行，在 `output` 末尾附一段说明，引导模型使用已有结果或自行抓取。只含不支持命令且无 `search_query` 时返回 200 说明文本，result 为 nil 不计费；供应商全部失败返回 502 `web_search_failed` 不计费；部分成功按成功结果返回并计费。
- R6 模拟资格：与 `resolveCodexWebSearchBridgeDecision` 同一判定：账号/渠道 Web Search Emulation 开启、系统设置开启、存在可用供应商；执行器优先取 `s.openAIWebSearchExecutor`。
- R7 开关关闭：所有账号行为与现状完全一致（含 API Key 404 换号、PAT 自动兜底）。
- R8 前端：`features/alphaSearch/extra.ts`（key 常量、`supportsAlphaSearchViaResponses(platform,type)`、`readAlphaSearchViaResponses`、`applyAlphaSearchViaResponsesExtra`）与 `AlphaSearchViaResponsesToggle.vue`（`data-testid="alpha-search-via-responses-toggle"`），按 Lite 降级开关模式接入 Edit/Create 账号弹窗（仅 `openai` + `apikey` 显示，平台切换重置，保存写入 extra）；文案 `locales/{zh,en}/admin/accountsAlphaSearch.ts` spread 进 `accounts.ts` `openai` 段并加入配对测试。
- R10 AnySearch 文本结果解析（返工，实测发现）：AnySearch MCP 实际返回 `content[{type:"text"}]` 的 Markdown 文本（`## Search Results (N results, …)` + `### N. 标题` + `- **URL**: <url>` + `- 摘要 … date: <日期>`），`backend/internal/pkg/websearch/anysearch.go` 的文本回退只认 JSON，导致退化成一条无 URL 的整段文本。需在文本回退中先按该格式解析为结构化 `SearchResult{URL,Title,Snippet,PageAge}`，解析不到再退化为原来的单条文本结果。
- R11 无 URL 结果保留（返工）：alpha 本地模拟的跨查询去重只对有 URL 的结果按 URL 去重，无 URL 的文本结果保留并进入 `output`，`results` 条目省略 `url` 字段；避免供应商退化结果被整体丢弃成"零结果"。
- R9 领域隔离与文档：后端逻辑放 build 私有文件 `openai_alpha_search_responses_bridge.go`（开关、上游翻译、证据判定、兜底编排）与 `openai_alpha_search_emulation.go`（资格、命令解析、供应商执行、输出格式化），`ForwardAlphaSearch` 只增加一次薄调用；`protocol-adapter-guidelines.md` 在 Alpha Search scenario 后新增本 scenario 并在既有 contracts 中标注例外。

## Acceptance Criteria

- [ ] AC1（R1）：访问器对 `openai apikey + extra true` 为真；OAuth、PAT、deepseek 平台、缺失/非布尔/false 均为假。
- [ ] AC2（R2/R3）：开关开启 + base_url `https://relay.example` + 上游返回含 `url_citation` 的 SSE：客户端收到 200 `{"output":"…","results":[{"type":"text_result","ref_id":"turn0search0","url":"https://example.com/news","title":"Example News"}]}`；上游 URL 为 base_url 派生的 Responses 端点，`Authorization: Bearer sk-test`，无 `ChatGPT-Account-ID`、无 Lite 头，body `tools.0.type=web_search`、`stream=true`、`store=false`；result `WebSearchCalls=1`、`UpstreamEndpoint=/v1/responses`。
- [ ] AC3（R3）：上游 SSE 只含 `web_search_call` 输出项、无 `url_citation` 时同样判定为已搜索并返回 200。
- [ ] AC4（R4/R5）：上游 2xx 但无搜索证据、模拟资格满足（注入 `openAIWebSearchExecutor`）：客户端收到 200，`output` 含供应商结果标题与 URL 和 `turn0search0` 标记，`results[0].ref_id=turn0search0`，`WebSearchCalls=1`。
- [ ] AC5（R4）：上游 404 且模拟资格满足 → 同 AC4 走模拟并计费；上游 404 且模拟不可用 → 返回 `UpstreamFailoverError{404}`，下游未写入，与现状一致。
- [ ] AC6（R4）：上游 2xx 无搜索证据且模拟不可用 → 502 JSON `error.code=web_search_failed`（沿用 Responses 模拟搜索路径的错误形态，`error.type=api_error`），result 为 nil。
- [ ] AC7（R5）：两条 `search_query` 返回重复 URL 时去重；`filters.blocked_domains` 命中的结果被剔除；`search_context_size=low` 时执行器收到 `maxResults=3`；`ref_id` 连续编号。
- [ ] AC8（R5）：只含 `open` 的请求返回 200，`output` 含不支持说明，`results` 为空，result 为 nil；`search_query` 与 `open` 并存时结果正常返回且说明附在末尾。
- [ ] AC9（R5）：执行器对所有查询返回错误 → 502 `web_search_failed`，result 为 nil；部分查询失败 → 按成功结果返回并 `WebSearchCalls=1`。
- [ ] AC10（R7）：开关关闭时既有 `TestForwardAlphaSearch*` 全部不变通过。
- [ ] AC11（R8）：`extra.ts` 单测覆盖写入/删除/保持与 `supports*` 边界；Toggle 组件渲染与 v-model；Edit/Create 弹窗在 `openai apikey` 显示、`deepseek apikey` 与 `openai oauth` 不显示，保存 payload `extra.openai_alpha_search_via_responses=true`，关闭后删除该键且保留其它 extra；中英文 key 成对且非空。
- [ ] AC13（R10）：AnySearch 返回上述 Markdown 文本时，`Search` 结果为逐条结构化项，`URL`/`Title`/`Snippet`/`PageAge` 正确；非该格式的纯文本仍退化为单条 `Title:"AnySearch"` 结果；既有 JSON 形态用例不变。
- [ ] AC14（R11）：执行器返回一条无 URL 与一条有 URL 的结果时两条都保留，`output` 含两条文本，`results` 有 2 项且无 URL 项不含 `url` 键；有 URL 的重复项仍被去重。
- [ ] AC12（R9）：`gofmt -l` 为空；`go test -tags=unit ./internal/service ./internal/handler -run 'AlphaSearch|WebSearch' -count=1` 通过；前端 `pnpm vitest run src/features/alphaSearch src/components/account/__tests__/EditAccountModal.spec.ts src/components/account/__tests__/CreateAccountModal.spec.ts src/i18n/__tests__/buildFeatureLocaleExtensions.spec.ts` 与 `pnpm typecheck` 通过。

## Out of Scope

- 开关关闭时对上游 404 自动模拟（用户选择显式开关）。
- 向主对话 `/v1/responses` 注入 `web_search` 工具；Anthropic 协议路径。
- `open`/`find` 等网页抓取类命令的真实实现（本次只给说明）。
- Codex 目录侧压制 `use_responses_lite`；BulkEditAccountModal 批量设置。
- 上一轮发现的"chat 回退 + Lite 降级开关"回程缺陷（`openai_gateway_forward.go:158`），另行处理。
