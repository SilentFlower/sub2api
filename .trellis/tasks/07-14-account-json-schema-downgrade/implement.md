# OpenAI 兼容降级与权限工具桥接实现计划

## Checklist

1. 账号配置契约
   - 在 `internal/pkg/openai_compat` 增加 `openai_json_schema_to_json_object` 常量与严格 bool 读取测试。
   - 在 `service.Account` 增加仅 OpenAI API Key 生效的读取方法。
   - 锁定 OAuth、SetupToken、Grok 和非法值不生效。

2. JSON Schema 降级 helper
   - 在 `internal/pkg/apicompat` 实现 Responses 和 Chat 两种 raw JSON 转换。
   - 使用结构化 JSON 解析和局部写回，保留未知顶层字段。
   - 注入稳定的 JSON 输出约束；Chat 消息插入位置保持 system/developer 前缀顺序。
   - 添加纯函数表驱动测试，覆盖合法、关闭条件、非法结构和工具 Schema 不变。

3. 网关接入
   - 在 `Forward` 的账号选定后、路由分支前应用 Responses helper。
   - 在 `ForwardAsChatCompletions` 根据实际 body shape 应用 Chat 或 Responses helper。
   - 添加脱敏结构化日志，记录账号、路径和转换类型。
   - 覆盖 passthrough、Responses→raw Chat、raw Chat 和 Chat→Responses 最终上游 body。

4. `request_permissions` 请求契约
   - 增加 function 工具查找 helper。
   - 在 Responses→raw Chat fallback 中把“是否声明 request_permissions”传给非流式恢复和流式状态。
   - 添加请求转换回归测试，确保 JSON Schema 原样保留且不变成 `{input:string}`。

5. 非流式权限恢复
   - 使用 `encoding/xml` 实现严格权限标记 parser。
   - 支持说明前缀 + 单个 network 权限 + 空白尾随。
   - 严格命中后合成 Chat function tool call，再复用现有 Responses 回程转换。
   - 添加误判保护和非法输入测试。

6. 流式权限恢复
   - 扩展 `ChatCompletionsToResponsesStreamState`，增加跨 chunk 起始标记探测和候选缓冲。
   - 正常文本仅延迟最短标签前缀；候选成功时合成普通 function call 生命周期。
   - 候选失败或流结束不完整时补发原始文本。
   - 覆盖跨 chunk、前缀说明、真实 tool call 优先和完整终止事件。

7. 前端账号开关
   - 在 OpenAI API Key 创建/编辑区域增加“JSON Schema 兼容降级”开关和风险说明。
   - 保存到 `extra.openai_json_schema_to_json_object`；关闭时删除键并保留其它 extra。
   - 同步中英文账号管理 i18n 文案。
   - 添加创建/编辑组件测试和平台可见性断言。

8. 定向验证
   - `cd backend && go test -tags=unit ./internal/pkg/apicompat`
   - `cd backend && go test -tags=unit ./internal/pkg/openai_compat ./internal/service -run 'JSONSchema|RequestPermissions|ResponsesChatFallback|ChatCompletions' -count=1`
   - `cd frontend && pnpm vitest run src/components/account/__tests__/CreateAccountModal.openai.spec.ts src/components/account/__tests__/EditAccountModal.spec.ts`
   - `cd frontend && pnpm typecheck`
   - `cd frontend && pnpm lint:check`
   - `git diff --check`

9. 全范围检查
   - 运行 Trellis check-all 路由。
   - 复核 PRD、设计和实现的一致性。
   - 确认没有修改数据库 schema、migration、API 响应 envelope 或其它账号平台行为。

## Risky Files

- `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go`：流式状态机复杂，必须保持事件顺序、output_index 和终止生命周期。
- `backend/internal/service/openai_gateway_forward.go`：Responses 热路径，配置关闭时不能引入全量 JSON 解码或行为变化。
- `backend/internal/service/openai_gateway_chat_completions.go`：同时支持真实 Chat body 和 Responses-shaped body，shape 判断顺序必须正确。
- `frontend/src/components/account/CreateAccountModal.vue`、`EditAccountModal.vue`：保存 extra 时必须复制既有对象，不能覆盖探测、额度和其它账号配置。

## Rollback Points

- JSON Schema helper 和网关调用点可独立回滚；extra 键保留但无人读取时无副作用。
- 权限 parser/流式扫描由 request_permissions declared guard 隔离，可独立回滚。
- 前端开关可先隐藏，后端默认关闭仍保持兼容。

## Pre-Start Review

- 两项需求已确认合并到同一任务。
- JSON Schema 配置只面向 OpenAI API Key，默认关闭。
- `request_permissions` 按 function tool 处理，首版只恢复截图确认的 network 权限标记。
- 不新增数据库 migration，不自动重试上游，不实现通用 grammar 解析器。
