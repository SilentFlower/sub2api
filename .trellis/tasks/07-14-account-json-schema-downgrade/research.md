# OpenAI 兼容降级研究记录

## 官方协议事实

- OpenAI Function Calling 文档说明，custom tool 的原生 `input` 是自由字符串；custom grammar 用于约束该字符串，而不是把它变成 JSON object。
- 官方文档：<https://developers.openai.com/api/docs/guides/function-calling#custom-tools>
- 因此，Responses custom tool 经 Chat function 桥接时使用 `input:string` 包装属于协议能力降级；真正丢失的是 custom tool call 通道和 grammar 采样约束。

## Codex `request_permissions` 事实

- 当前 OpenAI Codex 源码把 `request_permissions` 定义为普通 function tool，不是 custom tool。
- 工具定义位置：<https://github.com/openai/codex/blob/main/codex-rs/core/src/tools/handlers/shell_spec.rs>
- 处理器位置：<https://github.com/openai/codex/blob/main/codex-rs/core/src/tools/handlers/request_permissions.rs>
- 参数结构：
  - `reason`：可选字符串。
  - `environment_id`：可选字符串。
  - `permissions`：必填对象。
  - 网络权限使用 `permissions.network.enabled`。
- Codex 测试使用 Responses `function_call` 事件调用 `request_permissions`，不是 `custom_tool_call`。

## sub2api 当前行为

- `ResponsesToChatCompletionsRequest` 会把 Responses function tool 的名称、描述、参数 Schema 和 strict 字段映射到 Chat function tool。
- Chat→Responses 回程只会把上游真实 `tool_calls` 还原为 function/custom/tool_search/namespace 调用。
- DeepSeek 把权限意图作为普通 assistant 文本输出时，当前桥不会合成工具调用，因此 Codex 只能显示原始文本。
- 截图中的实际文本包含说明句和权限标记：
  ```text
  我需要先获取网络权限才能帮您搜索台风最新情况。
  <request_permissions><requests><permission>network</permission></requests></request_permissions>
  ```

## 历史定位

- 相关 custom/tool_search/namespace 桥接 PR：<https://github.com/Wei-Shaw/sub2api/pull/3989>
- 初始 custom bridge 提交：`75fb3c41c272163e02970d23df6c793f1519acf1`。
- `0.1.152` 同步只包含 tool_search 参数反序列化等后续调整，没有改变 `request_permissions` function Schema 透传。

## 设计结论

- JSON Schema 降级与权限调用恢复共享同一任务，但使用独立 helper 和测试。
- `request_permissions` 请求侧保持 function JSON Schema，不转换为 custom 或通用字符串参数。
- 回程只恢复截图已确认的网络权限标记，并要求入站声明同名 function 工具。
- 流式路径只缓冲权限标记候选，不全量缓冲所有正常文本。
- 未知权限、格式不完整、存在非空尾随文本或未声明工具时原样作为 assistant 文本处理。
