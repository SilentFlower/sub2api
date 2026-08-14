# Implementation Plan — DeepSeek 缺失推理内容自动降级

## 1. 后端系统设置

- [x] 在 `domain_constants.go` 定义 `SettingKeyEnableDeepSeekMissingReasoningAutoDowngrade`。
- [x] 在 service `SystemSettings`、handler DTO 和 update request 中添加准确对应的 bool 字段与 JSON tag。
- [x] 在 `InitializeDefaultSettings` 写入 `true`，在 `parseSettings` 对缺失/空值回退 `true`。
- [x] 在 `setting_update.go` 持久化字段，并在保存成功后立即刷新对应进程内缓存。
- [x] 在 admin GET、局部更新合并、保存响应和 `diffSettings` 中接入字段。
- [x] 补设置解析、更新、局部更新和审计测试。

## 2. DeepSeek 兼容策略

- [x] 新建 `openai_deepseek_missing_reasoning_policy.go`，实现默认开启的 SettingService 缓存读取。
- [x] 使用 `gjson` 实现最终模型 guard 和 assistant 工具调用历史检测。
- [x] 使用 `sjson` 实现 `thinking.type=disabled` 设置与 `reasoning_effort` 删除。
- [x] 保证已显式 disabled 且无 effort 时幂等返回 `changed=false`。
- [x] 新建专项测试文件，覆盖 PRD Behavior Matrix、空白/null/非字符串、`reasoning` 别名和设置缓存刷新。

## 3. Chat 上游发送链路接线

- [x] 在 `forwardAsRawChatCompletions` 的所有现有 body 改写之后、reasoning effort 提取之前调用兼容策略。
- [x] 在 `forwardResponsesViaRawChatCompletions` 的 provider/fast policy 之后、service tier 与 reasoning effort 提取及 web-run 分支之前调用兼容策略。
- [x] 在 `forwardResponsesViaWebRunChatCompletions` 循环内，每次 marshal 最新 Chat body 后、实际发送前调用兼容策略。
- [x] 在 `forwardAnthropicViaRawChatCompletions` 的 provider/context/fast policy 之后、reasoning effort 提取和实际发送前调用兼容策略。
- [x] 为四个发送点传入稳定来源标识，并由共享策略集中记录来源可区分的安全结构化 info 日志。
- [x] 传播策略错误，确保失败时不发送上游请求。
- [x] 扩展 raw Chat tests，验证命中后上游 body、关闭开关后的原样透传和完整 reasoning 历史不降级。
- [x] 扩展 Responses fallback tests，验证首次转换命中、完整 reasoning item 不命中，以及 web-run 续轮新增缺失历史后命中。
- [x] 扩展 Anthropic fallback tests，验证 assistant `tool_use` 历史命中；共享策略专项测试覆盖无工具历史不命中和关闭开关透传。
- [x] 复跑现有 DeepSeek reasoning_content 请求/响应透传测试。

## 4. 管理端 UI

- [x] 在 `frontend/src/api/admin/settings.ts` 同步 GET/UPDATE 类型。
- [x] 新建 `features/deepSeekReasoning/DeepSeekMissingReasoningDowngradeToggle.vue`。
- [x] 新建中英文 `settingsDeepSeekReasoning.ts` locale owner，并在主 settings locale 稳定末尾展开。
- [x] 在 `SettingsView.vue` 增加组件薄接入、默认 true 表单字段和保存 payload 字段。
- [x] 更新 settings view 测试 harness，补加载、切换、保存和默认值测试。

## 5. Verification

- [x] `cd backend && gofmt` 格式化本次修改的 Go 文件。
- [x] `cd backend && go test -tags=unit ./internal/service -run 'Test.*DeepSeek.*MissingReasoning|TestForwardAsRawChatCompletions.*DeepSeek|TestForwardResponsesVia.*DeepSeek|TestForwardAnthropicVia.*DeepSeek|TestSettingService.*DeepSeek'`
- [x] `cd backend && go test -tags=unit ./internal/handler/admin -run 'Test.*Settings.*DeepSeek|TestDiffSettings'`
- [x] 按受影响范围运行 `cd backend && go test -tags=unit ./internal/service ./internal/handler/admin`。
- [x] `cd frontend && pnpm vitest run` 运行新增组件及 SettingsView 相关测试。
- [x] `cd frontend && pnpm typecheck`
- [x] `cd frontend && pnpm lint:check`
- [x] `git diff --check`
- [x] 按 build 私有功能隔离指南检查共享文件仅保留薄接入，并用 `git merge-tree` 复核未新增明显 main -> build 冲突。
- [x] 修复设置保存与旧 singleflight 读取并发时的缓存覆盖窗口，并通过定向 `-race` 回归。
- [x] 同步管理设置 GET API 契约快照，并通过 `cd backend && go test -tags=unit ./...` 全量单元测试。
