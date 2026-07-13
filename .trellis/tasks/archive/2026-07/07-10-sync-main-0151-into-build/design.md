# Design: 同步 main 0.1.151 到 build 并保护定制特性

## 合并边界

- 目标分支：`build`。
- 来源分支：实施前刷新后的 `origin/main`。
- 预期来源提交：`e316ebf5`；若 fetch 后变化，必须重新生成冲突矩阵并更新任务材料。
- 合并方式：`git merge --no-commit --no-ff origin/main`，解决和验证完成前不创建 merge commit。
- 回滚点：创建 `backup/build-before-main-0151-<build短提交号>`，准确指向合并前 `build` HEAD。
- 交付方式：验证通过后创建双父 merge commit，并普通推送到 `origin/build`。

若实施开始时 `origin/build` 不再等于本地合并前 `build`，停止合并并重新评估；若验证后 push 因远端并发更新被拒绝，不使用 force push，也不自动吸收未知远端提交，先报告并重新规划。

## 核心不变量

1. `main` 0.1.151 的修复必须完整进入 `build`。
2. `build` 的定制能力不能因整文件选边或自动合并而静默丢失。
3. 协议适配记录的 usage、tool call、身份头和缓存字段必须反映最终上游请求/响应语义。
4. Codex 内置版本仍只有一个生产字面量来源。
5. 已发布 migration 不修改；新 migration 只按 forward-only 方式合入。
6. 推送只能是普通 fast-forward 更新远端 `build`，不能改写远端历史。

## 文本冲突解决矩阵

| 文件 | 冲突原因 | 最终策略 |
| --- | --- | --- |
| `openai_gateway_grok.go` | main 与 build 使用不同 effort 提取入口 | 保留 provider 归一化后的最终 body 提取，删除 main 侧多余局部变量 |
| `openai_gateway_grok_test.go` | 嵌套别名归一化与扁平字段保留使用不同测试输入 | 两类场景都保留，统一断言上游 body 与 usage metadata 为最终 `high` |
| `openai_gateway_messages_chat_fallback.go` | main 旧 Responses 中转代码与 build 直连桥接重构发生错位冲突 | 保留 build 的直连桥接、custom UA、Beta Fast、错误/failover 与 quota 语义；旧中转块不落入 header builder |
| `openai_oauth_passthrough_test.go` | 双方在同一位置新增不同身份回归测试 | 同时保留 GPT-5.6 当前身份测试和 codex-tui 配对测试 |

## 自动合并复核矩阵

### 协议与用量

- `apicompat/types.go`：保留 custom/tool_search/namespace 与 cache creation 字段，同时确认 build 既有 ChatThinking、effort 和流事件类型未退化。
- `openai_gateway_responses_chat_fallback.go`：Responses fallback 记录 custom、tool_search 和 namespace 声明，非流式和流式回程按原工具类型还原；不改变 Anthropic Messages 直连桥接。
- `openai_gateway_chat_completions_raw.go`、`messages.go`：保留 build 的 provider 最终 effort、Grok 分流、Anthropic sticky 和错误处理；吸收 main 的 cache creation usage 与身份修复。
- `openai_gateway_request_body.go`、`openai_model_mapping_test.go`：用户级 Fast/Flex 与 GPT-5.6 模型感知 `max` 同时成立。

### Codex 身份与生图

- `openai_codex_client_identity.go` 继续持有唯一版本常量；`openai_codex_identity.go` 只负责最终 UA/originator 配对和最低 version 修正。
- `account_usage_service.go`、gateway HTTP/passthrough/WS、账号测试路径都在最终 UA 改写后执行身份配对。
- `openai_codex_transform.go`、`openai_gateway_forward.go` 保留 build 的账号级生图策略和桥接，同时吸收 main 对 `image_gen` namespace、`additional_tools`、`tool_choice` 及 raw/WS payload 的识别和过滤。

### 设置与前端

- `OpenAIFastPolicyRule.user_ids` 从 middleware 可信用户 ID 进入 service 规则匹配，再通过 DTO/API/设置页保存；用户专属规则优先于全局规则。
- build 的 `openai_image_generation_main_model` 与 `openai_image_generation_reasoning_effort` 字段、表单和 i18n 保持不变。
- `UseKeyModal.vue` 中 GPT-5.6 四个配置各保留一份，并包含 `max` variant。

### 定价与迁移

- GPT-5.6 Sol/Terra/Luna 同时保留 main 的长上下文倍率和 build 的 `supports_max_reasoning_effort=true`。
- `173_allow_cyber_blocked_usage_request_type.sql` 原样新增；运行 migration regression 测试，禁止改动已有 SQL。

## 数据与协议链路

```text
最终 User-Agent -> PairCodexClientIdentity -> originator/version -> Codex 上游
Responses tools -> Chat fallback flatten -> 上游 tool call -> namespace/custom/tool_search 还原
API Key 用户 ID -> ctxkey.UserID -> Fast/Flex 规则 -> service_tier 处理
上游 usage -> cache read/cache creation 归一化 -> billing/usage_logs
Codex image_gen 声明 -> 账号策略 -> HTTP/raw/WS 一致过滤或桥接
```

每条链路均以边界类型和现有 helper 为唯一契约，不在调用方复制解析规则。

## 验证策略

先运行冲突相关定向测试，再运行 package 级与全量质量门槛：

1. apicompat custom tools、namespace、tool_search、cache creation、Anthropic 直连桥接。
2. Grok 4.5 effort、Messages 强制 Chat、quota；OpenAI raw/Responses/Messages fallback。
3. Codex identity、GPT-5.6、image generation、Fast/Flex 用户范围、setup-token refresh、writer nil guard。
4. handler/middleware/repository/migration regression。
5. 前端 SettingsView、UseKeyModal、model whitelist、typecheck、lint。
6. 后端全量 unit、golangci-lint、`git diff --check` 和冲突标记检查。

## 回滚设计

- merge commit 前：验证失败且不适合继续修复时执行 `git merge --abort`，回到备份分支指向的状态。
- merge commit 后但 push 前：不改写已有历史，优先重新调整后追加修复提交；必要时删除未推送 merge 的操作须单独取得用户授权。
- push 后：使用 `git revert -m 1 <merge-commit>` 创建显式回滚提交并普通推送，禁止 force push。

