# Grok /v1/messages 强制 Chat Completions 技术设计

## 范围

本任务修改 Grok 账号在 Anthropic `/v1/messages` 兼容入口下的转发策略，并在前端开放同一配置项。现有 Responses 默认路径保持不变。

## 后端设计

### 分流规则

新增或复用一个 service 层判断函数，表达“OpenAI 兼容 Messages 入站是否应走 raw Chat Completions”：

- OpenAI APIKey：保持现有 `account.Type == apikey && !openai_compat.ShouldUseResponsesAPI(account.Extra)` 行为。
- Grok：当 `extra.openai_responses_mode == "force_chat_completions"` 时返回 true，不区分 OAuth/APIKey。
- 其它平台：返回 false。

该判断由 `OpenAIGatewayService.ForwardAsAnthropic` 使用，命中时调用现有 `forwardAnthropicViaRawChatCompletions`。

### 上游请求

`forwardAnthropicViaRawChatCompletions` 已包含：

- Anthropic Messages -> Chat Completions 请求转换。
- 上游强制流式并请求 usage。
- 根据客户端 `stream` 偏好输出 Anthropic SSE 或折叠 JSON。
- 通过 `resolveCCFallbackTarget` / `sendCCUpstreamRequest` 发起上游请求。

需要确认 `resolveCCFallbackTarget` 对 Grok OAuth/APIKey 都能使用 `GetAccessToken` 和 xAI `/chat/completions` URL；如果当前只按 OpenAI APIKey 设计，则在该函数或调用链中补齐 Grok 分支，复用 `xai.BuildChatCompletionsURL(account.GetGrokBaseURL())`。

### Usage endpoint

`resolveOpenAIUpstreamEndpoint` 当前只对 OpenAI APIKey raw Chat fallback 返回 `/v1/chat/completions`。本任务需要让 Grok `/v1/messages` 强制 Chat 模式也记录真实 upstream endpoint，避免账单和运维日志显示为 `/responses`。

### 兼容性

- 缺失 `openai_responses_mode`、非法值、`auto`、`force_responses` 均不改变 Grok `/v1/messages` 现有 Responses 路径。
- 字段名继续使用 `openai_responses_mode`，便于与 OpenAI APIKey 配置一致。
- 不改变 `openai_responses_supported` 探测字段语义；Grok 只响应显式 `force_chat_completions`，避免误用探测缓存影响 OAuth 账号。

## 前端设计

### 展示条件

将现有 “Responses API support mode” 配置从 `platform=openai && type=apikey` 扩展为：

- `platform=openai && type=apikey`
- `platform=grok && (type=oauth || type=apikey)`，如果创建页的 Grok OAuth 类型使用 `oauth-based` 分类，则按该分类映射。

### 保存逻辑

保存时在目标账号平台为 OpenAI APIKey 或 Grok 时处理 `extra.openai_responses_mode`：

- `auto`：删除该键。
- `force_responses` / `force_chat_completions`：写入该键。

保存必须基于已有 `extra` 复制后修改单键，避免丢失 `grok_usage_snapshot` 等字段。

### 文案

优先复用现有 `admin.accounts.openai.responsesMode*` 文案；如果文案明显写死 OpenAI 且在 Grok 场景不自然，再最小新增中英文 i18n key。

## 测试策略

- 后端 service 单测：
  - Grok OAuth + force_chat_completions：`/v1/messages` 上游 URL 是 `/chat/completions`。
  - Grok OAuth + auto/缺失：仍走 `/responses`。
  - Grok APIKey + force_chat_completions：走 `/chat/completions`。
  - OpenAI APIKey 原有强制 Chat 行为保持。
- 前端测试：
  - Grok 账号编辑页显示 Responses 模式选择。
  - 保存 Grok 账号时写入 `extra.openai_responses_mode` 且保留既有 `extra`。
- 类型检查：
  - `cd frontend && pnpm typecheck`

## 风险与回滚

- 风险：Grok OAuth 的 `/chat/completions` 与 `/responses` 响应能力不完全一致。通过显式配置规避默认影响。
- 风险：前端保存全量 `extra` 时误删运行态键。实现时必须基于现有 `extra` 合并。
- 回滚：删除强制分流判断和前端展示条件即可恢复现有行为；已写入的 `extra.openai_responses_mode` 在旧代码下对 Grok OAuth 无效。
