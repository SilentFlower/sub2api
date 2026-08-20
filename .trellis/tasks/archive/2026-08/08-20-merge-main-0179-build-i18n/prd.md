# 合并 main 0.1.179 到 build 并保护现有功能与 i18n

## Goal

将最新 `origin/main@2bc139ab5`（版本 `0.1.179`）以普通双父 merge 合入
`build@45ca348e8`，完整吸收主线新增功能、修复、迁移和生成代码，同时保留 build
已有的 OpenAI/Codex、Grok、DeepSeek、Antigravity、Web Search、Responses Lite、
容灾和管理端私有能力。合并结果必须同时通过硬冲突复核、无冲突语义复核和中英文
i18n 验收，不能把“Git 自动合并成功”当作功能正确的证据。

## Background

- 共同基线为 `359fd12b2`（main `0.1.178`）；当前双方独有提交数为 build 248、main 75。
- main 相对共同基线修改 210 个文件，约 `+9544/-1112`，目标版本为 `0.1.179`。
- `git merge-tree --write-tree --messages --name-only HEAD origin/main` 预演得到 5 个硬冲突文件、
  7 个冲突 hunk：
  - `backend/internal/service/openai_gateway_chat_completions.go`
  - `backend/internal/service/openai_gateway_messages.go`
  - `frontend/src/components/account/CreateAccountModal.vue`
  - `frontend/src/components/account/__tests__/CreateAccountModal.grok.spec.ts`
  - `frontend/src/components/account/__tests__/CreateAccountModal.spec.ts`
- 双方共同修改 47 个文件，其中 5 个硬冲突、42 个文本自动合并热点；688 个 build 独有
  变更路径和 163 个 main 独有变更路径在虚拟合并树中均与各自 owner 逐字一致。
- main 本轮修改 6 个 locale 主文件，新增自适应协议、渠道倍率文案，并更新长上下文说明；
  build 的领域 locale extension 仍需按最终聚合值复核。
- main 新增 `226_add_usage_log_effective_model_indexes_notx.sql`、`227_*`、`228_*`；已有
  `226_channel_monitor_quota_mode.sql` 在共同基线中就存在。迁移 runner 以完整 filename 为
  主键并按完整文件名排序，因此保留双方文件，不修改已发布迁移。

## Requirements

- R1. 使用 `git merge --no-commit --no-ff origin/main`，不 squash、不 rebase、不改写
  `build` 或 `main` 历史。
- R2. 5 个硬冲突文件必须按领域语义组合，禁止对共享大文件整体采用 ours/theirs：
  - Chat Completions 同时保留 build 的 Responses 形状转 raw Chat 转换，以及 main 的
    CN 自适应/显式协议分流和缓冲读取修复。
  - Messages 保留 main 的原生 Anthropic/自适应入口优先级，并继续使用 build 的
    `ShouldForwardAnthropicMessagesViaRawChatCompletions`，保持 Grok 显式强制 Chat、
    OpenAI API Key 能力探测和 CN 固定 Chat 协议语义。
  - Create Account 同时保留 build 的 Codex 自定义 UA、OpenAI Compatibility、Grok
    Force Chat、Web Search 字段，以及 main 的自适应协议和长上下文计费逻辑。
  - 两个前端测试文件同时保留双方 mock、状态和断言，不删除任一侧覆盖。
- R3. 对 42 个自动合并热点执行三方语义复核，至少覆盖：
  - OpenAI Responses/Chat/Messages、WebSocket 429 恢复、client tools、reasoning cache、
    input token 预检、容量恢复和模型映射。
  - Grok tool search、inline image/view_image、reasoning effort 和 build 强制 Chat。
  - CN provider 自适应协议、分协议 base URL、header override、composite 调度和账号表单。
  - 渠道倍率、长上下文计费、监控额度、Wire/构造器和生成结果。
  - build 的 Responses Lite、Web Search、DeepSeek 推理降级、自定义 UA、生图和 GIF 兼容
    薄接入是否仍由原领域 owner 生效。
- R4. build 独有路径不得因合并发生内容漂移；main 独有路径必须完整吸收。双方共同修改
  文件中若出现语义冲突，以保留双方当前有效能力、复用领域 owner、共享入口保持薄接入为准。
- R5. i18n 必须按最终聚合树验收：
  - main 新增/修改的 `accounts`、`channels`、`overview` 中英文文案成对存在。
  - build locale extension 的中英文 import、深层 spread、key 路径和最终有效值成对保留。
  - 不允许中文缺 key 回退英文、不允许双方都缺 key 显示 key path，也不允许后置 override
    静默覆盖 main 的新语义。
- R6. 保留 main 的 226-228 迁移和 build/base 已有迁移；不得重命名或修改已发布迁移，
  必须通过 runner、migration unit/integration 测试验证完整 filename 排序和幂等行为。
- R7. 合并后运行聚焦测试与前后端全量质量门禁；任何失败都必须定位为冲突处理回归、
  自动合并语义回归、上游自身问题或既有基线问题，不能直接忽略。
- R8. merge commit 和推送必须由 `trellis-push` 核对精确索引、双父提交和目标分支，
  经独立确认后仅普通推送 `build -> origin/build`，不得 force push。

## Acceptance Criteria

- [ ] 合并前重新确认 `build@45ca348e8`、`origin/main@2bc139ab5`、共同基线
  `359fd12b2` 和干净的非任务工作区。
- [ ] 5 个硬冲突文件、7 个 hunk 按 R2 组合完成，`git ls-files -u` 为空，源码无冲突标记。
- [ ] 47 个双方共同修改文件完成三方复核；688 个 build 独有路径保持与合并前 build
  一致，163 个 main 独有路径保持与目标 main 一致。
- [ ] OpenAI/Grok/CN provider、Responses Lite、Web Search、DeepSeek 降级、Codex 自定义
  客户端、生图和 Antigravity GIF 的聚焦回归通过。
- [ ] main 的自适应协议、CN header override、渠道倍率、长上下文计费、WebSocket 恢复、
  tool search 和 Responses 修复均有对应测试通过。
- [ ] 所有 migration 专项测试通过，main 的 226-228 文件和已有 `226_channel_monitor_quota_mode.sql`
  均保留且未改写。
- [ ] i18n 全量专项测试通过；最终 en/zh key、扩展注册和关键页面最终文案正确。
- [ ] 前端 `pnpm typecheck`、`pnpm lint:check`、`pnpm test:run`、`pnpm build` 全部通过。
- [ ] 后端 `go test -tags=unit ./...`、`go test -tags=integration ./...`、
  `golangci-lint run ./...` 全部通过；Go 源码已 gofmt，`git diff --check` 通过。
- [ ] 最终 commit 恰好有两个父提交：合并前 build 和 `origin/main@2bc139ab5`；
  `origin/main` 是 merge commit 的祖先。
- [ ] 经用户确认后普通推送到 `origin/build`，不推送或修改 `main`。

## Out Of Scope

- 不新增 main 与 build 均不存在的产品能力。
- 不借合并重构与冲突、自动合并回归或薄接入无关的模块。
- 不修改远端 `main`，不改写远端历史，不 force push。
- 不把现有 Web Search 或 Antigravity GIF 两个历史任务并入本任务。
- 不向真实 OpenAI、xAI、Kimi、Zhipu、DeepSeek 或 Antigravity 上游发送在线请求；
  真实上游冒烟作为部署后验证风险保留。
