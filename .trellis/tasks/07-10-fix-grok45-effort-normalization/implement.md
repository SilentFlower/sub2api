# 实施计划

## Step 1: 收敛 provider effort helper

- 在 `backend/internal/service/gateway_request.go` 抽取共享的嵌套/扁平 effort body 改写流程。
- 保留并调整 GLM mapper，使 `none/minimal` 规范为小写，既有 high/max 映射不变。
- 增加仅精确匹配 `grok-4.5` 的 mapper，落实 `low/medium/high` 饱和映射。
- 增加按最终上游模型选择 GLM/Grok 规则的统一入口。
- 增加从最终上游 body 原样提取 effort 的 helper，不过滤未知值，不推断默认值。

## Step 2: 覆盖 Grok Responses 与 WebSocket

- 在 `patchGrokResponsesBody` 设置最终模型后执行 Grok 4.5 effort 归一化。
- `forwardGrokResponses` 从 patched body 提取结果 effort。
- `ForwardAsAnthropic` 的 Grok Responses 分支从最终 `responsesBody` 回填结果 effort。
- `proxyOpenAIWSHTTPBridgeTurn` 从最终 patched body 回填结果 effort。
- 保持不支持字段、工具、配额快照和错误路径不变。

## Step 3: 覆盖 Chat fallback 与 GLM 遗漏入口

- `forwardAsRawChatCompletions` 在最终模型改写后调用 provider 分派 helper，并在 fast policy 后从最终 body 提取日志 effort。
- `forwardAnthropicViaRawChatCompletions` 同步使用 provider 分派 helper和最终 body 提取。
- `forwardResponsesViaRawChatCompletions` 增加 provider effort 归一化，补齐 GLM Responses 转 Chat 路径，并从最终 Chat body 提取日志 effort。
- 确认这些路径不再用 `ApplyThinkingEnabledFallback` 为缺失字段猜测档位。

## Step 4: 修正前端展示

- 修改 `frontend/src/utils/format.ts`，让 `none/minimal` 分别显示为 `None/Minimal`。
- 在 `frontend/src/utils/__tests__/` 新增 formatter 单测，覆盖空值、标准档位、分隔符变体、`none/minimal` 和未知值。

## Step 5: 后端测试

- 扩展 `gateway_request_test.go`：
  - Grok 4.5 全量映射表、大小写/分隔符、未知值、缺失值。
  - 非 4.5 Grok 模型不修改。
  - GLM `none/minimal`、既有 high/max 映射和未知值。
- 扩展 `openai_gateway_grok_test.go`：Responses body 和结果日志使用归一化后值，缺失值保持 nil。
- 扩展 `openai_gateway_chat_completions_raw_test.go`：Grok 与 GLM 上游 body、`OpenAIForwardResult.ReasoningEffort` 一致。
- 补 Messages Responses/强制 Chat、Responses 转 Chat 和 WebSocket HTTP bridge 的回归断言。
- 保留并运行现有 Grok 字段清理、错误处理、配额快照测试。

## Validation Commands

```bash
cd backend
go test -tags=unit ./internal/service -run 'TestNormalize.*ReasoningEffort|TestForward.*Grok|TestForward.*GLM|TestPatchGrokResponsesBody|Test.*ReasoningEffort|Test.*WSHTTPBridge'
go test -tags=unit ./internal/service
```

```bash
cd frontend
pnpm vitest run src/utils/__tests__/formatReasoningEffort.spec.ts
pnpm typecheck
pnpm lint:check
```

```bash
git diff --check
```

## Risk Files

- `backend/internal/service/gateway_request.go`
- `backend/internal/service/openai_gateway_grok.go`
- `backend/internal/service/openai_gateway_chat_completions_raw.go`
- `backend/internal/service/openai_gateway_messages.go`
- `backend/internal/service/openai_gateway_messages_chat_fallback.go`
- `backend/internal/service/openai_gateway_responses_chat_fallback.go`
- `backend/internal/service/openai_ws_http_bridge.go`
- `frontend/src/utils/format.ts`

## Rollback Points

- provider mapper 与共享 body helper 可单独回滚。
- 各转发路径的最终 body 日志提取可按文件回滚，不影响数据库结构。
- 前端 formatter 回滚不影响后端数据。
