# Structured Outputs 与 Web Search 能力兼容实现计划

## Checklist

1. 账号配置与纯函数契约
   - 定义 `openai_json_schema_to_json_object` 键及严格 bool 读取。
   - 扩展 `GetWebSearchEmulationMode` 到 OpenAI APIKey，保持其它平台/type guard。
   - 抽取账号三态、渠道默认和全局配置的统一决策 helper。
   - 添加账号/渠道 default、enabled、disabled 与旧 bool 兼容测试。

2. JSON Schema raw JSON 转换
   - 实现 Responses/Chat 两个 downgrade helper，保留未知字段。
   - 注入稳定的 best-effort Schema 约束，确保幂等且不修改 tool Schema。
   - 接入 Responses、Responses->Chat、passthrough 和直接 Chat 发送前路径。
   - 添加脱敏决策日志和端到端上游 body 测试。

3. AnySearch provider
   - 增加 provider 类型、JSON-RPC 请求和多形态响应归一化。
   - 支持可选 Bearer key、响应限长、错误截断和代理 client。
   - 接入 Manager 配置、配额预占/回滚与全局设置校验。
   - 增加 provider 单测及 Manager 回归测试。

4. Responses Web Search 决策
   - 解析 effective tools、tool_choice、text.format 和上游 Responses/Chat 能力。
   - 实现 pure/forced 接管、mixed-auto pass/reject 和 json_schema 冲突矩阵。
   - 在 `OpenAIGatewayService.Forward` 的 Chat fallback 之前执行。
   - 保证 native Responses 账号在模拟关闭时仍按原路径透传。

5. 本地模拟执行与 Responses writer
   - 从最后 user input 提取查询，处理 search_context_size 和 allowed_domains。
   - 复用 Manager、账号代理与 proxy failover。
   - 构造非流式 web_search_call、message、summary 和合法 url_citation。
   - 构造完整 Responses SSE 生命周期与 annotation 事件。
   - 成功结果设置 `WebSearchCalls=1`，失败不返回可计费 result。

6. DeepSeek 原生请求桥
   - 扩展 Responses/Anthropic Web Search DTO。
   - 支持 `web_search_preview -> web_search_20250305`、domain/location 和 forced choice。
   - 对无等价字段返回明确转换错误。
   - 回归 function/custom/namespace/tool_search 转换。

7. DeepSeek 原生回程桥
   - 解析 server_tool_use、web_search_tool_result、result error 和 citations。
   - 非流式输出 web_search_call 与 url_citation。
   - 流式聚合查询 JSON，输出搜索 item、annotation 和完整收尾事件。
   - 添加 DeepSeek/Anthropic 官方形态 fixtures，覆盖 success、empty、error 和强制 choice 400。

8. 管理端
   - OpenAI APIKey 创建/编辑页增加 JSON Schema toggle 与 Web Search 三态。
   - 渠道 OpenAI section 增加 Web Search 默认开关。
   - 全局 provider 增加 AnySearch 类型，保持密钥脱敏和旧配置。
   - 更新中英文文案、TypeScript 类型和组件测试。

9. 定向验证
   - `cd backend && go test -tags=unit ./internal/pkg/apicompat`
   - `cd backend && go test -tags=unit ./internal/pkg/openai_compat ./internal/pkg/websearch`
   - `cd backend && go test -tags=unit ./internal/service -run 'JSONSchema|WebSearch|Responses.*Anthropic|Anthropic.*Responses' -count=1`
   - `cd backend && go test -tags=unit ./internal/handler -run 'Responses|WebSearch|Usage' -count=1`
   - `cd frontend && pnpm vitest run src/components/account/__tests__/CreateAccountModal.openai.spec.ts src/components/account/__tests__/EditAccountModal.spec.ts`
   - `cd frontend && pnpm typecheck`
   - `cd frontend && pnpm lint:check`
   - `git diff --check`

10. 全范围检查
   - 运行 Trellis check-all 路由。
   - 逐条核对 PRD acceptance 与设计矩阵。
   - 运行后端完整单测及前端受影响测试集。
   - 确认无数据库、migration、动态跨平台路由和许可证污染。

## Risky Files

- `backend/internal/service/openai_gateway_forward.go`：OpenAI Responses 热路径；配置关闭时不能增加行为变化或无条件完整解码。
- `backend/internal/service/openai_gateway_responses_chat_fallback.go`：必须在工具被丢弃前完成接管/reject 决策。
- `backend/internal/pkg/apicompat/anthropic_to_responses_response.go`：流式状态机需要保持 output_index、content_index、sequence 和收尾事件一致。
- `backend/internal/pkg/apicompat/types.go`：共享 DTO 变更可能影响 Messages、Responses、Chat 多条转换链。
- `backend/internal/pkg/websearch/manager.go`：AnySearch 必须进入现有配额/代理逻辑，不能为新 provider 分叉调度。
- `frontend/src/components/account/CreateAccountModal.vue`、`EditAccountModal.vue`：extra 更新必须保留探测、额度和其它运行态字段。
- `frontend/src/views/admin/ChannelsView.vue`、`SettingsView.vue`：平台默认与全局 provider 配置必须保持旧 JSON 兼容。

## Rollback Points

- JSON Schema helper、service 调用点和前端 toggle 可作为一个独立回滚点。
- OpenAI Responses 模拟入口可通过账号 `disabled` 立即停用，并可独立回滚 writer。
- AnySearch adapter 可独立回滚，Brave/Tavily 不受影响。
- 原生 Anthropic request/response 扩展可独立回滚到基础工具映射。

## Pre-Start Review

- 两项升级保留在同一任务，但实现按独立阶段和回滚点组织。
- OpenAI DeepSeek 账号不动态切到 Anthropic endpoint。
- 本地模拟仅接管 pure/forced Web Search；mixed-auto 完整工具循环不在首版。
- JSON Schema 只显式配置，不自动重试，不降为 string。
- 本地模拟成功按现有分组 Web Search 单价计费一次，失败不计费。
- new-api 仅作为行为研究证据，不复制 AGPLv3 源码。
