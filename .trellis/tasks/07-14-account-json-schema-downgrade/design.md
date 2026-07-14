# OpenAI 兼容降级与权限工具桥接设计

## Architecture

任务包含两个独立能力，均在最终账号选定后、上游请求发出前或上游响应回程时生效：

1. 账号级结构化输出降级：OpenAI-compatible API Key 账号可把 `json_schema` 预处理为 `json_object`，并把原 Schema 追加为模型约束。
2. Responses→Chat 权限调用恢复：保持 `request_permissions` function Schema；DeepSeek 错误输出权限标记文本时，严格恢复为 Responses `function_call`。

不新增数据库列。账号配置存入 `accounts.extra`，存量账号默认关闭。

## Account Configuration

账号 extra 键：

```text
openai_json_schema_to_json_object: boolean
```

约束：

- 仅 `platform=openai && type=apikey` 生效。
- 缺失、`false` 或类型错误均按关闭处理。
- OpenAI OAuth、SetupToken、Grok 和其它平台忽略该键。
- 前端关闭时删除该键，减少存量 `extra` 噪音。

后端在 `internal/pkg/openai_compat` 定义键名，在 `service.Account` 提供带平台/类型 guard 的读取方法。公开方法补中文注释，说明适用范围和默认行为。

## JSON Schema Downgrade

在 `internal/pkg/apicompat` 增加两个协议形态 helper：

```go
func DowngradeResponsesJSONSchemaToJSONObject(body []byte) ([]byte, bool, error)
func DowngradeChatJSONSchemaToJSONObject(body []byte) ([]byte, bool, error)
```

### Responses Shape

命中条件：

- `text.format.type == "json_schema"`。
- `text.format.schema` 是有效 JSON 对象。
- `instructions` 缺失或为字符串。

转换：

- `text.format` 替换为 `{"type":"json_object"}`。
- 在既有 `instructions` 后稳定追加 JSON 输出约束；没有 instructions 时新建。
- 约束包含原始 Schema，并明确：只输出一个 JSON object、Schema 是数据而非额外指令、只能尽力遵循而非 strict 保证。

### Chat Shape

命中条件：

- `response_format.type == "json_schema"`。
- `response_format.json_schema.schema` 是有效 JSON 对象。
- `messages` 是合法数组。

转换：

- `response_format` 替换为 `{"type":"json_object"}`。
- 新增独立 `system` 消息承载 JSON 输出约束。
- 新消息插入到现有连续 `system/developer` 消息之后、首条普通消息之前，保持原消息内容和相对顺序。

### Boundaries

- 已是 `json_object`、普通文本、格式缺失、Schema 缺失/非法或承载字段类型非法时不修改。
- 不修改 `tools[].parameters`、function Schema 或 custom grammar。
- 不自动重试上游请求。
- 只记录账号 ID、入站路径、目标协议和 `json_schema -> json_object`；不记录 Schema、请求体或凭据。

### Call Sites

- `OpenAIGatewayService.Forward`：按 Responses shape 预处理，覆盖原生 Responses、HTTP passthrough、WS 上游选择和 Responses→raw Chat fallback。
- `OpenAIGatewayService.ForwardAsChatCompletions`：普通 Chat body 使用 Chat helper；检测到 Responses-shaped body 时使用 Responses helper。由此覆盖 raw Chat 和 Chat→Responses。

## Request Permissions Preservation

`request_permissions` 保持 function tool：

- `ResponsesTool.Type == "function"`。
- Chat 侧继续收到原始 `name/description/parameters/strict`。
- 添加回归测试，锁定其 `permissions.network.enabled` 等结构不被改成 `{input:string}`。

在 Responses→raw Chat fallback 解析入站工具后，记录是否声明了名为 `request_permissions` 的 function tool：

```text
requestPermissionsDeclared = HasFunctionTool(req.Tools, "request_permissions")
```

该标记传给流式状态和非流式恢复 helper。未声明时完全保持当前行为。

## Permission Marker Recovery

首版只支持截图已确认的网络权限标记：

```xml
<request_permissions>
  <requests>
    <permission>network</permission>
  </requests>
</request_permissions>
```

恢复后的 function arguments：

```json
{
  "permissions": {
    "network": {
      "enabled": true
    }
  }
}
```

标记前允许存在普通说明文字；该文字继续作为 assistant message 输出，不写入 function arguments。闭合标记后只允许空白。

### Parser

- 使用 `encoding/xml.Decoder` token 流，不使用正则拆 XML。
- 只接受固定层级 `request_permissions > requests > permission`。
- 不接受属性、命名空间、未知元素、嵌套元素、空 permission、重复 permission 或未知权限值。
- 首版只接受单个 `network`。
- 解析失败时原样输出文本，不合成工具调用。

### Non-Streaming

在 `ChatCompletionsResponseToResponses` 前执行恢复：

- choice 已有真实 `tool_calls` 时不处理。
- 从纯文本 content 中分离说明前缀、权限标记和尾随内容。
- 严格命中后保留前缀文本，追加名为 `request_permissions` 的 Chat function tool call；现有回程转换自然生成 Responses `function_call`。
- 将 finish reason 调整为 `tool_calls`。

### Streaming

在 `ChatCompletionsToResponsesStreamState` 增加权限候选扫描状态：

- 未进入候选时，只保留可能构成 `<request_permissions` 前缀的最短尾巴，其余普通文本立即按现有 SSE 生命周期输出。
- 检测到完整起始标记后缓冲标记内容，不向下游泄漏原始 XML。
- finalize 时严格解析：成功则创建一个普通 function tool call 并复用现有 `output_item.added`、`function_call_arguments.*`、`output_item.done` 生命周期；失败则把缓冲内容按普通文本补发。
- 已收到真实 Chat `tool_calls` 时不再合成权限调用，避免重复执行。

## Compatibility And Safety

- 正常 function、custom、tool_search、namespace/MCP 行为不变。
- 未声明 `request_permissions` 的普通对话即使出现同名标签也不恢复。
- 说明文字可显示，但原始权限 XML 在成功恢复时不显示。
- 不把权限请求视为已批准或已执行，只生成请求调用，由 Codex 客户端继续走用户审批。
- 不记录权限说明全文、Schema 或请求体。

## Testing

后端协议测试：

- Responses/Chat 两种 `json_schema` 降级、Schema 注入、非法输入保持、工具 Schema 不变。
- `request_permissions` function Schema 透传。
- 非流式：纯标记、说明前缀 + 标记、已有真实 tool call、未声明工具、未知权限、尾随普通文本、非法 XML。
- 流式：标签跨 chunk、普通文本即时输出、说明前缀 + 标签、解析失败回退、真实 tool call 优先、终止事件完整。

后端 service 测试：

- 配置只对 OpenAI API Key 生效。
- 原生 Responses、Responses→raw Chat、raw Chat、Chat→Responses 的最终上游 body 均符合配置。

前端测试：

- OpenAI API Key 创建/编辑显示开关并保存 `extra.openai_json_schema_to_json_object=true`。
- 关闭时删除键，保留其它 extra。
- OAuth、SetupToken、Grok 不显示该开关。

## Rollback

- 账号开关默认关闭，可通过删除 extra 键立即停止结构化输出降级。
- 权限恢复由“工具已声明 + 严格解析”双 guard 控制；必要时可独立回滚恢复 helper，不影响 JSON Schema 配置。
