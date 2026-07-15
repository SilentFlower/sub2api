# 技术设计：同步 main 0.1.156 到 build

## 合并策略

- 固定目标：`build=4f683e95`、`origin/main=d515c304`、merge base
  `7c717365`。实施前 fetch 后重新确认。
- 合并方式：`git merge --no-commit --no-ff origin/main`。
- 解决原则：main 新架构作为长期基线；build 已验证且 main 未等价覆盖的行为作为
  强制兼容层迁移，禁止仅为了减少冲突删除测试或产品能力。
- 回滚边界：merge commit 前可 `git merge --abort`；merge commit 后只允许后续
  修复提交，不自动 reset/amend。

## 冲突解决矩阵

### 1. `backend/cmd/server/wire_gen.go`

**决定：不手工选择任一侧，先解决 provider 源，再重新生成。**

最终生成图必须同时具备：

- main `ProvideOpenAIQuotaService`、`ProvideAccountUsageService`、
  `ProvideAccountTestService` 的 Agent Identity WS 注入。
- build `ProvideGrokBillingQuotaService` 和 `NewOpenAICodexResetService`。
- `ProvideAccountHandler` 同时注入 `grokOAuthService` 与
  `openaiCodexResetService`。
- `NewGrokOAuthHandler` 同时注入独立 `GrokBillingQuotaService` 与 main
  `TokenRefreshService` reconciler。
- `ProvideAccountUsageService` 不接收 `GrokQuotaService`，避免恢复账号列表自动
  Billing probe。

生成命令为 `cd backend && go generate ./cmd/server`，随后核对生成 diff。

### 2. `backend/internal/handler/admin/account_handler_available_models_test.go`

**决定：采用合并后的 15 参数构造签名，不选 14 参数的任一侧。**

- `grokOAuthService` 位于 `antigravityOAuthService` 之后。
- `openaiCodexResetService` 位于 `accountTestService` 之后。
- 当前冲突调用及所有自动合并成功的 `NewAccountHandler` 调用都按参数语义补 nil
  或真实 stub，禁止只为编译在末尾追加 nil。

### 3. `backend/internal/handler/admin/grok_oauth_handler.go`

**决定：字段和构造参数取并集，业务入口保持隔离。**

`GrokOAuthHandler` 同时保留：

- build `billingQuotaService *service.GrokBillingQuotaService`，只服务
  `QueryBillingQuota`。
- main `reconciler service.GrokOAuthReconciler`，只服务
  `ReconcileOAuthAccounts`。
- 共享 `grokOAuthService`、`adminService`、`quotaService` 和 `importProber`。

构造函数固定为：

```go
NewGrokOAuthHandler(grokOAuthService, adminService, quotaService, billingQuotaService, reconciler)
```

生产 Wire 传入两者，测试按场景只传所需 stub。补齐中文 Javadoc 的全部参数与
返回值。

### 4. `backend/internal/pkg/apicompat/anthropic_to_responses_response.go`

**决定：失败状态优先，随后采用 main 的 incomplete 语义。**

统一终态规则：

```text
SearchFailed=true  -> failed
StopReason=max_tokens -> incomplete / max_output_tokens
其它 -> completed
```

正常 `message_stop` 和缺失 `message_stop` 的 finalize 使用同一 helper，避免两个
路径再次漂移。保留并补齐 failed、incomplete、completed 三类测试。

### 5. `backend/internal/service/grok_quota_service_test.go`

**决定：保留 build 的 `TestAccountUsageServiceGrokRefreshReadsSnapshotsOnly`，
不采用 main 的账号 usage 自动 `ProbeBilling` 测试。**

理由：build 最新双链路 PRD 明确禁止账号列表自动触发 main Billing；main 的手动
`GrokQuotaService`、OAuth pool 和 reconcile 测试在其它测试中继续保留。该测试需
断言 force 参数也只读取快照/本地 usage，不产生 Billing/Responses 上游请求。

### 6. `backend/internal/service/openai_codex_transform.go`

**决定：保留两个层次的检测，并统一由“客户端可执行图片工具”总判断保护。**

- main `hasCodexImageGenerationFunctionTool`：识别顶层扁平和嵌套 function。
- build `hasOpenAIImageGenClientTool`：识别顶层或 `input.additional_tools` 中的
  可执行 namespace/扁平 function，并组合 main function 检测。
- 空 `image_gen` namespace 没有可执行 function，不算客户端图片工具，应允许
  hosted fallback。
- hosted 注入、`tool_choice` 改写和 bridge instructions 三处都先检查客户端图片
  工具总判断；不得只在其中一处加特判。
- `hasOpenAIImageGenerationTool` 继续作为权限、剥离和图片意图的总能力判断，不用
  它代替 hosted/client 执行域区分。

### 7. `backend/internal/service/openai_codex_transform_test.go`

**决定：保留双方测试语义，消除文本拼接和重复名字。**

- build：空 namespace 获得 hosted fallback、additional_tools namespace 的
  `tool_choice:none` 保持不变、Lite fallback。
- main：扁平/嵌套 `image_gen.imagegen` 不注入 hosted，相似名称仍走 hosted。
- 每类断言独立命名，不能以“已有另一侧类似测试”为由删除边界用例。

### 8. `backend/internal/service/openai_gateway_messages_chat_fallback.go`

**决定：采用 main 新直连桥 API 和状态机，保留 build 的上游传输契约。**

- 请求转换使用 main `AnthropicToChatCompletionsRequest`。
- `applyOpenAICompatModelNormalization` 得到的最终 reasoning effort 写入新
  `chatReq`，再执行 provider 归一化。
- 无论下游 `stream` 值，上游固定 `stream=true` 且
  `stream_options.include_usage=true`；下游非流式由本地折叠。
- 流式转换使用 main `ChatCompletionsToAnthropicStreamState`；客户端断开后继续
  消费和转换上游 chunk 以完成 usage/计费，只停止写出。
- 保留 build 的 custom UA、Beta Fast、xAI request ID、Grok quota snapshot、
  错误脱敏和 failover。
- `buildMessagesRawChatDebugHeaders` 只负责 Header，删除自动合并错位进去的响应
  转换代码。
- main 新桥文件成为唯一状态机。将 build 的稳定 typed content、attribution
  过滤、tool_result 排序、确定性 ID、thinking disabled、空流 frame 和 SSE->JSON
  折叠迁入新实现并保留测试后，删除旧
  `anthropic_chatcompletions.go` 及重复测试。
- `isClaudeCodeAttributionSystemText` 移到新桥或共享 helper，保证
  `anthropic_to_responses.go` 不出现未定义引用。

### 9. `backend/internal/service/openai_image_generation_controls_test.go`

**决定：保留 main 的扁平/嵌套 function 表驱动测试，并保留 build 的 Lite 专项
测试。**

Lite 专项额外锁定：上游不含 Lite header、`tool_choice:none` 不被覆盖、没有
hosted bridge 提示。重叠的 flat function 输入可以共用 fixture，但不能丢失这些
Lite 专属断言。

### 10. `backend/internal/service/openai_ws_forwarder_ingress_session_test.go`

**决定：将冲突的二选一第三轮扩展为独立轮次，覆盖四种输入。**

1. 非 Lite、无图片工具：注入 hosted tool。
2. Lite、只有 exec/collaboration 等非图片工具：注入 hosted tool，但保留 main
   归一化后的 collaboration additional tool 和显式 collaboration tool_choice。
3. Lite、`additional_tools` 中有可执行 image_gen namespace、`tool_choice:none`：
   不注入 hosted，保留 none。
4. 顶层扁平或嵌套 `image_gen.imagegen` function：不注入 hosted，不追加提示。

测试相应扩展 capture events/writes 数量；不把 build namespace 与 main function
场景压成同一个断言。

### 11. `backend/internal/service/wire.go`

**决定：provider 集合取能力并集，`AccountUsageService` 参数采用 build 业务边界。**

- 保留 `ProvideGrokBillingQuotaService`、`NewOpenAICodexResetService`。
- 采用 main `ProvideOpenAIQuotaService`、`ProvideAccountUsageService`、
  `ProvideAccountTestService` 以注入 Agent Identity WS invalidator。
- 从 `ProvideAccountUsageService` 及其 `NewAccountUsageService` 调用删除 main
  `grokQuotaService` 参数；不要修改 `GrokQuotaService` 本身或手动 API。
- 最终以生成后的 `wire_gen.go` 验证无循环依赖和遗漏 provider。

### 12. `frontend/src/components/admin/account/AccountActionMenu.vue`

**决定：emits 和菜单项取并集。**

同时保留 main `duplicate`、build `openai-codex-reset`、既有
`create-spark-shadow`。每个按钮继续受各自 computed 条件控制，点击后先 emit
业务事件再关闭菜单。

### 13. `frontend/src/views/admin/AccountsView.vue`

**决定：同时挂载 Codex reset modal 和账号复制事件。**

- 保留 `<OpenAICodexResetModal>` 及其 state/handler。
- `AccountActionMenu` 同时绑定 `@duplicate`、`@openai-codex-reset` 和
  `@create-spark-shadow`。
- 不合并三类操作的确认状态或 API 调用；各自失败不得清空另一操作的状态。

## Git 未报告的语义冲突

### 新旧桥接重复声明

虚拟合并树同时包含：

- build `backend/internal/pkg/apicompat/anthropic_chatcompletions.go`
- main `backend/internal/pkg/apicompat/chatcompletions_anthropic_bridge.go`

两者重复声明 stream state、constructor、chunk converter 和 finalize。解决方式是
迁移 build 契约到 main 新文件后删除旧文件，不允许通过重命名保留两套状态机。

### Handler 构造参数漂移

合并后的 `NewAccountHandler` 有 15 个参数，但大量测试调用仍是任一侧的 14 个；
`NewGrokOAuthHandler` 最终有 5 个参数，但调用点仍是 4 个。必须全仓扫描调用点并
按参数位置修复，不能等编译错误逐个补末尾 nil。

### Grok 自动 Billing 回接

main `ProvideAccountUsageService` 自动注入 `GrokQuotaService`，而 build 最新
`NewAccountUsageService` 已主动删除该依赖。最终 provider 只保留 Agent Identity
WS 注入，继续遵守独立 Billing 链路。

### 规范路径漂移

现有协议规范仍指向旧 `anthropic_chatcompletions.go` 和旧函数名。实现完成后需把
稳定性契约迁移到 main 新桥路径，并记录强制上游 SSE/本地折叠边界。

### 最终检查确认的协议取舍

- Chat tool delta 到 finalize 仍缺少 function name 时跳过该 pending tool call，
  不输出 `name:""` 的非法 Anthropic `tool_use`。`HasToolCall` 只在合法工具块
  实际开始后置为 true；若上游 `finish_reason=tool_calls` 但没有任何合法工具块，
  最终 `stop_reason` 使用 `end_turn`，避免客户端等待不存在的工具调用。
- token usage 采用 main 的完整拆分：
  `input_tokens=max(prompt_tokens-cached_tokens-cache_creation_tokens, 0)`；
  `cache_read_input_tokens` 和 `cache_creation_input_tokens` 分别返回。`cache_write_tokens`
  与 `cache_creation_tokens` 是同一数量的两种字段名，优先使用前者，不相加。
- main 新桥对未显式指定 thinking 的请求保留默认 `reasoning_effort=medium`；
  Responses Lite 内部标头只用于本地分支判断，测试必须断言其未透传上游。

## 验证重点

- 正向检查：逐条验证 PRD Acceptance Criteria。
- 反向检查：搜索旧桥符号、重复 state、`grokQuotaService` 注入 usage、Lite
  header 白名单、漏改 handler 调用点。
- 自动合并检查：对双方共同修改的 61 个文件复核，重点是 OpenAI gateway、
  Grok usage、handler tests、account frontend 和 Wire。
- 生成检查：只重新生成 Wire；本次 main 未修改 Ent schema，不无条件重生成
  Ent，避免无关生成噪声。
