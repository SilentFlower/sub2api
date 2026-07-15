# CPA Responses Lite 生图桥接对照

## 研究范围

- 对照仓库：`/root/project/CLIProxyAPI`
- 拉取后版本：`c8803713`，tag `v7.2.77`
- 相关历史：`631f7a65`，PR `#4192`，分支 `codex/fix-sol-responses-lite-image-tool`
- 目标：确认 CPA 对 Lite 标头、hosted `image_generation`、客户端 `image_gen` 与独立 Images API 的真实处理，而不是只依据提交标题推断。

## 已确认事实

1. CPA 在 `ensureImageGenerationTool` 中同时读取 HTTP Lite 标头与 WS `client_metadata.ws_request_header_x_openai_internal_codex_responses_lite`；命中后直接返回原请求体，不注入 hosted `image_generation`。
2. CPA 的普通 HTTP Codex 上游标头构造没有显式转发 `X-OpenAI-Internal-Codex-Responses-Lite`；生产代码中该标头只用于本地检测。
3. CPA 对非 Lite 请求自动注入 hosted `image_generation`，但 Spark、Free Plan、已有原生工具、已有 `image_gen` namespace 或扁平 `image_gen.imagegen` function 时跳过。
4. CPA 代理端不执行客户端 `image_gen`。该 namespace/function 由 Codex 客户端工具运行时执行，底层是否经过中转不由 namespace 声明保证。
5. CPA 另有 `/v1/images/generations` 和 `/v1/images/edits` 路径；这些 handler 会构造带 `tool_choice:{"type":"image_generation"}` 的 Responses 请求，因此是代理可控的图片链路。
6. CPA 定向测试明确锁定 Lite 请求不注入、WS metadata 保留、非 Lite 无工具时注入、namespace/function 不重复注入；本地执行这些定向测试全部通过。

## 与 sub2api 当前实现的差异

- sub2api 把 Lite 标头加入普通和 passthrough 上游白名单，并在 WS HTTP bridge 中把 Lite metadata 重新合成为上游 HTTP 标头；这会让不支持 Lite 的模型返回 `unsupported_value`。
- sub2api 的 HTTP 与 WS bridge 开关直接排除 Lite 请求，导致 Lite 且没有客户端图片工具时丢失原有 hosted fallback。
- sub2api 已有 namespace 防重复注入、`tool_choice` 保护、group 权限、账号策略、模型转换与计费能力；修复应组合这些既有能力，不复制 CPA 的全局配置模型。
- sub2api 当前客户端工具 predicate 重点覆盖 namespace；需要确认 namespace 扁平化后 `image_gen.imagegen` function 仍被识别，避免兼容转换后误注入 hosted 工具。

## 采用结论

- 采用 CPA 的标头边界：Lite 内部标头不进入 OpenAI HTTP 上游。
- 不采用 CPA 的“所有 Lite 一律不注入”：sub2api 在没有任何图片工具且 hosted bridge 已明确启用时恢复原生工具注入。
- 尊重客户端 `image_gen`：不静默替换、不追加 hosted 提示、不改客户端工具选择。
- 保留独立 Images API、权限、策略、模型转换和计费契约。

## 验证证据

CPA 定向命令：

```bash
go test ./internal/runtime/executor -run 'Test(CodexExecutorExecuteResponsesLiteHeaderDoesNotInjectImageGenerationTool|EnsureImageGenerationTool_ResponsesLite.*|EnsureImageGenerationTool_ImageGenNamespaceDoesNotInjectTool|EnsureImageGenerationTool_NoTools)' -count=1 -v
```

结果：全部通过。
