# Design: OpenAI 按请求类型区分响应头超时

## 边界与数据流

入站 `/v1/images/generations`、`/v1/images/edits` 已使用 `WithOpenAIImagesEndpoint` 标记。`/v1/responses` 和 `/v1/chat/completions` 在 handler 校验后持有客户端 `stream` 值。图片标记优先；文本非流式请求在转发前写入请求上下文标记。上游请求构建可能改写 `stream` 或改走 Responses，因此分类只读取入站语义。

OpenAI 上游 HTTP 请求进入 `doOpenAIUpstream` 后，只有 OpenAI 平台账号根据上下文选择图片、非流式文本或普通 OpenAI profile，再由 `httpUpstreamService` 选择 `ResponseHeaderTimeout`。OAuth 直调图片、Responses 桥接、API Key 图片、文本直转及 passthrough 均经过该入口。插件命中时仍由插件自身传输实现负责超时；生产当前没有启用 OpenAI OAuth 插件绑定，事故命中的 HTTP 上游路径由本任务覆盖。

## 配置与传输契约

- `gateway.openai_images_response_header_timeout` 默认 600 秒；`gateway.openai_nonstream_response_header_timeout` 默认 300 秒；均允许非负值，`0` 表示不限制响应头等待时间。
- 现有 `gateway.openai_response_header_timeout` 保持不变，生产值为 20 秒，作为流式文本及未分类 OpenAI 请求的超时。
- 新 profile 仅改变等待响应头的时间，复用 OpenAI 原有 HTTP/2 协商、连接保活、代理回退、TLS 与错误分类规则；`Do` 和 `DoWithTLS` 的 profile 读取保持一致。
- 客户端缓存键需要包含超时类别。已有池配置键包含实际超时时长，可在配置变化时重建对应连接池；不同类别即使数值相同也保持隔离。
- Compose 环境变量、示例 `.env`、示例 YAML 与配置校验同步新增两项设置。生产镜像使用默认 600/300，不修改当前 20 秒环境变量。

## 兼容性与验证

- 不修改 OpenAI 兼容接口的 URL、请求体、错误体、计费或账号选择。
- 聚焦测试覆盖 API Key 与 OAuth 图片上游请求 profile、文本入站 `stream` 分类、三类传输的实际超时及缓存隔离、HTTP/2 代理回退，以及其他平台不受新规则影响。
- 部署前保留原镜像 digest，核对构建提交与镜像；仅重建应用容器，观察健康、启动日志和运行环境变量。回滚采用原 digest 重建应用容器，不触碰数据库或 Redis。
- 同步图片最大等待 600 秒与外层代理或客户端等待时间可能相近。若后续出现外层超时，应单独调整代理保活或改用异步任务，不在本任务扩大范围。
