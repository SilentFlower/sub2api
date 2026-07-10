# 修复 Grok 4.5 effort 归一化

## Goal

为映射到 `grok-4.5` 的请求增加模型专属 reasoning effort 归一化，避免通用客户端发送跨模型档位时被 xAI 上游拒绝；同时修复 Grok 与 GLM 的 usage log 档位漂移，确保现有 `reasoning_effort` 字段等于最终请求体真正发往上游的字段值。

## Background

- 截至 2026-07-10，xAI 官方文档仅允许 `grok-4.5` 使用 `low`、`medium`、`high`，缺省为 `high`，且不能关闭推理。
- Z.AI 官方文档允许 GLM-5.2 使用 `none`、`minimal`、`low`、`medium`、`high`、`xhigh`、`max`，并把 `low/medium` 映射到 `high`、`xhigh` 映射到 `max`。
- `backend/internal/service/openai_gateway_forward.go:38` 会让 Grok Responses 请求在通用 OpenAI 归一化之前提前分流。
- `backend/internal/service/openai_gateway_grok.go:140` 当前只改写模型并删除不支持字段，不改写 `reasoning.effort`。
- `backend/internal/service/openai_gateway_chat_completions_raw.go:76`、`backend/internal/service/openai_gateway_messages.go:396` 和 `backend/internal/service/openai_gateway_messages_chat_fallback.go:78` 会从原始请求或转换前对象保存 effort，后续请求体改写不会同步到 usage log。
- `backend/internal/service/openai_ws_http_bridge.go:191` 的 Grok WebSocket HTTP bridge 复用 `patchGrokResponsesBody`，应共享同一 Responses 归一化规则。
- `backend/internal/service/gateway_request.go:1293` 的 GLM helper 已形成“已知别名归一化、未知值原样透传”的行为基线，但尚未覆盖所有 Chat fallback 入口，且 `none/minimal` 的日志仍可能被折叠或兜底为其它值。
- `frontend/src/utils/format.ts:182` 当前把 `none/minimal` 格式化为 `-`，无法展示实际发往 GLM 上游的字段值。

## Requirements

### R1. Grok 4.5 档位映射

- 只对最终映射模型为 `grok-4.5` 的请求生效；`grok`、`grok-latest` 等别名在模型映射后自然命中。
- `none`、`minimal`、`low` 映射为 `low`。
- `medium` 映射为 `medium`。
- `high`、`xhigh`、`extra high`、`max`、`ultracode` 映射为 `high`。
- 大小写、首尾空白及 `-`、`_`、空格分隔符变体应按现有 GLM 机制识别。

### R2. 与 GLM 一致的归一化机制

- Grok 与 GLM 共用同一种请求体字段定位和改写流程，但分别使用模型官网对应的目标档位。
- 优先读取 `reasoning.effort`，缺失时读取 `reasoning_effort`，并只改写实际存在的路径。
- 完全未知值不静默猜测、不本地吞掉，保持原始请求值交给上游处理。
- 请求未提供 effort 时不新增字段。
- Grok 规则不得影响 `grok-4.20-multi-agent` 等其它 Grok 模型。

### R3. GLM 归一化补全

- 保持既有 `low/medium/high -> high`、`xhigh/extra high/max/ultracode -> max` 语义。
- `none/minimal` 应规范为小写并原样发往上游，不得被 usage fallback 错记为 `high`。
- Responses 转 Chat Completions fallback 等遗漏入口应执行与原生 Chat 路径一致的 GLM 归一化。

### R4. 路径覆盖

- 覆盖 Grok 直接 `/v1/responses`、直接 `/v1/chat/completions`。
- 覆盖 `/v1/messages` 的默认 Responses 与强制 Chat Completions 分支。
- 覆盖 Grok WebSocket HTTP bridge。
- 覆盖 GLM 的原生 Chat、Messages 转 Chat 和 Responses 转 Chat fallback。

### R5. Usage log 最终值契约

- `OpenAIForwardResult.ReasoningEffort` 必须从完成全部 provider-specific 改写后的最终上游请求体提取。
- 持久化 `usage_logs.reasoning_effort` 必须等于最终请求体中实际发送的值。
- 最终请求体没有 effort 字段时保持空值，不根据 Grok `high`、GLM `max` 或 thinking 状态推断默认档位。
- 完全未知值若被上游接受，usage log 应保留实际发送值；上游拒绝时沿用现有错误路径。

### R6. 前端展示

- 继续使用现有 `reasoning_effort` 字段，不新增数据库列或 API 字段。
- `none` 显示为 `None`，`minimal` 显示为 `Minimal`，不得显示为 `-`。
- 其它既有格式化行为保持不变。

### R7. 兼容性

- 不改变 Grok 4.5 现有的不支持字段清理、工具清理、配额快照、错误透传和 failover 语义。
- 不改变 `/v1/messages` 默认 Responses 与强制 Chat 路由各自的缺省 effort 行为，也不互相补齐默认值。
- 不新增归一化变动审计字段，不展示“原始值 -> 最终值”双值。

## Acceptance Criteria

- [ ] Grok 4.5 的已知档位和别名按 R1 映射为 `low/medium/high`。
- [ ] GLM 的已知档位按 R3 映射，`none/minimal` 规范为小写并保持原语义。
- [ ] `banana` 等完全未知值在 Grok 和 GLM 中均保持原样，沿用上游错误语义。
- [ ] 缺失 effort 的请求不新增字段，usage log 保持空值。
- [ ] 非 `grok-4.5` 模型不受 Grok 归一化影响，尤其保留多智能体模型的 `xhigh`。
- [ ] Grok Responses、Chat Completions、Messages 两种分支和 WebSocket HTTP bridge 均使用相同规则。
- [ ] GLM 原生 Chat、Messages 转 Chat、Responses 转 Chat fallback 均使用相同规则。
- [ ] 发生归一化后，最终上游请求体、`OpenAIForwardResult.ReasoningEffort` 和 usage log 三者一致。
- [ ] 前端使用记录把 `none/minimal` 显示为 `None/Minimal`。
- [ ] 不新增 migration、usage log 字段或归一化双值展示。
- [ ] 现有 Grok 字段清理、错误处理和配额快照测试继续通过。

## Out Of Scope

- 不统一 `/v1/messages` 默认 Responses 与强制 Chat 路由的缺省 effort 行为。
- 不记录客户端原始 effort，不展示归一化变化链。
- 不改变 Grok、GLM 之外模型的 provider-specific effort 语义。
