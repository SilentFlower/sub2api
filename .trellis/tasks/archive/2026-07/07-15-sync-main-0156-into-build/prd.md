# 同步 main 0.1.156 到 build 并保留定制能力

## 目标

将固定的 `origin/main=d515c304`（版本 `0.1.156`）合并到
`build=4f683e95`，完整吸收主线新增功能、修复和生成代码，同时保留 build 已
验证且主线未等价覆盖的协议、账号、额度和运维契约。合并结果必须经过冲突
语义复核、生成代码重建、后端与前端质量验证；本任务不自动 push、部署或执行
数据库 migration。

## 背景

- 共同祖先为 `7c717365`（版本 `0.1.155`）。
- `build...origin/main` 分叉为 build 独有 138 个提交、main 独有 132 个提交；
  main 侧包含 85 个非 merge 提交和 47 个 merge 提交。
- main 相对共同祖先修改 253 个文件，约 `+31,408/-1,710`，其中后端 210 个、
  前端 36 个、部署 3 个。
- `git merge-tree --write-tree build origin/main` 报告 13 个内容冲突。
- 隔离编译已确认 Git 未报告的新旧 Chat->Anthropic 状态机重复声明；任意统一
  选择 ours/theirs 都无法编译。
- 2026-07-15 的 build 任务已经重新确立 Grok 独立套餐额度链路和 Responses
  Lite 生图桥接契约；旧 `0.1.155` 合并任务中“删除独立 Billing 链路”的决定
  已被后续需求取代，不得沿用。
- 用户已确认继续保留 Grok 独立 `/billing-quota`，并将其作为账号列表唯一自动
  Billing 来源；其余冲突解决矩阵也已确认，无待定产品取舍。
- 历史 main->build 合并均采用备份分支、`--no-commit --no-ff`、逐冲突语义
  处理和定向测试，禁止整文件批量选择 ours/theirs。

## 需求

### R1. 合并边界与可恢复性

- 实施前重新 fetch 并固定 `build`、`origin/build`、`origin/main` 和 merge base；
  任一引用变化或冲突矩阵变化都必须停止并更新规划。
- 确认本地 `build` 与 `origin/build` 一致，创建
  `backup/build-before-main-0156-<build短SHA>`。
- 使用 `git merge --no-commit --no-ff origin/main`，验证通过前不创建 merge
  commit。
- 合并期间只暂存精确文件；禁止 `git add -A`、`git add .` 和全仓
  `git stash -u`。

### R2. build 必须保留的行为契约

- Grok 账号列表的套餐额度只允许独立 `/billing-quota` 链路自动刷新；
  `AccountUsageService` 不得重新注入 `GrokQuotaService` 或自动调用 main 的
  `ProbeBilling`。main `/quota`、OAuth 恢复和 reconcile 继续保留为手动或运维
  能力。
- Grok `/v1/messages` 显式 `force_chat_completions` 时，上游始终请求 SSE 和
  usage；下游非流式请求由本地折叠为 Anthropic JSON。保留 custom UA、xAI
  request ID、quota snapshot、错误脱敏和 failover。
- Anthropic->Chat 请求保持稳定 typed content、动态 attribution 过滤、并行
  tool_result 顺序、确定性 tool id、`thinking.disabled` 和空流完整 frame。
- Chat 上游的工具调用到流结束仍缺少 function name 时，不得输出空名字的
  Anthropic `tool_use`；若没有其它合法工具块，终态使用 `end_turn`。usage 中
  `input_tokens` 扣除 cache read 与 cache creation，且两类缓存 token 分别单独
  返回，避免重复计数。
- Responses Lite 内部标头只用于本地识别，不得透传上游。请求已有可执行
  `image_gen` namespace、扁平或嵌套 `image_gen.imagegen` function 时，不注入
  hosted `image_generation`、不追加 hosted 提示、不覆盖显式 `tool_choice`。
- Lite 请求没有原生或客户端图片工具且 hosted bridge 允许时，继续注入一个
  hosted `image_generation`；main 的 Responses Lite 工具归一化和明确的非图片
  `tool_choice` 仍需保留。
- build 的 OpenAI Codex reset 邀请重置、Grok 独立套餐额度、Web Search/Alpha
  Search、HA/DR 和 Trellis 资产不得被 main 同类但不等价的实现覆盖。

### R3. main 必须吸收的能力

- 保留 main 的 OpenAI Agent Identity、账号测试/额度服务的 WS 失效注入、
  passthrough failover 与错误脱敏、首输出超时、Responses SSE 修复。
- 保留 main 的 Grok OAuth 池恢复、TokenRefresh reconcile、Free 账号识别、
  vision/function tool 修复；不得用 build 旧依赖覆盖这些能力。
- 保留 main 的账号复制、重复创建保护、账号/分组 ID 列、DataTable 缓存、
  Server-Timing、调度缓存生命周期和 xAI URL 校验。
- main 的新 `chatcompletions_anthropic_bridge.go` 作为唯一长期桥接实现；在删除
  build 旧文件前，必须迁移并验证 R2 的稳定性契约。

### R4. 冲突与静默语义问题

- 按 `design.md` 的 13 文件冲突矩阵逐项处理，不得整文件无差别选边。
- 所有 `NewAccountHandler` 调用必须适配同时存在的 `grokOAuthService` 与
  `openaiCodexResetService` 参数；所有 `NewGrokOAuthHandler` 调用必须适配独立
  Billing 与 reconciler 参数。
- `ProvideAccountUsageService` 必须保留 Agent Identity WS 注入，但不得恢复
  `GrokQuotaService` 构造参数。
- 删除旧桥接实现前，迁移 `isClaudeCodeAttributionSystemText` 等跨文件依赖和
  非流式 SSE 折叠能力，确保不存在未定义符号或重复声明。
- 修改 provider 源后运行 `go generate ./cmd/server`，`wire_gen.go` 最终只接受
  生成结果，不手工维护冲突拼接。

### R5. 验证与交付

- 复核双方共同修改但 Git 自动合并成功的文件，重点覆盖 OpenAI gateway、
  Grok usage/provider、账号 handler 调用点、前端账户操作和生成代码。
- 运行协议适配、Grok、Codex 生图、账号 handler、前端账户页定向测试，再运行
  后端全量 unit、前端全量测试、typecheck、lint 和 `git diff --check`。
- 检查重复符号、冲突标记、旧桥接生产引用、Grok 自动 Billing 回接、Lite
  标头泄漏和 handler 构造参数错位。
- 输出 main 0.1.156 更新摘要、冲突处理记录、build 能力保留清单和残余风险。
- merge commit 与 push 均进入 `trellis-push` 单独确认，不从本任务创建授权中
  推断；不部署、不运行 migration。

## 验收标准

- [ ] 合并前备份分支准确指向实际的 pre-merge build HEAD。
- [ ] 固定的 main 被合并进本地 build，`git ls-files -u` 为空且不存在冲突标记。
- [ ] 13 个内容冲突均有文件级结论，自动合并高风险文件已完成反向覆盖扫描。
- [ ] 新旧 Chat->Anthropic 重复类型和函数已消除，旧桥接契约已迁移到 main 新
      实现，协议适配定向测试通过。
- [ ] 缺少 function name 的 pending tool call 不输出非法 `tool_use`；没有合法
      工具块时终态为 `end_turn`，usage 三类输入 token 相加不超过 prompt token。
- [ ] Grok 账号 usage 不自动触发 main Billing probe；独立 `/billing-quota`
      链路、缓存、DTO 和 UI 继续可用，main 手动 `/quota` 与 reconcile 同时可用。
- [ ] Lite 无图片工具时 hosted fallback 生效且不泄漏 Lite 标头；可执行 namespace、
      扁平 function、嵌套 function 场景均不注入 hosted 工具并保留 tool_choice。
- [ ] `SearchFailed` 产生 failed；`max_tokens` 产生 incomplete；其它正常结束产生
      completed，对应流式收尾测试通过。
- [ ] 账号复制、Codex reset、Spark shadow 三类账户操作同时存在，事件和弹窗互不
      覆盖。
- [ ] `NewAccountHandler`、`NewGrokOAuthHandler`、provider 和 Wire 构造链全部
      编译通过，生成文件与源 provider 一致。
- [ ] 后端全量 unit、前端 test/typecheck/lint、生成一致性和 Git 完整性检查通过；
      环境阻断必须有命令和错误证据。
- [ ] 未经单独确认不创建 merge commit、不 push、不部署、不执行 migration。

## 范围外

- 不修改已确认的 Grok 独立套餐额度产品边界。
- 不实现新的图片工具运行时、额度模型或数据库 schema。
- 不顺带重构无关 handler/service/frontend 结构。
- 不自动发布镜像、部署生产环境或清理历史备份分支。
