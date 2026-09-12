# Design: Codex Alpha Search 上游 Responses web_search 桥接与本地模拟兜底

## 1. 边界与文件所有权

| 文件 | 角色 | 改动性质 |
|---|---|---|
| `backend/internal/service/openai_alpha_search_responses_bridge.go`（新建） | build 私有领域 owner：开关常量与访问器、API Key 版 Responses 请求构造、真实搜索证据判定、上游结果/兜底编排 | 新增 |
| `backend/internal/service/openai_alpha_search_emulation.go`（新建） | build 私有领域 owner：模拟资格、alpha `commands`/`settings` 解析、供应商执行、`output`/`results` 格式化、不支持命令说明 | 新增 |
| `backend/internal/service/openai_alpha_search.go` | 上游共享入口 | 只在 PAT 分支之后增加一次薄调用；`parseOpenAIResponsesSSEForAlphaSearch` 扩展返回 `web_search_call` 证据（内部函数，签名变更由本包吸收） |
| `frontend/src/features/alphaSearch/extra.ts`、`AlphaSearchViaResponsesToggle.vue`、`__tests__/` | build 私有前端领域 | 新增 |
| `frontend/src/components/account/{Edit,Create}AccountModal.vue` | 共享入口 | 各加 1 个 import、1 个 ref、1 个 computed、1 个模板块、1 处重置/读取、1 处保存写入 |
| `frontend/src/i18n/locales/{zh,en}/admin/accountsAlphaSearch.ts` + `accounts.ts` spread + `buildFeatureLocaleExtensions.spec.ts` | 文案 | 新增文件，共享文件各 2 行 |
| `.trellis/spec/backend/protocol-adapter-guidelines.md` | 规范 | 新增 scenario，既有 Alpha Search contracts 加一条例外 |

## 2. 数据流

```
Codex exec → tools.web__run → POST /v1/alpha/search（Codex SearchClient）
  handler.AlphaSearch（调度不变）→ service.ForwardAlphaSearch
    ├─ PAT 账号 → forwardAlphaSearchViaResponsesWebSearch（不变）
    ├─ 开关开启的 openai apikey → forwardAlphaSearchViaUpstreamResponsesWebSearch（新）
    │     1. buildOpenAIAlphaSearchResponsesWebSearchBody（复用）
    │     2. buildOpenAIAlphaSearchAPIKeyResponsesRequest（新：base_url→Responses URL、Bearer、代理、header 覆盖）
    │     3. doOpenAIUpstream → 读 body
    │     4. 2xx：parse SSE → (output, results, searched)
    │          searched → 写 {"output","results"}，WebSearchCalls=1
    │          !searched → emulateOpenAIAlphaSearch（资格满足）/ 502 web_search_failed
    │        非 2xx：资格满足 → emulateOpenAIAlphaSearch；否则沿用 PAT 路径的 failover/透传分类
    └─ 其它 → 既有 alpha/search 透传（不变）
```

## 3. 契约

### 3.1 开关
- 键：`accounts.extra["openai_alpha_search_via_responses"] = true`。
- `func (a *Account) IsOpenAIAlphaSearchViaResponsesEnabled() bool`：`a != nil && a.IsOpenAIApiKey() && extra 为布尔 true`。

### 3.2 API Key 版 Responses 请求
- URL：`base := account.GetOpenAIBaseURL()`；为空用 `openaiPlatformAPIURL`；否则 `s.validateUpstreamBaseURL(base)` 后 `buildOpenAIResponsesURLForPlatform(account.Platform, validated)`（与 `buildUpstreamRequest` APIKey 分支一致）。
- 头：`buildOpenAIAuthenticationHeaders`（Bearer）、`Content-Type: application/json`、`Accept: text/event-stream`、账号自定义 UA（`GetOpenAIUserAgent`）或入站 UA、`account.ApplyHeaderOverrides`。不带 `OpenAI-Beta`、ChatGPT 账号头、Session/Conversation、Codex 身份头、Lite 头。
- 体：`buildOpenAIAlphaSearchResponsesWebSearchBody(alphaBody, upstreamModel)`（复用，`stream=true`、`store=false`、`tools=[{type:web_search,…}]`）。
- 传输错误：`s.handleOpenAIUpstreamTransportError(ctx, c, account, err, true)`（与 PAT 路径一致）。

### 3.3 真实搜索证据
- `parseOpenAIResponsesSSEForAlphaSearch` 扩展为返回 `(output string, results []any, searched bool)`：遍历事件时，`response.output_item.added|done` 的 `item.type == "web_search_call"`，或 `response.completed.response.output[].type == "web_search_call"`，或收集到 ≥1 `url_citation`，则 `searched = true`。PAT 路径忽略该值，行为不变。

### 3.4 上游非 2xx 与未搜索的分类
| 上游结果 | 模拟资格满足 | 模拟不可用 |
|---|---|---|
| 2xx 且 searched | 写回，计费 | 写回，计费 |
| 2xx 且 !searched | 本地模拟 | 502 `{"error":{"code":"web_search_failed","type":"api_error","message":"upstream did not perform web search","param":"tools"}}`（复用 `writeOpenAIResponsesWebSearchError`），`(nil, err)` 不计费 |
| 非 2xx | 本地模拟（不触发账号错误副作用） | 与 PAT 路径相同：满足 failover 条件返回 `UpstreamFailoverError`（副作用按 `shouldApplyOpenAIAlphaSearchAccountErrorSideEffects`），否则原样透传状态/body/白名单头，`(nil, nil)` |

### 3.5 本地模拟
- 资格 `alphaSearchEmulationEligible(ctx, c, account)`：`s.isOpenAIWebSearchEmulationEnabled(ctx,c,account) && s.settingService != nil && s.settingService.IsWebSearchEmulationEnabled(ctx) && manager != nil && manager.HasAvailableProvider(ctx, resolveAccountProxyURL(account))`。
- 输入解析（gjson，不绑定 DTO）：`commands.search_query[].{q,domains}`、`commands.image_query[].q`；`settings.filters.allowed_domains/blocked_domains`；`settings.search_context_size`；`max_output_tokens`；不支持命令集合 = `commands` 中存在且非空数组的 `open/click/find/screenshot/finance/weather/sports/time`。
- 执行：查询去重后最多 4 条，逐条 `executor(ctx, account, q, maxResults)`（`executor = s.openAIWebSearchExecutor ?? doWebSearchWithMaxResults`）；单条错误记录日志继续；结果经 `filterOpenAIResponsesSearchResults(results, allowed ∪ query.domains, blocked)`；跨查询按 URL 去重并顺序编号 `turn0search{i}`。
- 输出：
  ```
  Search results for "<q>":
  1. [turn0search0] <title>
  <url>
  <snippet>
  Published: <page_age>（有则输出）

  ...
  Unsupported commands in this gateway: open, find. Use the search results above, or fetch the page yourself.
  ```
  `max_output_tokens` 存在时截断到 `4 × tokens` 字符并附 `...<truncated>`。
- 返回：有 ≥1 结果 → 200 JSON、`&OpenAIForwardResult{Model: requestedModel, UpstreamModel: upstreamModel, UpstreamEndpoint: "/v1/alpha/search", WebSearchCalls: 1, Duration}`；无 `search_query` 只有不支持命令 → 200 说明文本、`(nil, nil)`；所有查询都失败 → 502 `web_search_failed`、`(nil, err)`；有查询但零结果 → 200 "No search results found"，`(nil, nil)`。
- 可观测：Info 日志 `openai alpha search emulation completed`（account_id、queries、providers、results）。

## 4. 兼容与回滚
- 开关默认关闭，所有既有路径零改动；关闭开关即回滚行为。
- `parseOpenAIResponsesSSEForAlphaSearch` 签名变更只影响本包内两处调用。
- 不改 handler 调度、计费口径、`web_search_price_per_call` 契约。

## 5. 取舍
- 不为 DeepSeek 跳过上游调用：用户明确要"走上游 websearch"，兜底只在上游无搜索证据时触发；代价是每次多一次上游 Responses 调用。
- `open` 等命令只给说明：避免引入抓取、HTML 清洗与 SSRF 防护；模型实录已证明会自行 curl。
- 模拟输出沿用 PAT 路径 `results` 形态（仅 url/title），保证 Codex UI 解析一致。
