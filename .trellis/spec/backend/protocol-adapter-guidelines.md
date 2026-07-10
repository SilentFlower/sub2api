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
  func resolveOpenAIUpstreamEndpoint(c *gin.Context, account *service.Account) string
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
func extractOpenAIUpstreamReasoningEffort(body []byte, requestedModel string, mappedModel string) *string
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
reasoningEffort := extractOpenAIUpstreamReasoningEffort(upstreamBody, originalModel, upstreamModel)
```

先按最终模型改写上游 body，再从实际发送体提取日志值；Grok/GLM 不推断默认档位，其它 provider 保留既有 fallback。

---

## Scenario: Grok CLI Billing 套餐额度快照

### 1. Scope / Trigger

- Trigger: 修改 Grok CLI Billing 查询、`GrokQuotaService.QueryBillingQuota`、`xai.BillingSnapshot`、账号 usage DTO、`extra` 缓存键或前端“Grok 套餐额度”展示时，必须按本节检查。
- 适用后端路径：
  - `backend/internal/pkg/xai/billing.go`
  - `backend/internal/service/grok_quota_service.go`
  - `backend/internal/service/grok_quota_fetcher.go`
  - `backend/internal/service/account_usage_service.go`
  - `backend/internal/handler/admin/grok_oauth_handler.go`
  - `backend/internal/server/routes/admin.go`
  - `backend/internal/repository/account_repo.go`
- 适用前端路径：
  - `frontend/src/api/admin/grok.ts`
  - `frontend/src/types/index.ts`
  - `frontend/src/components/account/AccountUsageCell.vue`
  - `frontend/src/components/account/GrokBillingQuotaCell.vue`
- 目标：新增 Grok CLI Billing subscription 展示时，必须与现有 Grok rate-limit header 快照完全隔离，不能替换或污染 `extra.grok_usage_snapshot`、`grok_request_quota`、`grok_token_quota` 和主动 probe 行为。

### 2. Signatures

- 管理端刷新 API：
  ```text
  GET /api/v1/admin/grok/accounts/:id/billing-quota
  ```
- Handler：
  ```go
  func (h *GrokOAuthHandler) QueryBillingQuota(c *gin.Context)
  ```
- Service：
  ```go
  func (s *GrokQuotaService) QueryBillingQuota(ctx context.Context, accountID int64) (*GrokBillingQuotaResult, error)
  ```
- xAI billing helpers：
  ```go
  func BuildBillingURL(baseURL string, formatCredits bool) (string, error)
  func ParseBillingPayload(data []byte) (*BillingPayload, error)
  func BuildBillingSnapshot(weeklyPayload, monthlyPayload *BillingPayload, now time.Time) *BillingSnapshot
  func BillingSnapshotFromRaw(raw any) (*BillingSnapshot, error)
  ```
- 缓存键与 DTO 字段：
  ```text
  accounts.extra.grok_billing_snapshot
  UsageInfo.GrokBillingQuota json:"grok_billing_quota,omitempty"
  ```
- 前端类型和 API：
  ```typescript
  export interface GrokBillingQuota
  export async function queryBillingQuota(id: number): Promise<GrokBillingQuotaResult>
  ```

### 3. Contracts

- 只支持 `platform=grok` 且 `type=oauth` 的账号；非 Grok 或非 OAuth 账号必须在本地拒绝，不能请求 Grok CLI Billing 上游。
- 查询必须通过 `GrokTokenProvider.GetAccessToken(ctx, account)` 获取 Grok OAuth access token；如果账号设置了 `ProxyID` 或已加载 `account.Proxy`，必须沿用该账号代理请求 billing URL。
- 上游地址固定从 `xai.DefaultCLIBaseURL` 构造：
  - `GET https://cli-chat-proxy.grok.com/v1/billing?format=credits`
  - `GET https://cli-chat-proxy.grok.com/v1/billing`
- 上游请求头必须包含 CLI billing 所需的非敏感固定头：`x-xai-token-auth: xai-grok-cli`、`x-grok-client-version`、`Accept`、`User-Agent`；`Authorization` 只能用于请求上游，不能写入缓存、日志或 API 响应。
- `?format=credits` 响应主要提供周 credits、周期结束时间和产品用量；普通 `/billing` 响应主要提供月 credits、billing period、plan 和按量付费 cap/used。合并逻辑必须通过 `BuildBillingSnapshot` 产出一个展示快照。
- 成功后只写入 `extra.grok_billing_snapshot`，且该键必须是 scheduler neutral extra key。不能写入、覆盖或复用 `extra.grok_usage_snapshot`。
- `GET /api/v1/admin/accounts/:id/usage` 只从 `extra.grok_billing_snapshot` 读取缓存并投影到 `grok_billing_quota`；缓存缺失或解析失败不能影响旧 Grok 请求/Token 额度的 unknown、observed、rate_limited 等状态。
- `BillingSnapshot` 只允许保存非敏感展示字段：`period_type`、`weekly_used_percent`、`weekly_reset_at`、`product_usage`、`monthly_limit_cents`、`monthly_used_cents`、`monthly_remaining_cents`、`monthly_used_percent`、`billing_period_start`、`billing_period_end`、`on_demand_cap_cents`、`on_demand_used_cents`、`on_demand_remaining_cents`、`on_demand_used_percent`、`plan_label`、`updated_at`、`stale`。
- 上游错误 body 必须截断并脱敏后才进入日志或错误响应：
  ```go
  bodyText := logredact.RedactText(truncate(strings.TrimSpace(string(bodyBytes)), 240), "authorization")
  ```
- 前端用户可见标题必须使用“Grok 套餐额度”，不能使用“CPA”作为产品区块名称。月额度是主进度条；周额度有有效数据时显示附加行；产品用量和按量付费状态与 CLIProxyAPI Management Center 展示口径对齐。

### 4. Validation & Error Matrix

- `GrokQuotaService`、`GrokTokenProvider` 或 `HTTPUpstream` 未配置 -> `500 GROK_QUOTA_NOT_CONFIGURED`。
- 账号不存在 -> `404 GROK_QUOTA_ACCOUNT_NOT_FOUND`。
- `platform != grok` -> `400 GROK_QUOTA_INVALID_PLATFORM`，且不得请求上游。
- `type != oauth` -> `400 GROK_QUOTA_INVALID_TYPE`，且不得请求上游。
- token 获取失败或为空 -> `502 GROK_QUOTA_TOKEN_UNAVAILABLE`。
- billing URL 构造失败 -> `500 GROK_BILLING_URL_INVALID`。
- 上游请求构造失败 -> `500 GROK_BILLING_REQUEST_BUILD_FAILED`。
- 上游网络请求失败 -> `502 GROK_BILLING_REQUEST_FAILED`。
- 上游返回 `401` -> 对外 `401 GROK_BILLING_UPSTREAM_ERROR`，body 脱敏截断。
- 上游返回 `403` -> 对外 `403 GROK_BILLING_UPSTREAM_ERROR`，body 脱敏截断。
- 上游返回 `429` -> 对外 `429 GROK_BILLING_UPSTREAM_ERROR`，body 脱敏截断。
- 上游其它 `4xx/5xx` -> 对外 `502 GROK_BILLING_UPSTREAM_ERROR`，body 脱敏截断。
- 上游 JSON 解析失败 -> `502 GROK_BILLING_PARSE_FAILED`。
- weekly 和 monthly 都没有可用 quota 字段 -> `502 GROK_BILLING_EMPTY`。
- `extra.grok_billing_snapshot.updated_at` 早于 TTL 或无法解析 -> `grok_billing_quota.stale=true`；旧 Grok request/token 额度状态保持原样。

### 5. Good/Base/Bad Cases

- Good: Grok OAuth 账号主动刷新套餐额度后，`extra.grok_billing_snapshot` 更新，`extra.grok_usage_snapshot` 保持原值。
- Good: 旧 Grok rate-limit headers 从未出现时，usage API 仍返回 `quota_unknown`，但可以附带已有 `grok_billing_quota`。
- Good: 周数据来自 `?format=credits`，月额度和按量付费数据来自普通 `/billing`，前端在独立“Grok 套餐额度”区块合并展示。
- Good: 上游错误 body 中包含 `Authorization` 或 token-like 字段时，日志和响应只出现脱敏后的内容。
- Base: 存量 Grok 账号没有 `grok_billing_snapshot` 时，账号列表不显示套餐额度旧值，且不会影响请求/Token 额度展示。
- Base: `on_demand_cap_cents` 为空或小于等于 0 时，前端显示按量付费未启用，而不是显示 0/0 进度条。
- Bad: 把 billing 的月 credits 写入 `grok_request_quota` 或 `grok_token_quota`，导致用户误以为它来自 xAI rate-limit headers。
- Bad: 用 `extra.grok_usage_snapshot` 保存 billing 快照，导致主动 probe、被动 header 采样和套餐额度互相覆盖。
- Bad: 非 OAuth Grok API key 账号也触发 CLI Billing 请求，因为 CLI Billing 依赖 Grok OAuth access token。
- Bad: 错误响应直接拼接完整上游 body，泄露 Authorization、access token、refresh token 或账号隐私字段。

### 6. Tests Required

- xAI 解析单测必须覆盖：
  - `BuildBillingURL` 对 `formatCredits=true/false` 的 URL。
  - `ParseBillingPayload` 的空 payload、非法 JSON、snake_case / camelCase 字段。
  - `BuildBillingSnapshot` 的周/月合并、缺字段、产品用量、按量付费 cap/used、剩余额度和 plan label。
- service 单测必须覆盖：
  - `QueryBillingQuota` 使用 `GrokTokenProvider.GetAccessToken` 取得 token。
  - 账号代理被传给 `HTTPUpstream.Do`。
  - 成功只写入 `grok_billing_snapshot`，不覆盖 `grok_usage_snapshot`。
  - `BuildUsageInfo` 在没有旧 header 快照时仍能附带 `GrokBillingQuota`。
- handler 单测必须覆盖：
  - 成功响应 envelope。
  - 非 Grok 账号返回 `GROK_QUOTA_INVALID_PLATFORM`，且不触发上游。
  - 非 OAuth 账号返回 `GROK_QUOTA_INVALID_TYPE`，且不触发上游。
  - 上游失败响应和日志不包含 access token、refresh token、`Authorization` 原文。
- 前端测试必须覆盖：
  - `AccountUsageCell` 中独立“Grok 套餐额度”展示。
  - 月额度主行、周额度附加行、产品用量、按量付费启用/未启用状态。
  - 缓存展示、TTL 懒刷新、主动刷新失败不影响旧 Grok 请求/Token 进度条。
- 建议运行：
  ```bash
  cd backend && go test -tags=unit ./internal/pkg/xai ./internal/service ./internal/handler/admin -run 'Grok.*Billing|Grok.*Quota|AccountUsage|Billing'
  cd frontend && pnpm vitest run src/components/account/__tests__/AccountUsageCell.spec.ts
  cd frontend && pnpm typecheck
  cd frontend && pnpm lint:check
  git diff --check
  ```

### 7. Wrong vs Correct

#### Wrong

```go
_ = s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
	grokQuotaSnapshotExtraKey: snapshot,
})
```

问题：把 CLI Billing 快照写进 `grok_usage_snapshot` 会覆盖旧 rate-limit header 额度，破坏现有请求/Token 进度条和主动 probe。

#### Correct

```go
_ = s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
	grokBillingSnapshotExtraKey: snapshot,
})
```

套餐额度必须使用独立缓存键 `grok_billing_snapshot`，由 `UsageInfo.GrokBillingQuota` 独立投影。

#### Wrong

```go
bodyText := truncate(strings.TrimSpace(string(bodyBytes)), 240)
return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_BILLING_UPSTREAM_ERROR", "upstream returned %d: %s", resp.StatusCode, bodyText)
```

问题：上游 body 可能包含 `Authorization`、token 或账号隐私字段，仅截断不足以防止泄露。

#### Correct

```go
bodyText := logredact.RedactText(truncate(strings.TrimSpace(string(bodyBytes)), 240), "authorization")
return nil, infraerrors.Newf(mapUpstreamStatus(resp.StatusCode), "GROK_BILLING_UPSTREAM_ERROR", "upstream returned %d: %s", resp.StatusCode, bodyText)
```

上游错误必须先截断再显式脱敏 `authorization` 字段，并保留 `mapUpstreamStatus` 的 401/403/429 映射语义。

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
