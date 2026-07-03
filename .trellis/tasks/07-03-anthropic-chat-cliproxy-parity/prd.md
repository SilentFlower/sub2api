# 对齐 CLIProxyAPI 的 Anthropic Chat 桥接

## Goal

将 OpenAI APIKey 且不走 Responses API 的 Anthropic `/v1/messages` → Chat Completions 直连桥接，调整为与 `/root/project/CLIProxyAPI` 的 Claude→OpenAI Chat 处理一致的稳定转换策略，减少 Chat prefix cache 因动态系统块、content 形态变化、随机 tool id 造成的重建。

用户价值：

- Claude Code / Anthropic 兼容客户端走只支持 `/v1/chat/completions` 的上游时，历史 replay 更稳定。
- `cached_tokens` 更容易命中，避免同一会话多轮工具调用后频繁重建 prefix cache。
- 兼容 Chat 上游的同时避免破坏 OpenAI tool message adjacency 规则。

## Confirmed Facts

- 当前 raw Chat fallback 入口在 `backend/internal/service/openai_gateway_messages.go:38`，APIKey 且 `ShouldUseResponsesAPI=false` 时直接进入 Chat 直连桥接，绕过后续 Responses 路径的 `prompt_cache_key` / digest 复用逻辑。
- 当前 Anthropic→Chat 请求转换在 `backend/internal/pkg/apicompat/anthropic_chatcompletions.go:28`。
- 当前 `system` 会被 `anthropicSystemText` 展平成单个 string，并作为 `role:"system"` 的 string content 发给 Chat；见 `backend/internal/pkg/apicompat/anthropic_chatcompletions.go:34` 和 `backend/internal/pkg/apicompat/anthropic_chatcompletions.go:81`。
- 当前 user 单 text 会被 `chatContentFromParts` 压成 string，多模态才保留 part array；见 `backend/internal/pkg/apicompat/anthropic_chatcompletions.go:157` 和 `backend/internal/pkg/apicompat/anthropic_chatcompletions.go:228`。
- 当前 assistant 多段 text 会 `strings.Join(..., "\n\n")` 压成 string；见 `backend/internal/pkg/apicompat/anthropic_chatcompletions.go:162`。
- 当前流式 Chat→Anthropic 遇到 tool delta 且 `tool_call.id` 为空时立即随机生成 `toolu_...`；见 `backend/internal/pkg/apicompat/anthropic_chatcompletions.go:407` 和 `backend/internal/pkg/apicompat/anthropic_chatcompletions.go:733`。
- Anthropic `tool_result` 没有独立 id，只有 `tool_use_id`，它必须引用上一轮 assistant 返回的 `tool_use.id`；转 Chat 时映射为 `role:"tool"` 的 `tool_call_id`。
- CLIProxyAPI 会过滤 `x-anthropic-billing-header:` 这类 Claude Code attribution/system text；见 `/root/project/CLIProxyAPI/internal/util/claude_attribution.go:8` 和 `/root/project/CLIProxyAPI/internal/translator/openai/claude/openai_claude_request.go:112`。
- CLIProxyAPI 对 text/image content 更倾向保留 typed part array；见 `/root/project/CLIProxyAPI/internal/translator/openai/claude/openai_claude_request.go:384`。
- CLIProxyAPI 将 assistant content、`reasoning_content`、`tool_calls` 放在同一条 assistant message 内，避免拆分破坏 tool adjacency；见 `/root/project/CLIProxyAPI/internal/translator/openai/claude/openai_claude_request.go:237`。
- CLIProxyAPI 流式响应按 tool index 累积 id/name/arguments，只有拿到有效 id/name 后才 emit tool_use start；见 `/root/project/CLIProxyAPI/internal/translator/openai/claude/openai_claude_response.go:237`。

## Requirements

- R1: Anthropic→Chat 转换必须过滤 Claude Code attribution/system text，至少覆盖前缀 `x-anthropic-billing-header:`，包括前导空白场景。
- R2: `system` 为 string 或 text block array 时，Chat message content 应保持 CLIProxyAPI 风格的 typed part array；过滤后无内容则不输出 system message。
- R3: user/assistant 的 text 和 image content 应尽量保持 typed part array，不因为单 text 场景在 string / array 之间切换。
- R4: assistant message 内的 text、兼容的 `reasoning_content`、`tool_calls` 应合并到同一条 assistant Chat message；不得为同一 Anthropic assistant content 拆出多条 assistant message。
- R5: `tool_result` 必须继续输出为 `role:"tool"`，并保持紧跟上一条 assistant `tool_calls` 的 adjacency；同一 Anthropic user 轮次中其它 text/image 内容应作为后续 user message 输出。
- R6: 历史 assistant `thinking` 默认不得作为普通 text 进入 Chat prompt；只有可判定为 GPT-compatible signature 的 thinking 才可转为 `reasoning_content`。若本仓库缺少签名兼容判断能力，先保持丢弃 thinking，并在设计中明确不扩大行为。
- R7: Chat→Anthropic 响应转换必须消除缺失 `tool_call.id` 时的随机 `tool_use.id`。上游给 id 时必须原样保留；上游不给 id 时必须生成确定性 id。
- R8: deterministic fallback id 必须满足 Anthropic `tool_use.id` 可用性，且同一 tool index/name/arguments 在重复请求中生成相同 id。
- R9: 对流式响应，不能因为 tool delta 的 id/name/arguments 分片顺序不同而把空 id、空 name 或随机 id 发送给客户端；当上游始终不提供 `tool_call.id` 时，接受延迟发送 Anthropic `tool_use` block，直到 name/arguments 足够生成确定性 fallback id。
- R10: usage 中 `cached_tokens` 到 Anthropic `cache_read_input_tokens` 的现有映射必须保持。
- R11: `thinking:{"type":"disabled"}` 透传并去掉 `reasoning_effort` 的现有行为必须保持。
- R12: 不在 raw Chat fallback 中引入新的外部配置项；行为默认生效。

## Acceptance Criteria

- [ ] `go test -tags=unit ./internal/pkg/apicompat` 通过。
- [ ] 新增或更新单元测试覆盖 attribution system text 被过滤。
- [ ] 新增或更新单元测试覆盖 system/user/assistant text content 在 Chat 请求中保持 typed part array。
- [ ] 新增或更新单元测试覆盖 assistant text + tool_use 被合并为单条 assistant message，且 tool_result 紧跟对应 assistant tool_calls。
- [ ] 新增或更新单元测试覆盖上游 tool_call.id 为空时，两次相同 name/arguments/index 生成相同 Anthropic `tool_use.id`。
- [ ] 新增或更新单元测试覆盖上游后续 chunk 才给 tool_call.id 时，最终使用上游 id，不提前发随机 fallback id。
- [ ] 现有 `thinking:{"type":"disabled"}` 相关测试仍通过。
- [ ] 现有 usage cached token 映射相关测试仍通过。
- [ ] 变更不记录完整 prompt、API key、Authorization header 或未脱敏上游请求体。

## Out of Scope

- 不把 raw Chat fallback 切回 Responses API。
- 不为 Chat Completions 上游新增通用 `prompt_cache_key` 协议字段，除非后续单独确认上游支持。
- 不修改 OAuth/Codex Responses 路径的 prompt cache key、continuation、digest replay guard 逻辑。
- 不改数据库、Ent schema、前端或管理后台配置。

## Decisions

- D1: 接受缺失 `tool_call.id` 时延迟发送 Anthropic `tool_use` block，以稳定 `tool_use.id` 和下一轮 `tool_result.tool_use_id` replay。代价是少数缺 id 上游的工具调用 UI 不再边生成边展示参数，而是参数完整后一次性出现。
