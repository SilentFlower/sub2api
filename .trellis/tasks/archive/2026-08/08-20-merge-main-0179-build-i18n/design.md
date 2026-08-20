# 合并 main 0.1.179 到 build 并保护现有功能与 i18n - 技术设计

## 1. 合并策略

在 `build` 上固定合并前 HEAD 和 `origin/main` 后执行：

```bash
git merge --no-commit --no-ff origin/main
```

合并阶段只解决当前 5 个硬冲突文件和由验证证明的语义回归。共享入口以 main 的最新公共
结构为基底，build 私有规则继续由现有领域文件拥有；不把 build 逻辑重新内嵌回上游共享
大文件。merge commit 前可通过 `git merge --abort` 回到合并前 HEAD。

## 2. 硬冲突矩阵

| 文件 / hunk | 处理方案 | 必须保留的契约 |
| --- | --- | --- |
| `openai_gateway_chat_completions.go` imports | 同时导入 build 使用的 `bytes` 与 main 使用的 `bufio`，其余 imports 交给 gofmt。 | build 的 Responses 形状转换和 main 的 buffered stream failover 均可编译。 |
| `openai_gateway_chat_completions.go` raw Chat 分流 | 采用合并后 main 的 `shouldForwardOpenAIResponsesViaRawChatCompletions(account)` 作为统一 predicate；命中且 `isResponsesShape` 时先调用 build 的 `convertResponsesShapeToRawChatBody`，再进入 `forwardAsRawChatCompletions`。 | main 的 CN adaptive/固定协议路由、OpenAI API Key 探测，以及 build 对 Cursor/Responses 形状请求的完整转换同时生效。 |
| `openai_gateway_messages.go` | 保留 main 已自动合入的 `IsAnthropicProtocol() || IsAdaptiveAPIProtocol()` 原生入口；后续使用 build 的 `ShouldForwardAnthropicMessagesViaRawChatCompletions(account)`。 | adaptive 走原生 Anthropic；OpenAI API Key、CN 固定 Chat 和 Grok 显式 force-chat 继续走 raw Chat。 |
| `CreateAccountModal.vue` imports | 合并 build 的领域组件/类型 import 与 main 的 `allSelectedGroupsEnableLongContextPricing`，并保留 main 自动合入的 adaptive helpers/types。 | Codex 自定义 UA、OpenAI Compatibility、Grok Force Chat、Web Search、自适应协议和长上下文 UI 同时存在。 |
| `CreateAccountModal.grok.spec.ts` | 保留 main 的 API Key placeholder 断言。 | Grok 表单使用最终 `apiKeyValuePlaceholder`，不退回脆弱的旧源码形态断言。 |
| `CreateAccountModal.spec.ts` hoisted names | 同时声明 `getWebSearchEmulationConfigMock` 和 `authIsSimpleMode`。 | build Web Search 设置读取与 main 简单模式 UI 测试同时工作。 |
| `CreateAccountModal.spec.ts` hoisted values | 同时初始化上述 mock 与状态对象，保留现有 store/admin API mock 接线。 | 测试运行时无未定义变量，双方用例均可执行。 |

## 3. 自动合并语义复核

共同基线三方集合固定为：

```text
build-only: 688 路径
main-only: 163 路径
both-changed: 47 路径 = 5 个硬冲突 + 42 个自动合并热点
```

复核规则：

1. build-only 路径的合并后 blob 必须等于合并前 HEAD；main-only 路径必须等于
   `origin/main`。
2. 47 个 overlap 按 `base -> build`、`base -> main`、最终索引三方比较，不只看冲突标记。
3. 优先检查改动量最大或同时承载私有能力的热点：
   - `openai_gateway_responses_chat_fallback*`
   - `openai_ws_http_bridge*`
   - `chatcompletions_responses_bridge.go`
   - `openai_gateway_passthrough.go`
   - `openai_gateway_grok*`
   - `account.go`、账号 Create/Edit/Bulk 表单和测试
   - `billing_service.go`、`ChannelsView.vue`、Wire 与 migration runner
4. 自动合并若让共享入口重复维护 build 规则，优先迁回现有领域 owner；只在循环依赖或
   中央 DTO/ProviderSet 不可拆分时保留最小中央接线。

## 4. 功能契约

### 4.1 OpenAI / Responses / Messages

- 保持前次 0.1.178 合并建立的最终模型映射 owner：普通 Responses、API Key 透传、
  compact 和 Responses-to-Chat fallback 继续按各自契约解析。
- 吸收 main 的 Responses input token 预检、reasoning content 回注缓存、客户端工具保留、
  buffered read failover、WebSocket later-turn 429 恢复和容量恢复。
- 保留 build 的 Responses Lite Header、Web Search、DeepSeek 缺失 reasoning 降级、
  Codex 自定义 UA、生图模型/effort 和 Grok Messages force-chat。

### 4.2 CN Provider / Account Form

- `api_protocol=adaptive` 按入站协议选择原生端点，`api_base_urls` 保留每协议地址；Kimi/GLM
  没有原生 Responses 时仍回退 Chat，DeepSeek 可使用原生 Responses。
- 创建、编辑、批量编辑同时保留 build extra 字段和 main 的 adaptive、header override、
  long-context billing；复制现有 credentials/extra 后只增删目标键，禁止覆盖其它配置。
- Wire 结果同时保留 build 的 Codex reset service 注入和 main 的 monitor fetcher config 注入。

### 4.3 Migration

- runner 以完整 `filename` 作为 `schema_migrations` 主键，并通过 `sort.Strings` 排序；
  同号不同名文件可以独立记录和执行。
- `226_channel_monitor_quota_mode.sql` 已存在于共同基线，不属于本次 build/main 冲突；
  main 的 `226_add_usage_log_effective_model_indexes_notx.sql` 按完整文件名排在其前。
- 不重命名、不合并、不修改任一已发布迁移；通过 migration unit/integration 测试验证。

## 5. i18n 策略

- main 本轮新增键：账号自适应协议、渠道 fast/flex 与输入输出/cache 倍率；修改
  `overview.longContextHint` 的最终语义。
- 合并后比较最终聚合的 en/zh 树，而非只看 6 个源文件；build 的
  `accountsCodexCustomClients`、`accountsOpenAICompatibility`、
  `accountsOpenAIImageGenerationOverrides`、Web Search、Responses Lite 等扩展继续成对注册。
- 当前搜索未发现 build 扩展覆盖本轮 main 新增键；真实 merge 后仍运行 key collision、
  message compile、build extension 和组件双语言最终文案测试。
- 若后置 override 与 main 新值同 key，更新领域 override 吸收 main 新语义，不能简单删除
  override 或接受旧值继续覆盖。

## 6. 验证与提交边界

- 先聚焦测试冲突文件和 42 个 overlap，再跑完整 unit/integration、前端全量和 lint/build。
- 全量测试失败先与合并前基线、目标 main 和单侧测试对比归因；只有证据表明是合并回归时
  才修改合并结果。
- merge commit 只在 `git ls-files -u` 为空、索引文件集合可归属、检查通过后由
  `trellis-push` 创建；必须验证两个父提交顺序和 `origin/main` 祖先关系。
