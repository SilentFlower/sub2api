# Design: 合并 main 并配置 Responses Lite Header 模型策略

## 1. 设计目标

本任务同时完成两个不可分割的交付：

1. 在当前未提交的 `main -> build` 合并现场中，重新审查并解决 12 个文本冲突，保留双方可组合能力。
2. 用系统设置替代 `build` 的“所有模型都剥离 Responses Lite 标记”，按每次转发的最终上游模型决定 HTTP Header、WS metadata 和 WS -> HTTP Header 是否透传。

合并目标不是让索引从 `UU` 变为 clean，而是保证 `main` 的新能力和 `build` 私有能力在跨文件调用链上同时成立。已经手工移除 marker 的工作区内容只作为候选，不作为已批准结果。

## 2. 已确认的外部协议事实

### 2.1 官方 Codex

- Responses Lite 是模型能力，不只是 sub2api 的本地提示。
- HTTP/Compact 使用 `X-OpenAI-Internal-Codex-Responses-Lite: true`。
- WebSocket 使用 `client_metadata.ws_request_header_x_openai_internal_codex_responses_lite=true` 表示同一请求级模式。
- Lite 请求还会使用 `input.additional_tools`、developer message、`reasoning.context=all_turns` 和 `parallel_tool_calls=false`。
- 2026-07-16 核对的模型目录中，`gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna` 启用 Lite；`gpt-5.4`、`gpt-5.4-mini`、`gpt-5.5` 未启用 Lite。

### 2.2 sub2api PR #4380

- PR `#4380` 的实现提交为 `56650d6a`，合并提交为 `4a50d053`。
- 该 PR 只在 Lite 请求归一化中补齐 `reasoning.context=all_turns`。
- 该 PR 的 HTTP 回归模型是 `gpt-5.6-terra`，并继续断言 Header 透传。
- 因此它解决了 Lite-capable 模型的 body 契约缺失，但没有解决 `gpt-5.4-mini` 等非 Lite 模型的能力不匹配。

### 2.3 CLIProxyAPI

- `/root/project/CLIProxyAPI` 已更新到 `09da52ad`。
- HTTP executor 使用 Lite Header/metadata 识别请求布局并避免重复注入 hosted 生图工具，但没有把 HTTP Lite Header 加入上游请求。
- 直连 WebSocket 保留 metadata。
- 该实现提供了执行域边界参考，但它不是本任务的最终策略来源；本任务按官方模型能力和管理员配置决定透传。

### 2.4 被替代的旧决策

- 旧任务 `.trellis/tasks/archive/2026-07/07-15-fix-codex-responses-lite-image-bridge` 规定 Header 永不透传。
- `.trellis/spec/backend/protocol-adapter-guidelines.md` 当前仍记录该旧契约。
- 本任务完成后必须更新协议规范，以“最终上游模型 + 系统阻止列表”的新契约替代旧结论。

## 3. 设置数据契约

### 3.1 字段

- Setting key：`openai_responses_lite_header_blocked_models`
- 后端字段：`OpenAIResponsesLiteHeaderBlockedModels []string`
- API JSON 字段：`openai_responses_lite_header_blocked_models`
- 持久化格式：JSON `string[]`
- 默认值：`["gpt-5.4","gpt-5.4-mini","gpt-5.5"]`

不新增数据库表、Ent schema 或 migration；继续使用现有 key-value settings 表。

### 3.2 缺失、空值和非法值

| 存储/API 状态 | 语义 |
| --- | --- |
| 设置键缺失 | 使用三个精确默认项 |
| 存储值为合法 `[]` | 管理员显式取消全部阻止规则，不回退默认值 |
| 更新请求未携带字段 | 保留数据库现值；旧客户端不会意外覆盖 |
| 更新请求显式携带 `[]` | 持久化空数组 |
| 存储 JSON 非法或元素类型错误 | 记录不含敏感信息的 warning，运行时回退默认列表 |
| 更新项 trim 后为空 | `400` 拒绝保存 |
| `*` 不在末尾或出现多个 | `400` 拒绝保存 |

更新 DTO 使用 `*[]string` 区分“未提供”和“显式空数组”。`SystemSettings` 和查询响应仍使用 `[]string`。

### 3.3 归一化与匹配

- 每项执行 `strings.TrimSpace`。
- 保持首次出现顺序并稳定去重。
- 精确匹配和仅末尾 `*` 的前缀匹配复用现有 `matchModelPattern`。
- 模型匹配保持现有大小写敏感语义，不新增另一套通配符实现。
- 默认列表使用精确项，管理员可自行配置如 `gpt-5.4*` 的家族规则。

### 3.4 热路径缓存

`SettingService` 新增专用缓存和 singleflight：

- 成功 TTL：60 秒。
- DB/解析错误 TTL：5 秒，并使用默认列表。
- DB 查询使用独立 5 秒超时和 `context.WithoutCancel`，与现有网关设置缓存一致。
- 设置保存成功后先 `Forget` singleflight，再把归一化后的新列表直接写入缓存。
- 缓存中的 slice 在存取边界复制，避免调用方修改共享数据。
- 运行时暴露模型判断方法，例如 `ShouldBlockOpenAIResponsesLiteHeader(ctx, finalModel)`；转发路径不直接读取 repository。

## 4. 运行时策略

### 4.1 统一决策

每个 request/attempt/WS turn 生成一次策略决策：

```text
Lite 入站信号
  + account platform
  + 账号模型映射
  + compact / OAuth 模型归一化
  + 本次最终上游模型
  + SettingService 阻止规则
  -> allow 或 block
```

规则：

- 不是 Lite 请求：不改任何 Lite Header、metadata 或 body。
- 非 OpenAI 平台：不得收到该 OpenAI 内部标记；Grok 继续走现有专用清理。
- OpenAI 最终模型未命中：保留 Header/metadata，并执行 PR `#4380` 的 Lite 工具布局和 `all_turns` 归一化。
- OpenAI 最终模型命中：删除 Header/metadata，不执行 Lite 专属归一化，不强制新增或覆盖 `reasoning.context`。

### 4.2 有限兼容降级

命中阻止列表时只做以下两件事：

1. 删除 HTTP Header、WS metadata 或禁止 WS -> HTTP 重建 Header。
2. 跳过网关的 Lite 专属 `reasoning.context=all_turns` 强制补齐。

不尝试把 Lite body 完整转换成标准 Responses：

- 客户端已有 developer message 保持。
- 客户端已有 `input.additional_tools` 保持。
- 客户端已有 `parallel_tool_calls` 保持。
- 客户端显式提供的 `reasoning.context` 保持，不主动删除。

原因是网关无法可靠判断哪些 developer message 或工具原本属于标准顶层字段；完整逆转换会改变客户端指令和工具语义。

### 4.3 Managed HTTP

- 恢复 `main` 对 Lite Header 的可透传能力，但不能只靠静态白名单决定最终结果。
- 在账号映射、compact 映射和 OAuth 模型归一化后得到策略模型；图片模型转主模型等可能改变最终模型的分支也必须复用同一解析结果。
- 未命中时才运行 `normalizeOpenAIResponsesLiteToolsPayload`，保留 namespace -> `additional_tools` 和 `all_turns` 行为。
- 命中时跳过该 Lite 专属 normalizer，后续 JSON Schema、生图、Spark、权限和 Codex OAuth 变换仍按各自契约执行。
- 构造上游请求的最后边界再次按最终 body model 防御性检查：允许则保留 Header，阻止则 `Del`，避免后续模型改写造成陈旧决策。

### 4.4 Passthrough HTTP

- `openaiPassthroughAllowedHeaders` 恢复 Lite Header 候选项。
- 先完成 compact 映射和 OAuth passthrough body 归一化，再以 body 中实际上游模型判断。
- 未命中时 Header 透传并应用现有 Lite body 归一化；命中时删除 Header、跳过 Lite 专属 body 归一化。
- passthrough 不因 hosted bridge 开启新增图片工具；账号显式 `strip` 等原契约不变。

### 4.5 直连 WebSocket

- `response.create` 每一轮独立解析客户端模型；缺省模型继续复用上一轮客户端模型。
- 每轮先计算 `normalizeOpenAIModelForUpstream(account, account.GetMappedModel(originalModel))`，再判断阻止列表。
- 未命中：保留 metadata，并执行 Lite 工具布局与 `all_turns` 归一化。
- 命中：删除 `client_metadata.ws_request_header_x_openai_internal_codex_responses_lite`；若 `client_metadata` 变空可删除空对象；不执行 Lite normalizer。
- 会话内从 `gpt-5.6-terra` 切到 `gpt-5.5`，或反向切换时，必须按当前 turn 重新判断。

### 4.6 WebSocket HTTP bridge

- bridge 只在 payload metadata 仍表明 Lite 且最终模型未命中时设置 HTTP Lite Header。
- 命中模型的 metadata 已在 ingress 删除；bridge 内仍按最终 mapped model 做防御性判断，禁止重建 Header。
- Grok bridge 永不设置该 Header。
- `UpstreamTerminalEvent`、reasoning effort 提取、replay input、图片统计和错误/failover 逻辑保持。

## 5. 生图执行域边界

- hosted `image_generation` 由 OpenAI Responses 上游执行。
- 客户端 `image_gen` namespace/function 由 Codex 客户端执行。
- 顶层 `tools` 和 `input.additional_tools` 中的可执行 namespace，以及扁平 `image_gen.imagegen` function，都用于阻止重复 hosted 注入。
- 只有描述文本提到 `image_gen.imagegen` 不算工具声明。
- 已有客户端 `image_gen` 时，不注入 hosted 工具、不追加 bridge 提示、不改明确 `tool_choice`。
- 没有图片工具时，hosted fallback 仍由 group、全局/频道/账号、显式 `strip`、compact 和 Spark 门禁决定；Lite allow/block 不是 bridge 总开关。
- 独立 Images API 和批量图片 API 不受本设置影响。

## 6. 冲突解决矩阵

| 冲突文件 | `build` 侧能力 | `main` 侧能力 | 最终处理 |
| --- | --- | --- | --- |
| `backend/cmd/server/wire_gen.go` | Codex reset、独立 Grok billing quota 等依赖 | Grok quota `cfg`、审计、上游计费探测等依赖 | 不手工拼接；先合并 ProviderSet/构造器，再执行 `go generate ./cmd/server`，以生成结果为准 |
| `backend/internal/handler/admin/grok_oauth_handler_test.go` | `NewGrokOAuthHandler` 增加独立 billing quota 参数和相关测试 | `NewGrokQuotaService` 增加 `cfg` 参数 | 测试构造同时传入 `cfg` 槽位和 billing service 槽位，保留独立套餐额度、脱敏及 quota/reset 回归 |
| `backend/internal/service/grok_quota_service_test.go` | usage log、本地额度和 build 回归 | 新构造器 `cfg` 与上游 URL/行为回归 | 所有构造调用补 `cfg`，保留双方测试；不删除 build 的本地用量断言 |
| `backend/internal/service/image_generation_intent.go` | 严格识别可执行 `image_gen` namespace、扁平 function，避免重复 hosted 注入 | Grok 将被动 namespace/`additional_tools` 与明确图片意图区分 | 合并两套 predicate：OpenAI 保留严格客户端工具识别，Grok 使用平台专用显式意图；补回 Header 小写 key 常量 |
| `backend/internal/service/openai_gateway_service.go` | Web Search 真实调用计费说明和 build 字段布局 | WS 上游终态字段及 `SucceededForScheduling` | 同时保留；恢复 Lite Header 候选白名单，但最终透传由模型策略边界决定；公共方法注释使用中文 |
| `backend/internal/service/openai_images_test.go` | OAuth Images 使用后台主模型/思考预算回归 | `tool_usage.image_gen` 数值解析、防 hostile exponent 回归 | 两组测试都保留，避免相邻新增测试互相覆盖 |
| `backend/internal/service/openai_responses_lite_tools_test.go` | 旧 build 断言 Header 全局为空及生图工具边界 | PR #4380 断言 Header 透传和 `all_turns` | 改为新策略矩阵：`gpt-5.6-terra` 透传 + `all_turns`；默认阻止的 `gpt-5.5`/`gpt-5.4-mini` 删除 Header且不强制 context；managed/passthrough 都覆盖 |
| `backend/internal/service/openai_ws_forwarder_ingress_session_test.go` | Lite 生图 fallback、客户端 `image_gen` 不接管 | Lite namespace 迁移和 `all_turns` | 保留图片矩阵并拆分 allow/block 模型；同一会话增加模型切换，断言 metadata/context 每轮重新判断 |
| `backend/internal/service/openai_ws_http_bridge.go` | 使用统一 `extractOpenAIUpstreamReasoningEffort` | WS `UpstreamTerminalEvent` 与调度成功语义；metadata 重建 Header | 保留 build reasoning helper和 main 终态字段；Header 重建改成按最终模型条件执行 |
| `backend/internal/service/wire.go` | Codex reset、独立 `GrokBillingQuotaService` | Grok quota `cfg`、`ProvideUpstreamBillingProbeService` | ProviderSet 同时注册；`ProvideGrokQuotaService` 接受 `cfg + usageLogRepo`；`AccountUsageService` 不注入主 quota service，避免账号列表跨入 main Billing 探测链路 |
| `frontend/src/components/account/CreateAccountModal.vue` | Web Search/JSON Schema 相关 extra 构建 | Grok OAuth 自定义 base URL、请求头和切平台清理 | 使用空格缩进合并两套状态；先构建 credentials 并应用上游配置，再用 `buildGrokExtra` 保留其它 extra |
| `frontend/src/components/account/EditAccountModal.vue` | JSON Schema/Web Search extra 增删 | 上游计费探测开关 | 基于既有 `extra` 副本逐键更新，三类字段同时保留；APIKey/非 APIKey 清理边界不互相覆盖 |

## 7. 跨文件语义复核

仅解决 12 个 `UU` 不足以保证合并正确，至少复核以下自动合并链路：

- `AccountUsageService`：只读取既有 Grok 快照并投影本地窗口，不注入 `GrokQuotaService`、不主动调用 `ProbeBilling`；main `/quota` 与 build 独立 `/billing-quota` 各自由自己的入口触发。
- `GrokQuotaService`：同时保留 `cfg`、`usageLogRepo` 和现有 variadic 测试兼容签名，生产 Provider 使用完整参数。
- `openaiAllowedHeaders` / `openaiPassthroughAllowedHeaders`：恢复 main 候选项后由新策略终态过滤。
- `SettingService -> handler DTO -> frontend API -> SettingsView`：字段名、空数组语义和默认值必须完整往返。
- `OpenAIForwardResult`：WS 终态、reasoning effort、Web Search 次数、图片/视频字段同时存在。

## 8. 前端设计

- 设置入口放在现有 Gateway Forwarding/OpenAI 区域，不新增独立页面。
- 使用现有紧凑列表编辑模式：每行一个模型规则输入框，删除按钮使用项目 Icon，提供添加按钮。
- 初始值和 API 类型使用 `string[]`；保存前 trim、稳定去重并检查空项/通配符位置。
- 用户可见文案全部进入 `frontend/src/i18n/locales/{zh,en}/admin/settings.ts`。
- 前端校验用于即时反馈，后端仍是最终校验边界。

## 9. 回滚与提交边界

- 当前备份分支：`backup/build-before-main-0157-934982ae`。
- merge commit 前若整体方案不可用，可在用户明确授权后执行 `git merge --abort`；本任务不会自动回滚。
- Header 策略实现可按“设置链路”“HTTP”“WS”分段定位，但交付时必须全部通过，不能只修一条传输路径。
- `wire_gen.go` 只在依赖图稳定后生成。
- 验证完成前不创建 merge commit；本任务不自动 push。
- push 后如需回滚，应使用 `git revert -m 1 <merge-commit>`，禁止 force push。
