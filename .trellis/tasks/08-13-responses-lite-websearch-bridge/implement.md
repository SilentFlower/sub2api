# 实施计划：Responses Lite Web Search 内部桥

## 1. 基线与上下文

- [x] 读取 `implement.jsonl`、三件套与研究记录，确认本任务只修改 HTTP Chat fallback。
- [x] 运行现有 Responses Chat fallback、typed Web Search、`web.run`、Lite tools 与前端 channel feature 定向测试，记录基线。
- [x] 实现前再次核对 `Account`、`Channel`、`APIKey`、Responses/Chat tool DTO 和 EditAccountModal/ChannelsView 实际字段。

## 2. 后端策略 owner

- [x] 新增 `backend/internal/service/codex_web_search_bridge.go`，定义 `codex_web_search_bridge` 常量、账号/渠道 override 和服务级有效值解析。
- [x] 复用现有 bool/platform override helper、官方 Codex 判定、Lite Header parser、API Key context、Web Search 三态与 provider readiness，不重复解析规则。
- [x] 为账号优先级、渠道继承、默认关闭、非法类型、非目标账号和 provider readiness 增加单测；空 manager、空 provider 集合、缺失 API Key、过期 provider 和可用 provider 均已覆盖。所有新增公开方法已补完整中文 Javadoc 风格注释。

## 3. Chat fallback 隐式注入

- [x] 在 `forwardResponsesViaRawChatCompletions` 中增加薄接线：独立扫描 `EffectiveResponsesTools` 识别显式 typed Web Search，并结合转换后的 `web.run` 判断；不能用 nullable typed config 代替存在性，只有不存在显式 Web 工具时才评估隐式桥。
- [x] 按 absent/auto/required/none/forced-other 矩阵保持 `tool_choice`；`required` 仅在已有客户端工具时允许桥接。
- [x] 使用默认 typed internal config 调用现有 `addOpenAIResponsesTypedWebSearchTool`，保持固定名称冲突 400 和 `parallel_tool_calls=false`。
- [x] 把隐式配置加入现有 `internalWebTools` 并复用 `forwardResponsesViaWebRunChatCompletions`，不新增 provider、循环、writer 或计费路径。
- [x] 增加安全决策日志，不记录查询全文、结果、body 或凭据。

## 4. 后端行为测试

- [x] 覆盖生产形态：官方 Codex + Lite Header + 无显式搜索工具 + OpenAI APIKey Chat fallback，确认首轮 Chat tools 包含 `sub2api_web_search`。
- [x] 覆盖模型不搜索：一次 Chat、provider 0 次、`WebSearchCalls=0`、其它工具与普通文本正常回程。
- [x] 覆盖模型搜索：单轮/多轮、同 call ID、同账号续跑、流式/非流式 `web_search_call`、来源、annotation、usage 与计费。
- [x] 覆盖桥关闭、搜索策略关闭、全局/provider 不可用、非 Lite、非 Codex、原生 Responses、compact、显式 typed/`web.run`、choice 变体和名称冲突；manager 为 nil、内部 provider 集合为空和 provider 过期均失败关闭。
- [x] 复跑 provider 失败、代理 failover、并行违规、查询/轮次上限、结构化输出、客户端断连与既有 Web Search 回归。

## 5. 前端功能模块

- [x] 在 `frontend/src/features/webSearch/codexBridge.ts` 实现账号三态和渠道配置读写，保持未知 `extra`/`features_config` 字段不丢失，并补相邻 Vitest。
- [x] 新增渠道桥接 Toggle 和账号桥接 Field；使用 typed model、i18n、可访问 label/disabled 状态，不硬编码文案。
- [x] 在 `ChannelsView.vue` 增加最小字段、回填、序列化和组件装配；仅 OpenAI、全局搜索可用时展示，但不因渠道 Web Search 默认值关闭而禁用，确保账号强制搜索策略仍可与渠道桥接组合。
- [x] 在 `EditAccountModal.vue` 为 OpenAI APIKey 增加继承/开启/关闭回填与保存；CreateAccountModal 保持继承渠道。
- [x] 新增独立中英文 Web Search bridge locale 扩展，并在 admin locale 稳定位置深层展开；补最终有效 key 和页面装配测试。

## 6. 质量验证

```bash
cd backend
gofmt -w internal/service/codex_web_search_bridge.go internal/service/codex_web_search_bridge_test.go internal/service/openai_gateway_responses_chat_fallback.go internal/service/openai_gateway_responses_chat_fallback_test.go
go test -tags=unit ./internal/service -run 'Test.*(Codex.*WebSearch|Responses.*WebSearch|WebRun|Responses.*ChatFallback)' -count=1
go test -tags=unit ./internal/pkg/apicompat -count=1
go test -tags=unit ./internal/service ./internal/pkg/apicompat -count=1
go test -tags=unit ./... -count=1
golangci-lint run ./...

cd ../frontend
pnpm exec vitest run src/features/webSearch src/views/admin/__tests__/ChannelsView.websearch.spec.ts src/components/account/__tests__/EditAccountModal.spec.ts
pnpm typecheck
pnpm lint:check
pnpm build

cd ..
git diff --check
```

实际文件名若调整，`gofmt` 只处理本任务修改的 Go 文件，不使用通配符改写无关文件。若仓库标准 lint 命令与上面不同，以 `backend/Makefile`、`frontend/package.json` 和质量规范中的现有命令为准。

实际验证结果：

- `go test -tags=unit ./... -count=1` 通过。
- `pnpm test:run` 通过：242 个测试文件、1639 个用例。
- `pnpm typecheck`、`pnpm lint:check`、`pnpm build` 和 `git diff --check` 通过。
- CI 固定版本 `golangci-lint v2.9.0` 默认构建标签全量运行 `run ./... --timeout=30m` 通过：0 issues。
- 默认构建标签下的 `TestFetchCodexModelsManifestDefaultClientVersion` 与空/过期 provider 桥接回归测试通过。

## 7. 风险与回滚点

- fallback 接线后先确认默认关闭时请求 body、Chat tools 和调用次数完全不变。
- 共享循环不得因隐式桥改变显式 typed Web Search/`web.run` 的 config、错误文本、来源或计费。
- 前端共享页面只留薄装配，避免与 `07-18-websearch-settings-thin-layer` 竞争 SettingsView 和 provider 配置文件。
- 无 migration 和数据变更；关闭账号/渠道桥开关即可运行时回滚，代码回滚恢复旧行为。
- 本任务不执行部署、镜像更新、容器重启或生产配置修改。
