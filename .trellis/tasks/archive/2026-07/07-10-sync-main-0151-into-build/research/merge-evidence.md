# 合并证据记录

## 分支状态

- 记录时间：2026-07-10。
- 当前分支：`build`。
- 当前本地 `build`：`939b896b`，与当前本地记录的 `origin/build` 一致。
- 当前本地 `main`：`e316ebf5`，与当前本地记录的 `origin/main` 一致。
- merge base：`9a2f11b4e21763cb7003ea29921d9a672ab50b1f`。
- 独有提交：`build` 82，`main` 31。
- 实施阶段已执行 `git fetch origin main build`；远端未前进，`origin/build=939b896b`、`origin/main=e316ebf5`，分叉、merge base 和冲突列表与规划一致。

## Git 预演

命令：

```bash
git merge-tree --write-tree --messages build main
```

规划阶段虚拟合并树：`ccead42d4d03d0066f8f1ba412a2e8044ab25134`。

fetch 后复核虚拟合并树：`3f9f54873c164213e2662b11d092783a115aa4ab`；冲突文件与自动合并范围未变化。

确认 4 个文本冲突：

1. `backend/internal/service/openai_gateway_grok.go`
2. `backend/internal/service/openai_gateway_grok_test.go`
3. `backend/internal/service/openai_gateway_messages_chat_fallback.go`
4. `backend/internal/service/openai_oauth_passthrough_test.go`

双方共同修改 25 个文件。除上述 4 个文本冲突外，以下 21 个文件由 Git 自动合并，但必须人工复核语义：

1. `backend/internal/handler/dto/settings.go`
2. `backend/internal/pkg/apicompat/types.go`
3. `backend/internal/repository/account_repo.go`
4. `backend/internal/service/account_usage_service.go`
5. `backend/internal/service/openai_codex_transform.go`
6. `backend/internal/service/openai_codex_transform_test.go`
7. `backend/internal/service/openai_gateway_chat_completions_raw.go`
8. `backend/internal/service/openai_gateway_chat_completions_raw_test.go`
9. `backend/internal/service/openai_gateway_forward.go`
10. `backend/internal/service/openai_gateway_messages.go`
11. `backend/internal/service/openai_gateway_request_body.go`
12. `backend/internal/service/openai_gateway_responses_chat_fallback.go`
13. `backend/internal/service/openai_model_mapping_test.go`
14. `backend/internal/service/settings_view.go`
15. `backend/resources/model-pricing/model_prices_and_context_window.json`
16. `frontend/src/api/admin/settings.ts`
17. `frontend/src/components/keys/UseKeyModal.vue`
18. `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`
19. `frontend/src/i18n/locales/en/admin/settings.ts`
20. `frontend/src/i18n/locales/zh/admin/settings.ts`
21. `frontend/src/views/admin/SettingsView.vue`

## 文本冲突分析

### `openai_gateway_grok.go`

- `build`：在 provider 归一化完成后，通过 `extractFinalOpenAIReasoningEffort(patchedBody)` 记录最终实际发送值。
- `main`：通过 `extractOpenAIReasoningEffortFromBody(patchedBody, originalModel)` 记录模型感知值。
- 处理：保留 `build` 的最终请求体语义。Grok 4.5 属于 provider-specific 路径，usage 必须记录归一化后实际发送给上游的值，不能从原始模型重新解释。

### `openai_gateway_grok_test.go`

- `build` 用嵌套字段 `reasoning.effort=xhigh` 验证归一化为 `high`。
- `main` 用扁平字段 `reasoning_effort=high` 验证兼容值被保留。
- 处理：保留两类覆盖，确保嵌套别名归一化和扁平兼容字段都映射到最终 `high`，并断言 `OpenAIForwardResult.ReasoningEffort` 与上游 body 一致。

### `openai_gateway_messages_chat_fallback.go`

- `build` 已将 Anthropic Messages fallback 重构为直接 `Anthropic <-> Chat Completions` 桥接，并在冲突位置处理 custom UA。
- `main` 的冲突块来自旧的 `Chat Completions -> Responses -> Anthropic` 非流式中转路径，Git 将其错误对齐到 header builder。
- 处理：保留 `build` 的 custom UA 与直连桥接；不得把 main 的旧中转代码插入 `buildMessagesRawChatDebugHeaders`。main 对 `ChatCompletionsResponseToResponses` 新签名的适配只保留在仍使用 Responses fallback 的调用点。

### `openai_oauth_passthrough_test.go`

- `build` 新增 GPT-5.6 Sol/Terra/Luna 使用当前统一 Codex 身份的回归测试。
- `main` 新增 codex-tui UA 原样保留、`originator` 按最终 UA 配对的回归测试。
- 处理：两个测试都保留；它们覆盖互补契约，不应二选一。

## 自动合并语义证据

- Codex 身份：`main` 新增 `openai_codex_identity.go` 和 `PairCodexClientIdentity`，负责最终 UA/originator 配对；`build` 的 `openai_codex_client_identity.go` 继续唯一持有 `0.144.1`、CLI UA、probe version 和默认 TUI UA。两者互补，不复制版本常量。
- GPT-5.6 定价：虚拟合并结果同时保留 main 的长上下文阈值 `272000`、输入倍率 `2.0`、输出倍率 `1.5`，以及 build 的 `supports_max_reasoning_effort=true`；三档基础价和 cache creation 价格一致。
- GPT-5.6 OpenCode 配置：虚拟合并结果中 `gpt-5.6`、sol、terra、luna 各出现一次，均保留 `max` variant，没有重复条目。
- reasoning effort：虚拟合并结果仍保持“仅 GPT-5.6 保留 `max`，其它模型折叠为 `xhigh`”以及 OpenAI OAuth compact 的 `max -> xhigh` 例外。
- Fast/Flex 用户范围：DTO、service、middleware context、前端 API、i18n 和设置页形成 `user_ids` 完整链路，同时保留 build 的生图主模型和 reasoning effort 设置字段。
- apicompat：虚拟合并结果同时保留 build 的既有 Chat/Thinking 类型和 main 的 custom tool、tool_search、namespace、cache creation usage 字段。
- 图片生成：main 的 `image_gen` namespace 识别/过滤与 build 的账号级显式工具策略、生图桥接和配置路径自动组合；必须以 HTTP、raw passthrough 和 WebSocket 测试确认。
- Migration：main 新增 `173_allow_cyber_blocked_usage_request_type.sql`，当前 build 最高编号为 172；文件名不冲突。历史重复编号 151/154/158/160 不在本次修改范围，禁止改写。

## 用户确认

- 合并验证通过后创建双父 merge commit。
- 使用普通 push 直接更新 `origin/build`。
- 禁止 force push，不更新其它分支。
