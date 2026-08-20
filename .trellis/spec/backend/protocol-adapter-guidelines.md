# Protocol Adapter Guidelines

> 本项目后端协议适配、上游桥接和缓存稳定性契约。

---

## Scenario: Anthropic Messages ↔ OpenAI Chat Completions 直连桥接

### 1. Scope / Trigger

- Trigger: 修改 Anthropic `/v1/messages` 与 OpenAI-compatible `/v1/chat/completions` 的请求或响应互转时，必须按本节检查。
- 适用路径：`backend/internal/pkg/apicompat/chatcompletions_anthropic_bridge.go`。
- 账号粘性路径：`backend/internal/handler/openai_gateway_handler.go`。
- 入口场景：OpenAI APIKey 且不走 Responses API 的 raw Chat fallback。该路径不会经过 Responses 的 `prompt_cache_key` / digest replay guard，因此 payload 前缀本身必须稳定。
- 缓存目标：稳定 Chat prefix cache，避免动态 attribution system block、string/array content 形态切换、随机 `tool_use.id` 造成缓存重建。

### 2. Signatures

- 请求转换：`AnthropicToChatCompletionsRequest(req *AnthropicRequest) (*ChatCompletionsRequest, error)`
- 流式响应转换：`ChatCompletionsChunkToAnthropicEvents(chunk *ChatCompletionsChunk, s *ChatCompletionsToAnthropicStreamState) []AnthropicStreamEvent`
- 流式收尾：`FinalizeChatCompletionsAnthropicStream(s *ChatCompletionsToAnthropicStreamState) []AnthropicStreamEvent`
- 非流式折叠：`ChatCompletionsStreamToAnthropicResponse(chunks []*ChatCompletionsChunk, model string) *AnthropicResponse`

### 3. Contracts

- `system` 可以是字符串或 text block array。转换到 Chat 时必须输出 typed content part array，例如：
  ```json
  {"role":"system","content":[{"type":"text","text":"stable system"}]}
  ```
- Claude Code attribution text 属于动态 system block，必须过滤。判断规则是去掉前导空白后以 `x-anthropic-billing-header:` 开头。
- user/assistant 的 text 和 image content 应保持 typed part array，不因为单 text 场景压成 JSON string。
- assistant 的 text/image 和 `tool_use` 必须合并为同一条 `role:"assistant"` message；`tool_use` 转为 Chat `tool_calls`。
- Anthropic `tool_result` 没有独立 id，只有 `tool_use_id`。转 Chat 时输出 `role:"tool"`，并把 `tool_use_id` 写入 `tool_call_id`。同一 Anthropic user 轮次中的其它 text/image 必须放在后续 user message，以保持 Chat tool adjacency。
- 多个并行工具的 `tool_result` 可能因实际执行耗时以不同顺序到达。转换到 Chat 后，紧跟上一条 assistant `tool_calls` 的连续 `role:"tool"` messages 必须按该 assistant `tool_calls` 的 id 顺序规范化；未知 id 保持相对顺序并排在已知 id 后面。不要跨过普通 user/text message 重排。
- 历史 assistant `thinking` 默认不能当普通 text 注入 Chat prompt；当前直连桥接只保留请求级 `thinking:{"type":"disabled"}` 透传，并在该场景省略 `reasoning_effort`。
- Chat 上游返回非空 `tool_call.id` 时，Anthropic `tool_use.id` 必须原样使用。
- Chat 上游缺失 `tool_call.id` 时，不能使用随机 id。必须在 index/name/完整 arguments 可确定后生成 fallback：
  ```text
  seed = index + "\n" + name + "\n" + args
  id = "toolu_" + hex(sha256(seed) 前 12 bytes)
  ```
- 流式响应中，如果缺失 `tool_call.id` 或 function name，先累积 pending tool call，不发送空 id、空 name 或随机 id 的 `tool_use` block。上游后续 chunk 补 id 时使用上游 id；始终不补 id 时，在 finalize 阶段用完整 arguments 生成确定性 fallback。
- 上游正常结束但只返回 `[DONE]`、没有任何 Chat chunk 时，finalize 仍必须输出完整 Anthropic frame：`message_start`、`message_delta`、`message_stop`。不能因为未见 chunk 就返回空流，否则 Anthropic 客户端会把一次正常空回答误判为协议中断。
- usage 映射保持 Anthropic 语义：`cache_read_input_tokens = cached_tokens`；
  `cache_creation_input_tokens` 优先读取 `cache_write_tokens`，缺失时读取
  `cache_creation_tokens`；`input_tokens = max(prompt_tokens - cached_tokens - cache_creation_input_tokens, 0)`。
  `cache_write_tokens` 与 `cache_creation_tokens` 是同一数量的两种字段名，不得相加。
- `/v1/messages` 账号粘性 key 优先级必须是：显式 `session_id` / `conversation_id` / `prompt_cache_key` > Anthropic `metadata.user_id` > content fallback。`metadata.user_id` 只用于账号 sticky，不直接作为上游 `prompt_cache_key`，避免固定上游缓存键压住后续 turn 的缓存滚动。

### 4. Validation & Error Matrix

- `AnthropicToChatCompletionsRequest(nil)` -> 返回 `anthropic request is nil` 错误。
- message `content` 既不是字符串也不是可解析的 block array -> 返回 JSON 解析错误。
- text 为空、全空白或为 attribution block -> 不输出该 content part；system 过滤后为空则不输出 system message。
- `tool_result.content` 为空或无法提取 text -> Chat tool message content 使用 `"(empty)"`。
- 连续 tool messages 跟在包含多个 `tool_calls` 的 assistant 后面，且 `tool_call_id` 可匹配 -> 按 assistant `tool_calls` 顺序重排。
- 连续 tool messages 中存在未知 `tool_call_id` -> 已知 id 按 `tool_calls` 顺序排前，未知 id 保持原相对顺序排后。
- tool messages 中间出现普通 user/text message -> 不跨 message 重排，避免改变用户轮次语义。
- Chat tool delta 缺 id 但已有 name/args -> pending；finalize 时生成确定性 fallback id。
- Chat tool delta 缺 name 到 finalize -> 无法构造合法 Anthropic `tool_use`，跳过该
  pending tool call；若没有其它合法工具块，即使上游 `finish_reason=tool_calls`，
  最终 `stop_reason` 也使用 `end_turn`。
- 上游 SSE 只返回 `data: [DONE]` 且没有 chunk -> 输出 `message_start`、`message_delta(stop_reason=end_turn)`、`message_stop`。
- Chat usage cached tokens 大于 prompt tokens -> Anthropic `input_tokens` 归零，不产生负数。
- `thinking.type == "disabled"` -> 输出 Chat `thinking.type=disabled`，且 `reasoning_effort` 必须为空。
- Anthropic body 含 `metadata.user_id` 且无显式 session 信号 -> 账号 sticky key 来自 `reqModel + "-" + metadata.user_id`，不被 model/tools/首条 user content fallback 覆盖。
- Anthropic body 同时含显式 session 信号和 `metadata.user_id` -> 显式 session 信号优先。

### 5. Good/Base/Bad Cases

- Good: 同一 Anthropic 历史 replay 转出的 Chat messages 在单 text、多 text、多模态之间都保持 `content` array 形态。
- Good: Claude Code 每轮变化的 `x-anthropic-billing-header:` 不进入上游 Chat payload。
- Good: 并行工具结果即使按完成时间反序返回，Chat payload 中紧跟 assistant 的 tool messages 仍按上一条 `tool_calls` 顺序稳定输出。
- Good: 空但正常完成的上游 Chat SSE 仍给 Anthropic 客户端完整收尾 frame，客户端不会收到零字节成功响应。
- Good: 上游只给出 tool call ID/arguments、始终不给 function name 时，不输出
  `name:""` 的非法 `tool_use`，并以 `end_turn` 完成。
- Good: Claude Code 同一 `metadata.user_id` 即使首条用户内容或工具定义不同，账号 sticky key 仍稳定命中同一个账号绑定。
- Base: 上游稳定返回 `tool_call.id`，桥接直接复用该 id，客户端下一轮 `tool_result.tool_use_id` 可稳定引用。
- Base: 上游不返回 `tool_call.id`，相同 index/name/arguments 多次请求生成相同 `toolu_` fallback。
- Bad: 为缺 id 的 tool call 调用 `crypto/rand` 或拼接请求 id、时间戳、message id。
- Bad: 先发送 fallback id，后续 chunk 又收到上游 id，导致同一次工具调用在客户端侧 id 漂移。
- Bad: 把 `tool_result` 后面的用户文本排在 `role:"tool"` 之前，破坏 Chat Completions 的 tool adjacency。
- Bad: 按并行工具完成时间直接输出多个 `role:"tool"` message，导致同一历史 replay 的 Chat 前缀随本地工具耗时漂移。
- Bad: 在 `/v1/messages` handler 中先用 content fallback 生成非空 session hash，再跳过 `metadata.user_id`，导致同一 Claude Code 会话漂到多个 OpenAI-compatible 账号。

### 6. Tests Required

- 协议适配改动至少运行：
  ```bash
  cd backend
  go test -tags=unit ./internal/pkg/apicompat
  ```
- 如果网关选择、raw Chat fallback 或 service 层消息处理也受影响，补充运行：
  ```bash
  cd backend
  go test -tags=unit ./internal/service -run 'Test.*Messages|Test.*Chat|Test.*Anthropic|Test.*OpenAI'
  ```
- 必须覆盖的断言点：
  - attribution system text 被过滤。
  - system/user/assistant text content 保持 typed part array。
  - assistant text + `tool_use` 合并为单条 assistant message。
  - `tool_result` 输出为 `role:"tool"`，且 `tool_call_id` 来自 `tool_use_id`。
  - 多个并行 `tool_result` 反序到达时，输出顺序仍跟上一条 assistant `tool_calls` 一致。
  - 缺失上游 `tool_call.id` 时 fallback id 可重复。
  - 后续 chunk 才提供 `tool_call.id` 时，最终使用上游 id。
  - 上游只返回 `[DONE]` 时，streaming 路径仍输出 `message_start`、`message_delta`、`message_stop`。
  - `thinking:{"type":"disabled"}` 透传，且不输出 `reasoning_effort`。
  - cached/cache creation token 分别映射，且
    `input_tokens + cache_read_input_tokens + cache_creation_input_tokens` 不超过
    上游 `prompt_tokens`。
  - `/v1/messages` 账号 sticky：`metadata.user_id` 优先于 content fallback，显式 session 信号优先于 `metadata.user_id`。

### 7. Wrong vs Correct

#### Wrong

```go
toolID := tc.ID
if toolID == "" {
	toolID = generateRandomToolUseID()
}
events = append(events, s.openToolBlock(idx, toolID, tc.Function.Name)...)
```

问题：第一次 replay 和第二次 replay 的 `tool_use.id` 可能不同，下一轮 `tool_result.tool_use_id` 和 Chat prefix cache 都会被扰动。

#### Correct

```go
if tc.ID == "" || tc.Function.Name == "" {
	s.storePendingToolCall(idx, tc)
	return nil
}
```

后续在 finalize 阶段基于 index/name/完整 arguments 生成确定性 `toolu_` fallback；如果上游补发 id，则优先使用上游 id。

#### Wrong

```json
{"role":"user","content":"hi"}
```

问题：单 text 是 string，多 text 或图片又变成 array，历史前缀的 JSON 结构会随上下文形态变化。

#### Correct

```json
{"role":"user","content":[{"type":"text","text":"hi"}]}
```

typed content part array 是该桥接路径的稳定输出形态。

---

## Scenario: Grok `/v1/messages` 强制 Chat Completions 路由覆盖

### 1. Scope / Trigger

- Trigger: 修改 Grok 账号在 OpenAI 兼容 `/v1/messages` 入站下的 Responses / Chat Completions 分流、`extra.openai_responses_mode` 保存逻辑、raw Chat fallback 凭据解析或 usage upstream endpoint 记录时，必须按本节检查。
- 适用后端路径：
  - `backend/internal/service/openai_gateway_messages.go`
  - `backend/internal/service/openai_gateway_cc_pipeline.go`
  - `backend/internal/service/openai_gateway_messages_chat_fallback.go`
  - `backend/internal/handler/openai_chat_completions.go`
- 适用前端路径：
  - `frontend/src/components/account/CreateAccountModal.vue`
  - `frontend/src/components/account/EditAccountModal.vue`
- 目标：只在管理员显式设置 `extra.openai_responses_mode="force_chat_completions"` 时让 Grok `/v1/messages` 走 xAI `/chat/completions`；默认、`auto` 和 `force_responses` 必须保持现有 `/responses` 行为。

### 2. Signatures

- 分流判断：
  ```go
  func ShouldForwardAnthropicMessagesViaRawChatCompletions(account *Account) bool
  ```
- raw Chat fallback 目标解析：
  ```go
  func (s *OpenAIGatewayService) resolveCCFallbackTarget(ctx context.Context, account *Account) (apiKey string, targetURL string, err error)
  ```
- usage endpoint 记录：
  ```go
  func resolveOpenAIUpstreamEndpoint(c *gin.Context, account *service.Account, result *service.OpenAIForwardResult) string
  ```
- 前端 extra 写入必须保持字段名和值：
  ```typescript
  type OpenAIResponsesMode = 'auto' | 'force_responses' | 'force_chat_completions'
  extra.openai_responses_mode?: OpenAIResponsesMode
  ```

### 3. Contracts

- Grok 分流只接受 `platform="grok"` 且 `type` 为 `oauth` 或 `apikey` 的账号；其它 Grok 类型即使带 `force_chat_completions` 也不进入 raw Chat fallback。
- Grok 只响应显式 `openai_responses_mode="force_chat_completions"`：
  - 字段缺失、非法值、`auto`、`force_responses` -> 继续走 `/responses`。
  - `force_chat_completions` -> `ForwardAsAnthropic` 调用 `forwardAnthropicViaRawChatCompletions`。
- OpenAI APIKey 原契约不变：仍按 `!openai_compat.ShouldUseResponsesAPI(account.Extra)` 决定是否走 raw Chat fallback。
- Grok raw Chat fallback 凭据必须通过 `GetAccessToken(ctx, account)` 获取：
  - OAuth 使用有效 access token。
  - APIKey 使用 `api_key`。
  - 空凭据返回错误，不应构造无鉴权上游请求。
- Grok raw Chat fallback URL 必须来自账号 Grok base URL 的 `/chat/completions`，不要复用 OpenAI APIKey 专用 URL 构造。
- raw Chat fallback 无论下游 `stream` 值都必须向上游发送 `stream=true` 和
  `stream_options.include_usage=true`；下游非流式请求由网关缓冲 SSE 后通过
  `ChatCompletionsStreamToAnthropicResponse` 折叠为 Anthropic Messages JSON。
- Grok raw Chat fallback 成功或上游错误后都应更新 xAI quota snapshot；错误路径应继续走 Grok 上游错误处理和 failover 语义。
- usage/upstream endpoint 记录必须反映实际路径：
  - Grok `/v1/messages` + `force_chat_completions` -> `/v1/chat/completions`。
  - Grok `/v1/messages` + 默认/`auto` -> `/v1/responses`。
  - Grok 原生 `/v1/chat/completions` -> `/v1/chat/completions`。
- 实际端点解析优先级固定为：`OpenAIForwardResult.UpstreamEndpoint` > 当前请求 runtime context > 账号和入站端点 fallback。转发结果必须优先于可能陈旧的 runtime context；错误路径没有 result 时使用 runtime context。
- 前端创建/编辑 Grok 账号时，保存 `openai_responses_mode` 必须基于现有 `extra` 复制后只增删该键，保留 `email`、`grok_usage_snapshot`、`quota_*`、限额和通知配置等其它键。

### 4. Validation & Error Matrix

- `account == nil` -> 分流判断返回 false；usage endpoint 退回入站端点推导。
- `platform=grok,type=oauth,extra.openai_responses_mode` 缺失 -> `/v1/messages` 走 `/responses`。
- `platform=grok,type=oauth,extra.openai_responses_mode="auto"` -> `/v1/messages` 走 `/responses`。
- `platform=grok,type=oauth,extra.openai_responses_mode="force_responses"` -> `/v1/messages` 走 `/responses`。
- `platform=grok,type=oauth,extra.openai_responses_mode="force_chat_completions"` -> `/v1/messages` 走 `/chat/completions`，Authorization 使用 OAuth access token。
- `platform=grok,type=apikey,extra.openai_responses_mode="force_chat_completions"` -> `/v1/messages` 走 `/chat/completions`，Authorization 使用 API key。
- Grok `force_chat_completions` 但 token/APIKey 为空 -> 返回缺失凭据错误，不发送上游请求。
- Grok raw Chat 上游返回 `xai-request-id` 而非 `x-request-id` -> request id 和 ops 事件应使用 `xai-request-id`。
- 前端选择 `auto` -> payload `extra` 删除 `openai_responses_mode`，其它 `extra` 键保持。
- 前端选择 `force_chat_completions` -> payload `extra.openai_responses_mode` 写入该值，且不显示 OpenAI APIKey endpoint capabilities 复选项给 Grok。

### 5. Good/Base/Bad Cases

- Good: Grok OAuth 账号 `extra` 中已有 `grok_usage_snapshot` 和 `email`，编辑页选择 `force_chat_completions` 后 payload 仍保留这些键，只新增 `openai_responses_mode`。
- Good: Grok `/v1/messages` 强制 Chat 的非流式下游响应仍是 Anthropic Messages JSON，流式下游响应仍是 Anthropic SSE frame。
- Base: Grok 未配置 `openai_responses_mode` 的存量账号继续走 `/responses`，不受 OpenAI APIKey 探测字段影响。
- Base: OpenAI APIKey raw Chat fallback 行为保持不变，可继续使用 `openai_responses_supported=false` 或显式 `force_chat_completions`。
- Bad: 把 Grok `openai_responses_supported=false` 当作强制 Chat 信号，会让探测缓存误改 OAuth 账号默认路径。
- Bad: 保存 Grok 设置时用新对象覆盖 `extra`，导致 `grok_usage_snapshot`、限额配置或邮箱丢失。
- Bad: raw Chat fallback 里调用 `account.GetOpenAIApiKey()` 处理 Grok OAuth，会让 OAuth 账号无法进入 `/chat/completions`。

### 6. Tests Required

- 后端 service 单测至少覆盖：
  - `ShouldForwardAnthropicMessagesViaRawChatCompletions` 对 OpenAI APIKey、Grok 缺省、Grok `auto`、Grok `force_responses`、Grok OAuth/APIKey `force_chat_completions` 的返回值。
  - Grok OAuth `force_chat_completions`：`ForwardAsAnthropic` 上游 URL 是 xAI `/chat/completions`，Authorization 是 Bearer access token，下游仍是 Anthropic 兼容响应。
  - Grok APIKey `force_chat_completions`：上游 URL 是 xAI `/chat/completions`，Authorization 是 Bearer API key。
  - Grok 缺省或 `auto`：`ForwardAsAnthropic` 上游仍是 xAI `/responses`。
- 后端 handler 单测至少覆盖：
  - Grok `/v1/messages` + `force_chat_completions` usage endpoint 为 `/v1/chat/completions`。
  - Grok `/v1/messages` + 缺省/`auto` usage endpoint 为 `/v1/responses`。
  - Grok 原生 `/v1/chat/completions` usage endpoint 为 `/v1/chat/completions`。
- 前端组件测试至少覆盖：
  - Grok 编辑页显示 Responses 模式下拉，不显示 OpenAI endpoint capabilities 复选项。
  - 写入 `force_chat_completions` 时保留既有 `extra`。
  - 改回 `auto` 时删除 `openai_responses_mode` 且保留其它 `extra`。
- 建议运行：
  ```bash
  cd backend && go test -tags=unit ./internal/service -run 'TestShouldForwardAnthropicMessagesViaRawChatCompletions|TestForwardAsAnthropic_.*Grok|TestForwardAsAnthropicForGrokUsesXAIResponses'
  cd backend && go test -tags=unit ./internal/handler -run 'TestResolveOpenAIUpstreamEndpointForGrokMessagesForceChat'
  cd frontend && pnpm vitest run src/components/account/__tests__/EditAccountModal.spec.ts
  cd frontend && pnpm typecheck
  ```

### 7. Wrong vs Correct

#### Wrong

```go
if account.Platform == PlatformGrok && !openai_compat.ShouldUseResponsesAPI(account.Extra) {
	return true
}
```

问题：`ShouldUseResponsesAPI` 会读取 `openai_responses_supported=false`，这会把探测状态误当作 Grok OAuth 路由覆盖，破坏存量默认 `/responses` 行为。

#### Correct

```go
mode, _ := account.Extra[openai_compat.ExtraKeyResponsesMode].(string)
return openai_compat.NormalizeResponsesSupportMode(mode) ==
	openai_compat.ResponsesSupportModeForceChatCompletions
```

Grok 只响应显式 `force_chat_completions`，其它状态保持 `/responses`。

#### Wrong

```typescript
updatePayload.extra = {
  openai_responses_mode: openAIResponsesMode.value
}
```

问题：覆盖整个 `extra` 会丢失 `grok_usage_snapshot`、`email`、限额配置等运行态或管理员配置。

#### Correct

```typescript
const newExtra: Record<string, unknown> = { ...currentExtra }
if (openAIResponsesMode.value === 'auto') {
  delete newExtra.openai_responses_mode
} else {
  newExtra.openai_responses_mode = openAIResponsesMode.value
}
updatePayload.extra = newExtra
```

只修改路由覆盖键，保留其它 `extra` 信息。

---

## Scenario: Grok 4.5 / GLM reasoning effort 归一化与 usage 日志一致性

### 1. Scope / Trigger

- Trigger: 修改 Grok 4.5 或 GLM 的 OpenAI-compatible `reasoning.effort` / `reasoning_effort` 改写、Responses ↔ Chat fallback、Anthropic Messages 桥接、WebSocket HTTP bridge 或 `usage_logs.reasoning_effort` 取值时，必须按本节检查。
- 适用后端路径：
  - `backend/internal/service/gateway_request.go`
  - `backend/internal/service/openai_gateway_grok.go`
  - `backend/internal/service/openai_gateway_chat_completions_raw.go`
  - `backend/internal/service/openai_gateway_messages.go`
  - `backend/internal/service/openai_gateway_messages_chat_fallback.go`
  - `backend/internal/service/openai_gateway_responses_chat_fallback.go`
  - `backend/internal/service/openai_ws_http_bridge.go`
- 适用前端路径：`frontend/src/utils/format.ts`。
- 目标：客户端跨模型发送档位别名时，只按最终上游模型的原生档位改写；usage 日志必须记录最终请求体实际发送的值，而不是客户端原值、模型默认值或 thinking 推断值。

### 2. Signatures

```go
func NormalizeGLMOpenAIReasoningEffort(body []byte, mappedModel string) ([]byte, bool)
func normalizeOpenAIReasoningEffortForProvider(body []byte, mappedModel string) ([]byte, bool)
func extractFinalOpenAIReasoningEffort(body []byte) *string
func extractOpenAIUpstreamReasoningEffort(body []byte, requestedModel string, mappedModel string, additionalModelCandidates ...string) *string
```

结果与持久化链路：

```text
最终上游 body
  -> OpenAIForwardResult.ReasoningEffort
  -> UsageLog.ReasoningEffort
  -> usage_logs.reasoning_effort
  -> formatReasoningEffort
```

### 3. Contracts

- 字段定位优先级固定为：已存在的 `reasoning.effort` > 已存在的 `reasoning_effort`。只改写命中的现有路径，不新增字段。
- 别名识别前执行 trim、转小写，并移除 `-`、`_`、空格；未知值返回空映射并保持原请求值。
- GLM guard：最终模型 trim/lower 后以 `glm-` 开头。
- Grok guard：最终模型必须大小写不敏感地精确等于 `grok-4.5`；不得使用 `grok-` 前缀匹配，避免改坏 `grok-4.20-multi-agent` 等支持 `xhigh` 的模型。
- 映射表：

| 客户端语义 | GLM 最终值 | Grok 4.5 最终值 |
|---|---|---|
| `none` | `none` | `low` |
| `minimal` | `minimal` | `low` |
| `low` | `high` | `low` |
| `medium` | `high` | `medium` |
| `high` | `high` | `high` |
| `xhigh` / `extra high` | `max` | `high` |
| `max` / `ultracode` | `max` | `high` |
| 未知值 | 原样透传 | 原样透传 |

- Grok / GLM 的 `OpenAIForwardResult.ReasoningEffort` 必须在完成模型改写、provider 归一化和 fast policy 后，从最终请求体提取；只 trim，不做白名单过滤。
- 最终请求体没有 effort 时，结果保持 `nil`，不得调用 `ApplyThinkingEnabledFallback` 猜测 `high`。
- 其它 provider 继续沿用既有模型后缀提取和 thinking fallback，不能因本规则发生全局行为变化。
- `/v1/messages` 默认 Responses 路径与强制 Chat 路径保持既有缺省差异：Responses 转换器当前会发 `medium`；强制 Chat 未产生 effort 时继续省略，不互相补默认值。
- 前端继续读取现有 `reasoning_effort` 字段；`none` 显示为 `None`，`minimal` 显示为 `Minimal`，空值才显示 `-`。

### 4. Validation & Error Matrix

- 已知别名 + 命中 GLM/Grok guard -> 改写命中路径，返回 `changed=true`。
- 已是目标原生值 -> 请求体保持不变，返回 `changed=false`。
- `banana` 等未知值 -> 请求体原样透传；上游接受时日志保留 trim 后实际值，上游拒绝时沿用现有错误路径。
- effort 字段缺失或 trim 后为空 -> 不新增字段，最终日志为 `nil`。
- 同时存在嵌套和扁平字段 -> 只处理、记录嵌套字段，扁平字段保持原样。
- 最终模型为 `grok-4.20-multi-agent` / `grok-4.3` -> 跳过 Grok 4.5 归一化。
- GLM `thinking.type=enabled` 但最终 body 没有 effort -> 日志为 `nil`，不得补 `high`。
- GLM `thinking.type=enabled` 且最终 body 为 `minimal` -> 上游和日志均为 `minimal`。

### 5. Good/Base/Bad Cases

- Good: 客户端向 `grok` 别名发送 `xhigh`，模型映射先得到 `grok-4.5`，最终上游 body 和 usage 日志都为 `high`。
- Good: GLM 收到 `MINIMAL`，最终上游 body 和 usage 日志都为小写 `minimal`，即使 thinking 已开启也不误记为 `high`。
- Good: Responses、原生 Chat、Messages 两种分支和 Grok WebSocket HTTP bridge 复用同一 provider 分派和最终值提取逻辑。
- Base: 未提供 effort 时保持缺失；Grok 上游自身的默认档位不写入本地日志。
- Base: 非 Grok 4.5 模型继续原样接收 `xhigh`。
- Bad: 在模型映射前按客户端 `grok`/`grok-latest` 名称判断，导致别名漏归一化。
- Bad: 从转换前 DTO 或客户端原始 body 回填 `ReasoningEffort`，导致 `xhigh -> high` 后日志仍写 `xhigh`。
- Bad: 为了日志非空，根据 thinking 状态或厂商默认值补 `high`。

### 6. Tests Required

- `gateway_request_test.go`：覆盖完整映射表、大小写/分隔符、嵌套优先、未知值、缺失值、非 4.5 Grok 模型和最终值提取。
- `openai_gateway_grok_test.go`：覆盖 Grok Responses、原生 Chat、Messages Responses 的上游 body 与结果 effort 一致。
- `openai_gateway_chat_completions_raw_test.go`：覆盖 GLM `xhigh -> max`、`MINIMAL -> minimal` 及结果日志一致。
- `openai_gateway_messages_chat_fallback_test.go`：覆盖 Grok/GLM 强制 Chat，并锁定强制 Chat 不补默认 effort。
- `openai_gateway_responses_chat_fallback_test.go`：覆盖 GLM Responses 转 Chat 归一化。
- `openai_ws_http_bridge_test.go`：覆盖 Grok 4.5 WebSocket HTTP bridge 的上游 body 和结果 effort。
- `formatReasoningEffort.spec.ts`：覆盖空值、标准档位、分隔符别名、`none/minimal` 和未知值。
- 建议运行：

```bash
cd backend && go test -tags=unit ./internal/service
cd backend && go test -tags=unit ./internal/pkg/apicompat
cd frontend && pnpm vitest run src/utils/__tests__/formatReasoningEffort.spec.ts src/components/admin/usage/__tests__/UsageTable.spec.ts
cd frontend && pnpm typecheck
cd frontend && pnpm lint:check
git diff --check
```

### 7. Wrong vs Correct

#### Wrong

```go
reasoningEffort := extractOpenAIReasoningEffortFromBody(originalBody, originalModel)
reasoningEffort = ApplyThinkingEnabledFallback(reasoningEffort, originalBody, mappedModel)
```

问题：读取发生在 provider 改写前，且 fallback 会把“最终未发送 effort”误记为 `high`。

#### Correct

```go
if normalizedBody, changed := normalizeOpenAIReasoningEffortForProvider(upstreamBody, upstreamModel); changed {
	upstreamBody = normalizedBody
}
reasoningEffort := extractOpenAIUpstreamReasoningEffort(upstreamBody, originalModel, upstreamModel, billingModel)
```

先按最终模型改写上游 body，再从实际发送体提取日志值；Grok/GLM 不推断默认档位，其它 provider 保留既有 fallback。

---

## Scenario: OpenAI reasoning effort 模型候选与 GPT-5.6 `max` 兼容

### 1. Scope / Trigger

- Trigger: 修改 OpenAI-compatible 请求中的 `reasoning.effort` / `reasoning_effort`、模型 effort 后缀、模型映射、billing model、Responses ↔ Chat fallback、Anthropic Messages raw Chat fallback 或 WebSocket usage 元数据时，必须按本节检查。
- 适用路径：`openai_gateway_request_body.go`、`gateway_request.go`、raw Chat、Messages/Responses fallback、Responses/Chat 兼容转换和 WebSocket passthrough。
- 目标：显式 effort 与后缀 effort 在模型映射后仍保持正确语义；只有 GPT-5.6 Sol/Terra/Luna 可以在普通请求中保留 `max`，usage 记录必须与最终上游请求或可恢复的模型后缀一致。

### 2. Signatures

```go
func extractOpenAIReasoningEffortFromBody(body []byte, modelCandidates ...string) *string
func extractOpenAIUpstreamReasoningEffort(body []byte, requestedModel string, mappedModel string, additionalModelCandidates ...string) *string
func normalizeOpenAIReasoningEffortForModel(raw, model string) string
func isOpenAIGPT56Model(model string) bool
func normalizeOpenAICodexCompactReasoningEffortForAccount(c *gin.Context, account *Account, body []byte) ([]byte, bool, error)
```

调用方有 billing model 时，参数关系必须为：`requestedModel=originalModel`、`mappedModel=upstreamModel`、`additionalModelCandidates=billingModel`。helper 内部由此形成 `upstream -> billing -> original` 的候选顺序。

### 3. Contracts

- 显式字段优先级固定为 `reasoning.effort` > `reasoning_effort`；命中显式值时，仅使用第一个非空模型候选判断模型感知归一化。
- 请求体没有 effort 时，按全部候选顺序推导模型后缀。OAuth/Codex 标准化可能剥离 upstream model 的 `-high` / `-xhigh` / `-max`，因此必须保留 billing 和 original model 候选。
- `max` 仅在第一个非空候选由 `isOpenAIGPT56Model` 识别为 `gpt-5.6-sol`、`gpt-5.6-terra` 或 `gpt-5.6-luna` 时保留；大小写、provider 路径和日期后缀允许存在。其它模型的 `max` 统一为 `xhigh`。
- GLM 与 Grok 4.5 继续走 provider-specific 分支：直接读取最终上游 body，不用模型后缀或 thinking fallback 覆盖实际发送值。
- raw Chat、Messages fallback、Responses fallback 有 billing model 时都必须传入；WebSocket 只有 original/mapped model 时可省略额外候选。
- OpenAI OAuth 的 `/responses/compact` 是明确例外：GPT-5.6 `max` 在 compact 子请求中降级为 `xhigh`。普通 Responses、OpenAI API Key compact 和其它平台 OAuth 不应用该降级。
- 本场景不新增 API、DTO、数据库字段或 migration；变化只影响上游请求体与 usage 元数据。

### 4. Validation & Error Matrix

| 条件 | 必须结果 |
|---|---|
| 显式 `max`，首个候选为 GPT-5.6 | 保留 `max` |
| 显式 `max`，首个候选非 GPT-5.6 | 归一化为 `xhigh`，后续候选不得反转判断 |
| body 无 effort，original model 为 `gpt-5.6-sol-max` | 从后缀恢复 `max` |
| body 无 effort，original model 为 `gpt-5.4-xhigh` | 从后缀恢复 `xhigh` |
| GLM/Grok 4.5 已完成 provider 归一化 | usage 记录最终 body 值，不应用后缀或 thinking fallback |
| OpenAI OAuth GPT-5.6 compact + `max` | 上游 body 与 usage 均为 `xhigh` |
| 所有候选均为空或无后缀，body 也无 effort | 返回 `nil`；仅非 provider-specific 路径可沿用既有 thinking fallback |

### 5. Good/Base/Bad Cases

- Good: 客户端模型 `sol` 映射到 `gpt-5.6-sol`，显式 `max` 使用 upstream model 判断并保持为 `max`。
- Good: OAuth 上游模型已去掉 `-xhigh`，usage 仍从 original model 后缀恢复 `xhigh`。
- Base: upstream、billing、original 是相同基名且无后缀，结果保持 `nil`，不伪造 effort。
- Bad: 只传 upstream model，导致映射或标准化剥离后缀后 usage 丢失。
- Bad: 让所有模型都保留 `max`，导致不支持该档位的 GPT/Codex 上游收到非法值。
- Bad: 把 original model 放在首位判断显式 `max`，导致别名映射后的 GPT-5.6 能力无法生效。

### 6. Tests Required

- `openai_reasoning_effort_candidates_test.go`：覆盖候选顺序、显式 `max` 首候选判定和后缀恢复。
- `openai_gpt56_max_test.go`：覆盖 Sol/Terra/Luna、非 GPT-5.6 降级、普通 Responses 与 OAuth compact 例外。
- raw Chat、Messages/Responses fallback 与 WebSocket 测试必须至少各覆盖一个 mapped/billing/original 候选或 GPT-5.6 `max` 场景。
- 建议运行：

```bash
cd backend && go test -tags=unit ./internal/service -run 'TestExtractOpenAIReasoningEffort|TestNormalizeOpenAIReasoningEffortForGPT56|TestOpenAIGatewayServiceForward.*Max|TestForwardAsRawChatCompletions_PreservesMappedGPT56MaxEffort|TestWSPassthroughUsageMeta'
cd backend && go test -tags=unit ./internal/service
git diff --check
```

### 7. Wrong vs Correct

#### Wrong

```go
reasoningEffort := extractOpenAIReasoningEffortFromBody(upstreamBody, originalModel)
```

问题：模型映射与 OAuth 标准化后的真实能力不由 original model 单独决定，且只传一个候选会丢失 billing/original 后缀恢复链路。

#### Correct

```go
reasoningEffort := extractOpenAIUpstreamReasoningEffort(
	upstreamBody,
	originalModel,
	upstreamModel,
	billingModel,
)
```

显式值由 upstream model 判断 GPT-5.6 `max` 能力；无显式值时再按 `upstream -> billing -> original` 恢复后缀，provider-specific 模型仍记录最终上游 body。

---

## Scenario: Grok 主 Billing 与配额探测单链路

### 1. Scope / Trigger

- Trigger: 修改 Grok OAuth Billing、xAI rate-limit header 探测、账号 usage 聚合、管理端配额 API、SSO 导入后探测或前端 Grok 配额展示时，必须按本节检查。
- 适用后端路径：
  - `backend/internal/pkg/xai/billing.go`
  - `backend/internal/pkg/xai/quota.go`
  - `backend/internal/service/grok_quota_service.go`
  - `backend/internal/service/grok_quota_fetcher.go`
  - `backend/internal/service/account_usage_service.go`
  - `backend/internal/repository/account_repo.go`
  - `backend/internal/handler/admin/grok_oauth_handler.go`
  - `backend/internal/server/routes/admin.go`
- 适用前端路径：
  - `frontend/src/api/admin/grok.ts`
  - `frontend/src/types/index.ts`
  - `frontend/src/components/account/AccountUsageCell.vue`
  - `frontend/src/views/admin/AccountsView.vue`
- 目标：只使用 main 的 `GrokQuotaService` 和 `grok_billing_snapshot` 承载 Billing；账号列表可自动刷新 Billing，但不得消费 Responses 模型额度。不得恢复独立 `/billing-quota` API、独立 service/parser、`grok_billing_quota` DTO 或前端独立额度组件。

### 2. Signatures

管理端 API 和 handler：

```text
GET  /api/v1/admin/grok/accounts/:id/quota
POST /api/v1/admin/grok/accounts/:id/reset-quota
POST /api/v1/admin/grok/sso-to-oauth
```

```go
func (h *GrokOAuthHandler) QueryQuota(c *gin.Context)
func (h *GrokOAuthHandler) ResetQuota(c *gin.Context)
func (s *GrokQuotaService) QueryQuota(ctx context.Context, accountID int64) (*GrokQuotaProbeResult, error)
func (s *GrokQuotaService) ProbeBilling(ctx context.Context, accountID int64) (*GrokQuotaProbeResult, error)
func (s *GrokQuotaService) ProbeUsage(ctx context.Context, accountID int64) (*GrokQuotaProbeResult, error)
func (s *AccountUsageService) GetUsage(ctx context.Context, accountID int64, force ...bool) (*UsageInfo, error)
```

Billing parser 和快照：

```go
func xai.BuildBillingURLWithValidator(baseURL string, formatCredits bool, validator xai.BaseURLValidator) (string, error)
func xai.ApplyCLIBillingHeaders(req *http.Request, accessToken string)
func xai.ParseBillingPayload(body []byte) (*xai.BillingPayload, error)
func xai.BuildBillingSummary(config *xai.BillingConfig) *xai.BillingSummary
func xai.MergeBillingProbeResult(previous, weekly, monthly *xai.BillingSummary, weeklyOK, monthlyOK bool) *xai.BillingSummary
```

```text
accounts.extra.grok_billing_snapshot -> xai.BillingSummary
accounts.extra.grok_usage_snapshot   -> xai.QuotaSnapshot
UsageInfo.GrokBilling                -> json:"grok_billing,omitempty"
```

前端调用和类型：

```typescript
queryQuota(id: number): Promise<GrokQuotaProbeResult>
AccountUsageInfo.grok_billing?: GrokBillingSummary | null
```

### 3. Contracts

- `GrokQuotaService` 只支持 `platform=grok` 且 `type=oauth`，通过 `GrokTokenProvider.GetAccessToken`、账号代理和 `HTTPUpstream` 访问上游。
- `ProbeBilling` 并发请求 weekly `/billing?format=credits` 与 monthly `/billing`，合并月/周额度、产品用量、预付和按量付费字段；任一窗口成功即可写入 `extra.grok_billing_snapshot`，失败窗口通过 `partial`、`failed_windows` 和各窗口 status 表达。
- `AccountUsageService` 必须注入 `GrokQuotaService`。Grok OAuth 账号 Billing 缺失、失败、不完整或超过 10 分钟时，账号 usage 刷新调用 `ProbeBilling`；一分钟内的重复失败重试受 `grokProbeRetryTTL` 限制，force 刷新绕过门禁并向调用方返回探测错误。
- 账号列表自动刷新只能调用 `ProbeBilling`，不得调用 `ProbeUsage`；因此一次刷新最多产生 weekly/monthly 两个 Billing GET，不得发送真实 Responses 请求消耗模型额度。
- `GrokQuotaFetcher.BuildUsageInfo` 从 `grok_billing_snapshot` 和 `grok_usage_snapshot` 构建同一份 `UsageInfo`。Billing 负责官方 7d/30d、套餐、金额和状态；rate-limit snapshot 负责 request/token header、429 和 4.5 Responses 套餐信号；本地 usage 负责免费 24h 与窗口统计。
- 显式 `QueryQuota` 先调用 `ProbeBilling`；Billing 已提供权威额度时直接返回，否则调用 `ProbeUsage` 获取 rate-limit headers，并以 `hybrid_probe` 合并结果。
- 前端 `AccountUsageCell` 只消费 `grok_billing`、`grok_request_quota`、`grok_token_quota` 和本地窗口字段，不维护独立 Billing 请求队列。`AccountsView` 的套餐判断按明确 credentials、4.5 Heavy 信号、`grok_billing_snapshot.plan`、usage/legacy fallback 的既有优先级执行。
- 历史 `grok_billing_quota_snapshot` 不再被生产代码读取、更新或展示，也不做数据迁移；repository 仍将该旧 key 保留在 `schedulerNeutralExtraKeys`，避免旧数据清理触发调度重建。
- Authorization 只用于上游请求，不得写入快照或 API DTO；失败正文只能截断后记录，不能泄露 token 或完整凭据。
- `reset-quota` 只保留稳定 API 契约；当前 xAI OAuth 不提供重置端点，service 返回明确的未支持错误，不能伪造成功。

### 4. Validation & Error Matrix

| 条件 | 结果 |
|---|---|
| service、repository、token provider 或 HTTP upstream 未配置 | `500 GROK_QUOTA_NOT_CONFIGURED` |
| 账号不存在 | `404 GROK_QUOTA_ACCOUNT_NOT_FOUND` |
| `platform != grok` | `400 GROK_QUOTA_INVALID_PLATFORM`，不得请求上游 |
| `type != oauth` | `400 GROK_QUOTA_INVALID_TYPE`，不得请求上游 |
| token 获取失败或为空 | `502 GROK_QUOTA_TOKEN_UNAVAILABLE` |
| base URL 不满足出站信任策略 | `400 GROK_QUOTA_BASE_URL_INVALID` |
| weekly/monthly 任一成功 | 返回并持久化合并 Billing；失败窗口写入 `partial/failed_windows` |
| weekly/monthly 都失败且原因相同 | 返回对应稳定上游错误，并保留可用的旧窗口数据 |
| weekly/monthly 失败原因不同 | `502 GROK_QUOTA_PROBE_PARTS_FAILED`，metadata 包含两个 status |
| Billing JSON 无法解析 | `502 GROK_QUOTA_BILLING_PARSE_ERROR` |
| 两个窗口都无有效数据 | `502 GROK_QUOTA_BILLING_EMPTY` |
| Billing 不足且 Responses probe 成功 | `/quota` 返回 `hybrid_probe`，Billing 与 header snapshot 分别写入既有 key |
| 更新 `grok_billing_snapshot` 或历史中性 key | 同步账号快照和缓存，但不得写 scheduler outbox |
| quota reset 请求 | `501 GROK_QUOTA_RESET_UNSUPPORTED` |

### 5. Good/Base/Bad Cases

- Good: 账号列表首次加载缺失 Billing 时，`AccountUsageService` 只发出 weekly/monthly 两个 GET，保存 `grok_billing_snapshot` 并投影 7d/30d 进度。
- Good: weekly 成功、monthly 失败时保留 weekly，返回 `partial=true`、`failed_windows=["monthly"]`，旧 monthly 可用数据按 merge 规则保留。
- Good: 显式 `/quota` 查询免费账号时，Billing 不足再执行一次 Responses probe，返回 `hybrid_probe` 和 rate-limit snapshot。
- Good: JWT/credentials 明确套餐与 Billing 套餐冲突时，前端按既有优先级选择，不从历史独立快照恢复旧值。
- Base: 存量账号只有 `grok_usage_snapshot` 时继续显示 request/token header；下一次账号 usage 刷新补充主 Billing 快照。
- Base: 新鲜 Billing 快照直接复用；force 刷新绕过 10 分钟 freshness 和一分钟 retry gate。
- Bad: 删除 `AccountUsageService` 构造器中的 `GrokQuotaService` 注入，使账号列表退化为只读本地快照。
- Bad: 账号列表自动调用 `ProbeUsage`，以真实 Responses 请求换取展示数据。
- Bad: 恢复 `/billing-quota`、`GrokBillingQuotaService`、`grok_billing_quota` DTO 或独立前端请求队列，重新形成重复 Billing 链路。
- Bad: 将旧 `grok_billing_quota_snapshot` 投影到当前套餐或进度条，造成历史展示数据覆盖 main 当前结果。
- Bad: 从 `schedulerNeutralExtraKeys` 删除历史 key，导致清理旧字段时产生无意义调度事件。

### 6. Tests Required

- `internal/pkg/xai/billing_test.go` 必须覆盖 URL、CLI headers、payload 解析、月/周归一化、plan、预付/on-demand 和空数据。
- `internal/service/grok_quota_service_test.go` 必须覆盖 Billing 重试、weekly/monthly 合并、部分成功、singleflight、`QueryQuota` Billing-only/hybrid、429/403、reset unsupported，以及账号 usage 只请求 Billing 的接线回归。
- `internal/service/grok_quota_fetcher_test.go` 必须覆盖 `grok_billing_snapshot`、`grok_usage_snapshot`、套餐 fallback、官方 7d/30d 和免费 24h 投影。
- `internal/repository/account_repo_grok_billing_test.go` 必须覆盖 `grok_billing_snapshot` 和历史 `grok_billing_quota_snapshot` 都是 scheduler-neutral 更新，不产生 scheduler outbox。
- handler/route 测试必须覆盖 `/quota`、标准 envelope、错误 reason 和凭据不泄露，并断言 `/billing-quota` 不再注册。
- 前端 `AccountUsageCell.spec.ts` 必须覆盖主 `grok_billing` 的周/月进度、免费 24h、header fallback、套餐和金额展示；`admin.grok.spec.ts` 不得重新导出独立 Billing API。
- 建议运行：

```bash
cd backend
go test -tags=unit ./internal/pkg/xai ./internal/service ./internal/handler/admin ./internal/server/routes ./internal/repository -run 'Grok|Billing|Quota|SSO' -count=1
cd ../frontend
pnpm vitest run src/api/__tests__/admin.grok.spec.ts src/components/account/__tests__/AccountUsageCell.spec.ts
pnpm typecheck
pnpm lint:check
git diff --check
```

### 7. Wrong vs Correct

#### Wrong

```go
func (s *AccountUsageService) getGrokUsage(ctx context.Context, account *Account, force bool) (*UsageInfo, error) {
	return s.grokQuotaFetcher.BuildUsageInfo(account), nil
}
```

问题：主体文件和快照读取仍存在，但账号列表不再刷新主 Billing，功能会静默退化为只读旧数据。

#### Correct

```go
if account.IsGrokOAuth() && s.grokQuotaService != nil &&
	(force || grokBillingSnapshotNeedsRefresh(account, time.Now())) {
	result, err := s.grokQuotaService.ProbeBilling(ctx, account.ID)
	// 将成功结果合并到当前 account，再由 fetcher 统一投影。
}
usage := s.grokQuotaFetcher.BuildUsageInfo(account)
```

账号 usage 在需要时只执行主 Billing 探测，再从主 Billing、header snapshot 和本地 usage 构建单一展示结果。

---

## Scenario: OpenAI Codex reset credit 元数据投影

### 1. Scope / Trigger

- Trigger: 修改 `backend/internal/repository/openai_codex_reset_client.go`、`backend/internal/service/openai_codex_reset_service.go` 或前端 `OpenAICodexReset*` 类型/组件时，必须按本节检查。
- 适用上游：ChatGPT Web 后端 `GET /backend-api/wham/rate-limit-reset-credits`。
- 适用管理端 API：`GET /api/v1/admin/accounts/:id/openai-codex-reset/status`。
- 目标：只投影 reset credit 的非敏感展示字段，避免记录或暴露 access token、refresh token、Cookie、完整 Authorization 或未脱敏上游响应。

### 2. Signatures

- 后端 client：`GetCredits(ctx context.Context, account service.OpenAICodexResetClientAccount) (*service.OpenAICodexResetCreditsResult, error)`
- 后端 service DTO：`service.OpenAICodexResetCreditStatus`
- 前端 API 类型：`OpenAICodexResetCreditStatus` in `frontend/src/api/admin/accounts.ts`
- 前端展示组件：`frontend/src/components/admin/account/OpenAICodexResetModal.vue`

### 3. Contracts

- 上游 credit 可包含：
  ```json
  {
    "id": "credit-1",
    "status": "available",
    "title": "Reset",
    "description": "...",
    "granted_at": "2026-07-01T12:00:00Z",
    "expires_at": "2026-07-08T12:00:00Z"
  }
  ```
- 本项目只透传非敏感字段：`id`、`status`、`title`、`description`、`granted_at`、`expires_at`。
- `granted_at` / `expires_at` 是可选字段。后端可接受 RFC3339/RFC3339Nano 字符串、Unix 秒时间戳、Unix 毫秒时间戳，并规范化为 UTC RFC3339 字符串。
- 无法解析或小于等于 0 的时间值视为缺失，不应让整个 credit 查询失败。
- 前端 API 类型必须保持 snake_case：`granted_at?: string`、`expires_at?: string`。
- 前端展示前必须通过统一日期格式化工具处理；格式化结果为空时不显示该行。

### 4. Validation & Error Matrix

- 上游 `expires_at` 为 RFC3339/RFC3339Nano 字符串 -> 后端响应 `expires_at` 为 UTC RFC3339。
- 上游 `expires_at` 为 Unix 秒数字 -> 后端响应 `expires_at` 为 UTC RFC3339。
- 上游 `expires_at` 为 Unix 毫秒数字 -> 后端响应 `expires_at` 为 UTC RFC3339。
- 上游 `expires_at` 缺失、`null`、空字符串、非日期字符串或小于等于 0 -> 后端省略该字段，前端不显示过期时间行。
- 上游请求失败 -> 继续走 `OPENAI_CODEX_RESET_UPSTREAM_FAILED` 错误路径，错误消息必须脱敏。

### 5. Good/Base/Bad Cases

- Good: 上游 `expires_at:"2026-07-06T12:00:00.123Z"`，API 返回 `expires_at:"2026-07-06T12:00:00Z"`，前端显示本地化过期时间。
- Good: 老上游响应没有 `expires_at`，弹窗仍正常展示 credit 标题、状态和描述。
- Base: 只修改展示字段，不改变 invite 发送、credit consume 或账号调度逻辑。
- Bad: 前端把字段写成 `expiresAt`，导致后端 snake_case 响应无法被类型契约覆盖。
- Bad: 后端把上游原始响应整体塞进 `rules`、日志或错误 message，泄露 token、账号或隐私字段。

### 6. Tests Required

- 后端 repository 单测必须覆盖：
  - RFC3339/RFC3339Nano 字符串规范化。
  - Unix 秒/毫秒时间戳规范化。
  - 字段透传到 `OpenAICodexResetCreditStatus.ExpiresAt`。
- 前端组件单测必须覆盖：
  - `expires_at` 有效时显示 i18n 文案和格式化时间。
  - 缺失或格式化为空时不显示过期时间行。
- 建议运行：
  ```bash
  cd backend && go test -tags=unit ./internal/repository ./internal/service -run OpenAICodexReset
  cd frontend && pnpm vitest run src/components/admin/account/__tests__/OpenAICodexResetModal.spec.ts
  cd frontend && pnpm typecheck
  ```

### 7. Wrong vs Correct

#### Wrong

```typescript
interface OpenAICodexResetCreditStatus {
  id: string
  expiresAt?: string
}
```

问题：前端类型改成 camelCase 后，实际 API 返回的 `expires_at` 不会被组件稳定消费。

#### Correct

```typescript
interface OpenAICodexResetCreditStatus {
  id: string
  expires_at?: string
}
```

保持后端 JSON tag 与前端 API 类型一致，组件只负责展示格式化后的值。

#### Wrong

```go
ExpiresAt string `json:"expires_at,omitempty"`
```

直接把上游原值当字符串透传会让数字时间戳、毫秒时间戳和非法值进入前端，导致显示不稳定。

#### Correct

```go
ExpiresAt openAICodexResetOptionalTimestamp `json:"expires_at"`
```

在上游 payload 边界先收窄并规范化时间格式；无法解析时投影为空值。

#### Wrong

```json
[
  {"role":"assistant","tool_calls":[{"id":"call_a"},{"id":"call_b"}]},
  {"role":"tool","tool_call_id":"call_b"},
  {"role":"tool","tool_call_id":"call_a"}
]
```

问题：并行工具完成顺序会进入 Chat payload。相同对话 replay 时，本地文件读取、Bash 或网络耗时变化会导致 prefix cache 边界漂移。

#### Correct

```json
[
  {"role":"assistant","tool_calls":[{"id":"call_a"},{"id":"call_b"}]},
  {"role":"tool","tool_call_id":"call_a"},
  {"role":"tool","tool_call_id":"call_b"}
]
```

连续 tool messages 按上一条 assistant `tool_calls` 顺序规范化，只在紧邻 tool result 区间内排序，不跨过普通 user message。

---

## Scenario: OpenAI Codex 上游客户端身份版本一致性

### 1. Scope / Trigger

- Trigger: 修改 OpenAI Codex 客户端版本设置或自动同步、HTTP/Chat Completions bridge/compact/OAuth passthrough/WebSocket 请求头、模型目录 `client_version`、账号用量探测、账号测试或后台 Codex User-Agent 时，必须按本节检查。
- 适用后端路径：
  - `backend/internal/service/openai_codex_identity.go`
  - `backend/internal/service/openai_gateway_forward.go`
  - `backend/internal/service/openai_gateway_passthrough.go`
  - `backend/internal/service/openai_ws_forwarder_payload.go`
  - `backend/internal/service/openai_codex_models_service.go`
  - `backend/internal/service/openai_codex_version_sync_service.go`
  - `backend/internal/service/account_usage_service.go`
  - `backend/internal/service/account_test_service.go`
  - `backend/internal/service/setting_gateway_runtime.go`
  - `backend/internal/service/wire.go`
- 目标：所有 OAuth Codex 出站请求使用同一个运行时生效版本，并让 `User-Agent`、`originator`、`version` 三者自洽；编译期常量只作为设置缺失或读取失败时的兜底，避免某条路径携带陈旧或矛盾身份被上游降载或拒绝。

### 2. Signatures

```go
func (s *SettingService) GetOpenAICodexClientVersion(ctx context.Context) string
func (s *SettingService) GetOpenAICodexCanonicalUserAgent(ctx context.Context) string
func (s *SettingService) InvalidateOpenAICodexClientVersionCache()
func SetCodexCanonicalUserAgentResolver(resolver func() string)
func SetCodexIdentityEnforcementEnabled(enabled bool)
func NormalizeCodexClientVersion(version string) string
func enforceCodexIdentityHeadersWithUA(h http.Header, overrideUA string)
func (s *OpenAIGatewayService) FetchCodexModelsManifest(ctx context.Context, account *Account, clientVersion, ifNoneMatch string) (*CodexModelsManifest, error)
```

### 3. Contracts

- 生效版本优先级固定为：管理员 `openai_codex_client_version` 覆写 → 自动同步值 `openai_codex_client_version_synced` → 编译期 `codexCLIVersion`。设置仓储缺失、读取失败、值为空或值非法时必须回退，不能把空值或未校验文本送往上游。
- `openai_codex_client_version_synced` 只允许版本同步服务写入，管理接口将其作为只读状态输出；`openai_codex_version_auto_sync_enabled` 只控制自动同步，不改变显式管理员覆写的最高优先级。
- `codexCLIVersion`、`codexCLIUserAgent` 和 `openAICodexProbeVersion` 是编译期兜底与语义别名，不是运行时唯一版本源。消费者不得新增第二套动态版本选择逻辑。
- `GetOpenAICodexCanonicalUserAgent` 必须用生效版本重建 UA 的版本段。后台或账号自定义 UA 只贡献客户端形态以及 OS、架构、终端等后缀；需要固定版本时应设置版本号并关闭自动同步，不能依赖旧 UA 中的版本文本。
- `wire.go` 必须通过 `SetCodexCanonicalUserAgentResolver` 注入 `GetOpenAICodexCanonicalUserAgent`。resolver 缺失或返回无法配对的 UA 时整体回退编译期规范身份。
- 默认开启身份强制统一。所有携带 `originator` 的 OAuth Codex 出站路径必须在最后一次 UA 改写后调用 `enforceCodexIdentityHeadersWithUA`，保证 `User-Agent`、`originator`、`version` 来自同一身份；没有 `originator` 的兼容桥接请求不得被该函数补回身份头。
- `gateway.disable_codex_identity_enforcement=true` 是回滚开关：保留最终客户端 UA，只配对 `originator`，并对低于 `codexUpstreamMinVersion` 的 `version` 做最低纠正。该开关不得绕过 Header 基本配对规则。
- `FetchCodexModelsManifest` 的 query `client_version` 保留调用方兼容语义：非空显式值 trim 后原样使用；空值使用 `GetOpenAICodexClientVersion`，服务不可用时回退 `openAICodexProbeVersion`。OAuth 请求头仍走统一身份 resolver，因此显式 query 不得覆盖 Header `Version`；自定义 API Key 上游沿用其独立兼容路径，不应用 OAuth 强制身份改写。
- HTTP Responses、Chat Completions bridge、compact、OAuth passthrough、WebSocket、alpha/search、模型目录、账号用量探测和账号测试必须复用同一收口规则。`min_codex_version` / `max_codex_version` 仍只属于入站准入门控，不能成为出站版本源。
- 身份修正不得改变模型名、路由、认证、404 model-not-found 识别或模型级冷却策略。

### 4. Validation & Error Matrix

| 条件 | 必须结果 |
|---|---|
| 管理员版本有效且非空 | 覆盖自动同步值与编译期兜底；规范 UA 和 Header `version` 使用该版本 |
| 管理员版本为空、同步版本有效 | 使用同步版本；规范 UA 与 Header `version` 同步更新 |
| 两个设置均为空、非法，或设置仓储读取失败 | 使用 `codexCLIVersion`，请求仍可构造 |
| 后台或账号 UA 含历史版本 | 保留客户端形态和后缀，用当前生效版本重建版本段 |
| 候选 UA 无法识别或 resolver 返回非法 UA | 整体回退规范 Codex 身份，不混用第三方 UA 与 Codex originator |
| 默认强制统一且请求携带 `originator` | `User-Agent`、`originator`、`version` 三者同源自洽 |
| 请求不携带 `originator` | `enforceCodexIdentityHeadersWithUA` 不修改请求头 |
| 禁用身份强制统一 | 保留可识别的最终 UA，配对 originator，并最低纠正过旧 version |
| 模型目录 `clientVersion` 为空 | query 使用运行时生效版本；OAuth 身份头也使用同一运行时身份 |
| 模型目录 `clientVersion` 非空 | query 使用显式值；OAuth Header `version` 仍使用运行时生效版本 |
| 模型目录走自定义 API Key 上游 | 保留显式 query/Header 兼容行为，不执行 OAuth 身份强制统一 |
| 自动同步关闭或拉取失败 | 不覆盖已保存同步值；版本选择继续按既定优先级工作 |

### 5. Good/Base/Bad Cases

- Good: 自动同步写入新稳定版后，HTTP、passthrough、WebSocket、模型目录 OAuth Header、探测和账号测试在缓存失效后使用同一个新版本。
- Good: 管理员用版本设置固定客户端版本，同时自定义 UA 的系统后缀；最终 UA 形态保留，版本段与 Header `version` 一致。
- Base: 模型目录调用方显式传入历史 `client_version`，URL query 保留该值用于兼容；OAuth 身份 Header 仍按运行时规范身份发送。
- Base: 自定义 API Key 上游继续接收调用方显式版本和 Header override，不被 ChatGPT OAuth 身份规则覆盖。
- Bad: 从 `client_version` query、入站 UA 或 `min_codex_version` 推导 OAuth Header `version`，形成第二个运行时版本源。
- Bad: 逐字透传后台或账号 UA 的旧版本段，使自动同步只更新 Header `version` 而 UA 仍陈旧。
- Bad: 在 HTTP、WebSocket、模型目录或探测路径分别拼装身份头，导致 `originator` 与 UA 首段不配套。
- Bad: 用 `min_codex_version` 作为上游 `Version`，把入站准入策略错误耦合到上游客户端身份。
- Bad: 为规避一次 404 而关闭 model-not-found 识别或缩短冷却，掩盖真正不存在模型的错误。

### 6. Tests Required

- `openai_codex_identity_test.go` 必须覆盖规范 resolver、账号 UA 重建、非法 UA 回退、禁用强制统一、无 originator 不处理、幂等与版本最低纠正。
- `openai_codex_version_sync_service_test.go` 必须覆盖管理员覆写、同步值、编译期兜底、读取失败、自动同步开关、只前进不回退以及规范 UA 重建。
- `openai_codex_models_service_test.go` 必须分别覆盖空值动态版本、显式 `client_version`、OAuth 统一 Header 和自定义 API Key 上游例外。
- OAuth passthrough 测试必须以 `gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna` 为表项，断言模型名原样转发且内置 UA 使用当前版本。
- `openai_gateway_chat_completions_test.go`、`openai_oauth_passthrough_test.go` 和 `openai_ws_forwarder_success_test.go` 必须断言各协议复用统一身份。
- 现有 compact、alpha/search、账号测试、用量探测和后台设置 API 测试必须继续通过。
- 现有 404 model-not-found 测试必须继续通过，至少包括识别、模型级冷却和不封禁整个账号三类断言。
- 必须运行：

```bash
cd backend && go test -tags=unit -count=1 ./internal/service
cd backend && go test -tags=unit -count=1 ./...
cd backend && GOTOOLCHAIN=go1.26.6 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.9.0 run --timeout=30m --build-tags=unit --new-from-rev=HEAD ./...
git diff --check
```

### 7. Wrong vs Correct

#### Wrong

```go
headers.Set("User-Agent", account.GetOpenAIUserAgent())
headers.Set("Originator", "codex_cli_rs")
headers.Set("Version", clientVersion)
```

问题：账号 UA、固定 originator 和调用方 query 版本来自不同来源，可能形成无法通过上游校验的身份三元组，也会绕过管理员覆写与自动同步。

#### Correct

```go
headers.Set("Originator", openai.CodexDefaultOriginator)
enforceCodexIdentityHeadersWithUA(headers, s.codexIdentityOverrideUA(account))
```

统一收口从规范 resolver 取得运行时生效版本，重建 UA 版本段并配对 originator；模型目录显式 `client_version` 只保留在 URL query，自定义 API Key 上游则保持独立兼容路径。

---

## Scenario: Codex Alpha Search 独立端点转发

### 1. Scope / Trigger

- Trigger: 新增或修改 Codex Responses Lite 的 standalone `alpha/search` 路由、OpenAI 账号调度、模型映射、上游 URL、query/body 透传、响应头过滤或 failover 行为时，必须按本节检查。
- 适用后端路径：
  - `backend/internal/handler/endpoint.go`
  - `backend/internal/handler/openai_alpha_search.go`
  - `backend/internal/server/routes/gateway.go`
  - `backend/internal/service/openai_alpha_search.go`
- 目标：把仍在演进的 alpha 请求和响应作为不透明 JSON 转发，只读取调度必需的 `model` 和会话粘性使用的 `id`，避免复制未稳定 schema。

### 2. Signatures

```go
const EndpointAlphaSearch = "/v1/alpha/search"

func NormalizeInboundEndpoint(path string) string
func DeriveUpstreamEndpoint(inbound, rawRequestPath, platform string) string
func (h *OpenAIGatewayHandler) AlphaSearch(c *gin.Context)
func (s *OpenAIGatewayService) ForwardAlphaSearch(ctx context.Context, c *gin.Context, account *Account, body []byte) (*OpenAIForwardResult, error)
```

必须注册的 HTTP 路径：

```text
POST /v1/alpha/search
POST /alpha/search
POST /backend-api/codex/alpha/search
```

### 3. Contracts

- 三个入站路径都必须归一化为 `EndpointAlphaSearch`，且只允许 OpenAI 分组进入 handler。
- 请求体必须是合法 JSON，`model` 必须是 trim 后非空的字符串；`id` 可缺失，只用于生成 session hash。
- channel model mapping 先在 handler 应用，账号级 model mapping 再由 service 应用；除最终模型名外，不重建请求体。
- 入站 query 参数逐值追加到目标 URL，未知 JSON 字段和请求结构原样保留。
- OpenAI OAuth 账号固定转发到 `https://chatgpt.com/backend-api/codex/alpha/search`。
- OpenAI API Key 账号使用已校验的账号 base URL，并通过 `buildOpenAIEndpointURL(base, "/v1/alpha/search")` 构造目标；未配置 base URL 时使用 `https://api.openai.com/v1/alpha/search`。
- OAuth 请求缺少入站 `Version` 时使用 `codexCLIVersion`；显式 `Version` 原样保留。客户端未提供 `OpenAI-Beta` 时不得额外注入该头。
- 调度必须复用用户并发、账号并发、session sticky、模型限制、账号健康、最大切换次数和现有 failover 副作用。
- 上游非切换错误原样返回状态、body 和白名单响应头；可切换错误必须先返回 `UpstreamFailoverError`，不得提前写下游响应。
- 仅上游 2xx 表示一次真实成功的搜索：返回非 nil `OpenAIForwardResult`，并设置 `WebSearchCalls=1`。已原样透传的非 2xx 或重定向返回 `(nil, nil)`，不得计费；failover 和传输错误继续通过 error 返回。
- handler 只在 result 非 nil 时使用 mandatory usage 池记录用量；池满时同步兜底，避免成功搜索漏扣费。请求体哈希、channel mapping、账号、订阅和 quota platform 必须进入现有 `RecordUsage` 链路。
- 按次价格来自 `groups.web_search_price_per_call`；null 使用默认 `0.01 USD/次`，显式 `0` 表示免费。最终费用为调用次数 × 单次价格 × 分组倍率。
- `web_search_price_per_call` 必须在 migration、Ent schema/生成代码、Group service/DTO、API key cache snapshot、前端 Group/Create/Update 类型和管理页之间保持 snake_case 契约一致。
- 不修改 `/responses`、hosted web search 或 web search emulation 的 wire contract；按次计费只由 Alpha Search 成功结果的 `WebSearchCalls` 触发。

### 4. Validation & Error Matrix

| 条件 | 必须结果 |
|---|---|
| API Key 缺失或无分组 | `401 authentication_error` |
| 分组平台不是 OpenAI | `404 not_found_error`，不调度账号 |
| 请求体为空、非法 JSON 或读取失败 | 对应 `400`；超过限制时 `413` |
| `model` 缺失、非字符串或 trim 后为空 | `400 invalid_request_error` |
| 无可用账号且未发生切换 | 复用现有 no-account 分类结果 |
| 上游响应满足 failover 条件且尚未写响应 | 返回 `UpstreamFailoverError`，handler 切换账号 |
| failover 已耗尽 | 复用 `handleFailoverExhausted` 输出最终错误 |
| API Key base URL 校验失败 | 不发上游请求，按普通转发失败处理 |
| 上游普通 4xx/5xx 不满足 failover | 原样透传状态、JSON body 和允许的响应头 |
| 上游返回 2xx | 返回 `WebSearchCalls=1` 的 result，并提交一次 mandatory usage 记录 |
| 上游返回普通非 2xx 或重定向 | 响应原样透传，result 为 nil，不产生按次费用 |
| 分组单价为 null / 0 / 正数 | 分别使用默认 0.01 / 免费 / 显式单价，并应用分组倍率 |

### 5. Good/Base/Bad Cases

- Good: `/backend-api/codex/alpha/search?feature=standalone` 使用 OAuth 账号时，query 和未知 JSON 字段都到达 ChatGPT Codex standalone search。
- Good: API Key 账号配置 `https://compat.example/v4` 时，目标为 `https://compat.example/v4/alpha/search`，模型映射只替换 `model`。
- Base: 请求没有 `id` 时仍可调度，由现有 session hash fallback 决定粘性。
- Base: 上游返回不可切换的 `400` 时，客户端收到上游状态和 JSON 错误体，不把它改造成内部 envelope。
- Bad: 把 alpha 请求绑定到本地 DTO 后重新序列化，会丢失上游新加的未知字段。
- Bad: 把 standalone search 折叠进 `/responses` 或 hosted web search，会改变 URL、调度能力和 wire contract。
- Bad: 收到 `429` 后先 `c.Data` 再尝试 failover，会导致响应已经提交，无法安全切换账号。

### 6. Tests Required

- endpoint 单测必须覆盖三个入站路径都归一化为 `EndpointAlphaSearch`，以及 OpenAI 上游端点推导保持 `/v1/alpha/search`。
- route 单测必须覆盖三个 `POST` 路径已注册，非 OpenAI 分组返回 `404`。
- service 单测必须覆盖：
  - OAuth URL、Authorization、账号头、`Version`、query 和未知 body 字段透传。
  - API Key 自定义 base URL、Authorization 和模型映射。
  - 可切换上游错误返回 `UpstreamFailoverError`，且下游 writer 尚未写入。
  - 2xx 返回 `WebSearchCalls=1`；普通错误透传返回 nil result，不计费。
- billing/cache 单测必须覆盖默认价格、显式价格、免费价格、倍率、零调用和 API key cache snapshot round-trip。
- contract 与前端检查必须覆盖 `web_search_price_per_call` 的 null 字段、创建/编辑 payload 和价格预览。
- 建议运行：

```bash
cd backend
go test -tags=unit ./internal/handler ./internal/server/routes ./internal/service \
  -run 'AlphaSearch|WebSearch|NormalizeInboundEndpoint|DeriveUpstreamEndpoint' -count=1
go test -tags=unit ./internal/server ./internal/repository -count=1
cd ../frontend
pnpm vitest run src/i18n/__tests__/opsLocaleKeys.spec.ts
pnpm typecheck
```

### 7. Wrong vs Correct

#### Wrong

```go
var req AlphaSearchRequest
if err := c.ShouldBindJSON(&req); err != nil {
	return
}
body, _ := json.Marshal(req)
```

问题：alpha schema 仍在演进，本地 DTO 会静默丢弃未知字段，并使代理行为依赖发布节奏。

#### Correct

```go
body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
if err != nil || !gjson.ValidBytes(body) {
	// 在边界返回兼容错误。
}
model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
```

只读取调度所需字段，转发原始 JSON，并由既有 model replacement 精确替换模型名。

---

## Scenario: OpenAI Responses 最终上游模型解析

### 1. Scope / Trigger

- Trigger: 修改 OpenAI Responses 的账号模型映射、API Key/OAuth 透传、compact、Responses -> Chat Completions 回退、频道模型限制、Responses Lite 判定或 reasoning effort 记录时，必须按本节检查。
- 适用路径：
  - `backend/internal/service/openai_model_mapping.go`
  - `backend/internal/service/openai_gateway_forward.go`
  - `backend/internal/service/openai_gateway_passthrough.go`
  - `backend/internal/service/openai_gateway_request_body.go`
  - `backend/internal/service/openai_gateway_scheduling.go`
  - `backend/internal/service/openai_gateway_service.go`
  - `backend/internal/service/openai_responses_lite_policy.go`
- 目标：同一次 attempt 的请求体模型、频道限制、Lite 策略和 reasoning effort 必须基于同一个最终上游模型，不能各自重复模型映射顺序。

### 2. Signatures

```go
func resolveOpenAIAccountUpstreamModelForRequest(
	account *Account,
	requestedModel string,
	requireCompact bool,
) string

func extractOpenAIUpstreamReasoningEffort(
	body []byte,
	requestedModel string,
	mappedModel string,
	additionalModelCandidates ...string,
) *string
```

共享解析结果至少由以下调用点消费：`forwardOpenAIPassthrough`、`resolveOpenAIResponsesLitePolicyModel`、`normalizeOpenAICodexCompactReasoningEffortForAccount` 和 `isUpstreamModelRestrictedByChannel`。

### 3. Contracts

- `requestedModel` 必须先 trim；空模型返回空字符串，不能凭账号类型补默认模型。
- Responses -> Chat Completions 回退先于 passthrough 分支判定；该路径只应用普通 `model_mapping` 和既有上游归一化，不应用 compact 映射。
- passthrough compact 必须直接用入站/频道映射后的模型查询 `compact_model_mapping`，不能先套普通 `model_mapping`。
- OAuth passthrough 普通 Responses 必须应用普通账号映射，再执行 Codex 上游模型归一化。
- API Key passthrough 普通 Responses 必须保持入站/频道映射后的模型；账号残留的普通 `model_mapping` 不能改写实际请求。
- managed 普通请求沿用普通账号映射和上游归一化；managed compact 先应用普通映射，再按映射结果查询 compact 映射。
- `forwardOpenAIPassthrough` 写入 body 的模型必须来自共享解析函数；Responses Lite、频道限制和 compact reasoning 不能另行调用 `GetMappedModel` 拼装近似结果。
- passthrough reasoning effort 必须将实际 `upstreamModel` 作为候选传给 `extractOpenAIUpstreamReasoningEffort`；记录值不能只根据客户端模型或未生效的账号映射推断。
- failover 每次 attempt 都必须按当前账号重新解析；频道调度上下文存在模型映射时，共享解析函数接收该频道映射结果，而不是回退到最初客户端别名。

### 4. Validation & Error Matrix

| 请求路径 | 普通映射 | compact 映射 | 最终模型规则 |
|---|---|---|---|
| OAuth passthrough 普通 Responses | 应用 | 不应用 | 普通映射后执行 Codex 归一化 |
| API Key passthrough 普通 Responses | 不应用账号残留映射 | 不应用 | 保持入站/频道映射后的模型 |
| passthrough compact | 不先应用 | 直接按当前请求模型应用 | 使用 compact 映射结果 |
| passthrough 标记存在但 Responses 不受支持 | 应用 | 不应用 | 走 raw Chat fallback 的普通映射结果 |
| managed 普通 Responses | 应用 | 不应用 | 普通映射后执行既有归一化 |
| managed compact | 先应用 | 按普通映射结果应用 | 使用 compact 结果；未命中时沿用普通结果 |
| 模型为空 | 不应用 | 不应用 | 返回空字符串 |

所有分支解析完成后：请求体、频道 restriction、Responses Lite 和 reasoning effort 必须看到同一结果；任一消费者得到不同模型都属于阻塞回归。

### 5. Good/Base/Bad Cases

- Good: OAuth passthrough 的 `client-alias -> gpt-5.6` 最终写入 `gpt-5.6-sol`，频道限制和 Lite 判定也使用 `gpt-5.6-sol`。
- Good: API Key passthrough 即使残留 `client-alias -> gpt-5.5`，普通 Responses 仍向上游发送 `client-alias`，reasoning 也不把残留映射当成实际模型。
- Good: passthrough compact 同时配置普通映射和 compact 映射时，直接按 `client-alias` 命中 compact 模型，不经过普通映射。
- Good: passthrough 账号不支持 Responses 时，raw Chat fallback 使用普通账号映射，并忽略 compact 标记。
- Base: managed compact 未命中 compact 映射时，保持普通映射和既有归一化结果。
- Base: 频道没有映射时，按客户端请求模型进入相同分支矩阵。
- Bad: Lite 策略直接调用 `account.GetMappedModel(requestedModel)`，会错误阻止 API Key passthrough 或漏掉 compact/OAuth 最终模型。
- Bad: passthrough compact 先执行普通映射再查 compact 映射，会让仅按客户端别名配置的 compact 映射失效。
- Bad: 请求 body 使用最终模型，但频道限制或 reasoning 仍使用客户端模型，会造成调度通过后上游模型被限制，或 usage 记录错误档位。

### 6. Tests Required

- `TestResolveOpenAIAccountUpstreamModelForRequest` 必须覆盖 OAuth passthrough、API Key passthrough、passthrough compact、raw Chat fallback 和 managed compact 五条分支。
- `TestIsUpstreamModelRestrictedByChannel_OAuthPassthroughUsesAccountMapping` 必须钉死频道限制使用 OAuth passthrough 的最终账号模型。
- `TestApplyOpenAIResponsesLiteHTTPIngressPolicy_UsesPassthroughFinalModel` 必须覆盖 API Key 保持入站模型、OAuth 普通映射和 passthrough compact 直接映射。
- `TestNormalizeOpenAICodexCompactReasoningEffortForAccountUsesFinalCompactModel` 必须覆盖“最终 compact 为 GPT-5.6 才降级”和“仅普通映射为 GPT-5.6 不误降级”两个方向。
- `TestOpenAIGatewayService_APIKeyPassthrough_PreservesBodyAndUsesResponsesEndpoint` 必须断言 API Key passthrough body 不受残留普通映射影响，同时 reasoning 候选使用实际上游模型。
- 建议运行：

```bash
cd backend
go test -tags=unit ./internal/service \
  -run 'ResolveOpenAIAccountUpstreamModelForRequest|IsUpstreamModelRestrictedByChannel_OAuthPassthroughUsesAccountMapping|ApplyOpenAIResponsesLiteHTTPIngressPolicy_UsesPassthroughFinalModel|NormalizeOpenAICodexCompactReasoningEffortForAccountUsesFinalCompactModel|APIKeyPassthrough_PreservesBodyAndUsesResponsesEndpoint' \
  -count=1
```

### 7. Wrong vs Correct

#### Wrong

```go
upstreamModel := account.GetMappedModel(requestedModel)
if compact {
	upstreamModel = resolveOpenAICompactForwardModel(account, upstreamModel)
}
```

问题：所有路径都按“普通映射后 compact”处理，破坏 API Key 原生透传、passthrough compact 和 raw Chat fallback 的不同契约。

#### Correct

```go
upstreamModel := resolveOpenAIAccountUpstreamModelForRequest(
	account,
	requestedModel,
	compact,
)
reasoningEffort := extractOpenAIUpstreamReasoningEffort(
	body,
	requestedModel,
	upstreamModel,
)
```

共享解析函数拥有分支顺序；下游只消费最终模型，不重新解释账号类型、透传模式或 compact 映射。

---

## Scenario: Codex 生图桥接与 Responses Lite 工具边界

### 1. Scope / Trigger

- Trigger: 修改 Codex 图片工具注入、`tool_choice`、Responses Lite 标头/WS metadata、阻止模型设置、账号 Header Override、HTTP Responses 或 WebSocket Responses 入站归一化时，必须按本节检查。
- 适用后端路径：
  - `backend/internal/service/account_header_override.go`
  - `backend/internal/service/openai_codex_transform.go`
  - `backend/internal/service/openai_gateway_forward.go`
  - `backend/internal/service/openai_gateway_passthrough.go`
  - `backend/internal/service/openai_gateway_service.go`
  - `backend/internal/service/openai_responses_lite_policy.go`
  - `backend/internal/service/openai_ws_forwarder_ingress.go`
  - `backend/internal/service/openai_ws_v2_passthrough_adapter.go`
  - `backend/internal/service/openai_ws_http_bridge.go`
- 适用设置链路：`SettingService -> admin settings DTO/handler -> frontend SettingsView`。
- 目标：区分上游执行的 hosted `image_generation` 与客户端执行的 `image_gen`，并根据每次转发的最终上游模型统一决定 Lite Header/metadata 是否传播。

### 2. Signatures

```go
const SettingKeyOpenAIResponsesLiteHeaderBlockedModels = "openai_responses_lite_header_blocked_models"

func NormalizeOpenAIResponsesLiteHeaderBlockedModels(models []string) ([]string, error)
func (s *SettingService) ShouldBlockOpenAIResponsesLite(ctx context.Context, finalModel string) bool
func (s *OpenAIGatewayService) resolveOpenAIResponsesLitePolicyModel(
	ctx context.Context,
	account *Account,
	requestedModel string,
	compact bool,
) string
func (s *OpenAIGatewayService) applyOpenAIResponsesLiteHTTPBodyPolicy(
	ctx context.Context,
	account *Account,
	body []byte,
	finalModel string,
	headerValue string,
) ([]byte, bool, error)
func (s *OpenAIGatewayService) applyOpenAIResponsesLiteWebSocketPolicy(
	ctx context.Context,
	account *Account,
	body []byte,
	finalModel string,
) ([]byte, bool, error)
func (s *OpenAIGatewayService) enforceOpenAIResponsesLiteHTTPHeader(
	ctx context.Context,
	req *http.Request,
	account *Account,
	finalModel string,
)

func hasOpenAIImageGenClientTool(reqBody map[string]any) bool
func ensureOpenAIResponsesImageGenerationTool(reqBody map[string]any) bool
func ensureOpenAIResponsesImageGenerationToolChoiceAuto(reqBody map[string]any) bool
func applyCodexImageGenerationBridgeInstructions(reqBody map[string]any) bool
```

设置 API 字段：

```json
{
  "openai_responses_lite_header_blocked_models": ["gpt-5.4", "gpt-5.4-mini", "gpt-5.5"]
}
```

### 3. Contracts

- 设置键缺失时使用三个精确默认项：`gpt-5.4`、`gpt-5.4-mini`、`gpt-5.5`；显式存储 `[]` 表示允许所有模型透传，不能回退默认值。
- 更新 DTO 使用 `*[]string` 区分“字段未提供”和“显式空数组”；未提供时保留旧值，显式 `[]` 整体覆盖。
- 每条规则 trim 后不能为空，稳定去重；只支持精确匹配或一个位于末尾的 `*` 前缀规则，匹配保持大小写敏感。
- 运行时使用 `SettingService` 的 60 秒成功 TTL、5 秒错误 TTL 和 singleflight；存储 JSON 非法时记录不含敏感信息的 warning，并使用默认列表。
- Lite 决策必须使用完成账号映射、compact 映射、OAuth 归一化和图片主模型转换后的最终上游模型；failover 的每次 attempt 和 WS 的每个 turn 都重新计算。
- 只有入站 HTTP Header 为 `true` 或 WS metadata 为 `true` 才是 Lite 请求。账号 Header Override 不得注入 `X-OpenAI-Internal-Codex-Responses-Lite`；保存时拒绝，运行时也要防御性丢弃旧数据。
- OpenAI 最终模型未命中阻止列表时：HTTP managed/passthrough 保留 Header，WS 直连保留 metadata，WS HTTP bridge 可以重建 Header，并执行 Lite 工具布局和 `reasoning.context=all_turns` 归一化。
- OpenAI 最终模型命中阻止列表时：删除 HTTP Header/WS metadata，bridge 不得重建 Header，并跳过 Lite 专属 body normalizer。
- 命中阻止列表只执行有限兼容降级：客户端已有的 `reasoning.context`、developer message、`input.additional_tools`、`parallel_tool_calls` 和其它 body 字段保持原样，不做完整 Lite -> 标准 Responses 逆转换。
- 非 OpenAI 平台不得收到该内部标记；Grok 普通 Responses、媒体请求和 WS HTTP bridge 都必须保持 Header 为空。
- `image_generation` 是 OpenAI Responses hosted 工具，由上游执行；已有该工具时不得重复注入，旧 `format` / `compression` 字段仍按既有兼容契约归一化。
- `image_gen` 是 Codex 客户端工具。只有 namespace 内含 `type=function,name=imagegen` 时才视为可执行；兼容扁平形态严格匹配 `type=function,name=image_gen.imagegen`。
- 顶层 `tools` 与 `input` 中的 `additional_tools` 都必须参与客户端工具分类。仅在描述文本中出现 `image_gen.imagegen` 不构成工具声明。
- 已有客户端 `image_gen` 时不得注入 hosted 工具、不得追加 hosted bridge 提示，也不得把客户端 `tool_choice: "none"` 改为 `"auto"`。
- 没有原生或客户端图片工具且 bridge 有效时，注入一个 `image_generation`；此时 `tool_choice` 缺失或忽略大小写和空白后等于 `none` 时写为 `auto`。
- `auto`、`required`、其它字符串、明确工具选择对象和其它非字符串值必须保持。
- HTTP 只在 `codexImageGenerationBridgeEnabled` 为真时调用；WebSocket 只在 `codexBridgeEnabled` 为真时调用。
- group 禁止图片、全局/频道/账号未启用桥接、账号显式工具策略为 `strip`、compact 请求或 Spark 模型时，不得通过本归一化重新开放图片工具。
- HTTP 与 WebSocket 不得复制一份独立的 `none` 判断逻辑，避免协议分支漂移。
- passthrough 除 Lite 协议归一化和账号显式 `strip` 策略外不得注入 hosted 图片工具。

### 4. Validation & Error Matrix

| 条件 | Header / metadata | Lite body 归一化 | 结果 |
|---|---|---|---|
| 设置键缺失 | 使用三个默认阻止项 | N/A | 不创建空默认 |
| 设置值为合法 `[]` | 所有 OpenAI 模型允许 | allow 时执行 | 保存后立即生效 |
| 设置 JSON 非法或元素非法 | 回退默认列表 | 按默认规则 | warning + 5 秒错误缓存 |
| 更新规则为空或 `*` 位置非法 | N/A | N/A | `400 INVALID_OPENAI_RESPONSES_LITE_HEADER_BLOCKED_MODELS` |
| 非 Lite 请求 | 不新增标记 | 不执行 | body 和无关 Header 保持 |
| Lite + 最终模型 allow | 保留/重建标记 | 执行 | `reasoning.context=all_turns` |
| Lite + 最终模型 block | 删除/禁止重建 | 跳过 | 客户端原始 body 字段保持 |
| Lite + 客户端显式 context + block | 删除标记 | 跳过 | 显式 context 原值保持 |
| Grok 或其它非 OpenAI 平台 | 删除标记 | 不执行 | 内部 Header 不进入上游 |
| Header Override 配置 Lite Header | 保存拒绝、旧数据丢弃 | 不执行 | 普通请求不能被伪造成 Lite |

图片工具矩阵：

| 条件 | hosted 工具 | `tool_choice` |
|---|---|---|
| 无图片工具，bridge 有效 | 注入一个 | 缺失或 `none` 改为 `auto` |
| 已有原生 `image_generation` | 不重复注入 | `none` 可改为 `auto`，其它明确选择保持 |
| 可执行 `image_gen` namespace/扁平 function | 不注入 | 保持客户端值 |
| 仅描述文本提到 `image_gen.imagegen` | 按无图片工具处理 | 按 hosted 规则处理 |
| group 禁止、bridge 关闭、`strip`、compact 或 Spark | 不恢复注入 | 保持既有门禁语义 |

### 5. Good/Base/Bad Cases

- Good: Lite 请求从客户端别名映射到默认阻止的 `gpt-5.5` 后，按映射后的模型删除标记且保持客户端显式 context。
- Good: `gpt-5.6-terra` 未命中阻止列表时，managed、passthrough、WS 和 bridge 均保留 Lite 标记并补齐 `all_turns`。
- Good: 管理员显式保存空数组后，`gpt-5.5` 的 Lite 请求允许透传，而不是重新套用默认列表。
- Good: 同一 WS 会话从 allow 模型切到 block 模型再切回 allow，每个 turn 独立更新 metadata 和 context。
- Good: 旧账号数据试图通过 Header Override 注入 Lite Header 时，OpenAI 普通请求和 Grok 请求都不会收到该标记。
- Good: Codex HTTP 请求只带 `tool_choice: "none"`，桥接注入图片工具后同一请求被改成 `"auto"`，模型可选择调用图片工具。
- Good: WebSocket `additional_tools` 含可执行 `image_gen` 时保持客户端工具、提示和 `none`，不追加 hosted 工具。
- Base: 非 Lite 请求即使模型位于阻止列表，也不修改 body 或其它 Header。
- Base: block 模型原始 body 已有 developer message、additional tools 或 `parallel_tool_calls` 时保持原样。
- Base: 客户端明确使用 `"required"` 或图片工具对象时，桥接尊重其选择，不覆盖。
- Base: 管理员关闭桥接后，文本请求不会被注入图片工具，也不会因为 `none` 被改写。
- Bad: 只判断字段是否存在并直接返回，会让 `tool_choice: "none"` 永久禁止已注入的图片工具。
- Bad: 无条件把所有 `tool_choice` 写成 `"auto"`，会破坏 `required` 和明确工具选择。
- Bad: 看到 Lite header 或 metadata 就整体关闭 bridge，会让没有客户端图片工具的请求失去 hosted fallback。
- Bad: 对所有模型永久删除 Lite 标记，会破坏 Lite-capable 模型的官方请求布局。
- Bad: 无条件透传或只按客户端原始模型判断，会让非 Lite 模型返回 `unsupported_value`。
- Bad: 允许 Header Override 注入 Lite Header，会产生“Header 是 Lite、body 未归一化”的不一致请求，并把内部标记泄漏给 Grok。
- Bad: block 时删除 developer message、additional tools 或客户端 context，属于未经授权的完整协议逆转换。
- Bad: 在 HTTP 和 WebSocket 各自实现不同的字符串判断，会再次形成协议分支行为漂移。

### 6. Tests Required

- 设置测试必须覆盖：缺失键的三个默认项、显式 `[]`、非法 JSON 回退、trim、稳定去重、精确/末尾通配符、缓存命中、singleflight 和保存后刷新。
- Settings API/前端测试必须覆盖：查询/更新字段、未提供时保留、显式空数组、空项和非法通配符校验、中英文 i18n 与 API contract 快照。
- HTTP managed/passthrough 必须覆盖：allow、默认 block、自定义通配符、显式空列表和映射后最终模型；block 时不强制改写 context。
- WS 直连和 passthrough 必须覆盖 allow/block、模型映射和会话内 turn 切换；WS HTTP bridge 必须覆盖允许重建和阻止重建。
- Grok 回归必须断言普通 Responses、媒体和 WS HTTP bridge 不带 Lite Header；Header Override 的保存校验与运行时防御性过滤都要覆盖。
- 普通 OpenAI 请求必须覆盖：即使账号旧数据包含 Lite Header Override，也不能新增 Header，且 body 不执行 Lite normalizer。
- 纯函数测试至少覆盖：原生工具、可执行 namespace、空 namespace、顶层/`additional_tools`、扁平 function、相似名称、`none`、`auto`、`required` 和 Spark。
- HTTP service 回归测试必须断言桥接启用时 `none -> auto`，同时保留现有 bridge disabled、group disabled、`strip` 和明确工具选择测试。
- 真实 WebSocket ingress 必须分别覆盖 Lite 无工具时注入，以及 Lite 有客户端工具时不接管。
- 建议运行：

```bash
cd backend
go test -tags=unit ./internal/service \
  -run 'HeaderOverride|ImageGenerationToolChoice|ImageGenerationBridge|ResponsesLite|ImageGenClientTool|OpenAIWSHTTPBridge' \
  -count=1
go test -tags=unit ./internal/server -run TestAPIContracts -count=1

cd ../frontend
pnpm vitest run src/components/account/__tests__/credentialsBuilder.spec.ts \
  src/views/admin/__tests__/SettingsView.spec.ts \
  src/i18n/__tests__/localesMessageCompile.spec.ts
```

### 7. Wrong vs Correct

#### Wrong

```go
if isOpenAIResponsesLiteHeader(inboundHeader) {
	body, _, err = normalizeOpenAIResponsesLiteToolsPayload(body)
}
account.ApplyHeaderOverrides(req.Header) // 可以再次注入 Lite Header
```

问题：没有按最终模型判断，并允许账号覆写在 body 归一化之后制造 Lite Header，造成 Header/body 不一致和非 OpenAI 平台泄漏。

#### Correct

```go
finalModel := s.resolveOpenAIResponsesLitePolicyModel(ctx, account, requestedModel, compact)
if isOpenAIResponsesLiteHeader(inboundHeader) {
	body, _, err = s.applyOpenAIResponsesLiteHTTPBodyPolicy(
		ctx, account, body, finalModel, inboundHeader,
	)
}
account.ApplyHeaderOverrides(req.Header) // 禁止覆写名单已包含 Lite Header
s.enforceOpenAIResponsesLiteHTTPHeader(ctx, req, account, finalModel)
```

body 归一化只接受真实入站 Lite 信号；Header 在请求构造的最后边界按最终模型再次收口，Header Override 不能绕过协议所有权。图片 bridge 是否可用仍只由权限和图片策略决定，是否实际注入由原生/客户端工具分类决定。

---

## Scenario: OpenAI Structured Outputs 降级与多路径 Web Search 桥接

### 1. Scope / Trigger

- Trigger: 修改 OpenAI APIKey 账号的 `json_schema` 兼容、Responses Web Search 本地接管、Codex Responses Lite 隐式搜索桥、Responses -> Chat 的 typed Web Search / `web.run` 工具循环、Responses -> Anthropic -> Responses 原生搜索桥或 DeepSeek Anthropic SSE 聚合时，必须按本节检查。
- 适用路径：
  - `backend/internal/pkg/apicompat/json_schema_downgrade.go`
  - `backend/internal/pkg/apicompat/responses_to_anthropic_request.go`
  - `backend/internal/pkg/apicompat/anthropic_to_responses_response.go`
  - `backend/internal/service/openai_json_schema_downgrade.go`
  - `backend/internal/service/openai_responses_websearch.go`
  - `backend/internal/service/openai_responses_web_run.go`
  - `backend/internal/service/codex_web_search_bridge.go`
  - `backend/internal/service/openai_gateway_responses_chat_fallback.go`
  - `backend/internal/pkg/websearch/manager.go`
  - `backend/internal/service/gateway_forward_as_responses.go`
  - `frontend/src/features/webSearch/codexBridge.ts`
- 目标：显式配置后可把不受上游支持的 Structured Outputs 降为 `json_object`，并让 Web Search 选择原生转发、直接模拟、Chat 内部工具循环、Codex Lite 隐式桥或明确拒绝；不得把 Schema、搜索工具或 `tool_choice` 静默丢弃。

### 2. Signatures

```go
func DowngradeResponsesJSONSchemaToJSONObject(body []byte) ([]byte, bool, error)
func DowngradeChatJSONSchemaToJSONObject(body []byte) ([]byte, bool, error)
func (a *Account) IsOpenAIJSONSchemaToJSONObjectEnabled() bool
func (a *Account) GetWebSearchEmulationMode() string
func (c *Channel) CodexWebSearchBridgeOverride(platform string) *bool
func (a *Account) CodexWebSearchBridgeOverride() *bool
func (m *Manager) HasAvailableProvider(ctx context.Context, accountProxyURL string) bool
func EffectiveResponsesTools(req *ResponsesRequest) ([]ResponsesTool, error)
func (s *OpenAIGatewayService) handleOpenAIResponsesWebSearch(ctx context.Context, c *gin.Context, account *Account, body []byte) (*OpenAIForwardResult, bool, error)
func resolveOpenAIResponsesTypedWebSearchToolConfig(tools []apicompat.ResponsesTool, rawToolChoice json.RawMessage) (*openAIResponsesInternalWebToolConfig, error)
func addOpenAIResponsesTypedWebSearchTool(req *apicompat.ChatCompletionsRequest, config openAIResponsesInternalWebToolConfig) error
func ResponsesToAnthropicRequest(req *ResponsesRequest) (*AnthropicRequest, error)
func AnthropicToResponsesResponse(resp *AnthropicResponse) *ResponsesResponse
func AnthropicEventToResponsesEvents(evt *AnthropicStreamEvent, state *AnthropicEventToResponsesState) []ResponsesStreamEvent
func ChatCompletionsResponseToResponsesEvents(resp *ChatCompletionsResponse, model string, customTools map[string]bool, toolSearch bool, namespaceTools map[string]NamespacedToolName) []ResponsesStreamEvent
func (s *OpenAIGatewayService) forwardResponsesViaWebRunChatCompletions(ctx context.Context, c *gin.Context, account *Account, chatReq *apicompat.ChatCompletionsRequest, options openAIResponsesWebRunLoopOptions) (*OpenAIForwardResult, error)
func writeOpenAIResponsesFallbackErrorWithParam(c *gin.Context, statusCode int, errType, message, param string)
```

前端配置 helper：

```typescript
type CodexWebSearchBridgeMode = 'inherit' | 'enabled' | 'disabled'
function normalizeCodexWebSearchBridgeMode(value: unknown): CodexWebSearchBridgeMode
function applyAccountCodexWebSearchBridgeExtra(source: Record<string, unknown> | undefined, mode: CodexWebSearchBridgeMode): Record<string, unknown>
function applyChannelCodexWebSearchBridgeConfig(featuresConfig: Record<string, unknown>, sections: ChannelCodexWebSearchBridgeSection[]): void
function readChannelCodexWebSearchBridgeConfig(featuresConfig: Record<string, unknown> | null | undefined, platform: GroupPlatform): boolean
```

账号与渠道配置键：

```text
account.extra.openai_json_schema_to_json_object: boolean
account.extra.web_search_emulation: default | enabled | disabled
account.extra.codex_web_search_bridge: boolean
channel.features_config.web_search_emulation.openai: boolean
channel.features_config.web_search_emulation.anthropic: boolean
channel.features_config.codex_web_search_bridge.openai: boolean
```

### 3. Contracts

- `openai_json_schema_to_json_object` 只对 OpenAI APIKey 账号生效；只有布尔值 `true` 开启，缺失、`false`、字符串 `"true"` 或其它账号类型都保持原请求。
- Responses 只转换合法的 `text.format.type=json_schema` 且 `schema` 为 JSON object 的请求；输出 `text.format={"type":"json_object"}`，并把原 Schema 作为稳定的 best-effort instructions 约束。
- Chat 只转换合法的 `response_format.type=json_schema` 且 `json_schema.schema` 为 JSON object 的请求；输出 `response_format={"type":"json_object"}`，并在连续 system/developer 前缀后插入独立 system 约束。
- 降级 helper 必须基于 `json.RawMessage` 保留未知请求字段、messages 顺序以及 function/tool 参数 Schema；重复调用不能重复注入约束。非法 Schema 或不兼容 instructions 保持原请求，沿既有入口或上游错误处理。
- `web_search_emulation` 的 `default` 跟随对应渠道平台开关；`enabled` 强制允许本地模拟，`disabled` 强制禁止。全局搜索配置和可用 provider 仍是实际执行的必要条件。
- `codex_web_search_bridge` 只对 OpenAI APIKey 账号生效。账号严格布尔值优先于渠道 `codex_web_search_bridge.openai`；账号缺失时跟随渠道，渠道缺失、非法类型、分组或渠道读取失败时默认关闭。该开关只增加能力资格，不得绕过 `web_search_emulation`、全局搜索配置或 provider 可用性。
- Codex Lite 隐式桥只允许官方 Codex header，或网关显式开启 `ForceCodexCLI` 的客户端；同时要求 Lite Header 为真、普通 HTTP `/v1/responses`、OpenAI APIKey 最终走 Chat fallback。原生 Responses、WebSocket、`/responses/compact`、非 OpenAI APIKey 和非 Codex 请求不得进入。
- Lite 原始 Responses body 和模型清单不得被修改，也不能注入 hosted `web_search`。只有 Responses -> Chat 转换完成后，且请求没有显式 typed `web_search` 或 `namespace=web,name=run` 时，才可把现有 `sub2api_web_search` function 加入转换后的 Chat tools。
- 隐式桥对缺失或 `auto` 的 `tool_choice` 可注入；`required` 只有转换后至少存在一个客户端可执行工具时可注入，避免把隐式搜索变成唯一强制工具；`none`、强制 Web Search 和明确其它工具都不注入。固定代理名冲突继续返回 `400 invalid_request_error`、`param=tools`。
- `Manager.HasAvailableProvider` 是注入前的 fail-closed readiness：nil/空 manager、缺失必要 API Key、已过期 provider、已标记不可用的 provider 代理或配额耗尽都返回 false。Redis 读取异常与真实搜索执行保持同一容错语义，暂按可用处理，由执行阶段再次保留配额；不得仅以 manager 指针非空判断 provider 可用。
- 模型看到隐式工具但未选择时，只发送一次 Chat 请求，不调用 provider，`WebSearchCalls=0` 且不产生搜索费用；选择后必须复用现有内部循环、同一账号和模型续跑、call ID、来源、annotation、usage 聚合和成功查询计费，不建立第二套执行链。
- 前端账号模式只接受严格布尔值并映射为 `inherit|enabled|disabled`；保存 `inherit` 时删除账号字段，保存布尔覆盖时复制并保留其它 `extra`。渠道序列化只写启用的 OpenAI 平台并保留其它 `features_config`；全局 Web Search 不可用时可隐藏控件，但不能因渠道默认搜索关闭而禁用桥接开关，因为账号可强制开启 `web_search_emulation`。
- Responses 的有效工具必须同时读取顶层 `tools` 与 `input` 中的 `additional_tools`。本地模拟只接管唯一 Web Search 工具，或 `tool_choice` 明确选择 Web Search 的请求。
- Web Search 能力决策使用 `native_pass`、`chat_passthrough`、`direct_emulation`、`chat_tool_loop` 和 `reject`。该能力按协议条件作用于所有 OpenAI APIKey Chat fallback 账号，不得按 DeepSeek host、账号名或模型建立白名单。
- Chat fallback 的混合 typed Web Search 只在账号/渠道策略允许、恰好声明一个 typed `web_search`、`tool_choice` 缺失或为 `auto|required`，且 Responses -> Chat 转换后至少保留一个客户端可执行工具时启用。`image_generation` 等其它服务端工具不能被计入客户端可执行工具，否则会把 `required` 或缺失选择错误降成只剩内部搜索代理。
- typed Web Search 使用固定内部 function 名 `sub2api_web_search`，只声明 `search_query[].q`，并设置 `parallel_tool_calls=false`。注入前必须扫描转换后的 Chat tools；与 function、custom、namespace 摊平名或 `tool_search` 代理冲突时返回 `400 invalid_request_error`，`param=tools`，不得动态改名。
- 混合 typed Web Search 的其它 function、custom、namespace 和 `tool_search` Schema 必须原样保留。模型选择客户端工具时直接按既有 Responses 类型回传；模型选择内部搜索代理时由网关消费调用、按同一 call ID 回灌 tool result，再使用同一账号和模型续跑。
- `tool_choice=none` 或可保留的明确其它客户端工具选择不注入 typed Web Search 代理；明确强制 Web Search 继续走直接模拟。明确选择转换后不存在的服务端工具时必须返回 `400 invalid_request_error`、`param=tool_choice`，不能静默删除选择项。
- typed Web Search 复用原始工具的 `search_context_size`、`filters.allowed_domains`、`filters.blocked_domains` 和 `max_uses` 约束。为优先保持请求可执行，本地模拟接受并忽略 `external_web_access` 的缺失、`true` 和 `false`，始终调用实时搜索 provider；`false` 只是兼容降级，不代表实现了缓存/索引模式。`user_location`、`return_token_budget` 等其它无本地等价物的字段仍返回 `400 invalid_request_error`、`param=tools`。
- Chat fallback 必须用 namespace 映射精确识别 `namespace=web,name=run`；只有最终账号策略允许模拟时才进入服务端循环。全局 provider 开关不能把关闭或未配置的账号隐式改为开启。
- 服务端接管后只向 Chat 上游声明 `search_query` 与可选 `response_length`，并设置 `parallel_tool_calls=false`。不能声明或执行 `weather/open/click/find/screenshot/image_query/finance/sports`；天气问题由模型生成普通搜索词。
- 单次 `search_query` 必须是 1 至 4 项的数组，每项包含 trim 后非空的 `q`；单个入站请求累计最多执行 5 个查询。可选 `recency` 只生成 `recency_not_enforced` 警告。`response_length=short|medium|long` 分别限制每个查询回灌 3/5/10 条结果，缺失时使用 medium。
- `web.run` 和 typed Web Search 共用内部循环：内部模型请求固定使用同一账号、映射后模型和非流式 Chat；单个入站请求最多消费 5 轮 Web 工具调用、累计最多 5 个查询。typed Web Search 的 `max_uses` 还会收窄自身轮次；assistant tool call 与 `role=tool` 消息必须使用同一 call ID，缺失 ID 时生成稳定 fallback。
- 同一轮只允许一个内部 Web 工具调用；内部搜索与客户端工具并行返回时必须返回 `502 api_error`，不能部分执行。达到全局 5 轮或 typed Web Search 的 `max_uses` 后，网关必须回灌 `search_limit_reached` tool result，移除已耗尽的内部搜索工具并继续生成最终回答；不得向客户端返回内部轮次限制的 `502` 或 `response.failed`。仍有客户端工具时把遗留的强制 `tool_choice` 降级为 `auto`，没有剩余工具时清除 `tool_choice`。
- 已消费的内部 `web.run` function call 不能下发给客户端。每个已尝试的 `search_query` 必须投影为标准 Responses `web_search_call`：使用稳定 `ws_` ID、`action.type="search"`、原查询文本以及 `completed/failed` 状态，并排列在最终文本或其它客户端工具之前。
- 流式客户端必须收到 `response.output_item.added/done` 的 `web_search_call` 生命周期，后续最终响应事件的 `output_index` 必须按搜索项数量整体偏移，`response.completed.output` 也必须包含同一批搜索项。内部模型轮次仍保持缓冲，只有循环结束后才提交下游 SSE，避免搜索代理 failover 或后续模型错误发生在响应已提交之后。
- 所有内部模型轮次的 token usage 必须累加；`WebSearchCalls` 只累计真实成功完成的 provider 查询。参数错误、未支持命令、provider 失败和未实际执行的工具调用不计 Web Search 次数。
- `tool_choice=none` 或明确选择其它工具时不搜索；混合工具的空 choice、`auto`、`required` 不得被直接摘要模拟器抢先接管，只能由模型驱动的 `chat_tool_loop` 决定是否搜索。
- typed Web Search 真实成功后，普通文本响应在模型原文末尾追加 `Sources:`，来源仅取本次成功 provider 结果，按去掉 fragment 后的规范化 HTTP(S) URL 去重，最多 5 条。`url_citation` 的 rune 索引只覆盖网关追加的 URL；流式 delta、annotation、done、item 和 completed 快照必须一致。
- 最终没有文本、只返回客户端工具、没有成功来源，或请求使用 `text.format.type=json_schema|json_object` 时，不追加来源文本和 annotation；已执行搜索仍保留标准 `web_search_call`。
- 能力决策和循环日志只记录账号 ID、模型、工具选择类型、工具数、模式、轮次、成功查询数和安全 provider 标识；不得记录查询全文、搜索结果、请求体或凭据。
- 请求只能走 Chat fallback 且要求执行无法等价转换的 Web Search 时，返回 OpenAI 兼容能力错误；不得先删除服务端工具再让模型生成普通文本。
- 本地模拟与 `text.format.type=json_schema` 同时命中时返回 `400`。模拟器只生成搜索摘要，不能承诺 Structured Outputs。
- 原生 Responses -> Anthropic 映射使用 `web_search_20250305` 和 `name=web_search`；支持 `filters.allowed_domains`、`filters.blocked_domains`、`max_uses` 和 approximate `user_location`。
- `search_context_size`、`external_web_access`、`return_token_budget` 在 Anthropic 原生工具中没有等价字段，必须明确返回转换错误，不能静默忽略。
- Anthropic `server_tool_use(name=web_search)` 映射为 Responses `web_search_call`；`web_search_tool_result` 决定 completed/failed；`citations_delta` 或 text block citations 映射为 `url_citation`。
- DeepSeek SSE 的 `content_block.index` 是上游标识，不保证从 0 连续递增。缓冲聚合时必须维护 `upstream index -> finalResp.Content position` 映射，不能直接把上游 index 当 slice 下标。
- DeepSeek 可能在搜索结果块中发送描述后续文本的 `citations_delta`。当当前块不是 text 时先缓存 citation，等 text block 和被引用文本到达后再生成合法的 `item_id`、`start_index` 和 `end_index`。

### 4. Validation & Error Matrix

| 条件 | 必须结果 |
|---|---|
| JSON 降级未开启或账号类型不匹配 | 原样转发，不注入约束 |
| 合法 Responses/Chat `json_schema` 且账号开启 | 转为 `json_object`，保留 Schema 的 best-effort 约束 |
| Schema 不是 JSON object、instructions 类型不兼容 | 保持原请求，交给既有入口或上游报错 |
| Codex Lite + HTTP Chat fallback + 桥接/搜索策略开启 + provider 可用 + 无显式搜索 + 空 choice/`auto` | 只在转换后的 Chat tools 注入 `sub2api_web_search`，由模型按需选择 |
| 上述请求为 `required` 且转换后存在客户端工具 | 注入搜索候选并保留 `required`，不能把其它客户端工具删除 |
| 上述请求为 `required` 且转换后没有客户端工具 | 不注入，继续既有 Chat fallback 行为 |
| 桥关闭、搜索策略关闭、全局配置关闭、manager nil/空、provider 过期或配额耗尽 | 失败关闭，不注入内部工具，不返回桥接错误 |
| 非 Lite、非 Codex、原生 Responses、WebSocket、compact 或显式 typed Web Search/`web.run` | 不进入隐式桥，继续对应既有路径 |
| 隐式代理名与客户端 Chat tool 冲突 | `400 invalid_request_error`，`param=tools`，不得动态改名 |
| 模型未调用隐式搜索工具 | 一次 Chat、provider 0 次、`WebSearchCalls=0`，普通文本或客户端工具正常回程 |
| 模型调用隐式搜索工具 | 复用共享内部循环；标准 `web_search_call`、来源、citation、usage 和成功查询计费 |
| 纯 Web Search 或明确强制 Web Search，本地模拟可用 | 实际执行搜索并输出 Responses `web_search_call` 与 citations |
| Chat fallback + 一个 typed Web Search + 至少一个客户端工具 + 空 choice/`auto|required` | 注入 `sub2api_web_search`，由模型选择是否搜索，保留其它客户端工具 |
| Chat fallback + typed Web Search + 仅其它服务端工具 + 空 choice/`required` | `400 invalid_request_error`，`param=tools`，不得注入只剩搜索代理的 Chat 请求 |
| Chat fallback + typed Web Search + 明确选择转换后不存在的服务端工具 | `400 invalid_request_error`，`param=tool_choice` |
| typed Web Search 内部代理名与转换后的 Chat tool 冲突 | `400 invalid_request_error`，`param=tools` |
| typed Web Search 含 `user_location` 或 `return_token_budget` | `400 invalid_request_error`，`param=tools`，不调用上游或 provider |
| typed Web Search 的 `external_web_access` 缺失、为 `true` 或为 `false` | 按本地实时 Web Search 正常执行；`false` 明确按兼容策略降级，不承诺缓存模式 |
| `tool_choice=none` | 不执行本地搜索，继续正常能力决策 |
| 强制 Web Search 但未声明 Web Search 工具 | `400 invalid_request_error`，参数指向 `tool_choice` |
| Chat fallback 无法执行请求要求的 Web Search | `400 invalid_request_error`，参数指向 `tools` |
| 本地模拟与 `text.format=json_schema` 冲突 | `400 invalid_request_error`，参数指向 `text.format` |
| typed Web Search 账号允许模拟但全局 provider 不可用 | `503 web_search_unavailable`，不伪造成功、不计费 |
| `web.run` 未被账号策略开启 | 不做服务端执行，按现有 namespace function call 回给客户端 |
| `web.run.search_query` 合法 | 逐项执行 provider，回灌同 call ID，用同一账号续跑模型，并在最终 Responses 输出中投影 `web_search_call` |
| `web.run` 账号允许模拟但全局 provider 不可用 | 回灌 `web_search_unavailable` tool result，允许模型生成可诊断回答，不计 Web Search 次数 |
| `web.run` 只有未支持命令或参数非法 | 回灌稳定 tool error，不调用 provider、不计 Web Search 次数 |
| `web.run` provider 普通失败 | 回灌 `web_search_failed` tool result，允许模型生成可诊断回答，失败查询不计费 |
| `web.run` 账号代理不可用 | 返回 `UpstreamFailoverError`，由既有账号切换链重试 |
| 单次 `web.run` 或 typed Web Search 包含超过 4 个查询 | 回灌 `search_limit_exceeded` tool result，不执行该批 provider 调用 |
| `web.run` 累计查询超过 5 个 | 回灌 `search_limit_exceeded` tool result，不执行该批 provider 调用 |
| `web.run` 超过 5 轮搜索 | 回灌 `search_limit_reached`，移除内部搜索工具并继续生成最终回答，不执行第六轮 provider 调用 |
| typed Web Search 达到 `max_uses` 或内部循环达到 5 轮 | 回灌 `search_limit_reached`，移除已耗尽的内部搜索工具并继续生成最终回答，不执行超限 provider 调用 |
| 同一轮同时返回 `web.run` 与客户端工具 | 返回 `502 api_error`，不能部分执行或错配 tool result |
| typed Web Search 成功且最终返回普通文本 | 搜索项位于消息前；追加去重、最多 5 条的真实来源和精确 `url_citation` |
| typed Web Search 成功但最终返回客户端工具或结构化文本 | 保留 `web_search_call`，不追加 `Sources:` 或 annotation |
| Anthropic 原生工具含无等价高级字段 | 返回转换错误，不发送被截断语义的上游请求 |
| `web_search_tool_result_error` | 对应 `web_search_call.status=failed`，保留错误码 |
| citation 先于 text 到达 | 先缓存；文本和引用范围可确定后再输出 annotation |
| SSE block index 为 `5/6/7` 等稀疏值 | 通过映射聚合到连续本地 content，不丢块、不越界 |

### 5. Good/Base/Bad Cases

- Good: OpenAI APIKey 账号开启兼容后，Responses 经原生 `/responses`、Responses shape 经 Chat fallback、直接 Chat 请求都只向上游发送 `json_object`，原 Schema 仍作为输出约束存在。
- Good: Codex `gpt-5.6-sol` 保持 Responses Lite，客户端 body 没有显式搜索工具；账号或渠道桥接、现有搜索策略和 provider 都可用时，首轮 Chat 上游看到一次 `sub2api_web_search`，模型可选择不搜索或进入共享循环。
- Good: manager 已构建但 provider 因缺失凭据、过期、代理不可用或配额耗尽而没有候选时，桥接在注入前关闭，普通 Chat 请求不携带必然失败的内部工具。
- Good: `input` 中通过 `additional_tools` 声明的唯一 Web Search 能被识别；明确强制搜索时本地 provider 执行一次，返回完整 `web_search_call`、摘要和 URL citations。
- Good: Codex 在 `additional_tools` 声明 `web/run`，模型用 `search_query:[{"q":"杭州天气"}]` 调用时，网关执行搜索、按原 call ID 回灌；客户端收到标准 `web_search_call` 和最终模型回答，但看不到内部 `namespace=web,name=run` function call。
- Good: Chat fallback 请求同时声明 typed `web_search` 与 `wait`，`tool_choice=auto`；上游可选择 `wait` 并原样回给客户端，也可选择 `sub2api_web_search` 后由网关搜索并续跑，入口不再固定返回能力 400。
- Good: DeepSeek 依次发送 index `5` 的搜索调用、index `6` 的搜索结果、挂在 index `5` 上的 citation、index `7` 的文本时，最终 Responses 中搜索调用、文本和引用都完整。
- Base: 模拟配置关闭且上游原生 Responses 支持 Web Search 时保持 pass，不改变现有上游能力。
- Base: `web.run` 已声明但账号策略为 disabled 时保持客户端工具语义；网关不收窄 Schema、不调用 provider、不产生 Web Search 费用。
- Base: `tool_choice=none` 即使声明 Web Search 也不执行搜索。
- Base: 存量账号和渠道都没有 `codex_web_search_bridge` 时默认关闭；关闭 Lite、改走原生 Responses 或显式声明 typed Web Search/`web.run` 时继续现有路径。
- Base: `web_search + image_generation` 不包含客户端可执行工具；空 choice 或 `required` 明确返回 `param=tools`，不会把 `image_generation` 静默丢弃后改变选择语义。
- Bad: 在 Responses -> Chat 转换时直接丢弃 `web_search` 和对应 `tool_choice`，让客户端收到看似成功但实际未搜索的文本。
- Bad: 因 manager 指针非空就向 Lite Chat fallback 注入搜索工具；manager 内部可能没有任何可执行 provider，模型一旦选择就只会得到确定性失败。
- Bad: 把 hosted `web_search` 写回 Lite 原始 body，或根据提示词在模型选择前预执行搜索；前者违反 Lite 工具边界，后者让每次请求都产生搜索行为和费用。
- Bad: 只按原始 Responses 工具数量判断混合请求可执行；服务端工具在转换后被删除，会让 `required` 实际只剩内部搜索代理并被错误强制执行。
- Bad: 服务端实际执行了 `web.run` 搜索，却只返回最终 assistant message；Codex 日志中没有 `web_search_call`，用户无法判断回答是否真的联网。
- Bad: 把 `weather` 当成 `search_query` 的别名直接执行，或把内部 `web.run` function call 提前发给 Codex；前者伪造未实现能力，后者会让客户端与服务端重复执行。
- Bad: 把原 Schema 序列化成普通 string 作为 `response_format`，会产生无效协议且丢失对象语义。
- Bad: 使用 `finalResp.Content[event.Index]` 聚合 DeepSeek SSE；稀疏 index 会越界或把 delta 写到错误 block。
- Bad: citation 到达时没有 text item 就直接发 annotation；会产生空 `item_id` 或错误的字符索引。

### 6. Tests Required

- JSON Schema helper 和网关路径必须覆盖：配置 guard、Responses、Chat、Responses -> Chat、Responses shape on Chat、passthrough、幂等、非法 Schema、未知字段和工具 Schema 保留。
- Web Search 决策必须覆盖：顶层 tools、`additional_tools`、唯一搜索工具、混合工具、`auto|required|none`、强制搜索、强制其它工具、Chat fallback 拒绝、转换后零客户端工具、JSON Schema 冲突和 provider 不可用。
- Codex Lite 隐式桥必须覆盖：账号覆盖/渠道继承/默认关闭/非法类型、官方 Codex 与 `ForceCodexCLI`、Lite true/false、HTTP/WebSocket/compact、原生 Responses、显式 typed Web Search/`web.run`、choice 矩阵、固定名冲突、manager nil/空、缺失凭据、过期 provider、可用 provider和配额 readiness。
- 隐式桥 fallback 必须覆盖：模型不搜索时一次 Chat 且不计费；模型搜索时同 call ID 回灌、同账号续跑、流式/非流式 `web_search_call`、来源、Unicode citation、usage 聚合、provider 失败/代理 failover、查询/轮次上限、结构化输出和客户端断连。
- 前端必须覆盖：账号三态严格布尔归一化、`inherit` 删除字段、未知 `extra` 保留、渠道 OpenAI-only 序列化、未知 `features_config` 保留、管理页面回填/保存、全局配置不可用时的展示条件和最终中英文 key。
- typed Web Search 循环必须覆盖：模型不搜索、选择 function/custom/namespace/tool_search、代理名冲突、重复 typed 工具、无等价字段、`max_uses`、并行客户端调用、provider 失败、代理 failover、usage/成功调用数累计、流式/非流式来源、Unicode rune 索引、结构化文本和客户端断连。
- `web.run` 循环必须覆盖：顶层和 `additional_tools` 识别、Schema 收窄、天气改走普通搜索、非法/未支持参数、recency 警告、provider 失败、代理 failover、缺失 call ID、单次 4 查询边界、跨轮次累计 5 查询上限、5 轮软终止、无 `response.failed`、usage/成功调用数累计、流式缓冲和其它客户端工具回程。
- `web.run` 客户端可见事件必须断言：非流式 `output` 的搜索项位于最终消息之前；流式 `web_search_call added/done` 位于最终文本事件之前；所有后续 `output_index` 正确偏移；provider 普通失败投影为 `status=failed`；任何响应都不泄漏 `namespace=web` 的内部调用。
- 原生 Anthropic 桥必须覆盖：请求字段映射、无等价字段拒绝、非流式 search completed/failed、查询提取、URL citation、流式完整生命周期。
- SSE 聚合回归必须使用真实稀疏 index，并覆盖 citation 在搜索结果停止后、最终文本开始前到达的顺序；断言查询、搜索前文本、最终文本和 URL citation 都保留。
- 建议运行：

```bash
cd backend
go test -tags=unit ./internal/pkg/apicompat ./internal/service -count=1
go test -tags=unit ./internal/service -run 'WebRun|WebSearch' -count=1
```

### 7. Wrong vs Correct

#### Wrong

```go
contentIndex := *event.Index
finalResp.Content[contentIndex].Text += event.Delta.Text
```

问题：Anthropic/DeepSeek 的 block index 是流中的关联键，不是本地连续数组位置；隐藏块或服务端工具块会让 index 稀疏。

#### Correct

```go
contentPositionByUpstreamIndex[*event.Index] = len(finalResp.Content) - 1

contentPosition, ok := contentPositionByUpstreamIndex[*event.Index]
if ok {
	finalResp.Content[contentPosition].Text += event.Delta.Text
}
```

在 `content_block_start` 建立映射，后续 delta 通过映射定位本地内容；citation 属于非 text block 时先缓存，等 text block 出现后再附加。

#### Wrong

```go
return streamChatCompletionsAsResponses(c, firstUpstreamResponse)
```

问题：首个 Chat 响应可能只是 `web.run` function call。直接转发会泄漏服务端应消费的工具调用，且没有对应 tool result，模型无法继续生成最终回答。

#### Correct

```go
for webRunCall != nil {
	toolOutput := executeSearch(webRunCall)
	appendAssistantAndToolResult(chatReq, webRunCall, toolOutput)
	finalResponse = callSameAccountNonStream(chatReq)
}
return writeFinalResponsesResult(finalResponse)
```

内部轮次缓冲并使用同一账号续跑，只在得到最终文本或客户端工具调用后生成下游 Responses JSON/SSE；循环同时受查询数和工具轮次双重上限约束。

#### Wrong

```go
if toolCount > webSearchCount {
	_ = addOpenAIResponsesTypedWebSearchTool(chatReq, *typedWebSearchConfig)
}
```

问题：原始混合工具可能只有 `web_search + image_generation`。两者都是服务端工具，转换到 Chat 后没有客户端工具；此时注入代理会把缺失或 `required` 的选择语义收窄成“只能搜索”。

#### Correct

```go
chatReq, err := apicompat.ResponsesToChatCompletionsRequest(req)
if err != nil {
	return err
}
if len(chatReq.Tools) == 0 {
	err := errors.New("chat fallback mixed web_search requires at least one client-executable tool")
	writeOpenAIResponsesFallbackErrorWithParam(c, http.StatusBadRequest, "invalid_request_error", err.Error(), "tools")
	return err
}
if err := addOpenAIResponsesTypedWebSearchTool(chatReq, *typedWebSearchConfig); err != nil {
	return err
}
```

先以转换后的 Chat 工具集确认至少存在一个客户端可执行工具，再注入内部搜索代理；无法保留的明确其它工具选择则返回 `param=tool_choice`。

#### Wrong

```go
if getWebSearchManager() != nil {
	addOpenAIResponsesTypedWebSearchTool(chatReq, defaultCodexWebSearchBridgeToolConfig())
}
```

问题：manager 可以在所有 provider 被基础配置过滤后仍非空，也可能只剩过期、代理不可用或配额耗尽的 provider；仅检查指针会向模型暴露一个确定失败的工具。

#### Correct

```go
manager := getWebSearchManager()
if manager != nil && manager.HasAvailableProvider(ctx, resolveAccountProxyURL(account)) {
	if err := addOpenAIResponsesTypedWebSearchTool(chatReq, defaultCodexWebSearchBridgeToolConfig()); err != nil {
		return err
	}
}
```

在资格决策中同时校验现有搜索策略、全局配置和动态 provider readiness；注入后仍复用共享内部循环，不能在桥接层直接执行搜索。

## Scenario: DeepSeek 工具调用历史缺失推理内容自动降级

### 1. Scope / Trigger

- 最终上游协议为 Chat Completions、最终模型 trim/lower 后以 `deepseek-` 开头，并且历史
  assistant 工具调用可能缺少 DeepSeek thinking mode 要求的推理内容时，必须应用本节。
- 覆盖原生 Chat、Responses -> Chat fallback、Responses `web.run` 每轮续跑和
  Anthropic Messages -> Chat fallback；不修改 Responses 或 Anthropic 转换器本身的字段语义。

### 2. Signatures

系统设置与 API 字段：

```text
SettingKeyEnableDeepSeekMissingReasoningAutoDowngrade
  = "enable_deepseek_missing_reasoning_auto_downgrade"
```

```json
{
  "enable_deepseek_missing_reasoning_auto_downgrade": true
}
```

服务与策略签名：

```go
func (s *SettingService) IsDeepSeekMissingReasoningAutoDowngradeEnabled(ctx context.Context) bool

func applyDeepSeekMissingReasoningPolicy(
	body []byte,
	upstreamModel string,
	enabled bool,
) (deepSeekMissingReasoningPolicyResult, error)

func (s *OpenAIGatewayService) applyDeepSeekMissingReasoningAutoDowngrade(
	ctx context.Context,
	account *Account,
	upstreamModel string,
	body []byte,
	sourcePath string,
) ([]byte, error)
```

稳定来源值：`chat_completions`、`responses_chat_fallback`、`responses_web_run`、
`anthropic_chat_fallback`。

### 3. Contracts

- 新安装持久化 `true`；存量环境缺 key、空值、repository/service 不可用或读取异常时按
  `true` 执行。管理员可以显式保存 `false` 关闭策略。
- 网关热路径使用每个 `SettingService` 实例独立的进程内缓存与 singleflight：成功 TTL
  60 秒、错误 TTL 5 秒、数据库读取超时 5 秒；不得每请求查询数据库。
- 保存配置后必须立即 `Store` 新缓存值。`singleflight.Forget` 不会取消已经开始的旧读取，
  因此加载器写缓存时必须以读取前的指针为 expected 执行 `CompareAndSwap`；CAS 失败时返回
  当前新缓存值，禁止旧数据库结果覆盖刚保存的设置。
- 只扫描最终 Chat JSON 的 `messages`。满足 `role="assistant"` 且 `tool_calls` 是非空数组的
  消息，必须至少有一个 trim 后非空的字符串 `reasoning_content` 或兼容字段 `reasoning`。
  缺失、`null`、空白字符串和非字符串都不可用。
- 任一上述消息缺少可用推理内容时，设置顶层 `thinking.type="disabled"` 并删除顶层
  `reasoning_effort`。若已经 disabled 且不存在 effort，保持 body 不变且不记录降级日志。
- 策略必须放在模型映射和现有 body 改写之后、reasoning effort 提取和实际发送之前。
  Responses `web.run` 必须在循环内对每一轮最新 body 重跑策略。
- 只在实际改写时记录 info：`component=openai.deepseek_missing_reasoning_policy`、账号 ID、
  最终模型、来源、缺失消息数和 `reason=assistant_tool_calls_missing_reasoning`；不得记录请求体、
  reasoning、工具参数、密钥或鉴权信息。
- 本策略不伪造 reasoning，不在上游 400 后重试，也不恢复 Anthropic -> Chat 转换器已丢弃的
  历史 thinking；它只对最终不可安全继续 thinking 的 DeepSeek Chat 请求做降级。

### 4. Validation & Error Matrix

| 条件 | 行为 | 结果 |
| --- | --- | --- |
| 非 `deepseek-*` 最终模型 | 不扫描、不改写 | 原样发送 |
| 开关为 `false` | 不扫描、不改写 | 保留上游原始行为 |
| 无 assistant 非空 `tool_calls` | 不改写 | 原样发送 |
| 每条工具调用历史都有可用 `reasoning_content` 或 `reasoning` | 不改写 | thinking 保持 |
| 任一工具调用历史缺推理内容 | disabled thinking，删除 effort | 发送降级后的 body |
| 已 disabled 且无 effort | 幂等返回 `changed=false` | 不记录误导日志 |
| 已 disabled 但仍有 effort | 只删除 effort | 记录实际改写 |
| Chat JSON 非法或结构化改写失败 | 返回本地错误 | 不发送半改写请求 |
| 设置 key 缺失 | 缓存默认 `true` | 自动降级可用 |
| 设置读取异常 | warning + 5 秒错误缓存，返回 `true` | 后续可自动恢复 |
| 保存与旧数据库读取并发 | CAS 阻止旧读取覆盖新缓存 | 保存值立即生效 |

### 5. Good/Base/Bad Cases

- Good：最终模型 ` DeepSeek-Reasoner `，assistant 有非空 `tool_calls` 但
  `reasoning_content=" "`，策略关闭 thinking 并删除 effort。
- Good：`reasoning_content` 缺失但 `reasoning` 是非空字符串，保持 thinking，不误降级。
- Good：Responses `web.run` 首轮历史完整，续轮新增缺推理内容的 assistant 工具调用；
  第二轮发送前触发降级。
- Good：管理员保存关闭值时，已在进行的旧设置读取随后完成，但 CAS 失败并返回新缓存值，
  下一请求仍使用关闭状态。
- Base：非 DeepSeek、没有工具调用或开关关闭时，body 不因本策略变化。
- Bad：只看客户端原始模型，模型映射到 DeepSeek 后漏检。
- Bad：只在 Responses 首次转换时检查，漏掉 `web.run` 续轮新产生的不完整历史。
- Bad：保存时只调用 `singleflight.Forget`；已经执行的旧 loader 仍可能在保存后覆盖新缓存。
- Bad：为通过上游校验伪造 `reasoning_content`，会把不存在的思考历史冒充为真实内容。

### 6. Tests Required

- 领域单测必须覆盖：模型 trim/lower guard、开关关闭、无工具调用、完整 reasoning、`reasoning`
  别名、空白/null/非字符串、幂等 disabled、只删除 effort、非法 JSON和安全日志字段。
- 设置测试必须覆盖：缺失默认 true、显式 false、缓存复用、读取异常短 TTL、保存后立即刷新，
  以及“旧读取阻塞 -> 保存新值 -> 旧读取完成”的确定性并发场景。
- 并发缓存测试必须运行 race detector：

```bash
cd backend
go test -race -tags=unit ./internal/service \
  -run 'TestSettingService_DeepSeekMissingReasoningPolicy_DefaultCacheAndRefresh' \
  -count=1
```

- 四个发送点必须用真实上游 body 断言策略接线；Responses `web.run` 必须覆盖后续轮次命中。
- Settings GET/PUT、局部更新、审计 diff、API contract、前端默认值/保存载荷和中英文最终文案
  必须同时覆盖。

### 7. Wrong vs Correct

#### Wrong

```go
s.deepSeekMissingReasoningPolicySF.Forget(refreshKey)
s.deepSeekMissingReasoningPolicyCache.Store(saved)

// 更早开始的 loader 随后无条件 Store(oldValue)
```

问题：`Forget` 只允许后续调用启动新 flight，不会取消旧 flight；旧读取可以在保存后回写陈旧值。

#### Correct

```go
expected := s.deepSeekMissingReasoningPolicyCache.Load()
loaded := readFromRepository()
if !s.deepSeekMissingReasoningPolicyCache.CompareAndSwap(expected, loaded) {
	return s.deepSeekMissingReasoningPolicyCache.Load().enabled
}
return loaded.enabled
```

保存路径直接替换缓存；旧 loader 仅在缓存仍是其读取前观察到的指针时才能提交结果，从而保证
新保存值不会被迟到读取覆盖。

## Scenario: 国产供应商自适应协议与分协议端点

### 1. Scope / Trigger

- 适用于 `kimi`、`zhipu`、`deepseek` API Key 账号使用
  `credentials.api_protocol="adaptive"` 的创建、编辑、连接测试和网关转发。
- 目标是按入站协议优先选择供应商原生端点，只在供应商没有对应端点时转换；账号计费模式
  `account_mode` 与 API 协议正交。

### 2. Signatures

```go
func (a *Account) GetAPIProtocol() string
func (a *Account) IsAdaptiveAPIProtocol() bool
func (a *Account) GetCNProtocolBaseURL(protocol string) string
```

```typescript
type CnApiProtocol = 'adaptive' | 'chat_completions' | 'anthropic' | 'responses'
type CnNativeApiProtocol = Exclude<CnApiProtocol, 'adaptive'>

function defaultCNAdaptiveBaseUrls(
  platform: 'kimi' | 'zhipu' | 'deepseek',
  mode: CnAccountMode
): Record<CnNativeApiProtocol, string>
```

### 3. Contracts

- 凭据字段：
  - `api_protocol: "adaptive"` 开启按入站协议分流。
  - `api_base_urls.chat_completions` 和 `api_base_urls.anthropic` 保存两个原生端点。
  - `api_base_urls.responses` 仅 DeepSeek 有非空默认值；Kimi、Zhipu 保存空值并回退 Chat。
  - `base_url` 必须镜像 Chat Completions 地址，兼容仍读取旧字段的调用点。
- 入站分流：
  - `/v1/chat/completions` 的标准 `messages` 请求 -> 原生 Chat Completions。
  - `/v1/messages` -> 原生 Anthropic `/v1/messages`。
  - `/v1/responses` 或误投到 Chat 路径的 Responses 形状请求：DeepSeek -> 原生 Responses；
    Kimi、Zhipu -> 转换为 Chat Completions。
- 显式固定协议优先于 `extra` 中可能陈旧的 Responses 探测/覆盖值；固定
  `chat_completions` 不得被 `force_responses` 改写，固定 `responses` 也不得被
  `force_chat_completions` 改写。
- 分协议 URL 缺失时按平台和 `account_mode` 使用后端/前端一致的官方默认端点。

### 4. Validation & Error Matrix

| 条件 | 行为 | 结果 |
| --- | --- | --- |
| `account == nil` 或非 CN 平台 | `GetAPIProtocol` 回退 Chat | 保持旧账号行为 |
| `api_protocol` 缺失或非法 | 回退 `chat_completions` | 不进入未知端点 |
| Kimi/Zhipu 显式配置 `responses` | 回退 `chat_completions` | 不请求不存在的 Responses 端点 |
| adaptive Chat URL 缺失 | 先读旧 `base_url`，再读官方默认值 | 历史账号可继续转发 |
| adaptive Anthropic/Responses URL 缺失 | 使用对应官方默认值 | 端点选择稳定 |
| Kimi/Zhipu 收到 Responses 入站 | 转换为 Chat 请求 | 上游 body 有 `messages`、无 `input` |
| DeepSeek 收到 Responses 入站 | 原生 Responses 转发 | 保留 `max_output_tokens`，清理不兼容会话字段 |
| 连接测试某原生端点返回非成功或非法流 | 立即停止并返回带协议名的错误 | 不发出后续端点请求，不发送 `test_complete` |

### 5. Good/Base/Bad Cases

- Good：DeepSeek adaptive 账号分别把 Chat、Messages、Responses 发到三个配置端点。
- Good：Kimi adaptive 账号收到 Responses 请求后转为 Chat，仍把 Messages 原生发到
  Anthropic 端点。
- Base：未配置 `api_protocol` 的存量 CN 账号继续走 `chat_completions` 和旧 `base_url`。
- Bad：仅根据 `openai_responses_supported` 或 `openai_responses_mode` 决定 CN 路由，覆盖管理员
  明确选择的固定协议。
- Bad：把 Responses 形状 body 原样发送给 Kimi/Zhipu Chat 端点，导致 `input`/`messages`
  结构不匹配。
- Bad：保存 adaptive 时只写 `api_base_urls` 而不同步 `base_url`，使旧读取点使用陈旧地址。

### 6. Tests Required

- 后端路由测试必须断言最终 URL 与最终上游 body 形状：

```bash
cd backend
go test -tags=unit ./internal/service \
  -run 'TestAdaptiveProtocol|TestFixedCN(Chat|Responses)Protocol'
```

- 账号连接测试必须覆盖 Kimi/Zhipu 的 Chat + Anthropic、DeepSeek 的三个端点，以及中途失败
  不继续请求：

```bash
cd backend
go test -tags=unit ./internal/service \
  -run 'TestAccountTestService_Adaptive'
```

- 前端默认端点测试必须逐平台、逐账号模式断言完整对象；创建/编辑测试必须断言
  `api_protocol`、`api_base_urls`、兼容 `base_url` 和从 adaptive 切回固定协议时删除旧 map：

```bash
cd frontend
pnpm exec vitest run \
  src/components/account/__tests__/credentialsBuilder.cnAdaptive.spec.ts \
  src/components/account/__tests__/CreateAccountModal.spec.ts \
  src/components/account/__tests__/EditAccountModal.spec.ts
```

### 7. Wrong vs Correct

#### Wrong

```go
if !openai_compat.ShouldUseResponsesAPI(account.Extra) {
	return s.forwardAsRawChatCompletions(ctx, c, account, body, defaultMappedModel)
}
```

问题：只读取异步探针/覆盖值会忽略 CN 固定协议和 adaptive 的平台能力，Responses 形状请求也
可能被原样送入只接受 `messages` 的端点。

#### Correct

```go
if account.IsAdaptiveAPIProtocol() {
	// 先按入站形状和平台能力选择原生端点或执行必要转换。
}
if shouldForwardOpenAIResponsesViaRawChatCompletions(account) {
	if isResponsesShape {
		body, err = convertResponsesShapeToRawChatBody(body)
	}
	return s.forwardAsRawChatCompletions(ctx, c, account, body, defaultMappedModel)
}
```

先让 adaptive 路由处理供应商能力，再让统一 predicate 处理固定 Chat 与非 CN API Key；命中
raw Chat 时必须先把 Responses 形状转换为标准 Chat body。
