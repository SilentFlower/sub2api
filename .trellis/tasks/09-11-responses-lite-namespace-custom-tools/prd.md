# Responses Lite 兼容：chat 桥接 custom 子工具修复 + 原生 Responses 账号级降级开关

## Goal

Codex 以 Responses Lite 形态请求 sub2api 时，无论账号上游是 Chat Completions 还是原生 Responses，都能拿到完整的客户端工具（含 code mode 的 custom `exec`、`apply_patch`）并正确回程。

- chat 回退路径：无条件修复 namespace 内 custom 子工具丢失的缺陷。
- 原生 Responses 路径：新增账号级开关，开启后把 Lite 请求降级为标准 Responses 再走既有客户端工具适配与还原。

## Background / Confirmed Facts

### 用户场景与 Lite 形态
- 用户拓扑：Codex（`gpt-6-astra`）→ new-api（重定向 `deepseek-flash`）→ sub2api（DeepSeek 账号）。Codex 自带目录对 `gpt-6-astra` 为 `use_responses_lite=true`、`tool_mode=code_mode_only`，sub2api 无法影响 Codex 是否发 Lite；用户明确不做目录侧压制。
- Codex `create_tools_json_for_responses_lite`（`codex-rs/tools/src/tool_spec.rs`）把所有 `Function`/`Freeform`(custom) 收进 `{"type":"namespace","name":"functions","tools":[...]}` 放入 `input[].additional_tools`；code mode 执行工具 `exec` 为 custom（`PUBLIC_TOOL_NAME="exec"`）。顶层 `instructions`/`tools` 为空，`reasoning.context=all_turns`，`parallel_tool_calls=false`。
- 上游社区：#6653（open）同场景但维护者按旧形态复现；#6062/#6206（open，CONFLICTING，作者已停止跟进）第 3 项实现同一 chat 桥修复并经真实 Codex Desktop 验证：保留 `functions__` 摊平名，回程 `custom_tool_call` 携带 `namespace:"functions"`。

### chat 回退路径现状（`backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go`）
- `:1081` `namespaceChildrenToChatTools` 仅接受 `function` 子工具；`:218` `NamespaceToolNames` 同样；`:179` `CustomToolNames` 只收顶层 custom。
- `:471` 历史 `custom_tool_call` 不做 namespace 摊平（`:412` `function_call` 分支有）。
- 回程 4 处：非流式 `:1316-1370`、流式宣告 `:1966-2010`、收尾 `:2040-2110`、最终 output `:2155-2190`，均经 `:249` `customToolCallName` 判定。
- 探针实测：Lite 形态下上游 chat tools 丢失 custom 子工具；上游返回 `functions__apply_patch` 时回程为普通 `function_call`。
- 既有 namespace function 子工具按"摊平名 + 回程带 namespace"工作（#5883 步骤 1 证实 Codex 接受）。

### 原生 Responses 路径现状
- 探针实测（DeepSeek `api_protocol=responses`/`adaptive`）：Lite body 原样转发到 `https://api.deepseek.com/responses`，`input[0]` 仍为 `additional_tools`。DeepSeek 文档：`tools` 仅 `function`，`custom` 仅允许 `apply_patch`（其它 400），未知输入项与工具类型静默忽略 → 上游零工具。
- `apicompat.AdaptResponsesClientTools`（`responses_client_tools.go:22`）只处理顶层 `tools`：`FlattenResponsesNamespacesExcept`（`responses_namespace.go:23`）跳过 custom 子工具；custom→function 降级、`:218` `rewriteClientToolHistory`、`:853` `restoreResponsesOutputClientTools`、`:782` `recordItem` 均按顶层 `CustomTools` 判定。`ResponsesNamespaceName = NamespacedToolName`（`responses_namespace.go:13`），两条路径共用映射契约。
- 适配调用点：DeepSeek 原生 `openai_gateway_forward.go:151`（仅 DeepSeek + APIKey + 非 compact）、OpenAI API-key passthrough `openai_gateway_passthrough.go:242`。还原接线：流式 `forward.go:1195`、`passthrough.go:118`；非流式 `openai_gateway_response_handling.go:1629/1729`、`passthrough.go:2334`。Kimi/MiniMax 原生与 OpenAI API-key 托管路径目前不适配。
- `additional_tools` 提升现成实现：`gateway_forward_as_responses.go:229` `liftResponsesAdditionalTools`（service 包内可复用）。
- Lite 头出站：`openai_responses_lite_policy.go:310` `enforceOpenAIResponsesLiteHTTPHeader` 只对 OpenAI 平台按阻止列表决定；非 OpenAI 平台不带。

### 账号级开关既有模式
- 后端：`accounts.extra` 布尔键 + `Account` 访问器（`account.go:2235` `IsOpenAIResponsesFlattenNamespacesEnabled`）；admin 更新走 `UpdateAccountExtra` key 级合并（`account_handler.go:1588`），无 extra 键白名单。
- 前端：`features/openAICompatibility/extra.ts` + Field 组件，接入 `EditAccountModal.vue:1826`、`CreateAccountModal.vue:3280` 的 `canConfigureResponsesMode` 区块；平台判定用 `isCNProviderPlatform`；文案通过 `locales/{zh,en}/admin/accounts*.ts` 扩展文件 spread 进 `accounts.ts`（`:771`）。build 私有目录 `frontend/src/features/responsesLite/` 已存在。

## Requirements

### R1 chat 桥接请求方向：namespace 内 custom 子工具降级为 chat function（无条件）
- `namespaceChildrenToChatTools` 接受 `type=custom`，名称 `flattenNamespaceToolName(ns,name)`，`parameters=customToolInputSchema`，保留 description；冲突检查同样生效。
- `NamespaceToolNames` 记录 custom 子工具，`NamespacedToolName` 新增 `Custom bool`。

### R2 chat 桥接历史方向
- `custom_tool_call` 输入项带 `namespace` 时转 chat tool_call 名为摊平名；无 namespace 行为不变。

### R3 chat 桥接回程方向
- 精确命中 `Custom=true` 条目时，非流式与流式（added / `custom_tool_call_input.delta|done` / done / 最终 output）输出 `custom_tool_call{name:裸名, namespace, input}`。
- 裸名别名：上游返回裸子工具名且该名不是顶层 function/custom、恰好一个 namespace custom 子工具拥有时同样还原；歧义保持 `function_call`。
- 既有行为（顶层 custom、`functions__<顶层custom>` 别名、namespace function、tool_search）不变；`ValidateToolCallArguments` 对 namespace custom 跳过 JSON 校验。
- 覆盖 `forwardResponsesViaRawChatCompletions` 与复用同一映射的 web.run/typed web search 循环。

### R4 原生 Responses 路径共享契约：namespace 内 custom 子工具
- `FlattenResponsesNamespacesExcept` 接受 custom 子工具：降级为 `function`（摊平名、`customToolInputSchema`、删除 `format`），映射 `Custom=true`。
- `rewriteNamespaceQualifiedCall` 对带 namespace 的 `custom_tool_call` 历史项：改 `function_call`、摊平名、`arguments=customToolCallArguments(input)`、删 `input`/`namespace`、`normalizeLoweredFunctionItemID`。
- 还原：`restoreResponsesOutputClientTools`、`restoreClientToolValue`、流式 `recordItem`/事件发射对 `Custom=true` 条目输出 `custom_tool_call{name:裸名, namespace, input}`，item id 按 `custom_tool_call` 重打；`clientToolEventPayload` 既已覆盖 namespace 工具。

### R5 账号级开关"Responses Lite 降级"
- extra 键 `openai_responses_lite_downgrade`（bool，默认关闭）；访问器 `IsOpenAIResponsesLiteDowngradeEnabled()` 仅对 `type=apikey` 且平台为 `openai` 或 CN 供应商（deepseek/kimi/zhipu/minimax）返回真。
- 仅对入站 HTTP Lite 请求（`X-OpenAI-Internal-Codex-Responses-Lite: true`）生效；开启时在 `Forward` 的 Lite 处理段最前（`openai_gateway_forward.go:99` 之前）执行降级：`liftResponsesAdditionalTools` 提升到顶层 `tools` 并删除该输入项；删除 `reasoning.context`；从入站 `c.Request.Header` 删除 Lite 头，使后续 `responsesLite` 判定、ingress policy 与出站头传播都按非 Lite 处理。
- 降级后的原生路径执行客户端工具适配：条件扩展为"开关开启且降级发生"时对 CN 原生（DeepSeek/Kimi/MiniMax）与 OpenAI API-key 托管路径调用 `adaptOpenAIResponsesClientTools`；OpenAI API-key passthrough 沿用 `:242` 既有调用；DeepSeek 原生既有条件不变。
- 开关关闭：所有路径行为与现状一致（chat 桥接修复除外）。
- chat 回退路径开关开启时同样先降级，桥接对两种形态结果一致。

### R6 前端账号开关
- `features/responsesLite/extra.ts`：`applyResponsesLiteDowngradeExtra(source, enabled: boolean | null | undefined)`（true 写入、false/null 删除、undefined 保持）与 `readResponsesLiteDowngrade(extra)`。
- `features/responsesLite/ResponsesLiteDowngradeToggle.vue`：标题、说明、`Toggle`，`data-testid="responses-lite-downgrade"`。
- 接入 `EditAccountModal.vue` 与 `CreateAccountModal.vue` 的 `canConfigureResponsesMode` 区块，`v-if` 为 apikey 且（`openai` 或 `isCNProviderPlatform`）；保存时经 `applyResponsesLiteDowngradeExtra` 写入 extra。
- 文案：`locales/{zh,en}/admin/accountsResponsesLite.ts`，spread 进 `accounts.ts` 的 `openai` 段；中英文成对。

## Acceptance Criteria

- [ ] AC1（R1）：`functions` namespace 含 custom `exec` + function `wait` 时，`ResponsesToChatCompletionsRequest` 输出 `functions__exec`（`customToolInputSchema`）与 `functions__wait`；`NamespaceToolNames` 含 `functions__exec -> {functions, exec, Custom:true}`。
- [ ] AC2（R2）：历史 `custom_tool_call{exec, functions, "pwd"}` 转为 tool_call `functions__exec` / `{"input":"pwd"}`。
- [ ] AC3（R3 非流式）：`functions__exec` 与唯一裸名 `exec` 均还原为 `custom_tool_call{exec, functions, input}`；`functions__wait` 仍为带 namespace 的 `function_call`。
- [ ] AC4（R3 流式）：added/done 为 `custom_tool_call` 且 `namespace:"functions"`，`custom_tool_call_input.done` 带 input，`response.completed.output` 一致，`ValidateToolCallArguments` 不报错。
- [ ] AC5（R3 歧义）：顶层 custom `exec` 与 `functions/exec` 并存各自精确还原；两个 namespace 各含 custom `exec` 时裸名不还原。
- [ ] AC6（R1-R3 服务级）：`Forward` chat 回退 + Lite 头 + 真实 Lite body（开关关闭）：上游 chat tools 含 `functions__exec`、`functions__wait`、`<ns>__<fn>`；上游流式返回三者后客户端 SSE 分别为 `custom_tool_call`(functions)、`function_call`(functions)、`function_call`(原 namespace)。
- [ ] AC7（R4 单元）：`AdaptResponsesClientTools` 对顶层 `functions` namespace 含 custom `exec` 输出 function `functions__exec`（`customToolInputSchema`，无 `format`），映射 `Custom:true`；带 namespace 的历史 `custom_tool_call` 改写为 `function_call{functions__exec, {"input":...}}`；非流式与流式还原输出 `custom_tool_call{exec, functions, input}`，item id 为 `ctc_` 前缀。
- [ ] AC8（R5 开关关闭）：DeepSeek `api_protocol=responses` + Lite 请求，上游 body 与现状一致（`additional_tools` 原样）。
- [ ] AC9（R5 开关开启）：同请求上游 body 无 `additional_tools` 与 `reasoning.context`，顶层 `tools` 含 `functions__exec`（function + input schema）、`functions__wait`、`mcp_demo__lookup`；出站无 Lite 头；上游流式返回 `functions__exec` 调用后客户端收到 `custom_tool_call{exec, functions}`。
- [ ] AC10（R5 OpenAI API-key）：`platform=openai, type=apikey`、`use_responses_api` 托管路径与 passthrough 路径，开关开启 + Lite 时上游 body 同 AC9 且无 Lite 头；开关关闭时行为不变。
- [ ] AC11（R5 访问器）：非 apikey、OAuth、Grok/Anthropic 平台账号访问器恒为 false；缺失/非布尔值为 false。
- [ ] AC12（R6）：`extra.ts` 单测覆盖写入/删除/保持；Toggle 组件渲染与 v-model；Edit/Create 弹窗在 `openai apikey` 与 `deepseek apikey` 显示开关、`openai oauth` 与 `grok` 不显示，保存 payload `extra.openai_responses_lite_downgrade=true`；中英文 key 成对且非空。
- [ ] AC13：`gofmt -l` 为空；`go test -tags=unit ./internal/pkg/apicompat ./internal/service -count=1` 通过；前端 `pnpm test`（相关 spec）与 `pnpm type-check` 通过。

## Out of Scope

- Codex 模型目录/清单侧压 `use_responses_lite=false`；new-api 侧改动。
- WebSocket 入站（WS metadata Lite）的降级。
- BulkEditAccountModal 批量设置该开关。
- `tool_choice` 指向 namespace 内 custom 子工具的转换（Lite 默认 `auto`）。
- Grok、Anthropic 协议账号的 Lite 处理。
- 上游 PR #6062 其余无关改动。
