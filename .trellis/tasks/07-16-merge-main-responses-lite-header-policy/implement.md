# Implement Plan: 合并 main 并配置 Responses Lite Header 模型策略

## 1. 合并现场审计

1. 记录 `HEAD=934982ae`、`MERGE_HEAD=b960ec19`、备份分支和 `git ls-files -u` 的 12 个文件。
2. 对每个冲突重新比较 stage 2、stage 3 和当前工作区；当前已移除 marker 的内容不得直接 `git add`。
3. 检查 `git status` 中非冲突自动合并文件，重点复核 `AccountUsageService`、Grok 构造器、OpenAI Header 白名单和设置 DTO 链路。

## 2. 解决业务源码冲突

1. 合并 `image_generation_intent.go`：保留严格客户端 `image_gen` 识别和 Grok 平台专用显式意图，恢复 `responsesLiteHeaderKey`。
2. 合并 `openai_gateway_service.go`：保留 `UpstreamTerminalEvent`/`SucceededForScheduling`、Web Search/图片/视频字段和 Header 候选项。
3. 合并 `openai_ws_http_bridge.go`：保留统一 reasoning effort 与 WS 终态，暂不直接沿用无条件 Header 重建。
4. 合并 Grok service/handler 测试和 `service/wire.go`，同时保留 `cfg`、usage log、独立 billing quota、Codex reset、upstream billing probe。
5. 合并前端创建/编辑账号弹窗，修复冲突现场出现的 tab/space 漂移，保留 extra 的复制更新语义。
6. 合并图片与 Lite/WS 测试文件，先保留双方场景，再按新 Header 策略调整断言。

## 3. 补齐自动合并的跨文件依赖

1. 从 `AccountUsageService` 移除 `grokQuotaService` 字段、构造器参数和主动 `ProbeBilling`；账号列表只读既有快照，独立 `GrokBillingQuotaService` 继续服务 `/billing-quota` 管理端套餐额度接口。
2. 搜索所有 `NewGrokQuotaService`、`NewGrokOAuthHandler`、`NewAccountUsageService` 调用和测试 stub，统一到最终签名。
3. 核对 `OpenAIForwardResult` 所有构造点，确保新增终态字段不丢失 build 的 reasoning/Web Search/图片字段。

## 4. 实现系统设置链路

1. 在 `domain_constants.go` 增加 setting key 和三个精确默认模型常量/只读默认构造入口。
2. 在 `SystemSettings`、handler DTO、`UpdateSettingsRequest`、Get/Update response mapping 增加 `openai_responses_lite_header_blocked_models`。
3. 实现后端归一化：trim、空项拒绝、稳定去重、仅末尾单个 `*`。
4. 设置键缺失时使用默认列表；合法 `[]` 保持为空；非法存储值记录 warning 并回退默认。
5. 更新请求字段使用 `*[]string`：nil 保留旧值，非 nil 包括空数组均整体覆盖。
6. JSON marshal 后写入 settings repository；保存成功刷新专用缓存。
7. 在 `SettingService` 增加 60 秒 TTL + singleflight + 5 秒错误 TTL 的专用缓存，并提供最终模型匹配方法。

## 5. 实现 HTTP 策略

1. 复用账号映射、compact 映射和 OAuth 模型归一化得到本次最终上游模型，不从原始客户端模型直接判断。
2. managed HTTP：
   - 非阻止模型运行 Lite 工具/`all_turns` normalizer并透传 Header。
   - 阻止模型跳过 Lite normalizer，并在上游边界删除 Header。
3. passthrough HTTP：在 compact/OAuth passthrough body 归一化后按 body 最终模型判断；同样覆盖 allow/block。
4. 在 request builder 终态再次检查最终 body model，防止图片主模型或后续 normalize 改写导致策略陈旧。
5. Grok 和其它非 OpenAI 平台继续禁止该内部 Header。

## 6. 实现 WebSocket 策略

1. 调整 ingress `response.create` 顺序：先取得当前 turn 原始/继承模型并完成映射，再判断 Lite 策略。
2. allow：保留 WS metadata并运行 Lite 工具/`all_turns` normalizer。
3. block：删除 metadata键、必要时清理空 `client_metadata`，不运行 Lite normalizer。
4. 每轮重新计算，覆盖同一连接内模型切换。
5. WS HTTP bridge 只在 allow 决策下从 metadata 设置 HTTP Header；内部按最终 mapped model再做防御性检查。
6. 保留 WS 终态调度、replay input、reasoning effort、图片统计、rate limit 和 failover 行为。

## 7. 前端设置页

1. 更新 `frontend/src/api/admin/settings.ts` 的查询/更新类型。
2. 在 `SettingsView.vue` Gateway Forwarding/OpenAI 区域增加模型规则列表编辑器。
3. 加载时复制服务端数组；保存时 trim、稳定去重，空项或非法 `*` 阻止提交；允许整个列表为空。
4. 增加中英文 label、hint、placeholder、add/remove 和 validation 文案。
5. 更新 `SettingsView.spec.ts`：默认加载、添加/删除、显式空数组、空项/通配符校验、保存 payload。

## 8. 测试调整

### 后端设置

- 缺失键返回三个默认项。
- 显式 `[]` 不回退默认。
- 合法精确/通配符、trim、稳定去重。
- 空项、中间 `*`、多个 `*` 被拒绝。
- cache hit 不重复查库；过期 singleflight 合并并发；保存后立即刷新。

### HTTP

- `gpt-5.6-terra` managed/passthrough：Header 保留，`reasoning.context=all_turns`，Lite 工具布局保留。
- 默认 `gpt-5.4-mini`/`gpt-5.5`：Header 删除，不强制新增/覆盖 context，客户端 body 字段保持。
- 模型映射从客户端别名映射到阻止/允许模型时，以映射后结果为准。
- failover 到映射不同的账号时，每次 attempt 重新判断。
- Grok 不收到 Header。

### WebSocket

- 直连 allow/block metadata 与 context。
- 同一会话 `gpt-5.6-terra -> gpt-5.5 -> gpt-5.6-terra` 每轮策略变化正确。
- WS HTTP bridge allow 时合成 Header，block 时不合成。
- bridge direct/HTTP 两条路径均使用最终映射模型。

### 生图与合并回归

- 客户端 namespace/扁平 `image_gen` 不重复 hosted 注入。
- 无客户端图片工具时 hosted fallback 继续服从原门禁。
- Grok 被动图片声明不误判。
- Images API 后台主模型/effort和 `tool_usage` 数值解析测试都保留。
- Grok quota、独立 billing quota、upstream billing probe、WS 终态调度、账号创建/编辑 extra 保留。

## 9. 生成与格式化

1. 对实际修改 Go 文件运行 `gofmt`。
2. 前端仅对实际修改文件使用项目 formatter；不运行会改动全仓的自动修复命令。
3. 依赖图稳定后执行：

```bash
cd backend && go generate ./cmd/server
```

4. 确认生成的 `wire_gen.go` 不含手工冲突内容，并通过 `wire_gen_test.go`。

## 10. 验证命令

### 合并完整性

```bash
git ls-files -u
rg -n '^(<<<<<<<|=======|>>>>>>>)' backend frontend .trellis/tasks/07-16-merge-main-responses-lite-header-policy
git diff --check
```

### 后端定向

```bash
cd backend && go test -tags=unit ./internal/service -run 'Test.*(ResponsesLite|ImageGen|ImageGeneration|OpenAIWSHTTPBridge|ProxyResponsesWebSocket|GrokQuota|GrokBilling|SucceededForScheduling)' -count=1
cd backend && go test -tags=unit ./internal/handler/admin -run 'Test.*(GrokOAuth|Settings)' -count=1
cd backend && go test -tags=unit ./cmd/server -run 'Test.*Wire' -count=1
```

### 前端定向

```bash
cd frontend && pnpm vitest run src/views/admin/__tests__/SettingsView.spec.ts src/components/account/__tests__/CreateAccountModal.spec.ts src/components/account/__tests__/CreateAccountModal.grok.spec.ts src/components/account/__tests__/EditAccountModal.spec.ts
cd frontend && pnpm typecheck
cd frontend && pnpm lint:check
```

### 完整门槛

```bash
cd backend && go test -tags=unit ./... -count=1
cd backend && GOTOOLCHAIN=go1.26.5 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.9.0 run --timeout=30m --build-tags=unit --new ./...
cd frontend && pnpm test:run
cd frontend && pnpm build
git diff --check
```

如环境或既有 main 变更导致完整门槛无法运行，必须记录具体命令、错误和已通过的替代验证，不能静默跳过。

## 11. 协议规范更新

更新 `.trellis/spec/backend/protocol-adapter-guidelines.md` 的“Codex 生图桥接与 Responses Lite 工具边界”：

- 删除“Header 永不透传”和“WS bridge 永不合成”的旧绝对规则。
- 写入默认阻止列表、最终模型匹配、HTTP/WS/bridge 矩阵和有限兼容降级。
- 保留 hosted `image_generation` 与客户端 `image_gen` 执行域边界。
- 更新 Required Tests 和 Wrong/Correct 示例。

## 12. 提交边界与回滚点

- 质量检查前不创建 merge commit。
- 用户确认提交前不运行 `git commit`；本任务不自动 push。
- merge commit 前的回退锚点为 `backup/build-before-main-0157-934982ae`。
- 任何需要 `git merge --abort`、删除提交或改写历史的动作都需用户再次明确授权。
