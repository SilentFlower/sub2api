# Design: 同步 main 0.1.152 到 build 并保护定制特性

## 合并边界

- 目标分支：`build`。
- 来源：实施前再次 fetch 后固定的 `origin/main`，随后把本地 `main` 纯快进到同一提交。
- 规划阶段目标：`origin/main=a1930ea6`，合并前 `build=origin/build=f59991b5`，共同基点 `e316ebf5`。
- 合并方式：`git merge --no-commit --no-ff main`；冲突解决和验证完成前不创建 merge commit。
- 回滚点：`backup/build-before-main-0152-<build短提交号>`，准确指向实际合并前的 `build`。
- 交付边界：验证通过后创建本地双父 merge commit，随后停止；未经用户再次确认不执行 push。

实施时若 `origin/main`、`origin/build` 或本地 `build` 与规划值不同，必须停止并重新生成 merge tree、冲突矩阵和任务材料。普通 push 被拒绝时不得 force push，也不得自动吸收未知远端提交。

## 工作区隔离

当前未提交内容分为两类：

1. 用户/历史已有内容：`.trellis/spec/backend/disaster-recovery-guidelines.md` 与 `.trellis/tasks/07-10-sync-main-0151-into-build/` 下的未跟踪材料。
2. 当前任务材料：`.trellis/tasks/07-13-sync-main-0152-into-build/`。

实际合并前只对第一类路径执行带 `-u` 的 pathspec stash；当前任务目录保持可见，不使用全仓 `git stash -u`。合并期间禁止 `git add -A`，只处理 Git 已自动暂存的合并结果和明确的冲突/规范文件，避免把当前任务或恢复后的旧材料误写进 merge commit。

merge commit 完成后恢复选择性 stash。当前任务材料单独记录任务进度，不与源码 merge commit 混合；用户已有修改不得被覆盖或回滚。

## 核心不变量

1. `main` 0.1.152 的修复和新增能力完整进入 `build`。
2. `build` 的 Codex 身份、生图、Anthropic 直连、Grok 路由/effort、GPT-5.6、容灾和 Trellis 定制不静默回退。
3. usage、计费、实际上游端点、缓存身份和 reasoning effort 必须反映最终上游请求/响应语义。
4. Codex 内置版本字面量仍只有 `openAICodexClientVersion` 一个生产来源。
5. migration 174 原样前向合入，既有 migration 不修改；Ent schema 与生成代码保持同步。
6. 两个独立测试或业务分支即使冲突在同一插入位置，也必须同时保留。
7. 本地 merge commit 必须有两个正确父提交，且未经确认不触碰远端 `build`。

## 文本冲突解决矩阵

| 文件 | 原因 | 处理策略 |
| --- | --- | --- |
| `handler/endpoint.go` | 双方只在 `DeriveUpstreamEndpoint` 公共注释处冲突 | 保留已自动合并的代码，重写为中文完整注释，说明 OpenAI/Grok 默认 Responses、原生端点保留和 runtime override |
| `handler/openai_alpha_search.go` | 双方独立新增 Alpha Search，main 又增加按次计费 | 采用 main 的 result/计费链路与新增依赖，保留 build 的中文公共 API 注释、调度和 failover 语义 |
| `handler/openai_chat_completions.go` | build 增加 Grok/Messages fallback，main 增加 result/context 实际端点和动态 Grok Chat/Responses 路由 | 使用三参数签名，优先级为 result、runtime context、账号/入站 fallback；Grok 原生 Chat 由实际转发结果记录，不再仅凭入站路径猜测；保留 nil、Messages 强制 Chat 和 OpenAI APIKey fallback |
| `service/openai_alpha_search.go` | `error` 返回与 `(*OpenAIForwardResult,error)` 返回冲突 | 使用 main 返回契约；仅成功 2xx 返回 `WebSearchCalls=1`，透传错误/重定向不计费；公共方法补齐中文参数和返回注释 |
| `service/openai_alpha_search_test.go` | 旧 error 断言与新 result/计费断言冲突 | 采用新签名并覆盖成功计费、普通错误不计费、failover 未写响应 |
| `service/openai_gateway_chat_completions_raw.go` | build 最终 effort 提取与 main cache identity/字段清理位于同一区域 | 保留 build 的 provider 归一化和最终 body effort 提取；加入 main 的 cache identity、`prompt_cache_key` 清理和九参数发送调用；避免重复局部变量 |
| `service/openai_gateway_grok.go` | build effort 归一化与 main Composer capability 清理冲突 | 先执行 provider effort 归一化，再执行 model capability 清理；Grok 4.5 和 Composer 各自命中自己的 guard |
| `service/openai_gateway_grok_test.go` | build 的嵌套 effort 测试与 main cache/tools/APIKey 测试重叠 | 同时保留 cache/tools、APIKey 和 effort 断言；嵌套输入断言 `reasoning.effort`，扁平输入单独覆盖 `reasoning_effort` |
| `service/openai_gateway_messages_chat_fallback.go` | build 强制 SSE/调试快照与 main 新发送签名冲突 | 保留 build 的强制上游 SSE、custom UA 和调试快照，调用新签名时追加空 cache identity；不能误用 `clientStream` 改变上游协议 |
| `service/openai_gateway_messages_chat_fallback_test.go` | effort 与最终 Codex identity header 断言冲突 | 两组断言同时保留，验证最终请求体 effort 和 UA/originator/version |
| `service/openai_gateway_service.go` | main 新增 context key 和旧版 `codexCLIVersion`，build 已集中定义版本 | 只加入 `openAIUpstreamEndpointContextKey`；拒绝重复 `codexCLIVersion` |
| `EditAccountModal.spec.ts` | build Grok Responses override 与 main xAI 默认 base URL 测试插入点相同 | 保留为两个独立 `it`，分别验证 extra 不丢失和 APIKey 默认 URL |

## Git 未报告的连带冲突

- `backend/internal/handler/endpoint_test.go` 自动合并后同时存在两参数和三参数调用；统一适配三参数签名，旧场景传 `nil`，并保留两组行为覆盖。
- `backend/internal/service/openai_codex_client_identity.go` 已定义 `codexCLIVersion`；若接受 `openai_gateway_service.go` 的 main 常量会重复声明，必须保持单点定义。
- `.trellis/spec/backend/protocol-adapter-guidelines.md` 仍记录 Alpha Search 旧返回签名和端点判定旧签名；实现完成后必须同步规范。

## 自动合并复核

### Alpha Search 计费链路

```text
alpha/search 成功 2xx
  -> OpenAIForwardResult.WebSearchCalls=1
  -> RecordUsage / CalculateCost
  -> group.web_search_price_per_call 或默认 0.01
  -> usage_logs / 分组管理界面
```

复核 migration 174、Ent schema/生成代码、Group service/DTO、API key cache snapshot、billing、GroupsView 和契约测试；非 2xx、failover 和 `WebSearchCalls=0` 不得按次收费。

### 上游端点记录链路

```text
OpenAIForwardResult.UpstreamEndpoint
  -> Gin runtime context
  -> account/inbound fallback
  -> usage / ops upstream_endpoint
```

Grok raw Chat、Responses bridge、Messages 强制 Chat、OpenAI APIKey raw Chat 和普通 OAuth Responses 都必须记录真实路径。Grok `/v1/chat/completions` 可能动态选择 raw Chat 或 Responses bridge，缺少 result/runtime 时默认 Responses。

### Grok 请求链路

```text
客户端 body
  -> 模型映射
  -> provider effort 归一化
  -> Composer capability / unsupported 字段清理
  -> cache identity 与请求头
  -> xAI Chat 或 Responses
  -> 最终 effort / quota / usage
```

重点复核 raw Chat、Responses、Messages、WebSocket HTTP bridge、APIKey/OAuth 分流、不可用模型诊断和 quota 持久化。

### Anthropic Messages 直连链路

```text
Anthropic Messages
  -> 确定性 Chat Completions body
  -> 强制上游 SSE + usage
  -> 流式直转或非流式折叠
  -> Anthropic 响应 + usage
```

保留 tool adjacency、确定性 tool id、sticky、custom UA、错误/failover 和 cache usage 契约。

### 前端与设置

- Grok OAuth `openai_responses_mode` 更新只增删该键，保留其它 `extra`。
- Grok APIKey 缺少 `base_url` 时使用官方 xAI 默认 URL。
- Alpha Search 单价字段保持 snake_case，并从后端 DTO 到 `frontend/src/types/index.ts`、GroupsView 完整一致。
- Fast/Flex、UseKeyModal、SettingsView 和中英文 i18n 的既有 build 能力不得因自动合并丢失。

## 验证策略

1. Git 索引、冲突标记、重复定义、旧调用签名和 `git diff --check`。
2. Alpha Search handler/service/route、按次计费、API key cache 和 group contract。
3. Grok Chat/Responses/Messages、cache identity、reasoning effort、APIKey/OAuth、quota 和 no-account 诊断。
4. Anthropic 直连桥接、Codex identity、生图、compact、WebSocket 和 OpenAI fallback。
5. migration 174、Ent 生成一致性、repository 和 API contract。
6. 前端 Grok 账号、GroupsView、Usage、UseKeyModal、SettingsView、typecheck 和 lint。
7. 后端全量 unit、golangci-lint；根据变更和环境决定 integration/migration runner 回归。

## 回滚设计

- merge commit 前：验证失败且不继续修复时执行 `git merge --abort`，随后从选择性 stash 恢复用户内容；备份分支提供提交级核对。
- merge commit 后但 push 前：不改写已创建历史；修复使用后续提交，删除或重做本地 merge commit必须另行取得用户授权。
- push 后：不在本次授权范围。未来如已推送需要回滚，使用 `git revert -m 1 <merge-commit>` 并普通推送，禁止 force push。
