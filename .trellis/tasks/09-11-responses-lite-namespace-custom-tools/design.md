# Design: Responses Lite 兼容（chat 桥接修复 + 原生降级开关）

## 1. 边界与所有权

| 部分 | 文件 | 所有权说明 |
| --- | --- | --- |
| 共享映射契约 | `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go`（`NamespacedToolName`） | 上游共享；新增 `Custom bool`，零值兼容 |
| chat 桥接 | 同上 | 上游共享；通用协议修复，函数形态对齐上游 PR #6062 第 3 项 |
| 原生路径适配/还原 | `backend/internal/pkg/apicompat/responses_namespace.go`、`responses_client_tools.go` | 上游共享；同样是通用修复 |
| Lite 降级策略（build 私有） | 新建 `backend/internal/service/openai_responses_lite_downgrade.go` | 领域 owner：访问器、降级函数、条件判定、专项测试 |
| 共享入口薄接线 | `openai_gateway_forward.go`、`openai_gateway_passthrough.go` | 只调用一次领域 API 并传播结果 |
| 前端 | `frontend/src/features/responsesLite/`（extra.ts、Toggle.vue、tests）、`locales/{zh,en}/admin/accountsResponsesLite.ts` | build 私有领域目录；弹窗只做绑定 |

不为 chat/原生路径的通用修复另立 owner 文件：它们触及的就是桥接核心函数，拆分需要导出内部状态且不降低冲突面。

## 2. 共享契约

```go
type NamespacedToolName struct {
	Namespace string
	Name      string
	Custom    bool // 子工具声明为 type=custom，回程需还原为 custom_tool_call
}
```

`ResponsesNamespaceName` 是其别名，原生路径自动获得该字段。

## 3. chat 桥接

- 请求：`namespaceChildrenToChatTools` 对 `custom` 子工具生成 `ChatTool{function{name: flat, parameters: customToolInputSchema}}`；`NamespaceToolNames` 写入 `Custom`。
- 历史：`case "custom_tool_call"` 读取 `namespace` 并摊平名字，与 `function_call` 分支对称。
- 回程：新增 `resolveCustomToolCall(name, customTools, functionTools, namespaceTools) (resolvedCustomToolCall{Name, Namespace}, bool)` 替代原 `customToolCallName` / `customNameForStreamTool`，判定顺序：
  1. `functionTools[name]` → 非 custom；
  2. `customTools[name]` → 顶层 custom；
  3. `namespaceTools[name]` 精确命中：`Custom` 真 → `{ns.Name, ns.Namespace}`，否则非 custom；
  4. 既有别名：`flatten(ns, <顶层custom>) == name` 且唯一；
  5. 新增裸名别名：`name` 不是任何摊平名/顶层名，且恰好一个 `Custom` 条目的 `Name == name`。
  流式收尾用 `customIdentityForStreamTool(state, idx, name)` 优先读取宣告时记录的 `toolNamespace[idx]`。四处发射点：非流式补 `Namespace`；`announceChatToolItem` 对 custom 记录 `toolNamespace[idx]`；收尾与最终 output 从 `toolNamespace` 取 namespace、名字用裸名。`responses_stream_event_wire.go` 的 `custom_tool_call` 项序列化同步输出 `namespace`。

## 4. 原生路径共享修复（apicompat）

- `FlattenResponsesNamespacesExcept`：子工具类型允许 `function|custom`；custom 子工具降级为 `{"type":"function","name":flat,"description":...,"parameters":customToolInputSchema}`（删除 `format`），`names[flat].Custom=true`。顶层碰撞表已含 custom 名。
- `rewriteNamespaceQualifiedCall`：新增 `custom_tool_call` 分支：`(namespace,name)` 摊平后命中 `Custom` 条目时改 `function_call`、`name=flat`、`arguments=customToolCallArguments(input)`、删 `input`/`namespace`、`normalizeLoweredFunctionItemID`。
- 还原：
  - `restoreResponsesOutputClientTools` / `restoreClientToolValue`：`NamespaceTools[name].Custom` → `custom_tool_call`，`ID=retypedResponsesToolCallItemID(id,"custom_tool_call")`，`Name=裸名`，`Namespace=ns`，`Input=extractCustomToolCallInput(arguments)`。
  - 流式 `recordItem`：`kind="custom"` 且记录 `restoredName/namespace`；custom 事件发射（added/done/`custom_tool_call_input.*`）写入 `Namespace` 与裸名；`restoreNamespaceEvent` 对 custom 调用不再覆盖。

## 5. Lite 降级开关（service 领域文件 `openai_responses_lite_downgrade.go`）

```go
const accountExtraKeyOpenAIResponsesLiteDowngrade = "openai_responses_lite_downgrade"

// IsOpenAIResponsesLiteDowngradeEnabled 报告账号是否开启 Lite → 标准 Responses 降级。
func (a *Account) IsOpenAIResponsesLiteDowngradeEnabled() bool
// applyOpenAIResponsesLiteDowngrade 在入站为 Lite 且开关开启时提升 additional_tools、
// 删除 reasoning.context 并移除入站 Lite 头。
func applyOpenAIResponsesLiteDowngrade(c *gin.Context, account *Account, body []byte) ([]byte, bool, error)
```

- 访问器：`a.Type == AccountTypeAPIKey && (a.IsOpenAI() || a.IsCNProvider()) && a.Extra[key] == true`。
- 降级：`decodeOpenAIJSONUseNumber` → `liftResponsesAdditionalTools`（复用 `gateway_forward_as_responses.go:229`，同包）→ `delete(reasoning, "context")`（reasoning 空对象时整体删除）→ `marshalOpenAIUpstreamJSON` → `c.Request.Header.Del(responsesLiteHeader)`；未发生提升且无 context 时返回 `changed=false` 但仍删头。
- 接线：`Forward` 在 `openai_gateway_forward.go:99` 之前调用一次；返回 `changed` 存入 gin 上下文键 `openai_responses_lite_downgraded`。原生适配条件在 `:151` 处扩展：

```go
liteDowngraded := openAIResponsesLiteDowngraded(c)
if !compactPath && needsOpenAIResponsesClientToolAdaptation(body) &&
	((nativeDeepSeekResponses && account.Type == AccountTypeAPIKey) ||
		(liteDowngraded && (nativeCNResponses || (account.IsOpenAIApiKey() && !passthroughEnabled)))) {
	...adaptOpenAIResponsesClientTools...
}
```

  passthrough 路径 `:242` 的既有适配只在 body 含 custom/tool_search 时触发；新增一个 else 分支：`openAIResponsesLiteDowngraded(c)` 为真时调用 `adaptOpenAIResponsesLiteDowngradedClientTools`，保证只含 namespace 的降级 body 也被摊平。WS HTTP bridge 不接线。

```go
// adaptOpenAIResponsesLiteDowngradedClientTools 对降级后的标准 Responses 请求无条件执行
// apicompat.AdaptResponsesClientTools（不以 custom/tool_search 出现为前置条件）。
func adaptOpenAIResponsesLiteDowngradedClientTools(body []byte) ([]byte, apicompat.ResponsesClientToolMapping, bool, error)
```
- 出站 Lite 头：入站头已删，`enforceOpenAIResponsesLiteHTTPHeader` 自然不再写入；非 OpenAI 平台本就不带。

## 6. 前端

- `features/responsesLite/extra.ts`：

```ts
export function applyResponsesLiteDowngradeExtra(source, enabled?: boolean | null): Record<string, unknown>
export function readResponsesLiteDowngrade(extra?: Record<string, unknown>): boolean
```

- `ResponsesLiteDowngradeToggle.vue`：`defineModel<boolean>()`，文案 key `admin.accounts.openai.responsesLiteDowngrade` / `...Desc`。
- 弹窗接线：在 `OpenAIJSONSchemaField` 之后放置，`v-if="isApiKeyAccount && (platform === 'openai' || isCNProviderPlatform(platform))"`；Edit 初始化从 `account.extra` 读；提交时 `extra = applyResponsesLiteDowngradeExtra(extra, value)`。
- 文案文件 `accountsResponsesLite.ts` 导出 `{ responsesLiteDowngrade, responsesLiteDowngradeDesc }`，在 `accounts.ts` `openai` 段 spread；中英文各一份。

## 7. 兼容性与风险

- 开关默认关闭，原生路径行为不变；chat 桥接修复对所有账号生效，仅增加原本被丢的工具。
- `NamespacedToolName` 新增字段零值兼容，现有结构体字面量测试不受影响。
- 风险：模型把摊平名再次变形（`functions.exec`）不在本次处理范围；Kimi/MiniMax 原生端点对降级后工具的接受度未实测，仅在开关开启时生效。
- 回滚：后端两个提交（apicompat + service/前端）可独立 revert；开关关闭即可恢复原生路径旧行为。
