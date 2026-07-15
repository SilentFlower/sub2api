# Design: 同步 main 0.1.155 到 build

## 合并边界

- 目标分支为 `build`，来源为实施前 fetch 后固定的 `origin/main`。
- 规划引用为 `build=a9ad55b3`、`origin/main=7c717365`、共同基点 `a1930ea6`。
- 合并方式使用 `git merge --no-commit --no-ff origin/main`，冲突解决和质量验证完成前不创建 merge commit。
- 合并前创建 `backup/build-before-main-0155-<build短提交号>`，准确指向实际合并前 build。
- 验证通过后允许创建本地双父 merge commit；未经单独确认不 push、不部署、不执行 migration。
- Phase 3.4 使用 `trellis-push` 的 merge-aware 路径：确认前不更新索引；确认后仅暂存 exact planned files，要求 cached 文件集合与计划完全一致且没有计划外 staged，再执行无 pathspec 的 merge commit 并核对两个父提交。
- fetch 后任一固定引用或冲突矩阵变化时，停止实施并回到规划更新本任务材料。

## 工作区隔离

- 其它未跟踪任务 `.trellis/tasks/07-14-deepseek-response-format-json-schema-compat/` 不得修改或暂存。
- 当前任务目录保持可见，用于记录合并证据，不混入源码 merge commit。
- 如需清理工作区，只允许对其它任务目录使用精确 pathspec stash；禁止全仓 `git stash -u`。
- 合并期间只对明确路径执行 `git add`，禁止 `git add -A`、`git add .` 或任何可能吸收未跟踪任务的命令。

## 核心不变量

1. main 0.1.155 的新增功能、修复、migration、Ent 与 Wire 结构完整进入 build。
2. main 已等价覆盖的 build 实现允许删除，不能为了保留提交痕迹维护重复 API、缓存或数据模型。
3. build 独有的 Codex reset 邀请重置、生图主模型/思考预算、namespace 桥接保护、Grok Messages 强制 Chat、Grok 4.5 effort、Anthropic 直连、HA/DR 和 Trellis 资产继续工作。
4. `image_gen` namespace 与旧原生 `image_generation` 桥接必须保持不同工具选择和提示词行为。
5. main 与 build 两套 OpenAI reset 入口独立存在，不能互相替换或共用错误的 DTO/API。
6. migration 只前向合入，已存在文件不改写；Ent schema 与生成代码一致。
7. 每个独立测试分支均保留；相邻插入冲突不能通过删除一侧测试解决。

## 冲突解决矩阵

| 文件 | 处理策略 |
| --- | --- |
| `backend/cmd/server/wire_gen.go` | 修改 Provider 源后运行 `go generate ./cmd/server`；最终同时构造 build `OpenAICodexResetService`、main `GrokQuotaService` 和 main `OpenAIQuotaService`，禁止手改生成结果作为最终方案 |
| `backend/internal/handler/admin/grok_oauth_handler_test.go` | 采用 main SSO/探测测试结构；删除旧独立 Billing endpoint 测试，保留不泄露凭据、错误脱敏和运行时 sanity 覆盖 |
| `backend/internal/pkg/xai/billing.go` | 采用 main `BillingSummary`、双窗口解析、部分成功合并、CLI header 与固定官方 Billing URL；删除 build `BillingSnapshot` 并行模型 |
| `backend/internal/pkg/xai/billing_test.go` | 采用 main 测试集；仅保留仍适用于 main 数据模型的边界样例，不保留旧结构断言 |
| `backend/internal/service/account_usage_service.go` | 采用 main Grok 自动 Billing probe、retry TTL、24h/7d/月度统计和 `grok_billing`；保留 build 其它平台、Codex reset 与用量定制 |
| `backend/internal/service/grok_quota_fetcher.go` | 采用 main Billing 与 rate-limit 快照合并、账号凭据 fallback；删除 build 独立 Billing TTL/旧字段 |
| `backend/internal/service/grok_quota_service.go` | 采用 main `QueryQuota`、singleflight、并行 Billing、局部成功、持久化与本地窗口；删除 `QueryBillingQuota` 旧返回类型和重复 fetch 路径 |
| `frontend/src/api/admin/grok.ts` | 采用 main `queryQuota`、SSO 导入和统一类型；删除 `queryBillingQuota` |
| `frontend/src/components/account/AccountUsageCell.vue` | 采用 main 免费/付费窗口、探测结果即时合并与 badge；删除 `GrokBillingQuotaCell` 引用和旧缓存更新函数 |
| `frontend/src/types/index.ts` | 采用 main `GrokBillingSummary`、三类本地窗口与长上下文字段；删除旧 `GrokBillingQuota` 并行类型，保留其它 build 类型 |
| `backend/internal/service/openai_codex_transform.go` | 以 main 统一 namespace/`additional_tools` 识别为基线；补 build namespace 跳过旧桥接、分流 `tool_choice`、可配置生图主模型/思考预算；GPT-5.6 max 采用 main |
| `backend/internal/service/openai_codex_transform_test.go` | 合并 main namespace/Responses Lite 覆盖与 build namespace 保护、旧桥接 `none→auto`、可配置主模型测试；删除 build 重复 GPT-5.6 旧实现断言 |
| `backend/internal/service/openai_compat_model_test.go` | 采用 main 的 compat 签名与 Fable 精确 dispatch；GPT-5.6 max 由 main candidate tests 覆盖，不恢复旧返回签名 |
| `backend/internal/service/openai_gateway_grok_test.go` | 同时保留 main 缺省模型/扁平 effort 与 build Grok 4.5 嵌套 effort 归一化测试 |
| `backend/internal/service/openai_image_generation_controls_test.go` | 同时保留 main Responses Lite 图片工具/header 透传与 build namespace 不注入旧桥接、旧原生 `none→auto` 回归 |
| `backend/internal/service/openai_ws_http_bridge_test.go` | 同时保留 main 空模型默认与 WS 事件测试、build Grok 4.5 effort 归一化测试 |

## 连带清理与保留

### Grok 旧 Billing 链路

合并时显式删除或清理：

- 后端旧 `QueryBillingQuota` handler、route、结果 DTO 和独立快照字段。
- 前端 `GrokBillingQuotaCell.vue`、旧 API、旧类型与旧事件处理。
- 只服务旧链路的测试和 i18n；共享文案若 main 仍使用则保留。

### build 独立 Codex reset

继续保留：

- `OpenAICodexResetService` 与 `openAICodexResetClient`。
- status、consume、invite 三个 build API。
- `OpenAICodexResetModal`、账号操作菜单、API 类型、i18n 和测试。
- main `OpenAIQuotaResetCell` 与 `/admin/openai/accounts/:id/reset-quota` 同时存在。

`ProvideAccountHandler` 需要同时接收 build reset service 与 main Grok quota service：先通过 `NewAccountHandler` 完成既有依赖构造，再设置 `grokImportProber`。测试可继续直接调用 `NewAccountHandler`。

### Codex 图片工具数据流

```text
请求工具
  -> main 统一识别 tools / additional_tools
  -> 若 image_gen namespace：不注入旧工具，不追加旧提示，不改 tool_choice
  -> 若旧原生 image_generation 桥接：缺失或 none 改为 auto
  -> 生图专用模型转换：读取管理员主模型与思考预算，空值使用 main 默认
  -> 上游 Responses
```

检测函数必须能区分“任意图片能力”和“旧原生 image_generation”，避免 main 的统一 predicate 把 namespace 错误带入 `none→auto` 分支。

## 自动合并复核

### OpenAI/Codex

- Responses namespace flatten/strip、`additional_tools`、图片工具 header、HTTP/WS 转发保持一致。
- GPT-5.6 max 使用 main 模型候选链路；build 生图设置和自定义 UA/身份不丢失。
- Anthropic Messages 直连的 tool adjacency、确定性 tool id、sticky、cache 与 thinking disabled 契约继续通过。
- Alpha Search 独立端点和计费能力继续存在。

### Grok

- `/v1/messages` 只有显式 `force_chat_completions` 才走 Chat，extra 更新不得覆盖 quota/邮箱等字段。
- Grok 4.5 effort 在 raw Chat、Responses fallback、Messages 和 WS HTTP bridge 中记录最终上游值。
- main SSO 导入、API Key 模型同步、第三方 base URL、媒体和 channel monitor 不被 build 旧路径覆盖。

### 数据库与运维

- main migration、Ent schema 和生成代码保持同一版本组合。
- build HA/DR、自动故障切换、镜像同步、Trellis 工作流和手动镜像构建不被 main CI/部署文件覆盖。
- README、部署示例和 Apple Container 文档按双方能力组合复核。

## 验证与回滚

- 合并中发现新增语义不确定项时停止，更新任务材料后再继续。
- merge commit 前验证失败可执行 `git merge --abort`，随后恢复精确 stash；备份分支用于提交级核对。
- merge commit 后不 amend、不 reset；后续问题使用修复提交。任何删除或重做 merge commit 需用户授权。
- push、部署和 migration 执行不属于本任务自动授权范围。
