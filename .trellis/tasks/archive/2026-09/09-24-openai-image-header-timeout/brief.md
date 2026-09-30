# Brief — 为 OpenAI 图片和非流式文本设置独立响应头超时

## Goal

- 让正常的图片生成、图片编辑和非流式文本请求不再被生产环境统一的 20 秒响应头等待时间提前截断。

## Scope

- OpenAI `/v1/images/generations` 与 `/v1/images/edits` 的内置 HTTP 上游请求默认等待响应头 600 秒，包括同步和流式图片请求。
- OpenAI `/v1/responses`（含 compact 兼容路径）与 `/v1/chat/completions` 的非流式文本请求默认等待响应头 300 秒。
- 新增两项可覆盖的部署配置，隔离三类 HTTP 客户端连接池，并补齐配置、传输及入口测试。
- 验证代码后，按现有生产发布方式更新应用容器并核对健康与生效配置；不发起付费图片生成测试。

## Non-Goals

- 不改变其他平台、WebSocket、Embedding、模型目录、账号测试和后台探测的超时策略。
- 不调整响应头之后的数据间隔超时、下游客户端超时或外层代理策略。
- 不修改生产环境现有的 `GATEWAY_OPENAI_RESPONSE_HEADER_TIMEOUT=20`。

## Key Decisions

- 图片端点优先使用 600 秒；文本端点按客户端入站 `stream` 区分，非流式使用 300 秒，流式继续使用原有 OpenAI 超时。
- 只有 OpenAI 平台账号使用新分档；不同分档的客户端缓存键隔离，保留既有 HTTP/2、代理和错误处理行为。
- 新配置允许非负秒数；`0` 表示不限制响应头等待时间。默认值分别为 600/300 秒。

## Key Context

- 请求 `1d01614e-dd29-4f85-83ae-aeabd47c27c0` 的 `/v1/images/edits` 在约 21.5 秒后由本站返回 502，上游未返回状态码；生产容器配置为 20 秒。
- 生产数据库没有启用 OpenAI OAuth 插件绑定，事故请求使用内置 HTTP 上游路径。
- 图片直调、图片 Responses 桥接、文本直转与 passthrough 共享 `doOpenAIUpstream`；传输层的 `ResponseHeaderTimeout` 来自 profile 对应的连接池配置。
- 任务材料：[prd.md](./prd.md)、[design.md](./design.md)、[implement.md](./implement.md)。

## Risks / Deferred

- 未来若启用 OpenAI OAuth 插件，其独立传输不受内置 HTTP 连接池配置控制，需要另行配置。
- 图片最长 600 秒可能与外层代理或客户端等待时间接近；若出现外层超时，后续另行处理。
- 部署时需核对镜像构建提交、保留旧 digest，且只重建应用容器。

## Acceptance

- 两个图片端点使用 600 秒；两个文本端点的非流式请求使用 300 秒，流式请求仍使用现有生产 20 秒。
- 相同账号与代理下三类连接池互不覆盖，HTTP/2 回退、代理路由、错误响应和计费不发生意外变化。
- 聚焦测试通过；生产应用更新后健康，运行配置核对通过。

## Next Step

- 代码、测试和规范更新已提交；同一提交的镜像构建、CI 和安全扫描通过。生产应用已更新且健康，三档运行值为 20/600/300 秒；任务验收已完成。
