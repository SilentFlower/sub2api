# Design: 修复 Codex Responses Lite 生图桥接

## 问题定义

当前实现把一个客户端内部能力提示同时用作本地 bridge 开关和上游协议开关：Lite 标头既关闭 hosted `image_generation` 注入，又被转发给上游。结果是无客户端图片工具的 Lite 请求失去 hosted fallback，而不支持 Lite 的模型还会因标头返回 `unsupported_value`。

修复的核心不是简单删除一个标头，而是拆开三个独立概念：

1. 入站请求是否使用 Lite 工具布局。
2. 请求是否已有 OpenAI 原生 hosted 图片工具。
3. 请求是否已有 Codex 客户端可执行图片工具。

## 核心不变量

- `X-OpenAI-Internal-Codex-Responses-Lite` 是客户端内部提示，不是 sub2api 可以无条件施加给上游模型的能力声明。
- 原生 `image_generation` 与客户端 `image_gen` 是不同执行域，不能用同一个 predicate 驱动全部注入、提示和 `tool_choice` 行为。
- 已有客户端 `image_gen` 时不接管其执行；没有任何图片工具且 hosted bridge 明确启用时，Lite 与非 Lite 使用相同 fallback。
- group 权限、账号 `strip` 策略、bridge 配置、compact、Spark、模型转换和计费优先于 fallback。
- passthrough 只删除不应泄漏的内部标头，不新增 payload 改写。

## 请求头边界

### 普通 HTTP 与 passthrough HTTP

- 从 `openaiAllowedHeaders` 和 `openaiPassthroughAllowedHeaders` 删除 Lite 标头白名单项。
- 入站 Gin header 仍可供当前请求本地判断或测试使用，但构造的 OpenAI 上游请求不得包含该标头。
- 不影响 `Originator`、`X-Codex-Beta-Features`、turn metadata 等现有允许标头。

### WSv2 与 WS HTTP bridge

- WS payload 中已有 `client_metadata` 保持原协议字段，不为本任务删除或重写。
- WS HTTP bridge 不再根据 metadata 执行 `upstreamReq.Header.Set(responsesLiteHeader, "true")`。
- 直接 WS 上游路径不新增 Lite 握手标头。

## 工具分类

实现保持或引入三个语义清晰的判断：

- `hasOpenAINativeImageGenerationTool`：只识别 `type=image_generation`。
- `hasOpenAIImageGenClientTool`：识别顶层和 `additional_tools` 中的 `image_gen` namespace，以及 namespace 扁平化后的 `type=function,name=image_gen.imagegen`。
- `hasOpenAIImageGenerationTool`：图片能力总判断，可组合前两者，继续服务权限和计费意图识别。

具体行为矩阵：

| 请求形态 | hosted bridge | 行为 |
| --- | --- | --- |
| 已有原生 `image_generation` | 任意 | 不重复注入；保留现有参数归一化与明确 `tool_choice` |
| 已有客户端 `image_gen` | 任意 | 不注入、不追加 hosted 提示、不改客户端 `tool_choice` |
| 无图片工具 | 关闭 | 不注入 |
| 无图片工具 | 开启且权限允许 | 注入一个原生工具，补 `tool_choice:auto` 与 hosted 提示 |
| group 禁止或账号 `strip` | 任意 | 保持拒绝或剥离 |
| compact / Spark | 任意 | 不恢复 hosted 注入 |

“保留原生工具”遵守现有兼容契约：旧 `format` / `compression` 字段仍可归一化到上游字段，不要求请求字节完全不变。

## HTTP 数据流

```text
入站 Responses
  -> 本地策略与模型映射
  -> group / account / channel bridge 决策
  -> 分类原生工具与客户端工具
  -> 无工具且 bridge enabled：注入 hosted 工具
  -> namespace：保持客户端执行语义
  -> 构造上游请求时过滤 Lite 标头
  -> 既有响应统计与图片计费
```

HTTP 的 `codexImageGenerationBridgeEnabled` 不再包含 Lite 排除条件。是否实际注入由统一工具分类函数决定，避免 bridge 开关与 payload 能力重复表达。

## WebSocket 数据流

WS 入站与 HTTP 调用同一组工具 helper。`codexBridgeEnabled` 不再因 Lite metadata 整体关闭；无工具时恢复 hosted fallback，已有 namespace 时 helper 自然跳过。WS HTTP bridge 只转换 payload/响应，不把 metadata 升格为上游 HTTP Lite 标头。

## 兼容性与风险

- 风险最高文件是 `openai_gateway_forward.go`：属于 Responses 热路径，不能为所有请求增加无条件 map 解码；只在 bridge 或既有图片条件命中时沿用当前解码分支。
- namespace flatten 可能早于图片判断发生，必须覆盖扁平 function，否则会错误地同时出现客户端和 hosted 工具。
- 删除白名单会影响 passthrough 的“所有允许头直传”预期，但这是明确的安全边界修复；payload 仍保持 passthrough。
- 本任务不保证客户端 `image_gen` 的二次网络调用经过 sub2api，也不改变其计费归属。

## 测试设计

### HTTP

- Lite + bridge enabled + 无工具：注入 hosted 工具、`auto`、提示；上游无 Lite 标头。
- Lite + 原生工具：仅一个原生工具，参数/明确 choice 保持现有契约；上游无 Lite 标头。
- Lite + 顶层 namespace、`additional_tools` namespace、扁平 function：不注入、不提示、不改 `none`。
- group disabled、bridge disabled、account strip、compact、Spark：保持原行为。
- passthrough：payload 不新增 hosted 工具，上游无 Lite 标头。

### WebSocket

- Lite metadata + 无工具：bridge enabled 时注入 hosted 工具。
- Lite metadata + `additional_tools image_gen`：metadata 和客户端工具保留，不注入 hosted 工具。
- WS HTTP bridge：上游 HTTP header 无 Lite，payload metadata 保持。

### 回归

- 图片权限和意图识别。
- 图片模型归一化、主模型/思考预算。
- 流式/非流式图片结果、usage、image count 与计费。
- header whitelist 与 passthrough 相关测试。

## 回滚

- 标头过滤与 hosted fallback 可分别作为回滚点。
- 若恢复 Lite fallback 引发特定上游工具冲突，可只回滚 bridge 条件，同时保留“内部标头不外泄”的安全修复。
- 不涉及数据库、配置迁移或前端数据，回滚不需要数据恢复。
