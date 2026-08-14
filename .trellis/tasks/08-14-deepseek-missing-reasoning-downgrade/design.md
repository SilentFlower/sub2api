# Design — DeepSeek 缺失推理内容自动降级

## 1. Overview

实现分为三个边界：系统设置的持久化与热路径缓存、DeepSeek Chat 上游请求兼容策略、管理端开关。核心协议规则由新的 build 私有领域文件拥有，原生 Chat、Responses 转 Chat 和 Anthropic Messages 转 Chat 的共享入口只传入最终模型、最终请求体和来源标识并接收改写结果。

## 2. Request Flow

```text
原生 /v1/chat/completions
Responses -> Chat fallback
Anthropic Messages -> Chat fallback
  -> 完成各入口现有协议转换、模型映射和请求体策略
  -> DeepSeek missing-reasoning compatibility policy
       1. 读取缓存后的全局开关
       2. 按最终 upstream model 判断 deepseek-*
       3. 扫描 assistant + non-empty tool_calls 历史
       4. 命中时设置 thinking.type=disabled
       5. 删除 reasoning_effort
       6. 按来源链路记录安全结构化日志
  -> 从最终 body 提取 reasoning effort
  -> 发送上游

Responses web-run loop
  -> 每轮根据最新 chatReq 重新序列化 Chat body
  -> 再次执行同一兼容策略
  -> 发送该轮上游请求
```

兼容策略放在各链路 `extractOpenAIUpstreamReasoningEffort` 或实际发送之前，确保 usage 记录反映实际发送体；放在所有现有请求体改写之后，确保检测和改写基于最终上游 payload。Responses web-run 必须在循环内每次发送前执行，因为不完整的 assistant 工具调用历史可能由上一轮上游结果新产生。

## 3. Backend Design

### 3.1 Setting Contract

- setting key：`enable_deepseek_missing_reasoning_auto_downgrade`
- service field：`EnableDeepSeekMissingReasoningAutoDowngrade bool`
- API JSON field：`enable_deepseek_missing_reasoning_auto_downgrade`
- default：`true`

需要同步的中央契约：

- `backend/internal/service/domain_constants.go`
- `backend/internal/service/settings_view.go`
- `backend/internal/service/setting_parse.go`
- `backend/internal/service/setting_update.go`
- `backend/internal/service/setting_service.go`
- `backend/internal/handler/dto/settings.go`
- `backend/internal/handler/admin/setting_handler.go`
- `backend/internal/handler/admin/setting_handler_update.go`
- `backend/internal/handler/admin/setting_handler_audit.go`

新安装在 `InitializeDefaultSettings` 写入 `true`。`parseSettings` 对 key 缺失或空值回退 `true`，保证存量环境升级后无需 migration。

### 3.2 Hot-Path Cache

在新的领域文件中实现 `SettingService` 快速读取：

- 每个 `SettingService` 实例拥有独立 `atomic.Value` 和 `singleflight.Group`，测试不共享全局状态。
- 正常 TTL 60 秒，读取错误 TTL 5 秒，数据库读取超时 5 秒，与现有 Responses Lite 设置模式一致。
- key 缺失、service/repository 为空或读取失败时返回默认 `true`。
- `UpdateSettings` 成功后 forget singleflight，并把本次保存值直接写入缓存，保证立即生效且下一请求不查库。

建议领域公开方法：

```go
func (s *SettingService) IsDeepSeekMissingReasoningAutoDowngradeEnabled(ctx context.Context) bool
```

### 3.3 Compatibility Policy

领域 owner：`backend/internal/service/openai_deepseek_missing_reasoning_policy.go`。

职责：

- 判断最终上游模型是否为 DeepSeek。
- 扫描原始 JSON 中的 message 历史并统计不兼容 assistant 工具调用消息。
- 在开关开启且存在不兼容历史时，用 `sjson` 设置 `thinking.type=disabled` 并删除 `reasoning_effort`。
- 统一接收来源链路标识，返回改写后的 body、是否实际改写、缺失消息数量和错误。
- 在实际改写时集中记录安全结构化日志，避免四个发送点复制日志逻辑。

内部结果建议使用私有结构，避免多个裸 bool/int 参数：

```go
type deepSeekMissingReasoningPolicyResult struct {
	body         []byte
	changed      bool
	missingCount int
}
```

来源链路使用领域内私有类型和稳定值：

- `chat_completions`
- `responses_chat_fallback`
- `responses_web_run`
- `anthropic_chat_fallback`

检测规则直接使用 `gjson` 读取原始 JSON，原因是需要区分字符串、`null`、对象和空值；不依赖 DTO 的 string 零值猜测字段是否可用。只把 trim 后非空的字符串视为有效推理内容。

若 `thinking.type` 已是 `disabled` 且 `reasoning_effort` 不存在，结果 `changed=false`；如果仍有 `reasoning_effort`，只删除冲突字段并返回 `changed=true`。

### 3.4 Gateway Wiring

四个实际发送点只增加薄调用，共享同一策略 owner：

- `backend/internal/service/openai_gateway_chat_completions_raw.go`：在全部现有 body 改写之后、reasoning effort 提取之前处理原生 raw Chat。
- `backend/internal/service/openai_gateway_responses_chat_fallback.go`：在 provider/fast policy 之后、service tier 与 reasoning effort 提取及 web-run 分支之前处理首次 Responses 转 Chat body。
- `backend/internal/service/openai_responses_web_run.go`：在循环内每次 marshal 最新 `chatReq` 后、调用 `sendCCUpstreamRequest` 前再次处理。
- `backend/internal/service/openai_gateway_messages_chat_fallback.go`：在 provider/context/fast policy 之后、reasoning effort 提取和上游发送之前处理 Anthropic 转 Chat body。
- 各调用传入 request context、account、最终 `upstreamModel`、已经完成其它策略处理的 body 和稳定来源标识。
- 策略出错时返回带上下文的错误，不发送上游请求。
- 使用改写后的 body 再调用 `extractOpenAIUpstreamReasoningEffort`。
- `changed=true` 时由共享策略通过 `logger.FromContext(ctx).Info` 记录：
  - `component=openai.deepseek_missing_reasoning_policy`
  - `account_id`
  - `upstream_model`
  - `source_path`
  - `missing_assistant_tool_call_messages`
  - `reason=assistant_tool_calls_missing_reasoning`
- 不记录 messages、reasoning、tool arguments 或 body。

## 4. Admin API And UI

### 4.1 API

系统设置 GET/PUT 沿用现有统一接口：

- GET DTO 返回必填 boolean。
- PUT request 使用 `*bool` 支持局部更新；字段未提交时保留 previous settings。
- 保存成功响应回传最终值。
- `diffSettings` 用 setting key 记录审计变更。

### 4.2 Frontend

领域目录：`frontend/src/features/deepSeekReasoning/`。

- `DeepSeekMissingReasoningDowngradeToggle.vue`：使用 `defineModel<boolean>`，内部复用现有 `Toggle`，只负责标题、说明和双向绑定；说明明确覆盖所有最终转为 DeepSeek Chat 的三类入口。
- `settingsDeepSeekReasoning.ts`：中英文各自独立 locale 扩展，主 `settings.ts` 只在 `gatewayForwarding` 稳定末尾展开。
- `SettingsView.vue`：仅导入组件、绑定 form 字段；表单默认值为 `true`，保存 payload 显式提交该字段。
- `frontend/src/api/admin/settings.ts`：GET 类型为必填 boolean，UPDATE 类型为可选 boolean，字段名保持 snake_case。

开关放在现有 `OpenAIImageGenerationSettings` 和 `ResponsesLiteBlockedModelsSettings` 附近，归入 OpenAI-compatible 转发兼容配置。

## 5. Error And Fallback Behavior

| 场景 | 行为 |
| --- | --- |
| setting key 缺失 | 默认开启并缓存 |
| setting 读取失败 | 记录设置读取 warning，短 TTL 回退开启 |
| 请求体结构化改写失败 | 返回本地错误，不向上游发送半改写请求 |
| UI 字段未提交 | handler 保留旧值 |
| 管理员关闭开关 | 不扫描/不改写请求体，原样交给上游 |
| 非 DeepSeek 模型 | 快速返回，不扫描 messages |

## 6. Testing Strategy

### Backend

- 新领域单测覆盖模型 guard、消息检测、别名、空值/类型、开关关闭、幂等和 body 改写。
- 设置单测覆盖缺失默认 true、显式 false、缓存复用和 UpdateSettings 立即刷新。
- raw Chat gateway 接线测试断言真实上游 body、最终 reasoning effort 和非命中透传。
- Responses 转 Chat fallback 测试覆盖缺失 reasoning 时降级、完整 reasoning item 时保持，以及 web-run 后续轮次新出现缺失历史时降级。
- Anthropic Messages 转 Chat fallback 测试覆盖 assistant `tool_use` 历史触发降级、无工具历史保持和关闭开关透传。
- handler/API contract 测试覆盖 GET、局部 PUT、保存响应和审计 key。
- 保留并运行现有 DeepSeek reasoning_content 请求及流式/非流式响应透传测试。

### Frontend

- 独立 toggle 组件测试验证 label/hint 和 v-model。
- SettingsView 测试验证后端 false 能加载、修改后 payload 提交、缺失值保留默认 true。
- i18n 中英文 key 结构与最终有效文案测试。
- 运行 typecheck、lint 和相关 Vitest。

## 7. Risks And Mitigations

- 风险：自动关闭 thinking 会降低当前请求的推理能力。
  - 缓解：只在 DeepSeek 工具调用历史已无法满足 thinking 协议时触发，并允许管理员全局关闭。
- 风险：模型别名在映射前不是 `deepseek-*`。
  - 缓解：只按最终 `upstreamModel` 判断。
- 风险：热路径配置读取增加数据库压力。
  - 缓解：使用进程内 TTL 缓存和保存后直接刷新。
- 风险：build 私有逻辑继续扩大共享热点冲突。
  - 缓解：协议规则、缓存和 UI 文案放入独立领域 owner，共享文件只做薄接线。
- 风险：只在 Responses 首次转换时检查会漏掉 web-run 循环中新产生的工具调用历史。
  - 缓解：在 web-run 每一轮实际发送前复用同一策略，并用专项测试固定行为。

## 8. Deferred

- 不提供按账号/渠道覆盖。
- 不提供降级次数仪表盘或 usage_logs 新字段。
- 不对 DeepSeek 上游 400 做识别后重试。
- 不改变 Responses/Anthropic converter 的字段映射，也不扩展到直接发送 Responses、Anthropic 或 WebSocket 的路径。
