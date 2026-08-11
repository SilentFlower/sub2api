# Design: 合并 main 0.1.173 到 build 并处理冲突

## 1. 设计目标

在当前已执行但未提交的 `main -> build` merge 现场中，解决 36 个冲突文件、52 个冲突块，并复核自动合并的共享热点。最终结果需要同时满足：

1. 吸收 `main@0b3fe95af` 的 `0.1.173` 架构、修复和跨层契约。
2. 保留仍有独立职责的 `build` 能力，但不维护已被 `main` 替代的重复实现。
3. 给每个行为型冲突一个可解释的所有者和唯一状态源，不以消除 marker 作为完成标准。

## 2. 逐项决策规则

| 情况 | 决策 |
| --- | --- |
| `main` 已统一了架构、数据源和运行时入口 | 采用 `main` owner，删除或迁移 `build` 重复实现，仅保留仍有效的回归约束 |
| 两侧处理同一请求的不同阶段 | 按数据流组合，明确先后顺序和最终上游值 |
| 两侧服务数据源、快照键、API 入口或消费者不同 | 保留两条链路，但禁止互相触发、覆盖或重复展示 |
| 一侧只是历史常量、wrapper 或旧 UI 投影 | 删除旧形态，将有效测试迁移到现有 owner |
| 运行时确实需要两种行为 | 先复用现有账号 extra 或系统设置；只有无法表达时才新增开关 |

## 3. 设置与 Codex 身份

### 3.1 设置字段并集

`build` 和 `main` 的设置字段职责不重叠，因此在同一条 Setting 数据流中组合：

- `build`：`openai.image_generation.main_model`、`openai.image_generation.reasoning_effort`、`openai_responses_lite_header_blocked_models`。
- `main`：`openai_codex_client_version`、`openai_codex_client_version_synced`、`openai_codex_version_auto_sync_enabled`。

后端必须同时更新 key 定义、DTO、默认值、parse、update、audit、runtime cache 和 settings view；前端同时更新 API 类型、初始值、加载、提交、组件和中英文文案。

`openai_codex_client_version_synced` 是同步服务的只读输出。管理面板可展示，但更新请求不得将其写回或清空。

### 3.2 Codex 身份所有者

`main` 已引入动态版本同步、管理员覆写、标准 UA 构造和现有回滚开关 `gateway.disable_codex_identity_enforcement`。因此：

- 采用 `main` 的 `openai_codex_identity.go` 及 `OpenAICodexVersionSyncService` 作为唯一 owner。
- 删除 `build` 的静态 `openai_codex_client_identity.go` 常量源，避免 `codexCLIVersion`、UA 和 probe version 重复定义。
- 将 `build` 对最低版本、GPT-5.6 和自定义 UA 的有效测试迁移到动态 resolver 上。
- 不新增兼容开关；回滚需求已由 `main` 现有开关表达。

`.trellis/spec/backend/protocol-adapter-guidelines.md` 中“Codex 上游客户端身份版本一致性”尚记录 `0.144.1` 静态常量契约，该段落相对本次 `main 0.1.173` 已过期。实施以当前 `main` 动态架构和调用方为事实源，完成后在 Phase 3.3 更新该规范，不为了匹配过期文档恢复静态实现。

## 4. OpenAI/Grok 转发数据流

### 4.1 Responses ingress

两侧处理的是不同层次，按以下顺序组合：

```text
入站 Responses body
  -> main: 修复 tools[].parameters.type = null
  -> build: 按最终上游模型决定是否应用 Lite tools 归一化
  -> 模型/账号/生图策略改写
  -> build: 最终边界收口 Lite Header
  -> main: 写入 routing hint 和诊断日志
```

`main` 的 null type 修复始终执行；Lite 归一化不能采用 `main` 的无条件路径，否则会破坏 `build` 按最终模型屏蔽 Lite 的契约。

### 4.2 Anthropic Messages -> Chat fallback

- 先执行 `main` 的上游响应模型观测。
- 路由判断统一调用 `build` 的 `ShouldForwardAnthropicMessagesViaRawChatCompletions`；该 helper 同时覆盖 OpenAI API Key 不支持 Responses 和 Grok `force_chat_completions`，比 `main` 的内联条件更完整。
- 只保留直接 `AnthropicToChatCompletions` bridge，不恢复另一条 Responses 中转，避免重复 fallback。
- 先应用 `main` reasoning policy，再应用 `build` Fast Policy。`build` 会把 `anthropic-beta` Fast Mode 显式映射为 `service_tier=priority`，所以 `main` “Messages body 没有 service_tier”的假设不适用于该路径。
- 保留 `build` 的上游强制 stream 与本地折叠兼容行为，并用定向测试确认非流式客户端语义不变。

### 4.3 Reasoning、审计与计费

- HTTP、passthrough、WebSocket bridge 和 Grok 都从最终上游请求体提取 reasoning effort。
- provider-specific 路径使用最终 upstream model；其它路径按 `upstream -> billing -> original` 候选顺序，最后才使用 thinking fallback。
- WebSocket 保留 `main` 的 `UpstreamResponseModel`、冲突标记和终态调度字段。
- Grok 保留 `main` 的搜索/图片计数传播，同时补入 `build` 的 `ResponseID` 和最终 body reasoning effort。

## 5. Grok 双链路边界

### 5.1 后端数据所有权

| 链路 | 主要入口 | 快照键 | 主要消费者 | 决策 |
| --- | --- | --- | --- | --- |
| `main` 主额度/主动探测 | `/quota`、Responses probe、reconciler | `grok_usage_snapshot`、`grok_billing_snapshot` | 主额度状态、SevenDay/ThirtyDay、本地用量投影 | 保留 |
| `build` 独立 CLI Billing Quota | `/billing-quota` | `grok_billing_quota_snapshot` | 管理端独立套餐额度组件 | 保留 |

保留两者的原因是 API、快照键、解析结构和消费者不同，不是默认“两边都要”。两条链路不得互相写入快照键或用失败状态覆盖另一条链路。

### 5.2 AccountUsageService 与 Wire

`main` 新增的 `grokQuotaService` 注入会让账号用量查询在快照过期或 force 时主动调用 `ProbeBilling`。这与 `build` 已确认的双链路契约冲突，因此不组合该注入：

- `AccountUsageService` 不保留 `grokQuotaService` 字段、构造器参数和主动 `ProbeBilling` 分支。
- 账号用量只读已有 `main` 快照、独立 Billing Quota 快照和本地用量，完成投影与 stale 标记。
- `GrokQuotaService` 本身、手动 API、reconciler 和 Responses probe 保留 `main` 行为。
- Wire ProviderSet 同时注册两个 service，但 `ProvideAccountUsageService` 使用 `build` 的业务边界。

### 5.3 前端投影

- 独立 `GrokBillingQuotaCell` 是周/月套餐额度的主展示。
- `main` 的 `grok_billing`/`SevenDay`/`ThirtyDay` 保留为 API 兼容和主额度内部数据，但账号列表不用它们生成套餐标签、周/月进度条、预付余额或按量付费展示。
- 独立快照缺失时，免费/付费判定按被动 `grok_usage_snapshot.subscription_tier`、credentials 和 entitlement 回退；保留 24h 软门槛与请求/Token header 显示。
- 主额度探测成功后，以服务端已持久化结果重载账号用量；如响应明确表示 `persisted=false`，才在当前 cell 临时合并返回快照，不把临时值伪装成已持久化状态。
- 独立 Billing Quota 刷新只更新 `grok_billing_quota`，不改写 `grok_billing`、forbidden 或主额度快照。

## 6. 账号编辑契约

- `account.go` 采用 `main` 的 URL 处理和日志依赖，同时保留 `build` 的定向 extra 字段。
- `EditAccountModal` 以原 `extra` 副本为基础按 key 更新，采用 `build` 更完整的 `applyOpenAICompatibilityExtra`，保留 Responses mode、JSON Schema downgrade 和 Web Search emulation。
- `main` 的通用 upstream billing probe 不放在 OpenAI-only 分支中，对所有支持的 API Key 平台生效。
- `BulkEditAccountModal` 保留 `build` Codex 自定义 UA 控件，使用 `main` 的 `allBillingProbeCapable` 决定通用批量探测。

## 7. 冲突决策矩阵

### 7.1 后端设置（10 文件）

| 文件 | 决策 | 依据 |
| --- | --- | --- |
| `handler/admin/setting_handler.go` | 组合 | 响应同时输出生图/Lite 与 Codex 版本字段 |
| `handler/admin/setting_handler_audit.go` | 组合 | 审计差异需要覆盖双方可写字段，排除 synced 只读值 |
| `handler/admin/setting_handler_platform_quota_test.go` | 保留双方测试 | 两个测试覆盖不同字段，不是重复断言 |
| `handler/admin/setting_handler_update.go` | 组合 | 更新请求同时支持双方可写字段，不写 synced |
| `handler/dto/settings.go` | 组合 | JSON 契约是字段并集 |
| `service/setting_gateway_runtime.go` | 组合 | Lite 模型策略与 Codex 版本 resolver 是独立运行时缓存 |
| `service/setting_parse.go` | 组合 | 双方 key 需各自解析，非二选一 |
| `service/setting_service.go` | 组合 | 默认值和完整 settings 结构同时需要 |
| `service/setting_update.go` | 组合 | 持久化与 cache invalidation 同时覆盖双方可写字段 |
| `service/settings_view.go` | 组合 | 对外 view 必须与 DTO 和前端一致 |

### 7.2 后端网关（8 文件）

| 文件 | 决策 | 依据 |
| --- | --- | --- |
| `service/openai_gateway_forward.go` | 按阶段组合 | 先执行 `main` tool null-type 修复，再执行 `build` 有条件 Lite 策略，最后保留 main routing hint/日志 |
| `service/openai_gateway_grok.go` | 组合 | 保留 `main` 计数传播，补入 `build` ResponseID 和最终 reasoning |
| `service/openai_gateway_messages.go` | 使用 build helper + main observation | build helper 覆盖更完整的 OpenAI/Grok 路由条件 |
| `service/openai_gateway_messages_chat_fallback.go` | 组合，以 build bridge 为主 | 直接 bridge、Fast Policy 和 stream 兼容是有效定制；吸收 main reasoning policy |
| `service/openai_gateway_passthrough.go` | 采用 main 身份 owner，组合 build Lite 终态策略 | 删除静态身份常量，但保留按最终模型收口 Lite Header |
| `service/openai_gateway_service.go` | 组合 | 保留 main 结果/审计字段与 build 必要策略依赖 |
| `service/openai_oauth_passthrough_test.go` | 迁移测试到 main 动态身份契约 | 保留有效覆盖，不断言已删除的静态常量 |
| `service/openai_ws_http_bridge.go` | 组合 | main 终态/响应模型 + build 最终请求体 reasoning |

### 7.3 账号、Grok 与 Wire（6 文件）

| 文件 | 决策 | 依据 |
| --- | --- | --- |
| `cmd/server/wire_gen.go` | 重新生成 | 生成文件不手工解冲突 |
| `handler/admin/grok_import_probe.go` | 保留 build 注释与 main 逻辑 | 两侧代码行为相同，build 注释明确超时从出队后计算的原因 |
| `service/account.go` | 采用 main import 并保留 build extra 合并逻辑 | `slog`/`url` 是 main 新增实现的真实依赖 |
| `service/account_usage_service.go` | 保留字段并集，拒绝 main 主动注入 | 保留 `ThirtyDay` 与 `GrokBillingQuota`；不注入 `GrokQuotaService`，维持双链路隔离 |
| `service/domain_constants.go` | 组合 | 设置 key 职责不重叠 |
| `service/grok_quota_fetcher.go` | 保留 main 快照/本地窗口兼容，拒绝 plan 投影 | `billing.Plan` 不得覆盖独立套餐 owner；不将 main plan 写入 `SubscriptionTier`/`SubscriptionTierRaw` |

### 7.4 前端（12 文件）

| 文件 | 决策 | 依据 |
| --- | --- | --- |
| `api/__tests__/admin.grok.spec.ts` | 保留两类 API 测试 | 独立 Billing 与 main Capabilities/SSO/密码是不同端点 |
| `api/admin/accounts.ts` | 组合 export | build Codex Reset 与 main batch usage 都有消费者 |
| `api/admin/grok.ts` | 组合 export | 保留 `queryBillingQuota`、`getCapabilities`、`validateSSOToken`、`authorizePassword` |
| `api/admin/settings.ts` | 组合字段 | 与后端 settings JSON 契约一致 |
| `components/account/AccountUsageCell.vue` | 以 build 独立 Billing UI 为唯一套餐 owner | 禁止从 main Billing 渲染套餐标签、周/月条、预付或按量付费；只保留被动 header/免费 24h 回退 |
| `components/account/BulkEditAccountModal.vue` | 组合 | build Codex UA 控件 + main 通用 billing probe 能力判定 |
| `components/account/EditAccountModal.vue` | 以 build compatibility helper 为主，吸收 main 通用 probe | build helper 更完整，main 扩展了非 OpenAI API Key 平台 |
| `components/account/GrokQuotaProbeCell.vue` | 保留 build 详细快照，只在无重复时显示 main 摘要 | 主额度探测组件不承担独立 Billing 展示 |
| `components/account/__tests__/AccountUsageCell.spec.ts` | 重建为组合行为矩阵 | 覆盖独立快照优先、免费 24h、fallback 及探测刷新 |
| `i18n/locales/en/admin/settings.ts` | 保留新 Codex 身份语义与 build feature spread | locale spread 可能在无文本冲突时静默覆盖，需检查最终值 |
| `i18n/locales/zh/admin/settings.ts` | 同上 | 中英文保持同一契约 |
| `views/admin/SettingsView.vue` | 组合 | 同时挂载 build 生图/Lite 设置和 main Codex 版本 UI，避免重复表单 owner |

## 8. 自动合并语义复核

解决 `UU` 后还要检查以下无 marker 但高风险的链路：

- `openai_codex_client_identity.go` 与 `openai_codex_identity.go` 是否存在重复常量、UA 构造或 probe version。
- `service/wire.go` 构造器签名是否已经静默保留旧参数，生成结果是否可重复。
- Settings locale 中 feature spread 的最终运行时文案，不只检查共享文件差异。
- `AccountUsageInfo` 的 `grok_billing_quota`、`grok_billing`、`thirty_day` 是否同时穿过后端 DTO、前端类型和组件。
- `grok_billing_snapshot.plan` 是否被错误投影到 `SubscriptionTier`，或被前端用于套餐标签/周月额度。
- OpenAI 所有 HTTP/WS/fallback 入口是否使用同一个动态 Codex 版本 resolver。
- 双方同时修改的测试、re-export、组件 spread 和配置默认值是否出现静默覆盖。

## 9. 生成、验证与提交边界

- 先修复源 Provider 和 constructor，再执行 `cd backend && go generate ./cmd/server`。
- 生成后检查 `wire_gen.go` 不含手工残留、重复构造或无效参数。
- 验证覆盖定向单测、后端全量 unit、前端 lint/typecheck/test/build、冲突 marker 和 `git diff --check`。
- 最后再用双方修改文件列表与 `git merge-tree` 做一次语义重审。
- 任务仅交付可提交的 merge 结果；不自动 commit、push 或部署。
