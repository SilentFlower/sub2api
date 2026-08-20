# Brief — 合并 main 0.1.179 到 build 并保护现有功能与 i18n

## Goal

- 将 `origin/main@2bc139ab5`（0.1.179）以双父 merge 合入
  `build@45ca348e8`，吸收主线新增能力和修复，同时保护 build 私有功能与中英文 i18n。

## Scope

- 处理 5 个硬冲突文件、7 个冲突 hunk，组合保留 OpenAI/CN/Grok 路由、账号表单和双方测试语义。
- 对 42 个无文本冲突的双方共同修改热点做三方语义复核，重点覆盖 Responses/Chat/Messages、
  WebSocket、Grok、账号协议、计费、渠道、Wire 和 migration。
- 验证 688 个 build 独有路径无漂移，完整吸收 163 个 main 独有路径。
- 保留 build 的 Responses Lite、Web Search、DeepSeek 推理降级、Codex 自定义 UA、生图、
  Antigravity GIF 等领域 owner 和薄接入。
- 吸收 main 的 adaptive protocol、CN header override、渠道倍率、长上下文计费、Responses
  input token/client tools/reasoning cache、WebSocket 恢复和 Grok tool-search 修复。
- 审计最终 en/zh 聚合树、build locale extension、迁移执行顺序，并完成前后端聚焦及全量验证。

## Non-Goals

- 不新增双方均不存在的产品能力，不做与合并风险无关的重构。
- 不修改或推送 `main`，不 squash/rebase，不改写历史或 force push。
- 不把现有 Web Search、Antigravity GIF 历史任务并入本任务。
- 不向真实第三方模型供应商发送在线请求。

## Key Decisions

- 使用 `git merge --no-commit --no-ff origin/main`；共享冲突按领域语义组合，禁止整文件选择 ours/theirs。
- Chat Completions 采用 main 的统一 CN/Responses 路由 predicate，命中 Responses 形状时先执行
  build 的 raw Chat 转换；同时保留 main 的缓冲读取修复。
- Messages 保留 main 的 native/adaptive 入口优先级，后续继续使用 build 的 force-chat helper，
  兼容 OpenAI API Key、CN 固定 Chat 和 Grok 显式 force-chat。
- Create Account 合并双方 imports、状态和 mocks，让 build 私有字段与 main adaptive/long-context
  字段共存，不互相覆盖 credentials/extra。
- i18n 验收比较最终聚合值；若 build 后置 override 与 main 新语义同 key，更新领域 override
  吸收新语义，不依赖自动合并或 fallback。
- 同号 migration 以完整 filename 独立排序和记录；保留所有已发布文件，不重命名、不改写。

## Key Context

- 共同基线：`359fd12b2`；build/main 独有提交数：248/75；main 修改 210 个文件，约 `+9544/-1112`。
- 硬冲突位于 `openai_gateway_chat_completions.go`、`openai_gateway_messages.go`、
  `CreateAccountModal.vue` 及其两个测试文件。
- main 本轮修改 6 个中英文 locale 主文件；当前检查未发现 build extension 覆盖本轮新增 key。
- `226_channel_monitor_quota_mode.sql` 已存在于共同基线；main 新增另一份完整文件名不同的 226
  migration，以及 227、228 migration，runner 以完整 filename 为主键并排序。
- merge commit 和推送仍受 `trellis-push` 精确文件与独立确认门禁约束。

## Risks / Deferred

- 42 个自动合并热点可能出现调用顺序、默认值、模型映射或最终文案静默回退，必须靠三方复核和测试发现。
- 全量测试失败需要区分合并回归、main 自身问题和既有 build 基线问题，不能无证据修改业务语义。
- 未执行真实 OpenAI、xAI、Kimi、Zhipu、DeepSeek、Antigravity 上游冒烟；该项留待部署后验证。
- 实施前若 `origin/main` 不再是 `2bc139ab5`，必须回到 planning 刷新冲突矩阵和 Brief。

## Acceptance

- 5 个冲突文件、7 个 hunk 解决后无冲突标记，`git ls-files -u` 和 `git diff --check` 通过。
- build-only/main-only 路径归属核对通过，47 个 overlap 完成三方语义复核。
- build 私有功能与 main 新功能的聚焦测试全部通过，migration 与 i18n 专项验证通过。
- 后端 unit/integration/lint 和前端 typecheck/lint/full Vitest/build 全部通过。
- 最终 commit 恰好包含合并前 build 与 `origin/main@2bc139ab5` 两个父提交，并仅普通推送到
  `origin/build`。

## Next Step

- 真实 no-commit merge、冲突解决、三方语义复核与前后端全量质量门禁已完成；下一步执行
  `trellis-update-spec` 评估，再由 `trellis-push` 展示精确双父 merge commit 计划并等待确认。
