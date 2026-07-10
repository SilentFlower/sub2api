# Design: 合并 main 到 build 并保留现有 feature

## 合并边界

- 目标分支：当前 `build`。
- 来源分支：实施前刷新后的 `origin/main`。
- 合并方式：`git merge --no-commit --no-ff origin/main`，先解决和验证，再创建 merge commit。
- 回滚点：创建 `backup/build-before-main-merge-<build短提交号>`；提交前可使用 `git merge --abort`，提交后可从备份分支恢复或对 merge commit 执行显式回滚。

## 总体策略

`main` 提供最新上游修复，`build` 提供定制业务能力。冲突解决遵循“保留双方语义、优先复用现有抽象”的原则：

- 结构和公共修复以 `main` 为准。
- `build` 已有 feature 及其回归测试必须保留。
- 已经在 `build` 中集中化的常量或 helper 不因合并重新分散。
- provider-specific effort 记录使用最终上游请求体；非 provider-specific 场景使用多模型候选恢复后缀 effort。

## 冲突解决矩阵

| 文件 | 冲突原因 | 解决方案 |
| --- | --- | --- |
| `account_usage_service.go` | `main` 新增 `openAICodexProbeVersion=0.144.1`，`build` 已将其集中到 identity 文件 | 已确认：保留 `build` 的集中定义和 `main` 的 `0.144.1` 值，吸收其它 usage/billing 改动，禁止重复常量 |
| `openai_gateway_chat_completions_raw.go` | `main` 在改写前提取多候选 effort；`build` 在 provider 归一化和 policy 后提取最终 effort | 保留 `build` 的最终请求体提取时机，并让 helper 接受 `upstreamModel/billingModel/originalModel` 候选 |
| `openai_gateway_messages_chat_fallback.go` | `build` 保留直连桥接与 Beta Fast tier；`main` 增加多候选 effort | 已确认：只保留一个 `forwardAnthropicViaRawChatCompletions` 入口，内部使用直连 `AnthropicToChatCompletions`，保留 Beta Fast 和 policy 后最终值，并吸收 main 的多候选、错误处理与 failover 修复 |
| `openai_gateway_responses_chat_fallback.go` | 同时修改 effort 提取时机 | 保留 provider 归一化后的最终值，并传入完整模型候选 |
| `openai_gateway_service.go` | `main` 在本文件定义 Codex UA/version，`build` 已集中定义 | 已确认：不恢复本地重复常量，继续使用 identity 文件 |
| `openai_ws_http_bridge.go` | `main` 增加 mapped/original 候选，`build` 记录 Grok 4.5 最终归一化值 | 使用组合 helper：provider 模型读最终 body，其它模型按 mapped/original 候选推导 |
| `setting_gateway_runtime.go` | `main` 在本文件定义默认 Codex UA，`build` 已集中定义 | 已确认：保留集中定义，settings 缓存继续引用 `DefaultOpenAICodexUserAgent` |

## Effort 组合设计（用户已确认）

合并后保留 `extractOpenAIReasoningEffortFromBody(body, modelCandidates...)` 的多候选能力，并调整 `extractOpenAIUpstreamReasoningEffort` 的调用契约：

1. 独立传入最终上游模型，用于判断 GLM/Grok 4.5 provider-specific 档位。
2. 对 provider-specific 模型直接从最终上游请求体读取 effort，避免把已归一化的 `high/max/minimal` 重新解释成客户端值。
3. 对其它模型按 `upstreamModel -> billingModel -> originalModel` 顺序提取；显式 `max` 使用首个非空候选判断 GPT-5.6 兼容性，缺省 effort 则逐个候选尝试后缀推导。
4. 仅在仍未得到 effort 时应用 `thinking.enabled` fallback。

用户已确认按该设计处理；它同时保留 `build` 的 Grok 4.5/GLM 真实 usage 语义和 `main` 的 Codex/GPT 后缀元数据修复。

## Anthropic Messages fallback（用户已确认）

`/v1/messages` 需要降级到 Chat Completions 时只维护一条内部实现：保留 `forwardAnthropicViaRawChatCompletions` 作为入口，使用 `build` 的 `AnthropicToChatCompletions` 直连转换。`main` 的 Responses 正常转发和 Responses -> Chat 专用 fallback 继续存在；被排除的只是同一 Anthropic raw Chat fallback 的第二套转换算法。

## 自动合并复核

以下双方共同修改但未产生文本冲突的文件必须查看最终 diff 和相关测试：

- `backend/internal/handler/openai_gateway_handler.go`
- `backend/internal/pkg/apicompat/types.go`
- `backend/internal/server/api_contract_test.go`
- `backend/internal/service/openai_gateway_chat_completions_raw_test.go`
- `backend/internal/service/openai_gateway_forward.go`
- `backend/internal/service/openai_gateway_messages.go`
- `backend/internal/service/openai_gateway_request_body.go`
- `backend/resources/model-pricing/model_prices_and_context_window.json`

重点检查：Grok messages 路由条件未被放宽、Anthropic sticky 优先级未回退、main 的多候选 helper 签名与所有 build 调用兼容、GPT-5.6 pricing 没有覆盖 build 自定义价格或模型条目。

预演后的具体结果：

- handler 自动合并同时保留 `build` 的 Anthropic `metadata.user_id` 粘性优先级和 `main` 的 compact SSE keepalive/failover 修复。
- apicompat types 同时保留 `build` 的 `ChatThinking` / `max` 类型表达和 `main` 的 `parallel_tool_calls`、cache creation usage 字段。
- messages 自动合并同时保留 Grok 显式强制 Chat 分流和 main 的 Responses 路径修复。
- pricing JSON 自动合并为 `main` 的 GPT-5.6 官方分档价格、cache write 成本，加上 `build` 的 `supports_max_reasoning_effort=true`。
- request body 存在一处语义冲突：自动合并会让所有模型都保留 `max`。用户已确认采用 `main` 的模型感知规则，仅 GPT-5.6 保留 `max`，其它 GPT/Codex 模型折叠为 `xhigh`；GLM/Grok 继续走 provider-specific 映射。

## 验证与提交

先运行冲突相关定向测试，再运行 service/package 级单测和前端 typecheck。测试通过后才提交 merge；merge commit 创建后以 normal 模式普通推送当前 `build` 到 `origin/build`，不 force push，也不额外合并到其它分支。若在提交前发现大范围回归，优先 `git merge --abort` 回到备份点重新调整方案，不在错误合并基础上叠加补丁。
