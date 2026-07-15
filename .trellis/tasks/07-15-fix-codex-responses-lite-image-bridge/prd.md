# 修复 Codex Responses Lite 生图桥接

## Goal

修复 Codex Responses Lite 请求在 sub2api 中错误透传内部标头、错误关闭 hosted 生图桥接的问题，同时明确区分 OpenAI 上游执行的原生 `image_generation` 与 Codex 客户端执行的 `image_gen` 工具，避免模型拒绝、重复工具和中转链路失控。

## Background

- `build` 在同步 `main 0.1.155` 后引入 Responses Lite 特判：入站携带 `X-OpenAI-Internal-Codex-Responses-Lite: true` 时关闭 hosted `image_generation` 自动注入，并把该内部标头转发给上游。
- 实际上游会对不支持 Lite 的模型返回 `unsupported_value`，错误信息为 `This model is not supported when using X-OpenAI-Internal-Codex-Responses-Lite.`。
- CPA 最新实现只使用 Lite 标头或 WS `client_metadata` 识别本地请求形态；普通 HTTP Codex 上游请求不显式转发该 Lite 标头。
- 原生 `image_generation` 是 OpenAI Responses hosted tool，由上游执行并返回 `image_generation_call`；`image_gen` 是 Codex 客户端 namespace/function 工具，由客户端工具运行时执行，代理端不能仅凭名称保证其底层请求经过 sub2api。
- CPA 在 Lite 请求中不注入 hosted 工具，也不在代理端执行 `image_gen`；它依赖客户端工具执行，或由客户端显式调用独立 `/v1/images/generations` / `/v1/images/edits` 路径。
- 本任务确认尊重客户端工具边界：客户端声明真实 `image_gen` 时不强制改写为 hosted 工具；保证所有客户端工具流量经过中转不属于本次修复。

## Requirements

- Lite 标头只用于 sub2api 本地识别，不得进入任何 OpenAI HTTP 上游请求头，也不得由 WS HTTP bridge 的 `client_metadata` 重新合成为上游 Lite 标头。
- 已有原生 `image_generation` 的请求必须保留其工具配置，不重复注入，不破坏已有明确 `tool_choice`。
- 顶层 `tools` 或 Responses Lite `input.additional_tools` 中确实存在可执行 `image_gen` namespace，或兼容转换后存在扁平 `image_gen.imagegen` function 时，不重复注入 hosted `image_generation`，不追加 hosted bridge 提示，不把客户端工具的 `tool_choice` 从 `none` 改为 `auto`。
- Lite 请求没有任何原生或客户端图片工具，且 Codex hosted bridge 经全局、频道或账号策略启用、用户组允许生图、账号显式策略不是 `strip`、请求不是 compact、模型不是 Spark 时，恢复注入原生 `image_generation`、必要的 `tool_choice: auto` 与 hosted bridge 提示。
- 保留现有用户组图片权限、账户显式工具策略、渠道/全局桥接覆盖、图片模型转换、主模型与思考预算配置、图片计费和响应统计行为。
- HTTP、WSv2 入站和 WS HTTP bridge 的 Lite 判断、工具分类与注入边界必须一致；passthrough 模式仍不得泄漏 Lite 内部标头，但不新增其既有 payload 修改行为。
- 不修改独立 `/v1/images/generations`、`/v1/images/edits` 和批量图片 API 的既有业务契约。

## Acceptance Criteria

- [ ] 入站带 Lite 标头、请求模型不支持 Lite、请求无客户端图片工具时，上游请求头不含 Lite 标头，且不会再触发 `unsupported_value` 模型错误。
- [ ] Lite + hosted bridge enabled + 无任何图片工具时，上游 payload 含且仅含一个原生 `image_generation`，`tool_choice` 为 `auto`，并包含 hosted bridge 提示。
- [ ] Lite 或非 Lite 请求已有原生 `image_generation` 时，不重复注入；原工具参数与明确工具选择保持现有契约。
- [ ] 顶层或 `additional_tools` 已有 `image_gen` namespace，或已有扁平 `image_gen.imagegen` function 时，不注入原生工具、不追加 hosted 提示、不修改客户端工具的 `tool_choice`。
- [ ] group 禁用、账号策略 `strip`、bridge 未启用、compact 或 Spark 场景不会被本修复重新开放 hosted 生图。
- [ ] 普通 HTTP、passthrough HTTP、WSv2 与 WS HTTP bridge 均有标头不泄漏和工具决策回归覆盖。
- [ ] 现有图片权限、模型转换、图片响应统计与计费相关测试继续通过。
- [ ] 后端定向单测、完整 service 单测、lint/格式检查和 `git diff --check` 通过；无法运行的检查需明确记录原因。

## Out Of Scope

- 不实现或托管 Codex 客户端 `image_gen` 工具运行时。
- 不新增“强制 hosted”策略，不移除或改写客户端 `image_gen`，也不承诺客户端工具执行流量经过 sub2api。
- 不改变 CPA 仓库代码。
- 不新增数据库 migration、前端配置项或图片计费模型。
- 不修改与生图无关的 Web Search、JSON Schema 或 Responses namespace 转换。
