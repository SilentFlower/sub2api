# build 私有能力合并回归审计

## 1. 审计基线

- `build`: `7a6ab3280`
- `main`: `fbfdcef81`，版本 `0.1.176`
- merge-base: `0b3fe95af`
- 模拟合并：9 个冲突文件、24 个内容冲突块；162 个文件自动合并。
- 既有能力清单来源：
  `.trellis/tasks/archive/2026-07/07-17-build-private-feature-isolation/research/build-private-feature-inventory.md`。

审计不以“文件仍存在”或“Git 自动合并成功”为通过标准。每项能力都检查：

1. 领域 owner 是否仍存在且没有被 main 的等价/冲突实现绕过。
2. 共享 handler/service/view 是否仍调用 owner。
3. 设置、account extra、DTO 与前端类型是否能完整往返。
4. 测试是否覆盖最终入口，而不只是孤立 helper。

## 2. 已确认的高风险交叉点

| 能力 | 0.1.176 交叉点 | 失效方式 | 处理与验证 |
| --- | --- | --- | --- |
| Codex 自定义 UA 放行 | Create/Edit/Bulk 三个表单均与 main 指纹模式冲突；Gateway 同时新增指纹处理 | 选择 main 块会删除 UI/payload；顺序错误会让限制检测与出站身份混淆 | 表单同时保留两个字段；运行时分别验证入口限制、统一 UA/originator/version 和 fingerprint ID 注入 |
| Codex reset | `AccountsView.vue` 存在其它区域冲突 | 用整个 main 文件覆盖会删除 modal、菜单事件和状态同步 | 只解决局部 Grok 冲突，保留 reset import/modal/event/API；执行 modal、API export、handler/service 测试 |
| Codex 身份与 main 指纹模式 | main 在 `openai_gateway_forward.go` 新增 device/session/full ID；build 固定 Codex 客户端身份 | 指纹逻辑可能覆盖统一 UA/version，或不同入口产生不一致 ID | 新增/保留组合测试：身份头保持统一；fingerprint 只控制设备/会话/turn 元数据；HTTP/WS 行为一致 |
| Responses Lite | main 修改 Gateway、passthrough、WS、apicompat，并新增 x_search/reasoning alias | final-model policy、Lite Header、工具归一化、生图/搜索桥接可能被后续 main 转换覆盖 | 覆盖 HTTP、WS、WS-v2、Chat fallback、final model allow/block、image/search 工具组合 |
| JSON Schema 降级 | 与 main 的 Responses/Chat 转换和 reasoning alias 共用桥接层 | schema helper 仍在但入口不再调用，或转换顺序导致降级失效 | 验证 API-key/OAuth、Chat/Responses fallback 和结构化输出专项测试 |
| Web Search / web.run / AnySearch | main 新增原生 `/x_search` 和 Chat/Responses `x_search` 往返 | build 本地搜索可能吞掉原生 x_search、sources，或混合工具循环顺序漂移 | 明确原生 x_search 与 build emulation/web.run 分流；覆盖 native、emulation、chat tool loop、mixed tools、sources |
| Codex 生图策略 | main 修改 OAuth 图片流失败转移；build 修改主模型、预算、tool_choice 和 Lite bridge | failover 修复可能绕过 build 选模/预算，或客户端 `image_gen` 被错误回退到 hosted 工具 | 同时执行图片流错误、主模型/预算、tool_choice、namespace 与 Lite bridge 测试 |
| Anthropic Messages ↔ Chat 直连桥 | `chatcompletions_anthropic_bridge.go` 有 2 个硬冲突；main 新增 reasoning alias | 选一侧会丢 alias、reasoning-only 回显、流式缓存或稳定 tool ID | 使用 `reasoningText()`，保留 build 缓存、visible fallback、合法工具过滤与确定性 ID；运行直连桥全套测试 |
| Provider reasoning / GPT-5.6 | main 修改 Grok 4.5/4.6 和 Chat/WS 路径；build 在最终 body 做 provider-specific 归一化 | Grok 4.6 被错误套用 4.5 规则，或 WS/Chat 与 Responses 不一致 | 按最终上游 provider/model 验证 Grok 4.5、Grok 4.6、GLM、GPT-5.6 max 的发送值和 usage 投影 |
| Grok force-chat | main 修改 Grok messages、quota/model 行为 | `openai_responses_mode=force_chat_completions` 仍保存但路由不再读取 | 保留 feature helper、messages route 和 gateway 专项测试；删除独立 Billing 时不得连带删除该 extra |

## 3. 中低风险但必须复核的能力

| 能力 | 主要 owner / 接入 | 验证重点 |
| --- | --- | --- |
| Codex Alpha Search | 独立 handler/service、endpoint/route/usage 注册 | 路由仍注册，Codex identity 和搜索计费未被 `/x_search` 混用 |
| Raw Chat 调试快照 | `gateway_debug_snapshot.go` 与 Gateway 薄调用 | main 新错误/failover 返回路径仍按配置记录且不泄露敏感头 |
| Antigravity GIF 多帧兼容 | `pkg/antigravity`、service/settings、SettingsView 面板 | 独立主体不被 main 删除；Wire、设置 API、Gemini retry 接入和前端面板仍完整 |
| OpenAI 生图设置面板 | `setting_openai_image_generation.go`、前端 feature 面板 | DTO/cache/保存 payload/运行时消费保持同一字段契约 |
| Web Search 设置薄层 | `features/webSearch`、SettingsView 薄装配 | main 设置变更不覆盖 provider/AnySearch 配置，locale 最终值正确 |
| CI、README、Trellis、HA/DR | 独立目录和 `README_CN.md` | 手动 GHCR、fork CLA 删除策略、README 定制说明、Trellis/HA 资产不被恢复或覆盖 |

## 4. Grok 独立套餐额度删除边界

删除或解除接入：

- 后端 `/admin/grok/accounts/:id/billing-quota` route、handler 方法及 route/contract 测试。
- `GrokBillingQuotaService`、`pkg/xai/billingquota`、UsageInfo 的
  `grok_billing_quota` 投影及 Wire provider。
- 前端 `features/grokBillingQuota/`、`GrokBillingQuotaCell` 专项测试、请求队列、
  admin API re-export、共享类型字段和 locale 扩展。
- `AccountUsageCell` 的独立组件装配与合并更新；`AccountsView` 的独立套餐解析。
- 只为独立链路存在的测试 fixture 与导入。

保留：

- main `/quota`、`grok_billing`、`grok_usage_snapshot`、JWT tier、Grok 4.6 和
  main 用量展示。
- build `features/grokForceChat`、provider reasoning 和登录用户脚本。
- repository 对历史 `grok_billing_quota_snapshot` 的调度中性处理；不再读取、更新
  或展示该值，本任务不做数据迁移。

## 5. 完成证据

- 冲突清单与双边修改文件逐项记录最终 owner。
- `rg` 证明删除链路无生产引用，保留能力的 owner 与入口仍双向可达。
- Wire 从源 provider 重新生成。
- 后端单元测试、前端 typecheck、功能矩阵定向 Vitest、全量测试和构建通过。
- 合并后的运行树再次与 main/build 双边文件集比对，检查 locale spread、设置默认值、
  route/ProviderSet 和自动合并代码的最终有效语义。

## 6. 实际合并结果

- 保护引用：`refs/backup/build-before-main-0176-20260814-000906-7a6ab3280`。
- `MERGE_HEAD` 与 `main` 均为 `fbfdcef8184ae4b2e224d5cfc47cf1d0e3742710`。
- 9 个冲突文件、24 个内容冲突块已逐块解决，未合并索引与冲突标记扫描均为空。
- Grok 独立套餐额度的后端 route/handler/service/parser/Wire provider，以及前端 API、
  类型、组件、队列、locale 和专项测试均已删除。
- 生产代码中与独立链路相关的名称已清空；仅保留 main 的上游失败文本分类器，以及
  repository 对历史 `grok_billing_quota_snapshot` extra key 的调度中性清理。

## 7. 功能回归结论

发现并修复 1 项实际回归：删除独立套餐额度链路并恢复 main 的账号用量实现时，
`ProvideAccountUsageService` 一度没有注入 main `GrokQuotaService`，会导致 build 的 Grok
账号刷新退化为只读本地快照，无法执行主 `/v1/billing` 探测。

最终处理：

- `AccountUsageService` 恢复 main 的主动 Billing 探测和本地窗口投影。
- Wire provider 显式注入 `GrokQuotaService` 并重新生成 `wire_gen.go`。
- 回归测试恢复为 `TestAccountUsageServiceGrokRefreshUsesBillingOnly`，断言两次
  `/v1/billing` 请求及本地 24 小时窗口并存。

未发现其它 build 私有特性消失或入口失效。静态 owner/入口扫描和全量测试均确认：
Codex 自定义客户端、reset、统一身份与 fingerprint 组合、Alpha Search、Responses Lite、
JSON Schema、Web Search/web.run/AnySearch、生图策略、Anthropic 直连桥、provider
reasoning、Grok force-chat、Raw Chat 调试快照和 Antigravity GIF 仍可达。

## 8. 验证结果

- 后端：受影响包定向测试通过；`go test -tags=unit ./...`、`go test ./...`、
  `go vet ./...`、CI 同版 `golangci-lint v2.9`（`0 issues`）和生产二进制构建通过。
- 前端：`pnpm lint:check`、`pnpm typecheck`、功能矩阵定向 Vitest、全量 Vitest
  `240` 个文件 / `1630` 条用例和 `pnpm build` 通过。
- 生成与一致性：`go generate ./cmd/server` 通过；`git diff --check`、
  `git diff --cached --check`、未合并索引和冲突标记扫描通过。
- 分支资产：手动 GHCR workflow、README_CN 定制、build 隔离规范、HA/DR 规范仍存在；
  fork 专用 CLA 删除策略仍保持。
