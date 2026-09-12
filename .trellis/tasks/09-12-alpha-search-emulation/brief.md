# Brief — Codex Alpha Search 上游 Responses web_search 桥接：账号级开关 + 本地模拟兜底

## Goal

- 让 Codex Responses Lite / code mode 通过 sub2api 使用 DeepSeek 等 OpenAI 兼容上游时，`tools.web__run` 触发的 `/v1/alpha/search` 能拿到真实搜索结果，而不是现在的 404/502。

## Scope

- 后端新增账号级开关 `openai_alpha_search_via_responses`（仅 `openai` 平台 API Key 账号），开启后 `ForwardAlphaSearch` 把 alpha 请求翻译成带 `web_search` 工具的流式 Responses 请求发往账号 base_url 派生的 Responses 端点（复用 PAT 路径的请求体构造与 SSE 解析，新增 API Key 版请求构造）。
- 真实搜索证据判定：上游 2xx 且出现 `web_search_call` 输出项或至少一条 `url_citation` 才算已搜索，写回 `{"output","results"}` 并 `WebSearchCalls=1`。
- 上游 2xx 无证据或非 2xx 时，若账号/渠道/系统 Web Search Emulation 开启且有可用供应商，自动改走本地模拟：只执行 `search_query`（`image_query` 视同），最多 4 条，`search_context_size` → 3/5/10，域名过滤、URL 去重、`turn0searchN` 编号，`open/click/find/…` 只在 `output` 末尾附说明。
- 模拟不可用时：2xx 未搜索 → 502 `web_search_failed` 不计费；非 2xx → 沿用 PAT 路径既有 failover / 透传分类。
- 前端 `features/alphaSearch/`：extra 助手、Toggle 组件，按 Lite 降级开关模式接入 Edit/Create 账号弹窗（仅 `openai` + `apikey` 显示），中英文文案与配对测试。
- 领域隔离：新建 `openai_alpha_search_responses_bridge.go` 与 `openai_alpha_search_emulation.go`，`openai_alpha_search.go` 只加一次薄调用并扩展 SSE 解析返回证据；`protocol-adapter-guidelines.md` 新增 scenario。
- 后端与前端单测覆盖 AC1-AC12。

## Non-Goals

- 开关关闭时对上游 404 自动模拟；开关关闭下一切行为与现状一致。
- 向主对话 `/v1/responses` 注入 `web_search`；Anthropic 协议路径。
- `open`/`find` 等网页抓取类命令的真实实现。
- Codex 目录侧压制 `use_responses_lite`；BulkEditAccountModal 批量设置。
- 上一轮发现的"chat 回退 + Lite 降级开关"回程缺陷（`openai_gateway_forward.go:158`），另行处理。

## Key Decisions

- 用显式账号级开关触发，而不是复用 Web Search Emulation 设置在 404 后自动兜底：官方 OpenAI Key 与未开启账号行为完全不变，代价是需要新增 extra 键、Toggle 与文案。
- 开启后先走上游 Responses `web_search`（用户明确要求"走上游的 websearch"），即使 DeepSeek 实测会忽略该工具；只在无真实搜索证据时才改走本地供应商，代价是 DeepSeek 每次搜索多一次上游 Responses 调用与 token 费用。
- 本地兜底只覆盖 `search_query`，其他 web.run 命令只给说明：避免引入抓取、HTML 清洗与 SSRF 防护，实录显示模型会自行 curl。
- 模拟输出沿用 PAT 路径 `results` 形态（`text_result` + `ref_id/url/title`），保证 Codex UI 解析一致；计费沿用 `WebSearchCalls=1` 按次口径。

## Key Context

- 入口：`backend/internal/handler/openai_alpha_search.go`（调度不变）→ `backend/internal/service/openai_alpha_search.go` `ForwardAlphaSearch`（`:72` PAT 分支后加薄调用）。
- 复用：`buildOpenAIAlphaSearchResponsesWebSearchBody`（`:292`）、`parseOpenAIResponsesSSEForAlphaSearch`（`:568`，扩展返回 `searched`）、`buildUpstreamRequest` APIKey 分支的 URL 规则、`buildOpenAIAuthenticationHeaders`、`handleOpenAIUpstreamTransportError`。
- 模拟：`doWebSearchWithMaxResults` / `s.openAIWebSearchExecutor`（`openai_gateway_service.go:499`）、`filterOpenAIResponsesSearchResults`（`openai_responses_websearch.go:542`）、资格判定对齐 `codex_web_search_bridge.go:108-120`。
- 前端模式参照：`features/responsesLite/extra.ts`、`ResponsesLiteDowngradeToggle.vue`、`EditAccountModal.vue:1868/3620/3939/5549`、`CreateAccountModal.vue:3318/4418/4846/5462`、`locales/{zh,en}/admin/accounts.ts:706/623` spread、`buildFeatureLocaleExtensions.spec.ts:49`。
- 约束：`protocol-adapter-guidelines.md` L923-1047（不透明 JSON、仅 2xx 计费、failover 先于写响应）、`build-private-feature-isolation-guide.md`、`quality-guidelines.md`、frontend `component-guidelines.md`。
- 事实依据：DeepSeek `/responses` 实测忽略 `web_search`；Codex 0.154 源码证实 Lite 模型永远走独立 `alpha/search`；用户账号 id=1 为 `openai` 平台 API Key 指向 DeepSeek，`web_search_emulation: enabled`。

## Risks / Deferred

- DeepSeek 场景每次搜索都会先付一次无效上游 Responses 调用；若后续想省掉，可再加"直接本地模拟"选项。
- 本地供应商对 `recency`、`user_location` 不生效；`open` 类命令仍需模型自行处理。
- `parseOpenAIResponsesSSEForAlphaSearch` 签名变更只影响本包，但 PAT 路径测试需回归。
- 延后：`openai_gateway_forward.go:158` chat 回退 + Lite 降级开关的 `custom_tool_call` 回程缺陷。

## Acceptance

- AC1 访问器边界：仅 `openai apikey + extra true` 为真。
- AC2/AC3 开关开启：上游 Responses 端点、Bearer、无 ChatGPT/Lite 头、`tools.0.type=web_search`；含 `url_citation` 或 `web_search_call` 即写回 alpha JSON 并 `WebSearchCalls=1`、`UpstreamEndpoint=/v1/responses`。
- AC4/AC5 兜底：上游无证据或 404 且资格满足 → 本地结果写回并计费；404 且不可用 → `UpstreamFailoverError{404}` 且下游未写入。
- AC6 2xx 无证据且不可用 → 502 `web_search_failed`，不计费。
- AC7-AC9 模拟细节：去重、`blocked_domains`、`low`→3、`ref_id` 连续；只含 `open` → 200 说明且不计费；全部失败 → 502，部分失败 → 计费返回。
- AC10 开关关闭：既有 `TestForwardAlphaSearch*` 不变通过。
- AC11 前端：extra 助手/Toggle/弹窗显示条件与 payload、中英文 key 成对。
- AC12：`gofmt -l` 为空；`go test -tags=unit ./internal/service ./internal/handler -run 'AlphaSearch|WebSearch'`、前端定向 vitest 与 `pnpm typecheck` 通过。

## Next Step

- `task.py start` 后从 implement.md 步骤 1 开始：新建 `openai_alpha_search_responses_bridge.go`，实现开关访问器与 API Key 版 Responses 请求构造。
