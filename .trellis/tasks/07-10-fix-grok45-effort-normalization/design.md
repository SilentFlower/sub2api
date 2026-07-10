# Grok 4.5 effort 归一化与日志一致性设计

## Architecture

本任务不新增数据结构，而是在请求转发边界统一完成两件事：

1. 根据最终映射模型，把请求体中的 effort 归一化到对应上游支持的档位。
2. 在所有请求体改写完成后，从最终上游 body 提取 effort，写入 `OpenAIForwardResult.ReasoningEffort`，再沿现有 usage log 链路持久化。

核心原则是“请求体是日志真值源”。不再从客户端原始 body、转换前 DTO、模型默认值或 thinking 状态推导本任务涉及路径的 usage effort。

## Normalization Design

### Shared body rewrite

在 `gateway_request.go` 复用现有 GLM 字段定位逻辑，抽取一个小型共享 helper：

- 优先读取 `reasoning.effort`。
- 嵌套字段缺失时读取 `reasoning_effort`。
- 对原值执行 trim、转小写，并移除 `-`、`_`、空格后交给 provider value mapper。
- mapper 返回空字符串表示“不认识该值”，请求体保持原样。
- mapper 返回已知目标值且与原始字段不同，使用 `sjson` 只改写命中的路径。
- JSON 改写失败时保持原 body 和现有 no-op 返回语义。

保留 `NormalizeGLMOpenAIReasoningEffort` 作为现有入口，并增加统一的 provider 分派入口，使遗漏路径可以按最终上游模型调用同一逻辑。

### Model guards

- GLM：最终模型 trim/lower 后以 `glm-` 开头。
- Grok：最终模型必须精确等于 `grok-4.5`，大小写不敏感。
- Grok 别名先通过现有账号模型映射解析为 `grok-4.5`，归一化 helper 不自行维护别名表。

### Mapping table

| 输入语义 | GLM 目标值 | Grok 4.5 目标值 |
|---|---|---|
| `none` | `none` | `low` |
| `minimal` | `minimal` | `low` |
| `low` | `high` | `low` |
| `medium` | `high` | `medium` |
| `high` | `high` | `high` |
| `xhigh` / `extra high` | `max` | `high` |
| `max` / `ultracode` | `max` | `high` |
| 未知值 | 原样透传 | 原样透传 |

## Final Effort Extraction

新增或收敛一个“最终上游字段提取”helper：

- 从已经完成模型改写、provider effort 归一化和 fast policy 的 body 中读取嵌套或扁平 effort。
- 只 trim，不做档位白名单过滤，不把 `none/minimal` 折叠为空。
- 字段不存在或为空时返回 `nil`。
- 未知值若最终确实发送并被上游接受，日志保留该实际值。

现有 `normalizeOpenAIReasoningEffort` 可继续服务模型后缀、旧 UI 兼容等需要“已知枚举”的场景；最终上游日志不能再依赖它。

## Data Flow

### Grok Responses

`patchGrokResponsesBody` 在设置最终模型后执行 Grok 4.5 effort 归一化，再继续删除不支持字段和工具。`forwardGrokResponses`、Messages Responses 和 WebSocket HTTP bridge 都复用该函数。

### Raw Chat Completions

`forwardAsRawChatCompletions` 和 `forwardAnthropicViaRawChatCompletions` 在最终模型写入后调用 provider 分派归一化；完成 fast policy 后，从最终 `upstreamBody/chatBody` 提取日志 effort。

### Responses to Chat fallback

`forwardResponsesViaRawChatCompletions` 在 Responses DTO 转为 Chat body 后执行 provider 分派归一化，补齐当前遗漏的 GLM 路径；日志同样从最终 Chat body 提取。

### Messages Responses result

`ForwardAsAnthropic` 不再从转换前的 `responsesReq.Reasoning` 回填结果，而从最终 `responsesBody` 提取，确保 Grok `xhigh -> high` 后日志为 `high`。

### WebSocket HTTP bridge

Grok 继续复用 `patchGrokResponsesBody`。结果构造时从已 patch 的 `body` 提取 effort，不再调用 thinking fallback 推断缺省档位。

## Frontend

只修改 `formatReasoningEffort`：

- `none` 返回 `None`。
- `minimal` 返回 `Minimal`。
- `low/medium/high/xhigh/max` 和未知值的现有格式保持不变。

增加独立 utils 单测，避免只在 UsageView 中 mock formatter。

## Compatibility

- 不新增 migration、Ent 字段、repository SQL、DTO 或前端 API 类型。
- `/v1/messages` 两条路继续保留各自缺省 effort：最终 body 发 `medium` 就记 `medium`，未发就记空。
- Grok 4.20 multi-agent 等非 4.5 模型完全跳过新规则。
- 未知值继续交由上游校验，保持与 GLM 当前策略和未来模型扩展兼容。

## Risks And Rollback

- 风险：最终 body 提取时机放错，可能在 fast policy 或协议转换前读取旧值。实现时必须把提取放在最后一次 body 改写之后、发送之前。
- 风险：把 Grok guard 写成前缀匹配会破坏其它 Grok 模型。必须精确匹配最终模型 `grok-4.5`。
- 风险：移除 thinking fallback 的范围过大可能改变其它 provider 日志。本任务只在明确改为“最终 body 真值源”的 Grok/GLM OpenAI-compatible 路径替换该逻辑。
- 回滚：provider 分派 helper、各路径最终 body 提取和前端 formatter 可分别回滚；无数据迁移和持久化兼容负担。
