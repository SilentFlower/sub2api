# Brief — 修复 Codex Responses Lite 生图桥接

## Goal

- 阻止 Codex Responses Lite 内部标头泄漏到 OpenAI 上游，并在 Lite 请求没有原生或客户端图片工具且 hosted bridge 已启用时恢复 `image_generation` fallback。

## Scope

- 普通 HTTP、passthrough HTTP 和 WS HTTP bridge 均不向 OpenAI 上游发送 `X-OpenAI-Internal-Codex-Responses-Lite`。
- HTTP 与 WSv2 不再仅因 Lite 形态整体关闭 hosted bridge；继续执行 group、全局/频道/账号、显式 `strip`、compact 与 Spark 门禁。
- 区分原生 `image_generation` 与客户端 `image_gen`，覆盖顶层工具、Responses Lite `additional_tools`、namespace 扁平化后的 `image_gen.imagegen` function。
- 已有原生工具时不重复注入；已有客户端工具时不注入 hosted 工具、不追加 hosted 提示、不改客户端 `tool_choice`。
- Lite 无任何图片工具且 hosted bridge 有效时，注入一个原生工具、必要的 `tool_choice:auto` 与 bridge 提示。
- 补齐 HTTP、passthrough、WSv2、WS HTTP bridge 及权限/策略/模型/计费回归测试。

## Non-Goals

- 不托管、执行、移除或改写客户端 `image_gen`，不承诺其二次网络流量经过 sub2api。
- 不新增“强制 hosted”策略、数据库 migration、前端配置或计费模型。
- 不修改 CPA、独立 Images API、批量图片 API，以及无关 Web Search、JSON Schema 或 namespace 转换。

## Key Context

- 回归由同步 `main 0.1.155` 后的组合行为引入：Lite 同时关闭 hosted bridge 并作为上游 header 透传；不支持 Lite 的模型会返回 `unsupported_value`。
- CPA `v7.2.77` 只用 Lite header/metadata 做本地检测，普通 HTTP 不显式透传；但 CPA 对所有 Lite 一律不注入，本任务只采用其 header 边界，不照搬其 fallback 策略。
- 风险文件集中在 `openai_gateway_forward.go`、`openai_codex_transform.go`、`openai_ws_forwarder_ingress.go`、`openai_gateway_service.go`；必须保持 lazy decode、模型映射、权限与计费顺序。
- WS payload 的 Lite `client_metadata` 可以保留，但 WS HTTP bridge 不得把它升格为上游 HTTP header。
- passthrough 只过滤内部 header，不新增 payload 改写。

## Acceptance

- Lite + 不支持 Lite 的模型 + 无客户端图片工具时，上游无 Lite header，不再因代理透传触发模型拒绝。
- Lite + bridge enabled + 无图片工具时，上游只有一个原生 `image_generation`，choice 为 `auto` 且包含 hosted 提示。
- 原生工具、顶层/`additional_tools` namespace、扁平 function 分别保持正确的防重复与 choice/提示行为。
- group disabled、account strip、bridge disabled、compact、Spark 不会被重新开放。
- 普通 HTTP、passthrough HTTP、WSv2、WS HTTP bridge 均有回归覆盖，现有图片权限、模型转换、usage、计费测试继续通过。
- 定向测试、完整 service 单测、静态检查、格式化和 `git diff --check` 通过。

## Next Step

- 用户确认本 brief 与规划三件套后运行 `task.py start`，随后进入 `trellis-route(implement)`，按路由结果实施，完成后进入 full-scope check。
