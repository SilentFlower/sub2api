# Implement Plan: 对齐 CLIProxyAPI 的 Anthropic Chat 桥接

## Checklist

1. 在 `backend/internal/pkg/apicompat/anthropic_chatcompletions.go` 增加或复用 attribution text 判断，过滤 `x-anthropic-billing-header:`。
2. 调整 `AnthropicToChatCompletions` 请求转换：
   - system 输出 typed text array；
   - user text/image 输出 typed content array；
   - assistant text/tool_use 合并为单条 assistant message；
   - 保持 tool_result adjacency；
   - 保持 thinking disabled 与 reasoning_effort 现有行为。
3. 调整 Chat→Anthropic 流式状态机：
   - 按 tool index 累积 id/name/arguments；
   - 非空上游 id 优先；
   - 缺 id 时使用 deterministic fallback id；
   - 防止空 id/name 覆盖已记录值；
   - 上游始终缺 id 时，延迟发送 tool_use block，直到可基于 name/arguments 生成确定性 fallback id。
4. 调整 Chat stream 聚合为非流式 Anthropic response 的 tool id fallback，同步使用 deterministic id。
5. 更新 `backend/internal/pkg/apicompat/anthropic_chatcompletions_test.go`：
   - attribution 过滤；
   - content array 稳定；
   - assistant content + tool_calls 同 message；
   - tool_result adjacency；
   - 缺 id deterministic；
   - 后续 chunk 才给 id 时使用上游 id。
6. 运行 gofmt。
7. 运行验证命令。

## Validation Commands

```bash
cd backend
go test -tags=unit ./internal/pkg/apicompat
go test -tags=unit ./internal/service -run 'Test.*Messages|Test.*Chat|Test.*Anthropic|Test.*OpenAI'
```

如第二条受现有环境或无关测试影响失败，至少记录失败用例和原因，并保留第一条作为核心质量门。

## Risk Points

- Chat 上游对 `content` typed array 的兼容性可能不一致。CLIProxyAPI 已采用该策略，但 sub2api 上游池可能更杂，需要测试覆盖并保留回滚点。
- 流式缺 id 时延迟 tool_use 会改变客户端收到工具调用的时机。该取舍已确认，用于换取稳定 replay/cache。
- thinking signature 兼容判断如果本仓库没有现成实现，不应临时引入不可信判断或新依赖。

## Rollback Points

- 请求 content typed array 变更可单独回滚。
- deterministic tool id 可独立保留，因为它直接修复 replay 不稳定。
- attribution 过滤可独立保留，因为它移除动态系统块，风险低。
