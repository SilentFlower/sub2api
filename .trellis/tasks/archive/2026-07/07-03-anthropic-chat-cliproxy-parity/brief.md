# Brief — 对齐 CLIProxyAPI 的 Anthropic Chat 桥接

## Goal

- 将 APIKey raw Chat fallback 的 Anthropic `/v1/messages` → OpenAI Chat Completions 直连桥接调整为 CLIProxyAPI 风格的稳定转换策略，减少 Chat prefix cache 因动态 system、content 形态变化、随机 tool id 造成的重建。

## Scope

- 修改 `backend/internal/pkg/apicompat/anthropic_chatcompletions.go` 的请求转换：过滤 `x-anthropic-billing-header:`，system/user/assistant text 尽量保持 typed content part array，assistant text/reasoning/tool_calls 合并为单条 assistant message，tool_result 继续保持 Chat tool adjacency。
- 修改同文件的 Chat→Anthropic 响应转换：按 tool index 累积 id/name/arguments，优先使用上游非空 `tool_call.id`，缺 id 时生成确定性 `toolu_...` fallback。
- 已确认的流式取舍：当上游始终不提供 `tool_call.id` 时，接受延迟发送 Anthropic `tool_use` block，直到 name/arguments 足够生成确定性 fallback id，以稳定下一轮 `tool_result.tool_use_id` replay。
- 更新 `backend/internal/pkg/apicompat/anthropic_chatcompletions_test.go` 覆盖 attribution 过滤、content array 稳定、assistant tool_calls 合并、tool_result adjacency、缺 id deterministic、后续 chunk 才给 id 时使用上游 id。

## Non-Goals

- 不把 raw Chat fallback 切回 Responses API。
- 不新增通用 Chat `prompt_cache_key` 字段。
- 不修改 OAuth/Codex Responses 路径的 prompt cache key、continuation、digest replay guard。
- 不改数据库、Ent schema、前端或管理后台配置。

## Key Context

- raw Chat fallback 入口：`backend/internal/service/openai_gateway_messages.go:38`，当前会绕过 Responses 路径的 prompt cache key/digest 逻辑。
- 请求/响应核心文件：`backend/internal/pkg/apicompat/anthropic_chatcompletions.go`。
- 当前随机 tool id 位置：`backend/internal/pkg/apicompat/anthropic_chatcompletions.go:407` 和 `backend/internal/pkg/apicompat/anthropic_chatcompletions.go:733`。
- Anthropic `tool_result` 没有独立 id，只有 `tool_use_id`，引用上一轮 assistant 返回的 `tool_use.id`；需要稳定的是响应侧生成的 `tool_use.id`。
- CLIProxyAPI 参考：过滤 attribution、typed content array、assistant content/tool_calls 同 message、流式 tool_call 按 index 累积。
- 保持 `thinking:{"type":"disabled"}` 透传并去掉 `reasoning_effort` 的现有行为。
- 保持 usage 中 `cached_tokens` → Anthropic `cache_read_input_tokens` 的现有映射。
- 不记录完整 prompt、API key、Authorization header 或未脱敏上游请求体。

## Acceptance

- `cd backend && go test -tags=unit ./internal/pkg/apicompat` 通过。
- 相关 service raw Chat/Anthropic 测试按计划运行或记录环境/无关失败原因。
- 单元测试覆盖 attribution system text 被过滤。
- 单元测试覆盖 system/user/assistant text content 在 Chat 请求中保持 typed part array。
- 单元测试覆盖 assistant text + tool_use 合并为单条 assistant message，且 tool_result 紧跟对应 assistant tool_calls。
- 单元测试覆盖上游 tool_call.id 为空时，两次相同 name/arguments/index 生成相同 Anthropic `tool_use.id`。
- 单元测试覆盖上游后续 chunk 才给 tool_call.id 时，最终使用上游 id，不提前发随机 fallback id。
- 现有 thinking disabled 和 usage cached token 映射测试仍通过。

## Next Step

- 用户确认 planning artifacts 和本 brief 后，运行 `task.py start .trellis/tasks/07-03-anthropic-chat-cliproxy-parity`，再进入 `trellis-route(implement)` 选择实现路线。
