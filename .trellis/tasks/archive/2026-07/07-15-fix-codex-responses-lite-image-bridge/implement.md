# Implement Plan: 修复 Codex Responses Lite 生图桥接

## 执行步骤

1. 读取任务上下文与协议规范，确认工作区无计划外修改。
2. 补齐或调整图片工具分类 helper：区分原生 `image_generation` 与客户端 `image_gen`，覆盖顶层、`additional_tools` 和扁平 `image_gen.imagegen` function。
3. 保持 `ensureOpenAIResponsesImageGenerationTool`、`ensureOpenAIResponsesImageGenerationToolChoiceAuto`、`applyCodexImageGenerationBridgeInstructions` 使用同一分类语义：原生工具不重复，客户端工具不接管。
4. 修改 HTTP bridge 决策：移除 Responses Lite 对 `codexImageGenerationBridgeEnabled` 的整体排除，继续保留 group、全局/频道/账号、显式 `strip`、compact 与 Spark 门禁。
5. 修改 WSv2 入站决策：移除 Lite metadata 对 `codexBridgeEnabled` 的整体排除，复用同一工具 helper 决定是否实际注入。
6. 从普通与 passthrough OpenAI 上游 header 白名单删除 Lite 内部标头；删除 WS HTTP bridge 根据 metadata 重建 Lite header 的逻辑。
7. 更新 HTTP 测试矩阵：Lite 无工具恢复 hosted fallback，原生/namespace/function 不重复注入，普通与 passthrough 上游均无 Lite header。
8. 更新 WebSocket 测试矩阵：Lite metadata 无工具时注入，已有 `additional_tools image_gen` 时保持客户端语义，HTTP bridge 不合成 Lite header。
9. 运行定向测试，按失败证据修复，不扩大到无关 Web Search、JSON Schema、Images API 或前端代码。
10. 运行完整后端 service 单测与静态质量检查，复核权限、模型转换、计费和响应统计未回退。
11. 进入 Trellis full-scope check，逐条核对 PRD acceptance、设计矩阵和实际 diff。

## 预计修改文件

- `backend/internal/service/openai_gateway_service.go`
- `backend/internal/service/openai_gateway_forward.go`
- `backend/internal/service/openai_codex_transform.go`
- `backend/internal/service/openai_ws_forwarder_ingress.go`
- `backend/internal/service/openai_ws_http_bridge.go`
- `backend/internal/service/openai_image_generation_controls_test.go`
- `backend/internal/service/openai_codex_transform_test.go`
- `backend/internal/service/openai_ws_forwarder_ingress_session_test.go`
- `backend/internal/service/openai_ws_http_bridge_test.go`
- 仅在现有 helper 测试不能覆盖 raw 意图识别时修改 `image_generation_intent_test.go`。

## 定向验证

```bash
cd backend
go test -tags=unit ./internal/service -run 'TestOpenAIGatewayServiceForward_.*Image|TestOpenAIBuildUpstreamRequestOpenAIPassthrough.*ResponsesLite|TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_.*Image|TestOpenAIWSHTTPBridge.*|TestProxyOpenAIWSHTTPBridge.*' -count=1
go test -tags=unit ./internal/service -run 'TestEnsureOpenAIResponsesImageGeneration|TestApplyCodexImageGeneration|Test.*ImageGenNamespace|Test.*ImageGenerationIntent' -count=1
```

## 完整验证

```bash
cd backend
gofmt -w <本任务修改的 Go 文件>
go test -tags=unit ./internal/service -count=1
go test -tags=unit ./internal/handler -run 'Image|Responses|OpenAI' -count=1
go test -tags=unit ./internal/server/routes -run 'Image|Responses|OpenAI' -count=1
go vet ./internal/service
cd ..
git diff --check
```

如仓库标准质量命令或 Trellis check 指定了更严格的 lint/typecheck，最终检查阶段一并执行。

## 风险文件

- `openai_gateway_forward.go`：OpenAI Responses 热路径，必须保持 lazy decode 与现有模型映射顺序。
- `openai_codex_transform.go`：共享图片工具识别、注入、剥离、choice 和提示逻辑，错误合并 predicate 会造成重复工具或客户端工具失效。
- `openai_ws_forwarder_ingress.go`：WS 长会话需保持 metadata、模型继承、权限和图片计费上下文。
- `openai_gateway_service.go`：header 白名单同时服务普通与 passthrough 构造，删除项必须有双路径测试。

## 回滚点

- 先独立完成并验证 Lite header 过滤。
- 再独立完成并验证 Lite hosted fallback。
- 工具分类 helper 与 HTTP/WS 调用点作为一个原子行为回滚，避免两条传输路径语义不一致。

## Pre-Start Review

- 本任务尊重客户端 `image_gen`，不强制其流量经过 sub2api。
- Lite metadata 可以留在 WS payload，但不能被提升为上游 HTTP header。
- passthrough 只过滤内部 header，不注入 hosted 工具。
- 已有原生工具继续执行现有字段归一化，不承诺字节级原样。
- 不修改数据库、前端配置和独立 Images API。
