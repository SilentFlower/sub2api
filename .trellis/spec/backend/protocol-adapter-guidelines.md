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
- Chat usage cached tokens 大于 prompt tokens -> Anthropic `input_tokens` 归零，不产生负数。
- `thinking.type == "disabled"` -> 输出 Chat `thinking.type=disabled`，且 `reasoning_effort` 必须为空。
- Anthropic body 含 `metadata.user_id` 且无显式 session 信号 -> 账号 sticky key 来自 `reqModel + "-" + metadata.user_id`，不被 model/tools/首条 user content fallback 覆盖。
- Anthropic body 同时含显式 session 信号和 `metadata.user_id` -> 显式 session 信号优先。

### 5. Good/Base/Bad Cases

- Good: 同一 Anthropic 历史 replay 转出的 Chat messages 在单 text、多 text、多模态之间都保持 `content` array 形态。
- Good: Claude Code 每轮变化的 `x-anthropic-billing-header:` 不进入上游 Chat payload。
- Good: 并行工具结果即使按完成时间反序返回，Chat payload 中紧跟 assistant 的 tool messages 仍按上一条 `tool_calls` 顺序稳定输出。
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
