# Protocol Adapter Guidelines

> 本项目后端协议适配、上游桥接和缓存稳定性契约。

---

## Scenario: Anthropic Messages ↔ OpenAI Chat Completions 直连桥接

### 1. Scope / Trigger

- Trigger: 修改 Anthropic `/v1/messages` 与 OpenAI-compatible `/v1/chat/completions` 的请求或响应互转时，必须按本节检查。
- 适用路径：`backend/internal/pkg/apicompat/anthropic_chatcompletions.go`。
- 账号粘性路径：`backend/internal/handler/openai_gateway_handler.go`。
- 入口场景：OpenAI APIKey 且不走 Responses API 的 raw Chat fallback。该路径不会经过 Responses 的 `prompt_cache_key` / digest replay guard，因此 payload 前缀本身必须稳定。
- 缓存目标：稳定 Chat prefix cache，避免动态 attribution system block、string/array content 形态切换、随机 `tool_use.id` 造成缓存重建。

### 2. Signatures

- 请求转换：`AnthropicToChatCompletions(req *AnthropicRequest) (*ChatCompletionsRequest, error)`
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
- usage 映射保持 Anthropic 语义：`cache_read_input_tokens = prompt_tokens_details.cached_tokens`，`input_tokens = max(prompt_tokens - cached_tokens, 0)`。
- `/v1/messages` 账号粘性 key 优先级必须是：显式 `session_id` / `conversation_id` / `prompt_cache_key` > Anthropic `metadata.user_id` > content fallback。`metadata.user_id` 只用于账号 sticky，不直接作为上游 `prompt_cache_key`，避免固定上游缓存键压住后续 turn 的缓存滚动。

### 4. Validation & Error Matrix

- `AnthropicToChatCompletions(nil)` -> 返回 `nil request` 错误。
- message `content` 既不是字符串也不是可解析的 block array -> 返回 JSON 解析错误。
- text 为空、全空白或为 attribution block -> 不输出该 content part；system 过滤后为空则不输出 system message。
- `tool_result.content` 为空或无法提取 text -> Chat tool message content 使用 `"(empty)"`。
- 连续 tool messages 跟在包含多个 `tool_calls` 的 assistant 后面，且 `tool_call_id` 可匹配 -> 按 assistant `tool_calls` 顺序重排。
- 连续 tool messages 中存在未知 `tool_call_id` -> 已知 id 按 `tool_calls` 顺序排前，未知 id 保持原相对顺序排后。
- tool messages 中间出现普通 user/text message -> 不跨 message 重排，避免改变用户轮次语义。
- Chat tool delta 缺 id 但已有 name/args -> pending；finalize 时生成确定性 fallback id。
- Chat tool delta 缺 name 到 finalize -> 无法构造合法 Anthropic `tool_use`，跳过该 pending tool call。
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
  - cached token usage 映射保持不变。
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

## Scenario: Grok 统一 Billing 与配额探测数据流

### 1. Scope / Trigger

- Trigger: 修改 Grok OAuth Billing、xAI rate-limit header 探测、账号 usage 聚合、管理端配额 API、SSO 导入后探测或前端 Grok 配额展示时，必须按本节检查。
- 适用后端路径：
  - `backend/internal/pkg/xai/billing.go`
  - `backend/internal/service/grok_quota_service.go`
  - `backend/internal/service/grok_quota_fetcher.go`
  - `backend/internal/service/account_usage_service.go`
  - `backend/internal/handler/admin/grok_oauth_handler.go`
  - `backend/internal/server/routes/admin.go`
- 适用前端路径：
  - `frontend/src/api/admin/grok.ts`
  - `frontend/src/types/index.ts`
  - `frontend/src/components/account/AccountUsageCell.vue`
  - `frontend/src/components/account/GrokQuotaProbeCell.vue`
- 目标：Billing、主动 Responses 探测、被动 rate-limit header 快照和本地 usage window 必须通过同一个 `GrokQuotaProbeResult` 聚合；禁止恢复独立 `billing-quota` API、旧 `BillingSnapshot`/`grok_billing_quota` 类型或重复展示组件。

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
```

xAI Billing 和缓存字段：

```go
func BuildBillingURL(formatCredits bool) string
func ApplyCLIBillingHeaders(req *http.Request, accessToken string)
func ParseBillingPayload(body []byte) (*BillingPayload, error)
func BuildBillingSummary(config *BillingConfig) *BillingSummary
func MergeBillingProbeResult(previous, weekly, monthly *BillingSummary, weeklyOK, monthlyOK bool) *BillingSummary
```

```text
accounts.extra.grok_billing_snapshot -> xai.BillingSummary
accounts.extra.grok_usage_snapshot   -> xai.QuotaSnapshot
UsageInfo.GrokBilling                -> json:"grok_billing,omitempty"
```

前端只暴露统一调用和类型：

```typescript
queryQuota(id: number): Promise<GrokQuotaProbeResult>
AccountUsageInfo.grok_billing?: GrokBillingSummary | null
```

### 3. Contracts

- 只支持 `platform=grok` 且 `type=oauth` 的账号；token 必须通过 `GrokTokenProvider.GetAccessToken` 获取，账号代理通过 `account.Proxy` 或 `ProxyRepository` 解析。
- `QueryQuota` 先调用 `ProbeBilling`。Billing 已提供 `usage_percent`、`used_percent`、有效月额度或 plan 时直接返回；免费档或 Billing 数据不足时，再调用 `ProbeUsage` 获取 Responses rate-limit headers，并合并为 `source="hybrid_probe"`。
- 账号列表的 usage 刷新只调用 `ProbeBilling`，不能为了展示配额自动消耗一次模型请求；只有管理端手动统一配额查询或 SSO 导入后的主动探测才允许执行 Responses probe。
- `ProbeBilling` 使用不同的 singleflight key 与 `ProbeUsage` 隔离，并发请求周窗口 `/billing?format=credits` 和月窗口 `/billing`。任一窗口成功即可返回；失败窗口保留旧快照对应域，并通过 `partial`、`failed_windows`、`weekly_updated_at` 和 `monthly_updated_at` 表达状态。
- Billing 请求固定使用 `xai.DefaultCLIBaseURL`，并通过 `ApplyCLIBillingHeaders` 设置 Bearer token、`x-xai-token-auth`、`x-grok-client-version`、`Accept`、`Content-Type` 和 CLI User-Agent。Authorization 只用于上游请求，不得写入缓存或 API DTO。
- Billing 只写 `extra.grok_billing_snapshot`；主动或被动 rate-limit headers 只写 `extra.grok_usage_snapshot`。两类快照可同时投影到 `UsageInfo`，但不得互相覆盖。
- `BillingSummary` 使用 `period_type`、`usage_percent`、周/月窗口、产品用量、月额度、已用额度、plan、来源、更新时间和部分失败元数据；前后端字段保持 snake_case。
- 本地 usage 统计按 Billing 可信度分流：有权威 Billing 时查询当前周/月窗口；免费档或 Billing 不足时查询滚动 24 小时。前端优先显示官方周比例，免费档显示 2M Token 的 24 小时估算，付费但无周比例时回退到 header 请求/Token 配额。
- 前端 `AccountUsageCell` 通过 `GrokQuotaProbeCell` 接收统一探测结果并即时合并 `billing`、`snapshot` 和本地窗口；不得再发第二个 `queryBillingQuota` 请求或维护 `GrokBillingQuotaCell`。
- `reset-quota` 只保留稳定 API 契约；当前 xAI OAuth 不提供重置端点，service 返回明确的未支持错误，不能伪造成功。

### 4. Validation & Error Matrix

| 条件 | 结果 |
|---|---|
| service、repository、token provider 或 HTTP upstream 未配置 | `500 GROK_QUOTA_NOT_CONFIGURED` |
| 账号不存在 | `404 GROK_QUOTA_ACCOUNT_NOT_FOUND` |
| `platform != grok` | `400 GROK_QUOTA_INVALID_PLATFORM`，不得请求上游 |
| `type != oauth` | `400 GROK_QUOTA_INVALID_TYPE`，不得请求上游 |
| token 获取失败或为空 | `502 GROK_QUOTA_TOKEN_UNAVAILABLE` |
| 周/月 Billing 任一成功 | 返回 `billing_probe`；失败域写入 `partial/failed_windows` 并保留旧值 |
| 周/月都返回相同错误或都被限流 | 返回对应映射错误；共同 `429` 返回 `429 GROK_QUOTA_PROBE_UPSTREAM_ERROR` |
| 周/月失败原因不同 | `502 GROK_QUOTA_PROBE_PARTS_FAILED`，metadata 包含两个 status |
| Billing JSON 无法解析 | `502 GROK_QUOTA_BILLING_PARSE_ERROR` |
| 周/月都没有有效配额且无旧值 | `502 GROK_QUOTA_BILLING_EMPTY` |
| Billing 不足且 Responses probe 成功 | 返回 `hybrid_probe`，同时包含 Billing、本地窗口和 header snapshot |
| Responses probe 返回 `429` | 返回包含 rate-limit snapshot 的结果，不把它改成探测失败 |
| quota reset 请求 | `501 GROK_QUOTA_RESET_UNSUPPORTED` |

### 5. Good/Base/Bad Cases

- Good: SuperGrok 周 Billing 有官方比例时，手动查询只返回 Billing 和对应周/月本地统计，不额外发模型请求。
- Good: 免费账号 Billing 没有 `usage_percent` 时，手动查询继续执行一次 Responses probe，并把 Billing、24 小时本地 Token 与 header snapshot 合并返回。
- Good: 周窗口成功、月窗口失败时，保留旧月字段，更新周字段，并返回 `partial=true`、`failed_windows=["monthly"]`。
- Base: 存量账号只有 `grok_usage_snapshot` 时继续显示 request/token header 配额；只有 `grok_billing_snapshot` 时也能显示官方 Billing。
- Base: 账号列表刷新 Billing 失败时使用缓存或 unknown 状态，不自动消耗模型额度；强制刷新时错误可返回给管理端。
- Bad: 同时保留 `/billing-quota` 与 `/quota`，导致一次用户操作发两套上游请求并维护两种前端状态。
- Bad: 将 Billing 写入 `grok_usage_snapshot`，或把 rate-limit headers 写入 `grok_billing_snapshot`。
- Bad: 账号列表每次渲染都调用 `ProbeUsage`，以一次真实 Responses 请求换取展示数据。
- Bad: 周/月任一失败就丢弃另一窗口成功结果和已有缓存。

### 6. Tests Required

- `internal/pkg/xai/billing_test.go` 必须覆盖 URL、CLI headers、payload 解析、周/月归一化、plan 推导、部分成功合并和旧域保留。
- `internal/service/grok_quota_service_test.go` 必须覆盖 Billing 并发探测、singleflight、代理、token、部分成功、免费档 hybrid probe、429 snapshot 和 reset unsupported。
- `internal/service/account_usage_service_test.go` 与 `grok_quota_fetcher_test.go` 必须覆盖列表只探测 Billing、TTL/retry、24h/7d/月度本地窗口和两个 extra 快照的组合投影。
- handler/route 测试必须覆盖统一 `/quota`、`/reset-quota`、SSO 导入后探测、标准 envelope 和凭据不泄露；必须断言不存在旧 `/billing-quota` 路由。
- 前端 API/组件测试必须覆盖单一 `queryQuota`、官方周比例、免费 24h 估算、付费 header 回退、探测结果即时合并和旧 Billing 组件/类型不存在。
- 建议运行：

```bash
cd backend
go test -tags=unit ./internal/pkg/xai ./internal/service ./internal/handler/admin ./internal/server/routes -run 'Grok|Billing|Quota|SSO' -count=1
cd ../frontend
pnpm vitest run src/api/__tests__/admin.grok.spec.ts src/components/account/__tests__/AccountUsageCell.spec.ts src/components/account/__tests__/GrokQuotaProbeCell.spec.ts
pnpm typecheck
pnpm lint:check
git diff --check
```

### 7. Wrong vs Correct

#### Wrong

```typescript
const billing = await adminAPI.grok.queryBillingQuota(account.id)
const quota = await adminAPI.grok.queryQuota(account.id)
```

问题：独立请求会重复访问上游、形成新旧快照和组件双状态，并使免费档的 Billing/header 合并逻辑分散到前端。

#### Correct

```typescript
const result = await adminAPI.grok.queryQuota(account.id)
usageInfo.value = {
  ...usageInfo.value,
  grok_billing: result.billing ?? usageInfo.value?.grok_billing,
  grok_request_quota: result.snapshot?.requests ?? usageInfo.value?.grok_request_quota,
  grok_token_quota: result.snapshot?.tokens ?? usageInfo.value?.grok_token_quota
}
```

统一 API 在 service 层决定 Billing-only 或 hybrid probe，前端只合并一个结果。

#### Wrong

```go
result, err := s.ProbeUsage(ctx, accountID)
```

问题：账号列表自动刷新时直接探测 Responses，会让一次展示刷新消耗模型配额。

#### Correct

```go
result, err := s.ProbeBilling(ctx, accountID)
```

账号列表只刷新 Billing；手动 `QueryQuota` 再根据 Billing 是否权威决定是否执行主动 Responses probe。

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

- Trigger: 修改 OpenAI Codex 内置客户端版本、HTTP/Chat Completions bridge/compact/OAuth passthrough/WebSocket 请求头、模型目录 `client_version`、账号用量探测、账号测试或后台默认 Codex User-Agent 时，必须按本节检查。
- 适用后端路径：
  - `backend/internal/service/openai_codex_client_identity.go`
  - `backend/internal/service/openai_gateway_forward.go`
  - `backend/internal/service/openai_gateway_passthrough.go`
  - `backend/internal/service/openai_ws_forwarder_payload.go`
  - `backend/internal/service/openai_codex_models_service.go`
  - `backend/internal/service/account_usage_service.go`
  - `backend/internal/service/account_test_service.go`
  - `backend/internal/service/setting_gateway_runtime.go`
- 目标：项目内置 Codex 上游身份只能有一个固定已发布版本源；所有默认 UA、`Version` 和探测版本由它派生，避免新模型因旧客户端身份被上游误判为不存在，并进一步触发现有模型级冷却。

### 2. Signatures

唯一身份常量必须保持以下派生关系：

```go
const (
	openAICodexClientVersion = "0.144.1"
	codexCLIUserAgent        = "codex_cli_rs/" + openAICodexClientVersion + " (Ubuntu 22.4.0; x86_64) xterm-256color"
	codexCLIVersion          = openAICodexClientVersion
	openAICodexProbeVersion  = openAICodexClientVersion
	DefaultOpenAICodexUserAgent = "codex-tui/" + openAICodexClientVersion +
		" (Ubuntu 22.4.0; x86_64) xterm-256color (codex-tui; " + openAICodexClientVersion + ")"
)
```

相关请求构造入口：

```go
func (s *OpenAIGatewayService) buildUpstreamRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token string, isStream bool, promptCacheKey string, isCodexCLI bool) (*http.Request, error)
func (s *OpenAIGatewayService) buildUpstreamRequestOpenAIPassthrough(ctx context.Context, c *gin.Context, account *Account, body []byte, token string) (*http.Request, error)
func (s *OpenAIGatewayService) buildOpenAIWSHeaders(ctx context.Context, c *gin.Context, account *Account, token string, decision OpenAIWSProtocolDecision, isCodexCLI bool, turnState string, turnMetadata string, promptCacheKey string) (http.Header, openAIWSSessionHeaderResolution, error)
func (s *OpenAIGatewayService) FetchCodexModelsManifest(ctx context.Context, account *Account, clientVersion, ifNoneMatch string) (*CodexModelsManifest, error)
func (s *AccountUsageService) probeOpenAICodexSnapshot(ctx context.Context, account *Account) (map[string]any, error)
func (s *SettingService) GetOpenAICodexUserAgent(ctx context.Context) string
```

### 3. Contracts

- 生产代码中的内置版本字面量只允许由 `openAICodexClientVersion` 持有；升级版本时只修改该值，不在消费者文件中复制版本字符串。
- 当前固定版本是 `0.144.1`，满足 GPT-5.6 Sol、Terra、Luna 的最低客户端版本 `0.144.0`。本项目不根据模型目录动态协商、升级或回退版本。
- 内置身份消费者必须保持以下关系：

| 路径 | 内置身份契约 |
|---|---|
| HTTP Responses / Chat Completions bridge | 既有强制或兜底分支使用 `codexCLIUserAgent` |
| Responses compact | 入站未提供 `Version` 时使用 `codexCLIVersion` |
| OAuth passthrough | 既有强制分支或非 Codex UA 兜底使用 `codexCLIUserAgent`；compact 缺少 `Version` 时使用 `codexCLIVersion` |
| WebSocket | 既有强制分支或 OAuth 非 Codex UA 兜底使用 `codexCLIUserAgent` |
| 模型目录默认请求 | query `client_version` 和 Header `Version` 使用 `openAICodexProbeVersion`，UA 使用 `codexCLIUserAgent` |
| 账号用量探测 / 账号测试 | 默认 `Version` 使用探测或 CLI 语义别名，默认 UA 使用 `codexCLIUserAgent` |
| 浏览器 UA 替换 / Codex reset | 后台设置为空时使用 `DefaultOpenAICodexUserAgent` |

- `FetchCodexModelsManifest` 收到非空显式 `clientVersion` 时，query `client_version` 与 Header `Version` 必须原样使用 trim 后的显式值；只在空值时回退到内置版本。其 `User-Agent` 仍使用项目内置 CLI UA。
- 账号 `credentials.user_agent`、已识别的入站 Codex UA 和后台显式 `openai_codex_user_agent` 按各路径既有优先级生效：
  - 普通 HTTP 先应用账号 UA，再应用 `gateway.force_codex_cli`，最后只对 OAuth 浏览器 UA 执行后台 Codex UA 替换。
  - OAuth passthrough 和 WebSocket 继续把最终非 Codex UA 兜底为内置 CLI UA；账号自定义 Codex UA 在未开启强制模式时保持。
  - `gateway.force_codex_cli=true` 继续具有强制覆盖语义；默认值 `false` 不得因版本升级而改变。
- `min_codex_version` / `max_codex_version` 只属于入站 `codex_cli_only` 门控，不得作为上游请求的版本源。
- 不修改 `isUpstreamModelNotFoundError`、`HandleUpstreamModelNotFound`、`upstreamModelNotFoundCooldown` 或模型级冷却键。本场景只通过发送兼容身份避免合法模型误触发既有 404 策略。
- 本场景不新增配置、环境变量、DTO、数据库字段或 migration，也不自动改写管理员已保存的 UA。

### 4. Validation & Error Matrix

| 条件 | 必须结果 |
|---|---|
| 使用项目内置默认身份 | CLI UA、TUI UA、compact `Version` 和探测版本都包含或等于 `openAICodexClientVersion` |
| 模型目录 `clientVersion` 为空 | query `client_version` 与 Header `Version` 都使用 `openAICodexProbeVersion` |
| 模型目录 `clientVersion` 非空 | query 与 Header 使用显式值，不被内置版本覆盖 |
| 账号配置了 Codex UA，且未开启强制模式 | 按既有路径保留该 UA |
| OAuth passthrough / WebSocket 最终 UA 不是 Codex UA | 按既有逻辑回退到 `codexCLIUserAgent` |
| `gateway.force_codex_cli=true` | 强制使用 `codexCLIUserAgent`，不改变其它路由决策 |
| 后台 `openai_codex_user_agent` 为空或读取失败 | 回退到 `DefaultOpenAICodexUserAgent` |
| 后台 `openai_codex_user_agent` 非空 | 使用显式设置，不自动升级其中的版本文本 |
| 入站设置了 `min_codex_version` / `max_codex_version` | 只影响客户端准入，不影响任何上游 UA、`Version` 或 `client_version` |
| 上游返回匹配的 404 model-not-found | 继续按既有规则写入 30 分钟模型级冷却，本场景不得吞掉或改写错误 |

### 5. Good/Base/Bad Cases

- Good: 将内置版本从旧值升级时只修改 `openAICodexClientVersion`，HTTP、compact、WebSocket、模型目录、探测和账号测试随编译期派生值同步更新。
- Good: GPT-5.6 Sol、Terra、Luna 模型名原样转发；触发内置身份时 UA 包含 `0.144.1`。
- Base: 管理员保存了自定义 Codex UA，版本升级后该显式值仍由管理员控制，不做迁移或静默覆盖。
- Base: 模型目录调用方显式传入历史 `client_version`，请求继续按显式值发出，便于兼容和诊断。
- Bad: 在 `openai_gateway_service.go`、`account_usage_service.go` 或设置文件中分别维护版本字面量，导致只升级部分路径。
- Bad: 用 `min_codex_version` 作为上游 `Version`，把入站准入策略错误耦合到上游客户端身份。
- Bad: 为规避一次 404 而关闭 model-not-found 识别或缩短冷却，掩盖真正不存在模型的错误。
- Bad: 无条件覆盖所有账号或入站 UA，破坏显式配置和现有路由优先级。

### 6. Tests Required

- `openai_codex_client_identity_test.go` 必须锁定唯一版本、语义别名、CLI/TUI UA 和 WebSocket UA 优先级。
- `openai_codex_models_service_test.go` 必须分别覆盖默认版本和显式 `client_version`，同时断言 query、Header `Version` 与 UA。
- OAuth passthrough 测试必须以 `gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna` 为表项，断言模型名原样转发且内置 UA 使用当前版本。
- Chat Completions bridge 至少覆盖一个 GPT-5.6 模型，断言桥接复用统一内置身份。
- 现有 compact、账号测试、用量探测、WebSocket 和后台 UA 测试必须继续通过。
- 现有 404 model-not-found 测试必须继续通过，至少包括识别、模型级冷却和不封禁整个账号三类断言。
- 建议运行：

```bash
cd backend && go test -tags=unit ./internal/service -count=1
cd backend && go test -tags=unit ./... -count=1
cd backend && GOTOOLCHAIN=go1.26.5 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.9.0 run --timeout=30m --build-tags=unit --new ./...
cd backend && rg -n --glob '*.go' --glob '!**/*_test.go' '0\.125\.0' internal
git diff --check
```

### 7. Wrong vs Correct

#### Wrong

```go
const codexCLIUserAgent = "codex_cli_rs/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color"
const codexCLIVersion = "0.144.1"
const openAICodexProbeVersion = "0.144.1"
```

问题：三个消费者独立维护同一版本，下一次升级很容易只修改其中一部分，再次形成 UA、compact 和探测版本漂移。

#### Correct

```go
const (
	openAICodexClientVersion = "0.144.1"
	codexCLIUserAgent        = "codex_cli_rs/" + openAICodexClientVersion + " (Ubuntu 22.4.0; x86_64) xterm-256color"
	codexCLIVersion          = openAICodexClientVersion
	openAICodexProbeVersion  = openAICodexClientVersion
)
```

所有内置身份从一个生产版本字面量派生；显式 UA、显式模型目录版本和入站版本门控仍保持各自独立契约。

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

## Scenario: Codex 生图桥接 tool_choice 归一化

### 1. Scope / Trigger

- Trigger: 修改 Codex 图片工具注入、`tool_choice`、账号显式图片工具策略、HTTP Responses 或 WebSocket Responses 入站归一化时，必须按本节检查。
- 适用后端路径：
  - `backend/internal/service/openai_codex_transform.go`
  - `backend/internal/service/openai_gateway_forward.go`
  - `backend/internal/service/openai_ws_forwarder_ingress.go`
- 目标：桥接已经注入或已有 `image_generation` 工具时，解除客户端 `tool_choice: "none"` 对图片工具的阻断，同时保留所有其它显式选择和禁用策略。

### 2. Signatures

```go
func ensureOpenAIResponsesImageGenerationToolChoiceAuto(reqBody map[string]any) bool
```

HTTP 与 WebSocket 必须调用同一归一化函数：

```go
if codexImageGenerationBridgeEnabled && ensureOpenAIResponsesImageGenerationToolChoiceAuto(decoded) {
	markDecodedModified()
}

if codexBridgeEnabled && ensureOpenAIResponsesImageGenerationToolChoiceAuto(payloadMap) {
	bridgeModified = true
}
```

### 3. Contracts

- 函数只处理已经包含 `image_generation` 工具、且模型不是 Codex Spark 的请求。
- `tool_choice` 缺失时写入字符串 `"auto"` 并返回 `true`。
- `tool_choice` 是字符串，且忽略大小写和首尾空白后等于 `"none"` 时，改写为 `"auto"` 并返回 `true`。
- `"auto"`、`"required"`、其它字符串、明确工具选择对象和其它非字符串值必须保持，并返回 `false`。
- HTTP 只在 `codexImageGenerationBridgeEnabled` 为真时调用；WebSocket 只在 `codexBridgeEnabled` 为真时调用。
- group 禁止图片、全局/频道/账号未启用桥接、账号显式工具策略为 `strip`、compact 请求或 Spark 模型时，不得通过本归一化重新开放图片工具。
- HTTP 与 WebSocket 不得复制一份独立的 `none` 判断逻辑，避免协议分支漂移。

### 4. Validation & Error Matrix

| 条件 | `tool_choice` 结果 | modified |
|---|---|---|
| 有图片工具，字段缺失 | `"auto"` | `true` |
| 有图片工具，值为 `"none"` | `"auto"` | `true` |
| 有图片工具，值为 `"  NONE  "` | `"auto"` | `true` |
| 有图片工具，值为 `"auto"` | 保持 | `false` |
| 有图片工具，值为 `"required"` | 保持 | `false` |
| 有图片工具，值为明确工具对象 | 保持 | `false` |
| 无图片工具 | 保持原值或缺失 | `false` |
| Codex Spark | 保持原值或缺失 | `false` |
| bridge 关闭或账号策略为 `strip` | 调用点不得执行归一化 | 不适用 |

### 5. Good/Base/Bad Cases

- Good: Codex HTTP 请求只带 `tool_choice: "none"`，桥接注入图片工具后同一请求被改成 `"auto"`，模型可选择调用图片工具。
- Good: WebSocket `response.create` 携带 `" NONE "` 时，发往上游的 payload 使用 `"auto"`，行为与 HTTP 一致。
- Base: 客户端明确使用 `"required"` 或图片工具对象时，桥接尊重其选择，不覆盖。
- Base: 管理员关闭桥接后，文本请求不会被注入图片工具，也不会因为 `none` 被改写。
- Bad: 只判断字段是否存在并直接返回，会让 `tool_choice: "none"` 永久禁止已注入的图片工具。
- Bad: 无条件把所有 `tool_choice` 写成 `"auto"`，会破坏 `required` 和明确工具选择。
- Bad: 在 HTTP 和 WebSocket 各自实现不同的字符串判断，会再次形成协议分支行为漂移。

### 6. Tests Required

- 纯函数表驱动测试至少覆盖：缺失、`none`、带空白/大小写的 `none`、`auto`、`required`、明确工具对象、无图片工具和 Spark。
- HTTP service 回归测试必须断言桥接启用时 `none -> auto`，同时保留现有 bridge disabled、group disabled、`strip` 和明确工具选择测试。
- 真实 WebSocket ingress 测试必须发送带 `tool_choice: "none"` 的 `response.create`，并断言上游 payload 包含图片工具和 `tool_choice: "auto"`。
- 建议运行：

```bash
cd backend
go test -tags=unit ./internal/service \
  -run 'ImageGenerationToolChoice|ImageGenerationBridge|ProxyResponsesWebSocketFromClient_InjectsCodexImage' \
  -count=1
```

### 7. Wrong vs Correct

#### Wrong

```go
if _, ok := reqBody["tool_choice"]; ok {
	return false
}
reqBody["tool_choice"] = "auto"
```

问题：把字段“存在”误当成“允许工具选择”，无法处理客户端显式发送的阻断值 `none`。

#### Correct

```go
choice, ok := reqBody["tool_choice"]
if !ok {
	reqBody["tool_choice"] = "auto"
	return true
}
choiceValue, isString := choice.(string)
if !isString || !strings.EqualFold(strings.TrimSpace(choiceValue), "none") {
	return false
}
reqBody["tool_choice"] = "auto"
return true
```

只解除 `none` 的阻断语义，并保留所有其它显式选择。

---

## Scenario: OpenAI Structured Outputs 降级与 DeepSeek Web Search 桥接

### 1. Scope / Trigger

- Trigger: 修改 OpenAI APIKey 账号的 `json_schema` 兼容、Responses Web Search 本地接管、Responses -> Chat 的 `web.run` 工具循环、Responses -> Anthropic -> Responses 原生搜索桥或 DeepSeek Anthropic SSE 聚合时，必须按本节检查。
- 适用路径：
  - `backend/internal/pkg/apicompat/json_schema_downgrade.go`
  - `backend/internal/pkg/apicompat/responses_to_anthropic_request.go`
  - `backend/internal/pkg/apicompat/anthropic_to_responses_response.go`
  - `backend/internal/service/openai_json_schema_downgrade.go`
  - `backend/internal/service/openai_responses_websearch.go`
  - `backend/internal/service/openai_responses_web_run.go`
  - `backend/internal/service/openai_gateway_responses_chat_fallback.go`
  - `backend/internal/service/gateway_forward_as_responses.go`
- 目标：显式配置后可把不受上游支持的 Structured Outputs 降为 `json_object`，并让 Web Search 选择原生转发、本地模拟或明确拒绝；不得把 Schema 或搜索工具静默丢弃。

### 2. Signatures

```go
func DowngradeResponsesJSONSchemaToJSONObject(body []byte) ([]byte, bool, error)
func DowngradeChatJSONSchemaToJSONObject(body []byte) ([]byte, bool, error)
func (a *Account) IsOpenAIJSONSchemaToJSONObjectEnabled() bool
func (a *Account) GetWebSearchEmulationMode() string
func EffectiveResponsesTools(req *ResponsesRequest) ([]ResponsesTool, error)
func ResponsesToAnthropicRequest(req *ResponsesRequest) (*AnthropicRequest, error)
func AnthropicToResponsesResponse(resp *AnthropicResponse) *ResponsesResponse
func AnthropicEventToResponsesEvents(evt *AnthropicStreamEvent, state *AnthropicEventToResponsesState) []ResponsesStreamEvent
func ChatCompletionsResponseToResponsesEvents(resp *ChatCompletionsResponse, model string, customTools map[string]bool, toolSearch bool, namespaceTools map[string]NamespacedToolName) []ResponsesStreamEvent
func (s *OpenAIGatewayService) forwardResponsesViaWebRunChatCompletions(ctx context.Context, c *gin.Context, account *Account, chatReq *apicompat.ChatCompletionsRequest, options openAIResponsesWebRunLoopOptions) (*OpenAIForwardResult, error)
```

账号与渠道配置键：

```text
account.extra.openai_json_schema_to_json_object: boolean
account.extra.web_search_emulation: default | enabled | disabled
channel.features_config.web_search_emulation.openai: boolean
channel.features_config.web_search_emulation.anthropic: boolean
```

### 3. Contracts

- `openai_json_schema_to_json_object` 只对 OpenAI APIKey 账号生效；只有布尔值 `true` 开启，缺失、`false`、字符串 `"true"` 或其它账号类型都保持原请求。
- Responses 只转换合法的 `text.format.type=json_schema` 且 `schema` 为 JSON object 的请求；输出 `text.format={"type":"json_object"}`，并把原 Schema 作为稳定的 best-effort instructions 约束。
- Chat 只转换合法的 `response_format.type=json_schema` 且 `json_schema.schema` 为 JSON object 的请求；输出 `response_format={"type":"json_object"}`，并在连续 system/developer 前缀后插入独立 system 约束。
- 降级 helper 必须基于 `json.RawMessage` 保留未知请求字段、messages 顺序以及 function/tool 参数 Schema；重复调用不能重复注入约束。非法 Schema 或不兼容 instructions 保持原请求，沿既有入口或上游错误处理。
- `web_search_emulation` 的 `default` 跟随对应渠道平台开关；`enabled` 强制允许本地模拟，`disabled` 强制禁止。全局搜索配置和可用 provider 仍是实际执行的必要条件。
- Responses 的有效工具必须同时读取顶层 `tools` 与 `input` 中的 `additional_tools`。本地模拟只接管唯一 Web Search 工具，或 `tool_choice` 明确选择 Web Search 的请求。
- Chat fallback 必须用 namespace 映射精确识别 `namespace=web,name=run`；只有最终账号策略允许模拟时才进入服务端循环。全局 provider 开关不能把关闭或未配置的账号隐式改为开启。
- 服务端接管后只向 Chat 上游声明 `search_query` 与可选 `response_length`，并设置 `parallel_tool_calls=false`。不能声明或执行 `weather/open/click/find/screenshot/image_query/finance/sports`；天气问题由模型生成普通搜索词。
- `search_query` 必须是 1 至 4 项的数组，每项包含 trim 后非空的 `q`；可选 `recency` 只生成 `recency_not_enforced` 警告。`response_length=short|medium|long` 分别限制每个查询回灌 3/5/10 条结果，缺失时使用 medium。
- `web.run` 内部模型请求固定使用同一账号、映射后模型和非流式 Chat。每个请求最多消费 2 轮 Web 工具调用、累计最多 4 个查询；assistant tool call 与 `role=tool` 消息必须使用同一 call ID，缺失 ID 时生成稳定 fallback。
- 已消费的 `web.run` 调用不能下发给客户端。最终文本或其它客户端工具继续通过现有 Chat -> Responses 转换返回；流式客户端只接收最终结果合成的完整 Responses SSE 生命周期。
- 所有内部模型轮次的 token usage 必须累加；`WebSearchCalls` 只累计真实成功完成的 provider 查询。参数错误、未支持命令、provider 失败和未实际执行的工具调用不计 Web Search 次数。
- `tool_choice=none` 或明确选择其它工具时不搜索；混合工具的空 choice、`auto`、`required` 不得被本地模拟擅自接管。
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
| 纯 Web Search 或明确强制 Web Search，本地模拟可用 | 实际执行搜索并输出 Responses `web_search_call` 与 citations |
| `tool_choice=none` | 不执行本地搜索，继续正常能力决策 |
| 强制 Web Search 但未声明 Web Search 工具 | `400 invalid_request_error`，参数指向 `tool_choice` |
| Chat fallback 无法执行请求要求的 Web Search | `400 invalid_request_error`，参数指向 `tools` |
| 本地模拟与 `text.format=json_schema` 冲突 | `400 invalid_request_error`，参数指向 `text.format` |
| typed Web Search 账号允许模拟但全局 provider 不可用 | `503 web_search_unavailable`，不伪造成功、不计费 |
| `web.run` 未被账号策略开启 | 不做服务端执行，按现有 namespace function call 回给客户端 |
| `web.run.search_query` 合法 | 逐项执行 provider，回灌同 call ID，并用同一账号续跑模型 |
| `web.run` 账号允许模拟但全局 provider 不可用 | 回灌 `web_search_unavailable` tool result，允许模型生成可诊断回答，不计 Web Search 次数 |
| `web.run` 只有未支持命令或参数非法 | 回灌稳定 tool error，不调用 provider、不计 Web Search 次数 |
| `web.run` provider 普通失败 | 回灌 `web_search_failed` tool result，允许模型生成可诊断回答，失败查询不计费 |
| `web.run` 账号代理不可用 | 返回 `UpstreamFailoverError`，由既有账号切换链重试 |
| `web.run` 累计查询超过 4 个 | 回灌 `search_limit_exceeded` tool result，不执行该批 provider 调用 |
| `web.run` 超过 2 轮搜索 | 返回 `502 api_error`，停止续跑，不执行第三轮 provider 调用 |
| 同一轮同时返回 `web.run` 与客户端工具 | 返回 `502 api_error`，不能部分执行或错配 tool result |
| Anthropic 原生工具含无等价高级字段 | 返回转换错误，不发送被截断语义的上游请求 |
| `web_search_tool_result_error` | 对应 `web_search_call.status=failed`，保留错误码 |
| citation 先于 text 到达 | 先缓存；文本和引用范围可确定后再输出 annotation |
| SSE block index 为 `5/6/7` 等稀疏值 | 通过映射聚合到连续本地 content，不丢块、不越界 |

### 5. Good/Base/Bad Cases

- Good: OpenAI APIKey 账号开启兼容后，Responses 经原生 `/responses`、Responses shape 经 Chat fallback、直接 Chat 请求都只向上游发送 `json_object`，原 Schema 仍作为输出约束存在。
- Good: `input` 中通过 `additional_tools` 声明的唯一 Web Search 能被识别；明确强制搜索时本地 provider 执行一次，返回完整 `web_search_call`、摘要和 URL citations。
- Good: Codex 在 `additional_tools` 声明 `web/run`，模型用 `search_query:[{"q":"杭州天气"}]` 调用时，网关执行搜索、按原 call ID 回灌，并只把最终模型回答返回客户端。
- Good: DeepSeek 依次发送 index `5` 的搜索调用、index `6` 的搜索结果、挂在 index `5` 上的 citation、index `7` 的文本时，最终 Responses 中搜索调用、文本和引用都完整。
- Base: 模拟配置关闭且上游原生 Responses 支持 Web Search 时保持 pass，不改变现有上游能力。
- Base: `web.run` 已声明但账号策略为 disabled 时保持客户端工具语义；网关不收窄 Schema、不调用 provider、不产生 Web Search 费用。
- Base: `tool_choice=none` 即使声明 Web Search 也不执行搜索。
- Bad: 在 Responses -> Chat 转换时直接丢弃 `web_search` 和对应 `tool_choice`，让客户端收到看似成功但实际未搜索的文本。
- Bad: 把 `weather` 当成 `search_query` 的别名直接执行，或把内部 `web.run` function call 提前发给 Codex；前者伪造未实现能力，后者会让客户端与服务端重复执行。
- Bad: 把原 Schema 序列化成普通 string 作为 `response_format`，会产生无效协议且丢失对象语义。
- Bad: 使用 `finalResp.Content[event.Index]` 聚合 DeepSeek SSE；稀疏 index 会越界或把 delta 写到错误 block。
- Bad: citation 到达时没有 text item 就直接发 annotation；会产生空 `item_id` 或错误的字符索引。

### 6. Tests Required

- JSON Schema helper 和网关路径必须覆盖：配置 guard、Responses、Chat、Responses -> Chat、Responses shape on Chat、passthrough、幂等、非法 Schema、未知字段和工具 Schema 保留。
- Web Search 决策必须覆盖：顶层 tools、`additional_tools`、唯一搜索工具、混合工具、`auto|required|none`、强制搜索、强制其它工具、Chat fallback 拒绝、JSON Schema 冲突和 provider 不可用。
- `web.run` 循环必须覆盖：顶层和 `additional_tools` 识别、Schema 收窄、天气改走普通搜索、非法/未支持参数、recency 警告、provider 失败、代理 failover、缺失 call ID、跨轮次 4 查询上限、2 轮上限、usage/成功调用数累计、流式缓冲和其它客户端工具回程。
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
