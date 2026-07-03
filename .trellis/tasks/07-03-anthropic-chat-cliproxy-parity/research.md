# Research: CLIProxyAPI Anthropic Chat Bridge Parity

## 问题重述

当前 sub2api 的 raw Chat fallback 已能完成 Anthropic Messages 与 OpenAI Chat Completions 的直连桥接，但 payload 形态更偏语义压缩：system/text 会被字符串化，tool_call 缺 id 时会随机生成 Anthropic tool_use id。用户观察到 Chat prefix cache 出现重建，希望对齐 CLIProxyAPI 的处理，并修复 tool id 不稳定。

## sub2api 当前行为

- `backend/internal/service/openai_gateway_messages.go:38`：APIKey 且不使用 Responses API 时直接走 raw Chat fallback。
- `backend/internal/pkg/apicompat/anthropic_chatcompletions.go:28`：`AnthropicToChatCompletions` 是请求转换入口。
- `backend/internal/pkg/apicompat/anthropic_chatcompletions.go:34`：system 被转换成一条 `role:"system"` Chat message。
- `backend/internal/pkg/apicompat/anthropic_chatcompletions.go:81`：`anthropicSystemText` 将 string 或 text blocks 展平成单个 string。
- `backend/internal/pkg/apicompat/anthropic_chatcompletions.go:128`：user content 中 `tool_result` 被提取为 `role:"tool"`，其它 text/image 作为 user message。
- `backend/internal/pkg/apicompat/anthropic_chatcompletions.go:162`：assistant blocks 被合并到单条 assistant message，但 text parts 被 `strings.Join(..., "\n\n")` 压成 string。
- `backend/internal/pkg/apicompat/anthropic_chatcompletions.go:228`：单 text part 被输出为 string，多 part 才输出 typed array。
- `backend/internal/pkg/apicompat/anthropic_chatcompletions.go:407`：流式响应里一旦打开 tool_use block，如果当前 delta 没有 id，会立即生成 fallback id。
- `backend/internal/pkg/apicompat/anthropic_chatcompletions.go:733`：fallback id 使用 `crypto/rand`，重复请求不稳定。

## CLIProxyAPI 可借鉴行为

- `/root/project/CLIProxyAPI/internal/util/claude_attribution.go:8`：定义 `x-anthropic-billing-header:` attribution 前缀识别。
- `/root/project/CLIProxyAPI/internal/translator/openai/claude/openai_claude_request.go:105`：system message 使用 `content:[]` typed array。
- `/root/project/CLIProxyAPI/internal/translator/openai/claude/openai_claude_request.go:112`：跳过空 system text 和 Claude Code attribution text。
- `/root/project/CLIProxyAPI/internal/translator/openai/claude/openai_claude_request.go:157`：assistant thinking、tool_calls、tool_results 在同一轮转换中统一处理。
- `/root/project/CLIProxyAPI/internal/translator/openai/claude/openai_claude_request.go:237`：assistant content、`reasoning_content`、`tool_calls` 放在同一条 assistant message。
- `/root/project/CLIProxyAPI/internal/translator/openai/claude/openai_claude_request.go:384`：text/image 转换为 typed content part。
- `/root/project/CLIProxyAPI/internal/translator/openai/claude/openai_claude_response.go:237`：流式响应只接受非空 JSON string id，避免空值覆盖已记录 id。
- `/root/project/CLIProxyAPI/internal/translator/openai/claude/openai_claude_response.go:267`：id/name 可能分 chunk 到达，因此每个 chunk 都重新检查是否可以 emit tool_use start。

## 缓存不稳定推断

1. Claude Code attribution/system text 若每轮变化且被放在 prompt 最前部，会导致 prefix cache 从 system 区域重建。
2. 随机 `tool_use.id` 会进入下一轮 `tool_result.tool_use_id`，导致工具调用后的历史 replay 不稳定。
3. string/array content 形态切换不一定导致同一实现内每轮变化，但会改变上游 tokenizer/cache 看到的边界，且与 CLIProxyAPI 的稳定 replay 风格不同。

## Tool Result ID 结论

Anthropic `tool_result` 没有独立 id，只有 `tool_use_id`。它引用上一轮 assistant 返回的 `tool_use.id`。因此需要稳定的是 assistant 响应转换生成的 `tool_use.id`，而不是 `tool_result` 本身。
