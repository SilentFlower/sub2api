# Brief — DeepSeek 缺失推理内容自动降级

## Goal

- 在无法修改 hertz/new-api 客户端的情况下，由 Sub2API 在请求最终以 Chat Completions 协议发送到 DeepSeek 上游前识别不完整的工具调用历史，并自动关闭当前请求的 thinking，避免 `reasoning_content` 缺失导致的上游 400。

## Scope

- 新增全局系统设置 `enable_deepseek_missing_reasoning_auto_downgrade`，新安装和存量缺失配置均默认开启，管理员可在“系统设置 -> 网关服务 -> 请求转发行为”关闭。
- 覆盖原生 raw Chat、Responses 转 Chat fallback、Anthropic Messages 转 Chat fallback；Responses web-run 的每次循环续轮发送也执行同一策略。
- 仅当最终上游模型为 `deepseek-*`，且任一 assistant 历史消息包含非空 `tool_calls`、同时缺少可用 `reasoning_content` 与 `reasoning` 时触发。
- 触发后设置顶层 `thinking.type=disabled`，删除顶层 `reasoning_effort`，再发送上游。
- 配置使用 SettingService 进程内缓存和 singleflight，保存后立即刷新；实际改写时记录不含敏感内容且可区分来源链路的结构化日志。
- 同步后端设置读取/局部更新/审计/DTO、前端 API 类型、独立 Toggle 组件、中英文文案及相关测试。

## Non-Goals

- 不修改 hertz/new-api 客户端，不伪造或补写 `reasoning_content`。
- 不新增专用渠道，不强制关闭全部 DeepSeek thinking 请求。
- 不做上游 400 后自动重试，不引入 Redis、数据库 migration 或降级统计面板。
- 不改变 Responses/Anthropic converter 的字段映射，不影响直接发送 Responses/Anthropic 上游、WebSocket、非 DeepSeek 模型或 reasoning 历史完整的请求。

## Key Decisions

- 采用方案 2：在 Sub2API 发送上游前自动检测并降级，而不是增加专用渠道或要求客户端改造。
- 开关默认开启，但允许管理员全局关闭；关闭后请求保持原样并沿用上游原始校验行为。
- 按模型映射后的最终 `upstreamModel` 判断 DeepSeek，避免客户端别名漏判。
- 任一 assistant 工具调用历史缺少有效推理内容就降级；`reasoning_content` 和 `reasoning` 任一 trim 后非空字符串均视为完整。
- 协议规则、缓存和前端 UI 文案使用独立 build 领域 owner，四个 Chat 实际发送点、共享 handler 和 SettingsView 只保留薄接入。
- Responses web-run 必须在循环内每轮发送前检查，因为不完整的 assistant 工具调用历史可能由上一轮上游结果新产生。

## Key Context

- 原生 Chat 入口：`backend/internal/service/openai_gateway_chat_completions_raw.go`。
- Responses fallback 入口：`backend/internal/service/openai_gateway_responses_chat_fallback.go`；web-run 循环发送点位于 `backend/internal/service/openai_responses_web_run.go`。
- Anthropic fallback 入口：`backend/internal/service/openai_gateway_messages_chat_fallback.go`。
- 各入口均在完成既有转换和 body 改写后、reasoning effort 提取或实际发送前调用同一策略；转换器本身不改。
- DTO 已定义 `reasoning_content`、`reasoning`、`tool_calls`、`thinking` 和 `reasoning_effort`；检测仍使用 `gjson` 读取原始 JSON，以正确处理缺失、null、空白和非字符串。
- 系统设置沿用现有键值存储；缺失 key 回退 true，无需 schema 或 migration。
- hertz 故障请求未显式发送 `reasoning_effort`，但 DeepSeek 默认 thinking 仍要求回传生成工具调用时的推理内容。

## Risks / Deferred

- 自动降级会牺牲命中请求的 thinking 能力，但仅在历史已经无法满足 DeepSeek thinking 协议时发生，并提供全局关闭开关。
- 本轮不处理 Anthropic→Chat 转换器丢弃历史 thinking 的现有行为。
- 本轮不提供按账号/渠道覆盖、持久化降级指标或错误后重试。

## Acceptance

- 系统配置缺失时后端实际行为和前端显示均为开启；管理员可保存关闭状态，刷新后保持，审计记录新增 setting key。
- 配置更新后当前进程立即生效，网关热路径不会每请求访问数据库。
- 原生 Chat、Responses 转 Chat、Anthropic Messages 转 Chat 命中的 DeepSeek 请求，上游 body 均包含 `thinking.type=disabled` 且不包含 `reasoning_effort`。
- Responses web-run 首轮未命中、续轮新出现不完整工具调用历史时，续轮仍会自动降级。
- 配置关闭、模型非 DeepSeek、没有 assistant 工具调用或历史推理内容完整时，请求不因本功能改变。
- `reasoning` 别名可阻止误降级；空白、null 和非字符串不能阻止降级。
- 实际改写产生带稳定来源标识的安全结构化日志，现有 DeepSeek reasoning 请求/响应透传测试继续通过。
- 后端相关单测、前端组件测试、typecheck、lint 和 `git diff --check` 通过。

## Next Step

- Check-All 已通过；等待进入规范更新与提交确认流程。
