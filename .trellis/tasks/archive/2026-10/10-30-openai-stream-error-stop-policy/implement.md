# OpenAI 流式停止规则实施计划

## 实施顺序

- [x] I1：最终 Brief 获得确认后，启动任务并执行 `trellis-route(target=implement)`、`trellis-before-dev`，读取相关模型和测试辅助定义。
- [x] I2：以用户提供的结构化错误载荷建立有行为断言的回归用例，确认现有代码会漏停；断言持久化错误状态与运行时调度阻断，而非只核对 mock 调用。
- [x] I3：补齐原生/透传 SSE 的错误入口与共用账号状态处理，识别结构化余额耗尽，沿用现有 502 和错误码筛选语义。
- [x] I4：补充首输出前后、错误与失败连续到达、后续成功、未命中规则及请求级错误的回归用例，核对共用非流式 SSE 转 JSON 路径。
- [x] I5：执行格式检查和针对性测试，再完成 `trellis-route(target=check)` 对应质量检查；根据真实发现修复和复查。

## 验证命令

使用项目要求的 Go 1.27.2；本地已存在对应工具链。

```bash
cd backend
GOTOOLCHAIN=go1.27.2 go version
GOCACHE=/tmp/sub2api-stream-policy-go-cache GOMAXPROCS=4 GOTOOLCHAIN=go1.27.2 GOFLAGS=-mod=readonly go test -p 2 -tags=unit ./internal/service -run '^TestOpenAIStreamErrorPolicy' -count=1
GOCACHE=/tmp/sub2api-stream-policy-go-cache GOMAXPROCS=4 GOTOOLCHAIN=go1.27.2 GOFLAGS=-mod=readonly go test -p 2 -tags=unit ./internal/service -count=1
GOCACHE=/tmp/sub2api-stream-policy-go-cache GOMAXPROCS=4 GOTOOLCHAIN=go1.27.2 GOFLAGS=-mod=readonly go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run --timeout=10m --build-tags=unit --new-from-patch=/tmp/sub2api-stream-policy-lint.patch ./internal/service
```

对修改的 Go 文件执行 `gofmt`；在仓库根目录执行 `git diff --check`。若检查阶段发现需要额外的包级验证或 lint，按实际风险补充，复用已通过且源码未变的证据。

本地初次执行被依赖缓存中的空文件阻断；从经 Go 校验的归档恢复后，`go mod verify` 已通过。使用独立构建缓存，避免旧模块索引继续返回空文件的解析错误；项目依赖版本未改变。

### 已取得的实现验证证据

- 原始源码 overlay：`/tmp/sub2api-stream-policy-baseline/overlay.json`；运行新增测试时余额耗尽等用例失败，确认能够复现漏停。
- 修复后定向测试：`/tmp/sub2api-stream-policy-focused.jsonl`，最终复测于 2026-10-30 00:53:05 +08:00 完成，退出码 0；59 个叶子场景通过，共 66 个测试结果节点。
- 完整服务层单测：`/tmp/sub2api-stream-policy-service.jsonl`，2026-10-30 00:36:58 +08:00 完成，退出码 0；15,664 个叶子场景通过，共 17,098 个通过节点，4 项既有条件测试跳过。后续仅修改本次新增测试的 Close 错误检查和 switch 写法，生产代码未变化；已用最终定向测试补验证，不重复运行完整集合。
- 增量静态检查：使用与 CI 一致的 golangci-lint v2.14.0，补丁包含两个生产文件和未跟踪的新增测试文件；`/tmp/sub2api-stream-policy-lint.log` 最终为 `0 issues.`，退出码 0。初次检查发现的 Close 错误未处理与 QF1003 测试规范问题已按 I5 修复并复查。
- 最终 Go 格式检查与 `git diff --check` 通过，`go.mod` / `go.sum` 无变化。

### Check-All 完成情况

- 路由：个人偏好 `check-all-inline`；深度：`requested=auto`、`effective=full`，原因是账号持久化状态与流式终止时序发生变化。
- 三件套实现、实现假设、完整性与规范三个维度通过；剩余 CHK / FBK 均为 0。
- 已同步研究记录中的验证事实、验收勾选与实施完成状态；未改需求或验收语义。
- 四项既有条件跳过：`TestAuthPendingIdentityService_UpsertAdoptionDecision_ClearsLegacyNullSessionReference`、`TestContentModerationTypeSafeLive`、`TestEstimateOpenAIInputTokens_CompareWithOpenAIAPI`、`TestPluginRuntimeIntegration`；本次定向场景没有跳过项。
- 上线后验证：经单独部署后，复核同类余额耗尽错误触发账号 `error` / `schedulable=false`，并排除后续调度；本轮未执行生产操作。

## 验收对应

- AC1 / R2：余额耗尽 + `[502]`，错误落库与运行时阻断均可观察，另一个账号保持可用。
- AC2 / R1 / R4：原生、透传、输出前后及终止事件时序矩阵。
- AC3 / R3 / R5：未命中列表、最终成功、请求级错误和既有限流/认证行为。
- AC4：测试命令、结果和检查结论写入任务进度，不以未经执行的命令声明通过。

## 评审与回滚

- 实现前：按 `trellis-task-brief` 展示最终 Brief 并等待确认；本轮普通“开始改”不代替该评审门槛。
- 实现后：按 Phase 2.2 检查流程交付结果，再按项目完成链推进规范记录和后续提交。
- 风险文件：`openai_gateway_passthrough.go`、`openai_gateway_response_handling.go`；保持改动集中，避免与本修复无关的格式或协议重构。
- 回滚点：本次代码与测试差异；未经另行授权不提交、推送、发布或部署。
