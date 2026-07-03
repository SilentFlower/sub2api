# Design: 对齐 CLIProxyAPI 的 Anthropic Chat 桥接

## Architecture Boundary

本任务只修改后端协议适配和 OpenAI 网关 raw Chat fallback：

- 请求转换边界：`backend/internal/pkg/apicompat/anthropic_chatcompletions.go`
- 响应转换边界：`backend/internal/pkg/apicompat/anthropic_chatcompletions.go`
- 网关调用边界：`backend/internal/service/openai_messages_chat_fallback.go`

不引入 DB、配置、前端或新路由。

## Data Flow

```text
Anthropic /v1/messages request
  -> apicompat.AnthropicToChatCompletions
  -> OpenAI-compatible /v1/chat/completions upstream
  -> ChatCompletionsChunkToAnthropicEvents / ChatCompletionsStreamToAnthropicResponse
  -> Anthropic SSE 或 Anthropic JSON response
```

## Request Conversion Strategy

1. 增加 Claude Code attribution 识别函数，过滤前导空白后以 `x-anthropic-billing-header:` 开头的 text。
2. 将 `system` 转换为 `role:"system"` 且 `content` 为 typed part array：
   ```json
   {"role":"system","content":[{"type":"text","text":"..."}]}
   ```
3. 将 user text/image 转换为 typed part array；不再因为单 text 压成 string。
4. user `tool_result` 仍提取为 `role:"tool"`：
   ```json
   {"role":"tool","tool_call_id":"<tool_use_id>","content":"..."}
   ```
   同一 Anthropic user message 中的 text/image 作为后续 `role:"user"` 输出，以保持 Chat tool adjacency。
5. assistant content 转换为单条 assistant message：
   - text/image -> typed content array
   - tool_use -> `tool_calls`
   - compatible thinking -> `reasoning_content`
6. thinking 兼容映射先保守实现：若本仓库没有 GPT-compatible signature 判断能力，则维持当前“历史 assistant thinking 不进 Chat prompt”的行为，避免把 Anthropic thinking 当普通 text 注入。

## Response Conversion Strategy

### 上游提供 tool_call.id

当 Chat 上游提供非空 `tool_call.id` 时，直接使用该 id 作为 Anthropic `tool_use.id`。如果 id/name/arguments 分不同 chunk 到达，转换器必须累计状态，不允许空 id 覆盖已记录 id。

### 上游缺失 tool_call.id

当前随机 fallback 会破坏 replay 稳定性。改为确定性 fallback：

```text
seed = chat_tool_call_index + "\n" + function_name + "\n" + normalized_arguments
id = "toolu_" + hex(sha256(seed))[0:24]
```

要求：

- 同一 index/name/arguments 重复请求生成相同 id。
- 不包含请求时间、随机数、message id、上游 request id 等非稳定字段。
- arguments 使用完整字符串；若为空则按 `{}` 处理。

### 流式发送取舍

若上游没有 id，确定性 id 依赖完整 arguments。为了避免先发随机 id，再在后续 replay 中变化，转换器应延迟该 tool_use block 的 start/delta/stop，直到 name/arguments 足够生成 fallback id。若上游提供 id，则可以保持现有按 chunk 流式输出。

该取舍已确认：稳定 replay/cache 优先于少数缺 id 上游的工具参数边生成边展示体验。

## Compatibility Notes

- 保持 `thinking:{"type":"disabled"}` 透传并省略 `reasoning_effort`。
- 保持 `stream_options.include_usage=true` 和 usage cached token 映射。
- 不新增 `prompt_cache_key`，因为 raw Chat 上游是否支持该字段不确定；本任务先把 payload 前缀本身做稳定。
- 不记录完整 prompt。若新增诊断日志，只能记录 hash、role、长度、tool id 等安全摘要。

## Rollback

若上游因 typed part array 兼容性出现问题，可回滚 `AnthropicToChatCompletions` 的 content 形态变更，保留 attribution 过滤和 deterministic tool id 作为独立修复。
