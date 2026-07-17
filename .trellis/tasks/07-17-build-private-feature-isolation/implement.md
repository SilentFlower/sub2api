# Implement Plan: 隔离 build 私有功能以降低上游合并冲突

## 1. 固定基线与功能归属

1. 记录开始时的 `HEAD`、`origin/main`、upstream 和工作区状态；确认没有真实 merge/rebase/cherry-pick。
2. 保存当前 `git merge-tree --write-tree --name-only HEAD origin/main` 的三个冲突文件作为对照。
3. 读取 `research/build-private-feature-inventory.md` 与 `research/build-commit-classification.md`，对每个拟迁移区块使用 `git log -p`、`git blame` 或对应 build 提交确认来源。
4. 生成实施清单，逐项标注：当前仍独有、main 已替代、保持现状、迁移、中央保留、生成文件或分支资产。
5. 如果某区块同时承载 main 通用行为，停止机械迁移，改为只抽离可独立验证的 build 判断或投影。
6. 对 15 个产品/协议领域和 5 类分支资产逐项打勾；不得只根据新增文件完成情况结束审计。

## 2. 后端账号与客户端策略

1. 审计 `account.go` 中 Codex 自定义客户端、JSON Schema/Web Search 等 build 专属方法。
2. 将确认属于单一功能的方法迁入同 package 的领域文件；receiver、签名和返回语义不变。
3. 审计 `openai_client_restriction_detector.go` 与 `openai_gateway_service.go`：
   - 复用现有 custom client matcher。
   - build 专属检测日志或决策迁出共享 service 主体。
   - 共享入口只提供 account/request 信息并消费结果。
4. 将对应 build 测试迁入功能测试文件；保留通用 gateway tests 在原文件。
5. 使用 `rg` 检查所有迁移方法和常量的调用点，没有复制实现或旧定义残留。
6. 将 Codex reset 的三个 handler 方法及专属请求绑定迁到 `account_handler_openai_codex_reset.go`；`AccountHandler` 字段、构造器和 route 只保留最小接入。
7. 保持独立 repository/service/modal 不变，补充 handler 文件迁移后的定向测试，确认 status/consume/invite reason、状态码和 envelope 不变。

## 3. 后端协议接入薄化

### 3.1 Responses Lite 与 Codex 生图

1. 复核 managed HTTP、passthrough HTTP、WS ingress、WS passthrough、WS HTTP bridge 五个接入点。
2. 保持 `openai_responses_lite_policy.go` 和现有 Codex image bridge 为规则所有者。
3. 入口只保留最终模型/入站信号准备、一次策略调用和错误传播；迁走入口内重复的模型规则、metadata/header 细节或 Lite 专属判断。
4. 不改变 hosted `image_generation` 与客户端 `image_gen` 的执行域、group/账号/compact/Spark 门禁。

### 3.2 Reasoning effort

1. 对照 `60d36274`、`05918460` 审计 GPT-5.6、Grok 4.5、GLM 的 build 归一化。
2. 将跨 gateway/fallback 重复的纯策略集中到明确的 reasoning effort 领域文件或现有 owner。
3. 各入口只传协议形态、最终模型和原始 effort，不重复维护候选表或映射。
4. 保持 usage 日志、前端展示和上游 body 的归一化结果一致。

### 3.3 JSON Schema、Web Search 与 Grok force-chat

1. 保持现有 downgrade、websearch、web.run 和 Grok chat bridge 主体文件。
2. 审计 gateway forward/fallback/messages/cc pipeline，只迁出 build 专属路由决定和 payload 变换；不拆碎工具循环或协议聚合主体。
3. 共享入口继续负责通用调度、计费、failover 和响应写入。

### 3.4 Raw Chat 调试快照

1. 从 `gateway_service.go`、`openai_gateway_service.go` 和 messages fallback 中识别环境解析、文件生命周期、snapshot 格式与调用点。
2. 将通用 debug snapshot 实现迁入明确命名的领域文件；两个 service 只持必要状态并调用一次 init/write。
3. 保持 `SUB2API_DEBUG_GATEWAY_BODY`、默认文件名、目录解析、header/body/extra 输出和禁用路径不变。
4. 将 Raw Chat 专项测试迁入独立测试文件，覆盖默认路径、目录路径、显式文件和禁用状态。

### 3.5 AnySearch provider

1. 保留 `pkg/websearch/anysearch.go` 作为请求/解析 owner，不迁入 manager 或 setting service。
2. manager/provider union 只保留 provider 枚举和一条构造注册，删除重复 endpoint/API key 分支。
3. Settings/Channels 接入只传结构化 provider config，不复制 AnySearch 请求规则。
4. 运行 provider、manager、config 和 ChannelsView 定向测试。

## 4. 后端 Grok Billing 与 Settings

1. 保留独立 `GrokBillingQuotaService`、billingquota parser、repository 写入和快照键。
2. 将 `AccountUsageService` 中仅属于独立套餐快照的投影/优先级迁入 Grok billing 领域文件；主流程只读取既有快照，移除 `GrokQuotaService` 注入和主动 `ProbeBilling`。
3. 将 `QueryBillingQuota` 及专属 handler 逻辑迁入 `grok_oauth_handler_billing_quota.go`；handler struct/constructor、route、service/repository 构造器和 ProviderSet 只保留最小注册。
4. 明确保留独立 `/billing-quota` 与 main `/quota` 双链路，不共享 DTO、快照键、自动刷新或失败状态；账号用量查询不得隐式触发任一路 Billing 请求。
5. 对 build 私有设置逐项审计 parse/validate/default/cache refresh：
   - 中央 struct/DTO 字段留在原文件。
   - 功能专用规则迁入领域 helper。
   - Get/Update 只调用 helper 并保持现有 JSON 契约。
6. GPT-5.6 主算法继续采用 main；只保留 `supports_max_reasoning_effort`、价格/展示元数据和 provider-specific 最终 effort 语义。
7. 搜索所有 interface stub/mock 和构造器调用，补齐迁移后仍需的签名。

## 5. 前端账号表单

1. 使用 Git 归属逐块审计 Create/Edit/Bulk，明确 build 私有块；不触碰 main 自有 Header Override、Grok Base URL Presets 等无关功能。
2. 优先抽离：
   - Codex 自定义客户端放行字段与转换。
   - OpenAI JSON Schema/Web Search 兼容字段。
   - Grok Messages 强制 Chat 的 extra 编辑。
   - 其它经 Git 确认为 build 私有且在多个弹窗重复的块。
3. Create/Edit 可共享的 UI 使用 typed props/emits 组件；Bulk 语义不同则使用独立 bulk 组件。
4. 对重复 extra 更新创建单一 typed helper：输入既有 extra，复制后只增删本功能键。
5. 父弹窗继续拥有 submit、API 调用、全量 reset 和最终 payload；子组件不得直接请求 API。
6. 保留父组件测试中的最终 credentials/extra payload 断言，并为新组件补局部交互测试。
7. 将 Codex reset API 函数与 DTO 迁入独立功能模块，中央 admin API 仅稳定 re-export；AccountsView 只保留 modal state 和事件装配。
8. 将 Grok Billing API、DTO 与现有队列/组件归入同一功能模块；`AccountUsageInfo` 只保留可选字段引用，禁止复制类型或转换字段命名。

## 6. SettingsView 与 I18n

1. 将生图设置和 Responses Lite 阻止模型列表等 build 私有 OpenAI/Codex 设置面板迁到领域组件，SettingsView 只保留 form 状态、加载和最终保存。
2. 列表型规则的 trim、去重和通配符校验使用单一 utility，不在页面和组件各写一套。
3. 将 ChannelsView 中 AnySearch 专属输入、可选 API Key 说明和条件展示迁入 provider 组件；父页面保留 config 与保存。
4. 将 build 私有 accounts/settings/channels 文案按 custom client、Codex reset、Grok Billing、Responses Lite、Web Search/AnySearch 等功能域迁入中英文扩展模块。
5. 在 `accounts.ts` / `settings.ts` / `channels.ts` 的正确嵌套对象中做稳定展开；不在 admin index 浅合并同名业务域。
6. 保持所有现有 `t('admin....')` key path 不变。
7. 运行 locale compile/no-collision 和相邻组件测试，确认中英文结构同构。

## 7. 预演冲突消除

### 7.1 WS 测试

1. 把当前 `KeepLeaseAcrossTurns` 恢复为通用两轮 lease/session 语义，不保留 build Lite 专属断言。
2. 新建功能测试文件，迁入 Lite `gpt-5.6-terra -> gpt-5.5 -> gpt-5.6-terra` 三轮场景。
3. 新测试继续断言同一 lease/连接、每轮 terminal event、metadata 和 reasoning context。
4. 确认原文件相对 main 共同基线不再包含 build Lite 改写区块，使 main 生图终态补丁可独立应用。

### 7.2 Locale

1. 从英文/中文 accounts 主文件移出 build 自定义 UA 等私有消息。
2. 在远离 main Codex 图片文案修改区的稳定对象边界展开扩展消息。
3. 当前 build 文案行为和 key path 不变；main 0.1.158 图片桥接文案由父任务后续合并。

## 8. 分支资产、中央契约与生成

1. 只读核对 HA/DR、`.github/workflows/my-ci.yml`、删除的 `cla.yml`、README_CN 和 Trellis/agents 资产仍归属于 build；不移动已独立内容。
2. 检查 DTO、`UsageInfo` / `AccountUsageInfo`、provider union、admin API re-export、routes、ProviderSet、contract、模型元数据和 Ent 是否因迁移产生必要变化；没有必要则不动。
3. 对实际修改 Go 文件运行 `gofmt`，前端只格式化实际修改文件。
4. 如果 service/repository 构造器或 ProviderSet 变化，执行仓库既有 Wire 生成命令并核对 `wire_gen.go`。
5. 不手工编辑生成代码来匹配期望 diff。

## 9. 定向验证

### 后端

```bash
cd backend
go test -tags=unit ./internal/pkg/openai ./internal/pkg/apicompat ./internal/pkg/websearch -count=1
go test -tags=unit ./internal/service -run 'Test.*(Codex|ResponsesLite|ImageGen|Reasoning|Grok|WebSearch|JSONSchema|Messages)' -count=1
go test -tags=unit ./internal/handler/... -run 'Test.*(Codex|Grok|Billing|Settings|Endpoint)' -count=1
go test -tags=unit ./cmd/server -run 'Test.*Wire' -count=1
```

### 前端

```bash
cd frontend
pnpm vitest run \
  src/components/account/__tests__/CreateAccountModal.spec.ts \
  src/components/account/__tests__/EditAccountModal.spec.ts \
  src/components/account/__tests__/BulkEditAccountModal.spec.ts \
  src/components/account/__tests__/GrokBillingQuotaCell.spec.ts \
  src/components/admin/account/__tests__/OpenAICodexResetModal.spec.ts \
  src/views/admin/__tests__/SettingsView.spec.ts \
  src/views/admin/__tests__/ChannelsView.websearch.spec.ts \
  src/i18n/__tests__/localesMessageCompile.spec.ts \
  src/i18n/__tests__/localesNoKeyCollision.spec.ts
pnpm typecheck
pnpm lint:check
```

## 10. 完整质量门槛

```bash
cd backend && go test -tags=unit ./... -count=1
cd backend && GOTOOLCHAIN=go1.26.5 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.9.0 run --timeout=30m --build-tags=unit --new ./...
cd frontend && pnpm test:run
cd frontend && pnpm typecheck
cd frontend && pnpm lint:check
cd frontend && pnpm build
git diff --check
```

检查还必须包含：

- `rg` 搜索重复规则、旧定义和 conflict marker。
- 对照迁移前测试映射，确认关键断言没有减少。
- 审查 diff，确保没有顺手重构 main 通用代码或改变产品行为。

## 11. 提交与父任务交接

1. 子任务通过 check 后更新必要规范，并进入 `trellis-push`；提交范围只包含隔离改动和本子任务文件。
2. 子任务提交/推送成功后，归档子任务并恢复父任务。
3. 父任务重新执行：

```bash
git fetch origin main
git merge-tree --write-tree --name-only HEAD origin/main
```

4. 比较原三个冲突：应已消除或显著缩小；任何新增/剩余冲突必须写回父任务设计和冲突矩阵。
5. 只有父任务完成新的 merge 预演和用户确认后，才执行真实 `main -> build` merge。

## 12. 回滚点

- 实现阶段尚未提交时，禁止使用破坏性 Git 命令清理工作区；按实际修改逐项修复。
- 子任务提交后如需撤销，使用普通 revert；不得 reset 或 force push。
- 本子任务不运行 `git merge`，因此不应出现 `MERGE_HEAD` 或冲突索引。
