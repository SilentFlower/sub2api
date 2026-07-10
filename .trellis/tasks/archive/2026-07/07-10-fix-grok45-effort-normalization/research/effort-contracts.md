# Effort 官方契约与代码路径调研

## 官方资料

核对日期：2026-07-10。

- Z.AI Deep Thinking：<https://docs.z.ai/guides/capabilities/thinking>
- GLM-5.2：<https://docs.z.ai/guides/llm/glm-5.2>
- xAI Reasoning：<https://docs.x.ai/developers/model-capabilities/text/reasoning>
- Grok 4.5：<https://docs.x.ai/developers/models/grok-4.5>

## 官方档位

### GLM-5.2

- 接受：`none`、`minimal`、`low`、`medium`、`high`、`xhigh`、`max`。
- `none/minimal`：跳过思考。
- `low/medium`：实际映射为 `high`。
- `xhigh`：实际映射为 `max`。
- `max`：默认且推荐。

### Grok 4.5

- 接受：`low`、`medium`、`high`。
- 缺省：`high`。
- 不能关闭推理。

## 本任务决策

| 输入语义 | GLM 上游字段 | Grok 4.5 上游字段 |
|---|---|---|
| `none` | `none` | `low` |
| `minimal` | `minimal` | `low` |
| `low` | `high` | `low` |
| `medium` | `high` | `medium` |
| `high` | `high` | `high` |
| `xhigh` / `extra high` | `max` | `high` |
| `max` / `ultracode` | `max` | `high` |
| 未知值 | 原样透传 | 原样透传 |
| 缺失字段 | 不注入 | 不注入 |

usage log 只记录最终上游请求体中的 effort；不保存客户端原始值，不推断厂商默认值，不新增数据库字段。

## 代码证据

- GLM 请求体归一化：`backend/internal/service/gateway_request.go:1293`。
- Grok Responses patch：`backend/internal/service/openai_gateway_grok.go:140`。
- Grok 在通用 OpenAI 流程前分流：`backend/internal/service/openai_gateway_forward.go:38`。
- raw Chat 当前先从原 body 提取日志：`backend/internal/service/openai_gateway_chat_completions_raw.go:76`。
- Messages Responses 当前从转换前 DTO 回填日志：`backend/internal/service/openai_gateway_messages.go:396`。
- Messages Chat 当前在 body 归一化前保存日志值：`backend/internal/service/openai_gateway_messages_chat_fallback.go:78`。
- Responses 转 Chat 当前缺少 GLM helper：`backend/internal/service/openai_gateway_responses_chat_fallback.go:59`。
- Grok WebSocket HTTP bridge 复用 Responses patch：`backend/internal/service/openai_ws_http_bridge.go:191`。
- 前端把 `none/minimal` 隐藏为 `-`：`frontend/src/utils/format.ts:199`。

## 兼容边界

- `/v1/messages` 默认 Responses 会注入 `medium`，强制 Chat 可能省略字段；本任务保留该既有差异。
- `grok-4.20-multi-agent` 支持 `xhigh`，因此 Grok 规则只能精确作用于最终模型 `grok-4.5`。
- 未知值继续交给上游，保持与 GLM 现状和未来官方扩展兼容。
