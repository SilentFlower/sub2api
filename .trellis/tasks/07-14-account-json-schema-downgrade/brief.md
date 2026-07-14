# Brief — OpenAI 兼容降级与权限工具桥接

## Goal

- 在同一任务中完成账号级 `json_schema -> json_object` 兼容降级，并修复 DeepSeek Chat 把 Codex 网络权限 function call 输出成权限标记文本、导致客户端无法审批的问题。

## Scope

- 为 `platform=openai && type=apikey` 账号增加 `extra.openai_json_schema_to_json_object` 开关，默认关闭。
- 覆盖原生 Responses、Chat→Responses、Responses→raw Chat、raw Chat 和 HTTP passthrough 的最终请求格式。
- 降级时把原 Schema 注入 `instructions/system`，明确只保证 JSON Object 和尽力遵循结构。
- 保持 `request_permissions` 为结构化 function tool，完整透传名称、描述、参数 Schema 和 tool choice。
- 当请求声明了 `request_permissions`、DeepSeek 又把单个 network 权限调用输出为已确认的 XML 标记时，严格恢复为 Responses `function_call`。
- 权限恢复覆盖流式与非流式；标记前说明文字保留，成功恢复时不显示原始 XML。
- 在 OpenAI API Key 创建/编辑表单增加开关，并补中英文文案与组件测试。

## Non-Goals

- 不新增数据库列或 migration。
- 不自动探测 JSON Schema 支持，不在上游报错后自动重试。
- 不对 OpenAI OAuth、SetupToken、Grok 或其它平台启用 JSON Schema 降级。
- 不把 `request_permissions` 改成 custom tool 或 `{input:string}`。
- 不实现任意 custom grammar/XML 解析器；首版只恢复截图确认的单个 network 权限标记。
- 不把权限请求视为已经批准或执行，仍由 Codex 客户端完成用户审批。

## Key Context

- 账号配置复用 `accounts.extra`，保存时必须保留探测、额度和其它运行态键。
- OpenAI 官方 custom tool 的原生 input 是字符串，但当前 Codex 的 `request_permissions` 是普通 function tool，参数包含必填 `permissions`，网络权限为 `permissions.network.enabled`。
- JSON Schema 降级 helper 放在协议适配层，网关只负责账号 guard、shape 选择和脱敏日志。
- 权限标记使用 `encoding/xml.Decoder` 严格解析；未声明工具、未知权限、非法层级、不完整标记或闭合标记后存在非空白内容时原样按文本处理。
- 流式状态机只能缓冲可能构成权限标签的最短尾部和已命中的候选，不能让所有正常回答失去流式输出。
- 高风险文件包括 `chatcompletions_responses_bridge.go`、`openai_gateway_forward.go`、`openai_gateway_chat_completions.go` 和账号创建/编辑大组件。

## Acceptance

- 开关关闭或缺失时，所有请求保持当前行为；开启后四条协议路径的最终上游请求使用 `json_object` 并携带原 Schema 约束。
- 已有 JSON Object、普通文本、非法格式和工具参数 Schema 不被误改；日志不包含 Schema、正文或凭据。
- OpenAI API Key 创建/编辑能保存、回显、关闭配置且不覆盖其它 extra；其它账号类型不显示开关。
- `request_permissions` 请求侧保留结构化 function Schema。
- DeepSeek 流式和非流式权限标记能恢复为可执行 function call，原始 XML 不再显示。
- 未声明工具、未知权限、非法标记和尾随普通文本不合成调用。
- function、custom、tool_search、namespace/MCP 现有回归测试继续通过。

## Next Step

- 用户确认 planning artifacts 和本 brief 后，运行 `task.py start`，随后进入 `trellis-route(implement)`，按项目默认的子代理实现路由执行并在完成后进入 check-all。
