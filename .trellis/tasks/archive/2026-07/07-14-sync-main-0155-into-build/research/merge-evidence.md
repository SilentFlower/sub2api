# main 0.1.155 合并证据

## 固定引用

- 目标分支：`build`。
- 规划时 `build` 与 `origin/build`：`a9ad55b3817c416fcf24804308faa19097a3b244`。
- 规划时 `origin/main`：`7c717365ef728e53cdcf6d639a4dd68226db03b2`，版本 `0.1.155`。
- 共同基点：`a1930ea6f29fc5f17ae0020f4e2d38e789c49d73`，版本 `0.1.152`。
- 分叉：build 独有 118 个提交，main 独有 112 个提交。
- main 相对共同基点修改约 315 个文件。
- 双方共同修改 53 个文件；虚拟合并报告 16 个文本冲突，其余 37 个自动合并文件仍需语义复核。

实施前必须重新 fetch 并核对以上引用。任一引用变化时重新生成虚拟 merge tree，不能沿用本文件的冲突数量。

## 16 个显式冲突

### Grok 配额与 Billing

1. `backend/cmd/server/wire_gen.go`
2. `backend/internal/handler/admin/grok_oauth_handler_test.go`
3. `backend/internal/pkg/xai/billing.go`，add/add
4. `backend/internal/pkg/xai/billing_test.go`，add/add
5. `backend/internal/service/account_usage_service.go`
6. `backend/internal/service/grok_quota_fetcher.go`
7. `backend/internal/service/grok_quota_service.go`
8. `frontend/src/api/admin/grok.ts`
9. `frontend/src/components/account/AccountUsageCell.vue`
10. `frontend/src/types/index.ts`

### Codex 图片工具

11. `backend/internal/service/openai_codex_transform.go`
12. `backend/internal/service/openai_codex_transform_test.go`

### 独立协议回归测试

13. `backend/internal/service/openai_compat_model_test.go`
14. `backend/internal/service/openai_gateway_grok_test.go`
15. `backend/internal/service/openai_image_generation_controls_test.go`
16. `backend/internal/service/openai_ws_http_bridge_test.go`

## 已确认的语义决策

### Grok

- 采用 main 的统一 `QueryQuota`、`ProbeBilling`、`ProbeUsage`、singleflight、本地 24h/7d/月度统计和 SSO 导入后探测。
- 删除 build 重复的独立 `queryBillingQuota` API、旧 Billing 快照类型、重复请求缓存和 `GrokBillingQuotaCell` 展示链路。
- 账号列表使用 main 的 `grok_billing`、探测结果即时合并、免费账号 24 小时估算和付费账号窗口统计。
- build 的 Grok `/v1/messages` 强制 Chat Completions、Grok 4.5 effort 归一化等独立协议能力不属于 Billing 重复实现，继续保留。

### GPT-5.6 max

- build 提交 `60d36274` 于 2026-07-10 02:50 首先补充 GPT-5.6 `max`。
- main 同日提交 `80b3d4c1` 增加模型感知处理，`c3ae5fc3` 增加原始模型候选提取。
- 最新 main 已覆盖显式 `reasoning.effort=max`、`gpt-5.6-sol-max` 后缀推导、上游模型规范化、compact 降级和 usage 元数据。
- 合并采用 main 实现，不保留 build 在通用 suffix 白名单和 `openai_compat_model.go` 中的重复旧实现。

### Codex 图片工具

- 使用 main 对顶层 `tools` 和 Responses Lite `additional_tools` 的统一图片工具识别。
- 顶层或 `additional_tools` 中存在 `image_gen` namespace 时，不注入旧 `image_generation` 工具，不追加旧桥接提示。
- namespace 的 `tool_choice` 完全尊重客户端，包括显式 `none`。
- 旧原生 `image_generation` 桥接继续保留 build 的 `tool_choice` 缺失或 `none` 改写为 `auto`；其它明确选择、Spark 和无图片工具不修改。
- 保留 build 的生图主模型与思考预算配置；main 固定 `gpt-5.4-mini` 仅作为空配置默认值。

### Codex reset 邀请重置

- main 的 `OpenAIQuotaService.ResetCredit`、`OpenAIQuotaResetCell` 和 `/admin/openai/accounts/:id/reset-quota` 完整吸收。
- build 的 `OpenAICodexResetService`、repository client、status/consume/invite 三个 API、独立弹窗和账号操作菜单完整保留。
- 两套链路保持独立，不拆分、不互调；Wire 同时注入 main 的 quota service、build reset service 和 main 的 Grok quota service。

## main 0.1.153 至 0.1.155 更新主题

- OpenAI/Codex：原生 Responses namespace、`additional_tools` 桥接、Responses Lite 图片工具保留、API Key 上游 Codex models、manifest failover、精确 Messages 映射、WS 生命周期、HTTP/2 keepalive、图片流与非流保活、账号 plan override、长上下文计费。
- Grok/xAI：统一 Billing 与主动探测、免费账号滚动 24 小时估算、导入账号探测、Web SSO 批量转 OAuth、API Key 模型同步、第三方 base URL、prompt cache identity、官方媒体 API、视频编辑/扩展、reasoning `content:null` 清理、监控与免费套餐徽章。
- 调度与稳定性：全量重建并发合并、账号/代理到期重建修复、调度缓存异常时间修复、plan-gated Codex 按账号冷却。
- 运维与管理：Server-Timing 指标、系统日志 host 过滤、嵌入静态资源缓存、DataTable 小数据量滚动稳定性。
- 部署与数据库：Apple Container 支持；长上下文计费、系统日志 host/index、Grok channel monitor、API Key 最新 IP 索引相关 migration 与 Ent 生成代码。

## 自动合并高风险范围

- `account_handler.go`、`wire.go`、`routes/admin.go`：build 独立 Codex reset 与 main Grok import prober/main reset 必须共存。
- `openai_gateway_forward.go`、`openai_gateway_request_body.go`、`openai_gateway_responses_chat_fallback.go`、`openai_ws_http_bridge.go`：namespace、图片桥接、GPT-5.6 max、Grok effort 和 Responses→Chat 工具契约不能回退。
- `grok_oauth_handler.go`、`account_repo.go`、`account.go`：移除旧 Billing API 时不能删除 SSO 导入、主动探测或 quota 持久化。
- `CreateAccountModal.vue`、`EditAccountModal.vue`、`AccountsView.vue`、i18n：保留 Grok Responses mode、build 独立 Codex reset 弹窗、生图设置和 main 新字段。
- migration、Ent、Wire：以 main schema 与生成结果为结构基线，再通过正式生成命令验证；禁止手工拼接不一致产物。

## 工作区隔离

- `.trellis/tasks/07-14-deepseek-response-format-json-schema-compat/` 属于其它未跟踪任务，必须保持未修改、未暂存。
- 当前任务目录只用于规划、研究和进度材料，不进入源码 merge commit。
- 合并期间禁止 `git add -A`；只暂存明确解决的冲突和经审查的合并文件。
