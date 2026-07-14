# OpenAI 兼容降级与权限工具桥接

## Goal

在同一任务中解决 OpenAI-compatible API Key 上游的两类 Responses→Chat/Responses 能力降级问题：

1. 提供可选的结构化输出兼容模式。当上游支持 JSON Object、但暂不支持 JSON Schema 时，管理员可按账号开启降级，避免请求因 `json_schema` 返回 `400 invalid_request_error`。
2. 修复 Codex 经 Responses→Chat 路径请求网络权限时，DeepSeek Chat 没有返回结构化 function `tool_call`，而把权限意图作为 `<request_permissions>...</request_permissions>` 普通 assistant 文本输出，导致客户端无法执行工具或弹出权限审批的问题。

默认行为保持原样透传，存量账号不受影响。

## Background

- Responses API 使用 `text.format.type=json_schema`，Chat Completions 使用 `response_format.type=json_schema`。
- 现有协议适配层会在 Responses 与 Chat Completions 之间保留 JSON Schema 语义，因此仅靠端点转换不会规避不支持 `json_schema` 的上游错误。
- 账号创建、编辑、持久化和前端表单已支持 `accounts.extra`，本功能可复用该字段，不需要新增数据库列或迁移。
- 需要覆盖的现有上游路径包括：原生 `/v1/responses`、Chat Completions 转 Responses、Responses 转 raw Chat Completions，以及 raw Chat Completions 直转。
- OpenAI 官方协议中，custom tool 的 `input` 原生就是字符串；将 custom tool 包装成 Chat function 的 `input:string` 不是本次权限问题的直接根因，但当前转换确实无法保留 custom grammar 的采样约束。
- 当前 OpenAI Codex 源码把 `request_permissions` 定义为普通 function tool，而不是 custom tool。其 JSON 参数包含可选 `reason`、可选 `environment_id` 和必填 `permissions`；网络权限的结构为 `permissions.network.enabled`。
- 现有 Responses→Chat 对 function tool 会透传 `name/description/parameters/strict`，因此请求侧必须继续保持 `request_permissions` 的结构化 Schema，不得把它转为通用 `input:string`。
- 截图证明 DeepSeek Chat 会把 `<request_permissions><requests><permission>network</permission></requests></request_permissions>` 作为普通 assistant 文本返回；当前回程只把真实 Chat `tool_calls` 还原为 Responses function calls，因此客户端无法触发权限审批。
- `2026-07-13` 同步的 `0.1.152` 没有修改这条 function 请求转换；本问题更符合第三方 Chat 模型未遵循 function calling、且回程缺少严格兼容恢复，而不是最近一次同步直接回归。
- 原始请求体已经无法取得，因此恢复逻辑必须保守：只有入站确实声明 `request_permissions` function 工具、assistant 输出包含一个完整匹配的已知权限标记、且闭合标记后没有非空白内容时，才能恢复为结构化 function call。标记前允许保留普通说明文字。

## Requirements

- 新增账号级结构化输出兼容配置，存储在 `accounts.extra`；字段缺失或非法值必须按关闭处理。
- 配置关闭时，请求体必须保持现有行为，不得改变 `json_schema`、`json_object` 或普通文本格式。
- 配置开启时，只降级响应输出格式中的 `json_schema`；不得修改 `tools[].parameters`、函数工具 Schema 或其它 JSON Schema。
- Responses 形态应将 `text.format` 从 `json_schema` 改为 `json_object`；Chat Completions 形态应将 `response_format` 从 `json_schema` 改为 `json_object`。
- 降级时必须保留原 JSON Schema，并把它作为只读输出约束追加到 Responses `instructions` 或 Chat Completions `system` 消息中；约束文本必须明确要求只输出 JSON，并说明 Schema 内容仅用于描述输出结构。
- Schema 注入属于尽力兼容：只保证请求切换到 JSON Object 模式，不得宣称仍具备 `strict` JSON Schema 保证。
- 降级逻辑必须按最终选中的账号生效，并覆盖原生 Responses、协议互转和 raw Chat 直转路径。
- 降级发生时记录结构化日志，至少包含账号 ID、入站端点、上游端点和原/目标格式；不得记录完整 Schema、请求体、密钥或凭据。
- 管理端创建和编辑账号时可配置该选项，保存时必须保留 `extra` 中其它配置和运行态字段。

### 权限工具桥接

- Responses→Chat 请求转换必须完整保留 `request_permissions` function 工具的名称、描述、JSON Schema 和 tool choice，不得降级成通用字符串参数。
- Chat→Responses 回程必须继续把真实 Chat `tool_calls` 还原为 Codex 可消费的 `function_call` 事件/输出。
- 当 DeepSeek 没有返回 `tool_calls`、而是返回完整权限标记时，仅允许在请求已声明 `request_permissions` function 工具的前提下，将受支持的标记严格解析为 `request_permissions` function call 参数。
- 首版恢复截图中已确认的网络权限：`permission=network` 映射为 `{"permissions":{"network":{"enabled":true}}}`；未知权限、重复冲突权限或不完整标记不得猜测。
- 修复不得把任意包含类似 XML 的普通 assistant 文本误判为工具调用；标记前允许普通说明文字继续作为 assistant message 输出，闭合标记后只允许空白。
- 现有 `exec`、`apply_patch` 等 custom 工具、普通 function 工具、`tool_search` 和 namespace/MCP 工具行为必须保持。
- 权限工具兼容失败时不得伪造已执行结果；仍应保留可诊断错误或原始响应语义。

## Acceptance Criteria

- [ ] 未配置或关闭兼容模式的账号继续原样发送 `json_schema`，与当前行为一致。
- [ ] 开启兼容模式的账号在 `/v1/responses` 上游请求中发送 `text.format.type=json_object`。
- [ ] 开启兼容模式的账号在 raw `/v1/chat/completions` 上游请求中发送 `response_format.type=json_object`。
- [ ] Chat Completions 转 Responses、Responses 转 raw Chat Completions 两条桥接路径同样执行降级。
- [ ] 降级后的最终上游请求包含原 Schema 的输出约束，并明确要求只输出 JSON；既有 `instructions/system` 内容保持且顺序稳定。
- [ ] 已经是 `json_object`、普通 `text`、格式缺失或格式结构非法时保持原样，不误改其它字段。
- [ ] 工具参数 Schema 保持原样。
- [ ] 创建、编辑账号能正确保存、回显和关闭配置，且不覆盖其它 `extra` 键。
- [ ] 降级日志不包含完整 Schema、请求正文或任何认证信息。
- [ ] 后端协议适配、网关路径和前端账号表单均有针对性测试。
- [ ] Codex 通过 Responses→Chat 路径请求网络权限时，下游收到可执行的 `request_permissions` function call，界面不再展示原始 `<request_permissions>...</request_permissions>` 文本。
- [ ] `request_permissions` 的请求侧 Chat function 保留结构化参数 Schema，不变成 `{input:string}`。
- [ ] 权限工具桥接覆盖流式和非流式上游响应。
- [ ] 未声明 `request_permissions`、权限标记不完整、闭合标记后包含非空白内容、权限值未知或解析失败时，不合成权限工具调用。
- [ ] `exec`、`apply_patch`、普通 function、`tool_search`、namespace/MCP 的现有转换测试继续通过。

## Scope

- 首版严格限制为 `platform=openai && type=apikey` 的 OpenAI-compatible 账号。
- OpenAI OAuth、SetupToken、Grok 及其它平台即使 `extra` 中意外存在同名配置也不得执行降级。
- 不自动探测上游是否支持 JSON Schema。
- 不在上游报错后自动重试，避免隐式重复请求和额外延迟。
- 两项修复在同一 Trellis 任务中规划、实现和检查，但保持独立的 helper、测试场景与回归边界。
- 首版按严格、最小范围恢复已确认的网络权限标记，不实现任意 custom grammar 解析器。
