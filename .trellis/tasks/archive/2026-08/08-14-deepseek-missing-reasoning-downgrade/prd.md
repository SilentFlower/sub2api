# DeepSeek 缺失推理内容自动降级

## Goal

在无法修改 hertz/new-api 客户端的前提下，由 Sub2API 在请求最终以 Chat Completions 协议发送到 DeepSeek 上游前识别不完整的工具调用历史，并自动关闭当前请求的 thinking，避免 DeepSeek 因历史 assistant 工具调用缺少推理内容而返回 400。该策略统一覆盖原生 Chat、Responses 转 Chat 和 Anthropic Messages 转 Chat。

## Background

- 故障请求最初通过 OpenAI-compatible `/v1/chat/completions` 进入 Sub2API 的 raw Chat Completions 转发路径；同类不完整历史也可能由 Responses 或 Anthropic Messages 请求转换为 Chat Completions 后产生。
- Sub2API 当前存在三类需要统一处理的入口：原生 raw Chat、Responses 转 Chat fallback，以及 Anthropic Messages 转 Chat fallback；Responses 的内部 web-run 还会在循环续轮中重新生成并发送 Chat 请求体。
- 最终上游模型为 `deepseek-*`，客户端历史中存在带 `tool_calls` 的 assistant message，但没有带回生成该工具调用时的 `reasoning_content`，也没有兼容别名 `reasoning`。
- DeepSeek thinking mode 要求后续请求完整回传工具调用轮次的推理内容；缺失时返回：`The reasoning_content in the thinking mode must be passed back to the API.`
- 该 hertz 请求没有显式发送 `reasoning_effort`，但 DeepSeek 当前默认启用 thinking，因此仍会触发校验。
- 参考文档：https://api-docs.deepseek.com/guides/thinking_mode

## Requirements

### 1. 自动降级规则

- 作用于所有最终通过 Chat Completions 上游发送的目标链路：OpenAI-compatible raw Chat、Responses 转 Chat fallback、Anthropic Messages 转 Chat fallback。
- Responses 转 Chat 的内部 web-run 循环每一轮实际发送前也必须执行同一检测，不能只处理首次协议转换结果。
- 不修改 Responses 或 Anthropic 转换器的协议语义；策略统一作用于各链路完成既有转换和改写后的最终 Chat JSON 请求体。
- 必须在模型映射和上游模型归一化完成后，按最终上游模型判断。
- 最终上游模型 trim/lower 后以 `deepseek-` 开头时才进入检测；其它模型和协议保持现状。
- 检查 `messages` 中所有 `role="assistant"` 且 `tool_calls` 为非空数组的历史消息。
- 任一上述消息同时缺少可用的 `reasoning_content` 和 `reasoning` 时，判定当前请求无法安全继续 thinking。
- “可用推理内容”必须是 trim 后非空的字符串；字段缺失、`null`、空字符串、纯空白或非字符串均视为不可用。
- 命中后把顶层 `thinking.type` 设置为 `disabled`，并删除顶层 `reasoning_effort`，再发送给上游。
- 客户端已经显式发送 `thinking.type=disabled` 且不存在 `reasoning_effort` 时，不重复改写、不记录误导性的“已降级”事件。

### 2. 系统配置

- 新增全局系统设置 `enable_deepseek_missing_reasoning_auto_downgrade`。
- 新安装默认持久化为 `true`；存量环境缺少该 key 时也必须按 `true` 解释。
- 管理员可以在“系统设置 -> 网关服务 -> 请求转发行为”关闭该开关。
- 关闭后，即使请求命中缺失推理内容条件，也必须原样保留 thinking 和 `reasoning_effort`，沿用上游原始行为。
- 配置保存后应立即刷新当前进程的热路径缓存，不等待缓存自然过期。

### 3. 性能与可观测性

- 网关请求热路径不得每次直接查询数据库；沿用现有 SettingService 的进程内缓存、singleflight 和短时错误回退模式。
- 配置缺失或读取异常时回退到默认开启；读取异常使用短 TTL，允许后续自动恢复。
- 实际发生请求体改写时记录结构化 info 日志，至少包含组件、账号 ID、最终上游模型、来源链路、缺失推理内容的 assistant 工具调用消息数量和稳定原因码。
- 日志不得记录完整请求体、推理内容、工具参数、密钥或鉴权信息。

### 4. 管理端配置链路

- 后端系统设置读取、局部更新、持久化、审计差异和返回 DTO 必须使用同一个 snake_case 字段名。
- 前端设置 API 类型、表单默认值、加载与保存载荷必须同步该字段。
- UI 使用现有 Toggle 交互，并提供中英文标题和说明；说明应明确默认开启、覆盖原生 Chat、Responses 转 Chat 和 Anthropic Messages 转 Chat 中的 DeepSeek 缺失推理历史请求，以及关闭后可能恢复上游 400。

## Behavior Matrix

| 最终发送链路 | 最终模型 | assistant 工具调用历史 | 系统开关 | 结果 |
| --- | --- | --- | --- | --- |
| raw Chat / Responses 转 Chat / Anthropic 转 Chat | `deepseek-*` | 至少一条缺少/空 `reasoning_content` 与 `reasoning` | 开启 | `thinking.type=disabled`，删除 `reasoning_effort` |
| 任一目标链路 | `deepseek-*` | 每条均有非空 `reasoning_content` 或 `reasoning` | 开启 | 请求保持不变 |
| 任一目标链路 | `deepseek-*` | 缺失推理内容 | 关闭 | 请求保持不变 |
| 任一目标链路 | 非 `deepseek-*` | 任意 | 任意 | 请求保持不变 |
| 任一目标链路 | `deepseek-*` | 没有 assistant `tool_calls` | 开启 | 请求保持不变 |

## Non-Goals

- 不修改 hertz/new-api 客户端，也不伪造、补写或猜测 `reasoning_content`。
- 不新增专用渠道，不按账号或渠道强制关闭所有 DeepSeek thinking 请求。
- 不做上游 400 后自动重试，因为首个失败请求仍会增加延迟和上游流量。
- 不修改 Responses 或 Anthropic Messages 转换器的字段映射，不影响直接发送 Responses/Anthropic 上游、WebSocket 或非 DeepSeek 的协议行为。
- 不新增数据库表、Ent 字段或 migration；继续使用现有系统设置键值存储。
- 不新增 Redis 缓存或持久化的降级统计面板。

## Acceptance Criteria

- [ ] 系统设置缺失时，后端和前端均展示/执行“自动降级已开启”。
- [ ] 管理员可以保存关闭状态，刷新页面后仍为关闭，审计差异包含新增 setting key。
- [ ] 配置更新后当前进程立即使用新值，网关热路径不会每请求访问数据库。
- [ ] 原生 raw Chat、Responses 转 Chat、Anthropic Messages 转 Chat 三类入口在最终模型为 `deepseek-*`，且任一 assistant 非空 `tool_calls` 消息缺少可用 `reasoning_content`/`reasoning` 时，上游请求体均包含 `thinking.type=disabled` 且不包含 `reasoning_effort`。
- [ ] Responses web-run 首轮未命中、后续工具调用续轮才出现不完整 assistant 历史时，续轮发送前仍能触发自动降级。
- [ ] 客户端历史完整、没有 assistant 工具调用、模型不是 DeepSeek 或配置已关闭时，上游请求体不因本功能发生变化。
- [ ] `reasoning` 兼容别名可阻止误降级；空白、`null` 和非字符串不能阻止降级。
- [ ] 实际改写时产生不含敏感内容且可区分来源链路的结构化日志；无需改写时不产生“已降级”日志。
- [ ] 现有 DeepSeek `reasoning_content` 请求/响应透传测试继续通过。
- [ ] 后端相关单元测试、前端组件测试、类型检查、lint 和 `git diff --check` 通过。

## Constraints

- build 私有逻辑必须放入明确的 DeepSeek compatibility 领域文件/前端功能目录；共享 gateway、设置 handler 和 `SettingsView.vue` 只保留薄接入和不可拆分字段。
- 解析和改写使用现有 `gjson`/`sjson` 结构化 API，不使用字符串替换。
- 所有新增公共类型或公共方法必须按项目规范补中文注释，并保持现有字段、JSON tag 和方法签名准确一致。
