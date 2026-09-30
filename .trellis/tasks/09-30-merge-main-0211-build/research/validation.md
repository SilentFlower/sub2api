# 合并实施与验证证据

记录日期：2026-09-30。仅记录本次实际执行结果；原始预览保留在 `approved-preview.md`，不作为实际合并后的行为证据。

## V0

### 固定输入与当前状态

- 原 build / 当前 HEAD：`9c914278cef7cfe571120d19a92b7a75e5bfc6f8`。
- 固定 main / 当前 MERGE_HEAD：`42bc7f6cffe24bcb471608e48e66b4a0afa1f882`。
- 共同基线：`a3eb7ef302961cba716dc78b39b93b60c467db0e`。
- 恢复分支：`backup/build-before-main-0211-9c914278c`，指向原 build。
- 已执行 `git merge --no-commit --no-ff 42bc7f6cffe24bcb471608e48e66b4a0afa1f882`；实际出现预览中的 10 个冲突文件，现已逐块解决。
- 未创建 merge commit，未 push 或部署；任务保持 `in_progress`，原 build 的本地工程提交仍在原 HEAD 历史中。
- 最终源码索引树：`a3826793ff5ad0fdea94fa3eab896c3070ebbdfc`；相对 HEAD 为 279 个文件、12428 行新增、991 行删除。10 项任务文档单独处于未跟踪状态。
- `git ls-files -u` 为空；工作树无未暂存源码变更；精确冲突标记扫描无命中；`git diff --check` 与 `git diff --cached --check` 均退出 0。
- `backend/go.mod`、`backend/go.sum`、`frontend/package.json`、`frontend/pnpm-lock.yaml`、Ent 和 migration 无变化。

## V1

### 共同文件、独立文件与定价资产

[shared-files-audit.md](shared-files-audit.md) 已逐项记录 71/71 个共同修改文件的契约、处置及证据，其中 10 项冲突、61 项自动合并。

使用 `git diff --name-only --no-renames -z` 生成两侧相对共同基线的文件集合，使用 `git ls-tree -rz` 比较最终索引和各自来源的模式、类型及 blob。197 个 main 独立文件中 194 项完全一致；716 个 build 独立文件中 710 项完全一致。脚本同时断言全部偏差恰好等于下表，并核对 HEAD、MERGE_HEAD 和备份 SHA；退出 0。

| 来源 | 必要偏差 | 原因 |
| --- | --- | --- |
| main | `backend/internal/pkg/apicompat/anthropic_to_responses_stream_tool_input_test.go` | 添加搜索输入与 function 首事件/delta 状态隔离回归。 |
| main | `backend/internal/service/client_disconnect_status_test.go` | 显式检查类型断言，修复 unit-tag 增量 lint 的 errcheck。 |
| main | `frontend/src/components/account/__tests__/ClaudeResetCreditsCell.spec.ts` | 连续确认前等待异步状态及按钮渲染，保留所有业务断言。 |
| build | `backend/internal/pkg/apicompat/anthropic_chatcompletions_test.go` | 公共转换器 disabled 使用标准 none；供应商扩展改由实际出站回归验收。 |
| build | `backend/internal/service/openai_alpha_search_responses_bridge.go` | 消费 searched/error 四返回契约；失败流本地回退或明确错误，避免伪成功。 |
| build | `backend/internal/service/openai_alpha_search_responses_bridge_test.go` | 增加失败、截断及本地结果回退的行为断言。 |
| build | `backend/internal/service/openai_provider_reasoning_effort.go` | 最终 body 显式关闭后不从原始模型后缀恢复未发送 effort。 |
| build | `backend/internal/service/openai_provider_reasoning_effort_test.go` | 回归 disabled 后缀与最终 none 的计费语义。 |
| build | `frontend/src/views/admin/__tests__/SettingsView.spec.ts` | 增加白名单选择/保存/重载/清空，并确认旧 Grok 设置仍保存。 |

另新增 service 私有 thinking helper、实际供应商出站测试、Messages -> Chat Ollama token 出站测试及真实双语 reset 测试，共 4 个文件，均在本次已确认范围内。

`backend/internal/web/embed_test.go` 在共同基线之后两侧原本都未修改，不属于上述 197/716 集合；本次额外修复两个静态资源 fixture，使请求和 MIME 断言使用 build 现有 `logo.svg`。原 branding 提交 `fa402b909` 已删除 `logo.png` 并新增 SVG，embed 测试首轮因此命中 SPA 回退；修复后测试通过，保留未指纹静态资源无长期缓存及指纹资源 immutable 缓存断言。最终脚本还单独断言基线未分叉文件偏差恰好只有这个测试文件、额外新增文件恰好 4 项。

定价 JSON 使用结构化对象比较：只新增 `gpt-6.1-sol`、`claude-sonnet-5-5`，与 main 对象一致，未删除或改写旧模型。

## V2

### 后端单元测试

Go 采用项目要求的自动工具链 `go1.27.0`。重命令串行执行，`GOMAXPROCS=4`、编译并发和测试并行度均为 4。

| 命令（backend 目录） | 结果 |
| --- | --- |
| `GOMAXPROCS=4 go test -p 4 -parallel 4 -tags=unit ./internal/pkg/apicompat ./internal/pkg/openai ./internal/pkg/websearch ./internal/service ./internal/handler -run 'AlphaSearch\|ToolInput\|ToolInputAndSearch\|HandleResponses.*(WebSearch\|TerminalUsage)\|GPT61\|CompatModelNormalization\|DisabledThinking\|ClampsActualOllama\|ForceChatCompletionsPreservesFinalModel' -count=1` | 退出 0；handler 无匹配用例，其行为由下一行全量 unit 覆盖。 |
| `GOMAXPROCS=4 go test -p 4 -parallel 4 -tags=unit ./...` | 修复旧 build 的 disabled 公共转换断言后，完整重跑退出 0；没有删除有效断言。 |
| `GOMAXPROCS=4 go test -p 4 -parallel 4 -tags=unit ./internal/service -run 'TransportErrorHandlers_ClientDisconnectRecordsNoOpsEvent\|AntigravityCompatTransportError_ClientDisconnectWrites499\|ExtractOpenAIUpstreamReasoningEffort_UsesMappedBillingOriginalOrder' -count=1` | 最终测试修订后退出 0，0.200 秒；未改业务代码，不重复已经通过的完整 unit。 |

实际通过的重点证据包括：

- 搜索：`TestForwardAlphaSearchViaResponsesUsesUpstreamWebSearch`、`AcceptsWebSearchCallEvidence`、`FallsBackToEmulationWhenUpstreamDidNotSearch`、`RejectsUnsuccessfulSearchStreams`、`IncompleteStreamUsesLocalResults`、`NoSearchAndEmulationUnavailable`；模拟结果无 URL、Provider 失败及混合工具预算分支也由完整 unit 覆盖。
- 工具：`TestAnthropicToolInputAndSearchStateRemainIndependent`、`ToolInputOnContentBlockStart`、`ToolInputSeedNotDuplicatedByDeltas`、工具恢复链及终态 usage 测试。
- 推理/模型：`TestForwardMessagesChatDisabledThinkingUsesFinalProvider`、`TestCompatModelNormalizationDisabledThinkingDoesNotDeriveEffort`、`TestGPT61SolRejectsDisabledReasoningBeforeForwarding`、映射与工具限制、最终 effort/tier、Lite/blocked models 和 WS ingress 测试。
- Ollama：`TestMessagesChatFallbackClampsActualOllamaOutboundTokens` 的实际出站默认/自定义上下文、零值、低于上限及非 Ollama 五种场景。
- 计费/路由：完整 handler/service unit 覆盖图片价格继承、显式零价/自定义价、Fast/Ultrafast、缓存、HTTP/WS、Schema/DeepSeek、粘性、图片桥接和非流式超时等现有路径。

## V3

### 后端 lint

使用 task-local `/tmp/sub2api-merge-tools/golangci-lint` v2.13.0，构建工具链为 go1.27.0，与 CI 版本一致；未修改仓库依赖。

| 命令（backend 目录） | 结果 |
| --- | --- |
| `GOMAXPROCS=4 GOFLAGS=-p=4 /tmp/sub2api-merge-tools/golangci-lint run --concurrency=4 --timeout=30m ./...` | 退出 0，`0 issues`；与 CI 默认命令的 tag 范围一致。 |
| 同上，附加 `--build-tags=unit` | 退出 1，报告 58 项测试代码问题。相同命令运行于原 build 的只读 git archive，规范化文件/位置后报告项相同；未扩展本次任务处理存量测试 lint。报告数量受 linter 的展示上限影响，不能声称这是仓库全部 lint 债务。 |
| 同上，附加 `--build-tags=unit --new-from-rev=HEAD` | 首轮发现上游客户端断开测试的未检查断言；修正后最终退出 0，`0 issues`。之后的 SVG 测试只在 embed tag 下编译，不改变此检查范围。 |

日志：`/tmp/sub2api-merge-backend-ci-lint.log`、`/tmp/sub2api-merge-backend-lint.log`、`/tmp/sub2api-merge-baseline-lint.log`、`/tmp/sub2api-merge-backend-new-lint.log`、`/tmp/sub2api-merge-backend-new-lint-final.log`。完整 unit-tag lint 不应描述为全部通过；本次验收分别依据 CI 全量 lint 和新增差异检查。

## V4

### 后端构建与生成代码

- `GOMAXPROCS=4 GOFLAGS=-p=4 make build`（backend）：退出 0。
- `backend/bin/server -version`：退出 0，版本 `0.2.11`。
- Wire provider、构造器、`wire_gen.go` 已按实际依赖核对，并经完整 unit 与构建验证；无需重新生成，不运行 Ent generate。
- `GOMAXPROCS=4 go test -p 4 -parallel 4 -tags=embed ./internal/web -count=1`：SVG fixture 修复后退出 0，0.039 秒，使用本轮真实前端构建产物。
- `GOMAXPROCS=4 GOFLAGS='-p=4 -tags=embed' make build`：退出 0，编译嵌入本轮前端资源的二进制；随后 `backend/bin/server -version` 再次退出 0，版本 `0.2.11`。

## V5

### 前端检查与构建

Node 重命令使用 `NODE_OPTIONS=--max-old-space-size=4096`，Vitest 显式限制最多 2 worker、最少 1 worker，重任务串行。

| 命令（frontend 目录） | 结果 |
| --- | --- |
| `NODE_OPTIONS=--max-old-space-size=4096 pnpm typecheck` | 退出 0。 |
| `NODE_OPTIONS=--max-old-space-size=4096 pnpm lint:check` | 最终测试修订后完整重检退出 0。 |
| `NODE_OPTIONS=--max-old-space-size=4096 pnpm exec vitest run src/components/account/__tests__/ClaudeResetCreditsCell.spec.ts src/components/account/__tests__/ClaudeResetCreditsCell.locales.spec.ts src/views/admin/__tests__/SettingsView.spec.ts --maxWorkers=2 --minWorkers=1` | 退出 0，3 文件、72 用例。 |
| `NODE_OPTIONS=--max-old-space-size=4096 pnpm test:run --maxWorkers=2 --minWorkers=1` | 最终退出 0，353 文件、2660 用例；182.79 秒。 |
| `NODE_OPTIONS=--max-old-space-size=4096 VITEST_MAX_THREADS=2 VITEST_MIN_THREADS=1 VITEST_MAX_FORKS=2 VITEST_MIN_FORKS=1 pnpm build` | 退出 0；实际运行 i18n 3 用例、vue-tsc 和 Vite；Vite 构建 33.91 秒。 |

首轮完整前端测试为 352 文件通过、1 文件失败：连续确认测试第二次查找确认按钮时未找到确认框；单独复核相关 72 用例全部通过。测试 helper 增加交互前的异步状态等待后，重新完整执行得到上表最终结果。没有改动 reset 产品代码或降低原断言；该记录不推断尚未证实的生产根因。

最终覆盖包括设置 43、账号创建 41、编辑 77、批量编辑 56、credentials builder 70、UseKeyModal 30、reset 原测试 27 与真实双语 2 用例。白名单回归检查合法 ID 去重、修改/清空、API string、重载和旧 Grok 设置保留。

构建产物位于已忽略的 `backend/internal/web/dist`，无已跟踪生成物变更。Vite 提示浏览器数据较旧及部分 chunk 超过 500 kB，不是构建失败，本次未修改依赖或拆包策略。

日志：`/tmp/sub2api-merge-frontend-tests.log`、`/tmp/sub2api-merge-frontend-focused.log`、`/tmp/sub2api-merge-frontend-tests-final.log`、`/tmp/sub2api-merge-frontend-build.log`、`/tmp/sub2api-merge-frontend-lint-final.log`。

## V6

### 最终国际化对象

实际读取最终 en/zh 导出对象及全部本地 spread/override，与固定 main 结构化比较：两侧各 8453 个叶子键，main 各 8417 键；main 有效键丢失 0、en/zh 集合差异 0、main 新改有效文案被旧 override 遮蔽 0。

最终全量测试包含以下专项且全部通过：`localeKeyCompleteness` 3、`localesMessageCompile` 2、`localesNoKeyCollision` 6、`buildFeatureLocaleExtensions` 11。

`ClaudeResetCreditsCell.locales.spec.ts` 使用真实最终语言对象、vue-i18n 和官方 message compiler，仅 mock API 与对话框外壳；中英文按钮、不可撤销确认文案、窗口插值、取消不兑换均通过。Vitest runtime-only 别名需要测试配置官方编译器，未改变生产 CSP 或 i18n 配置。

## V7

### 部署资产与隔离 Redis

- dev/local/默认三个 Compose 的空密码 command 均成功解析。
- 复杂测试密码包括空格、引号、dollar、斜杠；Compose config 的双 dollar 是序列化转义，不是实际密码损坏。
- 将解析后的三个 Redis 服务分别创建为隔离项目 `sub2api-merge-redis-0211-0/1/2`：仅 Redis 7 Alpine，禁用网络、无主机端口、独立临时数据、64 MiB/0.25 CPU。
- inspect 实际 argv 与原测试密码一致；三个容器真实 `redis-cli PING` 均为 `PONG`，持久化选项 `save 60 1`、`appendonly yes`、`appendfsync everysec` 保留。
- 运行退出 0，finally 已逐个 down --volumes；按 project label 查询无剩余容器，未接入业务 Redis/DB。

以下项目资产命令均退出 0：

```bash
sh deploy/tests/docker-compose-security-test.sh
sh deploy/tests/docker-compose-gateway-env-test.sh
sh deploy/tests/docker-compose-simple-mode-env-test.sh
sh deploy/tests/docker-runtime-resources-test.sh
```

## V8

### 数据与跨层契约

以下隔离证据均包含在已通过的完整 backend unit；读取实际 test setup 确认使用 mock/miniredis/内存状态，没有真实数据库或供应商请求。

| 场景 | 实际证据 |
| --- | --- |
| 余额在途预占 | `billing_inflight_cache_test.go`：20 并发/余额只容纳 3 个、首次准入、幂等释放、TTL、开关/订阅、Redis 失败、计费窗口内连续请求及续期。 |
| 异步任务生命周期 | `gateway_inflight_reservation_test.go`、`billing_inflight_reservation_test.go` 及 usage worker pool 测试：引用交接、任务持有期间不提前释放、丢弃/panic/关停、续期和同步余额扣减。 |
| Key 创建限制 | `api_key_create_count_test.go` 固定窗口 miniredis；`api_key_service_create_limit_test.go` 活动/创建/自定义 Key/零值/删除不返还创建次数/计数或 Redis 故障分支。默认值与配置源码共同核对。 |
| 风控白名单 | `TestUpdateSettingsCyberAllowlistRoundTripAndImmediateRefresh`：解析/保存/GET/即时 cache 刷新；handler DTO 更新省略语义；前端保存重载证据见 V5。 |
| Claude 重置 | `claude_reset_redeem_test.go`：same key replay、同组织并发只能一次 post、未知结果 fence、marker 后崩溃不重发、上游错误净化；handler 合同与前端确认/取消在分层测试中验证。 |
| 路由与配置 | 实际 API -> DTO -> service -> repository/cache -> gateway 调用链和 UI -> 事件/表单 -> API 类型均完成源码核对；完整 server/handler/service unit 和前端组件回归通过。 |

这里的分层测试与静态调用链核对不等于已执行 UI + 真实 API + 真实数据库的全链路联调。

## V9

### 未覆盖与交付边界

- [上线后验证] 实际供应商/OAuth 会话的成功搜索、工具/推理档位及流式 usage：由发布负责人在测试账号和成本可控条件下验收；预期失败流不记成功搜索，日志与实际上游 usage/effort/tier 一致。本次未调用付费模型。
- [上线后验证] 生产 Redis 预占并发、故障/恢复和余额一致性：由发布负责人按现有监控及回退基线小流量观察；预期无提前释放或永久占用，故障行为符合开关策略。本地 mock/miniredis 与隔离容器已验证可在提交前验证的部分。
- [上线后验证] Claude 实际账号 reset 券状态与组织幂等：查询可先核对；真实兑换只在发布负责人明确许可及具备可用测试券时执行，预期一次确认只消耗一次。本次未消费真实重置次数。
- 本轮不执行提交、推送、数据库迁移或部署。完成 Check-All 后按交互流程停止；任务状态不提前标记 complete。

正式检查采用 `requested=auto / effective=full / confidence=high`；三个维度均完成，11 项 AC 均有上述证据，剩余 CHK/FBK 为 0。DOC-001 同步 PRD/实施计划/任务 notes 的状态事实，DOC-002 更新共同文件清单的实际复核状态；不改变需求、设计取舍或已确认 Brief。
