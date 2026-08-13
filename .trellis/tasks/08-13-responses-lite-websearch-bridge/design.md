# 技术设计：Responses Lite Web Search 内部桥

## 1. 设计目标

让 Codex 保持 Responses Lite 时，在 OpenAI APIKey Chat fallback 链路中获得模型可选的 Web Search 能力。实现必须复用现有内部 Web 工具循环，保持其它客户端工具、Responses 流式/非流式回程、来源引用、usage 和计费语义，并能通过独立开关立即回滚。

## 2. 核心决策

- 新增独立配置键 `codex_web_search_bridge`，账号布尔覆盖渠道平台布尔；缺失默认关闭。
- 桥接开关只声明“允许 Codex Lite 隐式获得搜索工具”，实际执行还必须同时通过现有 `web_search_emulation` 策略和全局 provider 可用性检查。
- 不修改 Lite body，不注入 hosted `web_search`；只在 Responses -> Chat 转换完成后注入现有 `sub2api_web_search` function。
- 不新增搜索执行器、provider、计费或 Responses writer；复用 `openAIResponsesInternalWebToolConfig`、`addOpenAIResponsesTypedWebSearchTool` 和 `forwardResponsesViaWebRunChatCompletions`。
- 已显式声明 typed `web_search` 或 `web.run` 的请求继续走现有逻辑，隐式桥不参与。
- 本轮只支持 HTTP 普通 `/v1/responses` Chat fallback；WebSocket、compact 和 standalone alpha/search 保持原状。

## 3. 配置契约

新增持久化键：

```text
account.extra.codex_web_search_bridge: boolean
channel.features_config.codex_web_search_bridge.openai: boolean
```

有效值顺序：

1. OpenAI APIKey 账号存在布尔 `codex_web_search_bridge` 时直接使用。
2. 否则读取当前 API Key 所属渠道的 `codex_web_search_bridge.openai`。
3. 无 API Key 分组、渠道读取失败、字段缺失或类型非法时返回 `false`。

该值还要与以下条件做合取：

- `isOpenAIWebSearchEmulationEnabled(ctx, c, account)` 为真；
- `settingService.IsWebSearchEmulationEnabled(ctx)` 为真；
- `getWebSearchManager()` 非空。

不新增全局桥接配置，避免与渠道默认值形成第四层含糊优先级。运行时回滚可关闭账号覆盖、渠道桥接或现有 Web Search 策略任一项。

## 4. 资格判定

新增 build 私有领域文件 `backend/internal/service/codex_web_search_bridge.go` 作为策略 owner，共享 fallback 文件只做一次薄调用。建议新增以下公开/内部方法，并按项目要求补完整注释：

```go
func (c *Channel) CodexWebSearchBridgeOverride(platform string) *bool
func (a *Account) CodexWebSearchBridgeOverride() *bool
func (s *OpenAIGatewayService) isCodexWebSearchBridgeEnabled(ctx context.Context, c *gin.Context, account *Account) bool
```

隐式桥资格必须全部满足：

| 条件 | 结果 |
|---|---|
| 非官方 Codex 且未开启 `ForceCodexCLI` | 不注入 |
| Lite Header 不为真 | 不注入 |
| compact、WebSocket、非 OpenAI APIKey、原生 Responses | 不注入 |
| 独立桥开关关闭或现有搜索策略关闭 | 不注入 |
| 全局搜索未启用或 manager 不可用 | 不注入 |
| 已声明 typed `web_search` 或 `web.run` | 交给现有路径，不重复注入 |
| `tool_choice=none` 或明确选择其它工具 | 不注入 |
| `tool_choice` 缺失或 `auto` | 可注入 |
| `tool_choice=required` 且转换后有客户端工具 | 可注入 |
| `tool_choice=required` 且没有客户端工具 | 不注入，避免隐式搜索成为唯一强制工具 |

官方 Codex 判定复用现有 OpenAI header helper，并保留 `ForceCodexCLI` 兜底。Lite Header 使用现有 `isOpenAIResponsesLiteHeader` 解析，不自行比较字符串。

## 5. 请求数据流

```text
Codex Lite /v1/responses
  -> 现有 Lite ingress policy 与 namespace 处理
  -> OpenAI APIKey Chat fallback
  -> 解析 EffectiveResponsesTools 与 tool_choice
  -> 现有显式 typed web_search / web.run 决策
  -> 隐式桥资格判定
     -> 不满足：现有 Responses -> Chat 行为
     -> 满足：构造默认 internal web config
        -> 注入 sub2api_web_search
        -> parallel_tool_calls=false
        -> 进入共享内部 Web 工具循环
           -> 模型不搜索：一次 Chat 后直接写 Responses
           -> 模型搜索：provider -> tool result -> 同账号 Chat 续跑
        -> 标准 web_search_call + 最终 Responses 输出
```

默认隐式配置使用现有 typed 搜索能力：

- `Name=sub2api_web_search`；
- `Kind=typed_web_search`；
- `MaxResults=5`；
- `MaxRounds=2`；
- 不设置 allowed/blocked domains，因为客户端没有显式搜索工具可提供这些约束。

## 6. Chat fallback 接线

`forwardResponsesViaRawChatCompletions` 保持现有解析顺序：先读取有效工具和显式搜索配置，再转换 Chat request。显式搜索存在性必须独立扫描 `EffectiveResponsesTools`，不能使用 `typedWebSearchConfig != nil` 代替；纯 typed 搜索或 `tool_choice=none` 时配置可能为空，但仍然属于显式声明。转换后：

1. 扫描有效工具是否存在任何 typed `web_search`，并查找转换后的显式 `web.run`；已有声明继续装配现有 `internalWebTools` 或走既有能力路径。
2. 只有 typed `web_search` 与 `web.run` 都不存在时，才调用隐式桥资格 helper。
3. 资格满足时调用现有 `addOpenAIResponsesTypedWebSearchTool`；固定名称冲突沿现有 `400 invalid_request_error`、`param=tools` 返回。
4. 把默认配置加入 `internalWebTools`，沿现有逻辑关闭上游流式中间轮次并进入共享循环。

不修改 `ResponsesToChatCompletionsRequest`，避免让通用 apicompat 调用方都获得隐式搜索。也不增加 system/developer 提示注入，先复用现有工具名、description 和 JSON Schema，让模型基于工具声明自行决定。

## 7. 工具选择与回程

- 缺失/`auto`：模型可不调用搜索；未调用时 provider 和 `WebSearchCalls` 均为 0。
- `required`：只有已有客户端工具时才加入搜索候选，保持“至少一个工具”语义且不把隐式搜索变成唯一强制项。
- `none`/明确其它工具：不注入，不改写 choice。
- 同一轮内部搜索与客户端工具并行调用继续返回现有 502，禁止部分执行。
- 内部 `sub2api_web_search` call 必须由网关消费；客户端只看到标准 `web_search_call` 和原有 function/custom/namespace/tool_search 输出。
- 流式与非流式继续共用现有 source projection、最多 5 条来源和 rune 索引 annotation；结构化文本不追加来源后缀。

## 8. 前端装配

Build 私有 UI 放入 `frontend/src/features/webSearch/`：

- `codexBridge.ts`：账号三态归一化/写入、渠道配置读写的唯一 owner；账号 UI 三态为 `inherit|enabled|disabled`，持久化只写布尔或删除字段。
- `ChannelCodexWebSearchBridgeToggle.vue`：渠道 OpenAI 平台开关；全局 Web Search 不可用时不展示，文案明确桥接还要与渠道默认或账号强制开启的 `web_search_emulation` 同时满足。不能因为当前渠道搜索默认值关闭而禁用桥开关，否则会阻止账号级 `web_search_emulation=enabled` 与渠道桥接组合生效。
- `AccountCodexWebSearchBridgeField.vue`：OpenAI APIKey 编辑页的跟随渠道/开启/关闭选择器，并说明仍受 Web Search 策略与 provider 约束。

`ChannelsView.vue` 和 `EditAccountModal.vue` 只保留字段、组件装配和保存/回填调用。新增中英文文案放独立 Web Search locale 扩展，再在现有 admin locale 稳定位置展开，避免扩大与 main 及 `07-18-websearch-settings-thin-layer` 的冲突面。

不要求 CreateAccountModal 增加账号覆盖：新账号默认继承渠道即可，管理员需要例外时在编辑页设置。

## 9. 错误、日志与计费

- 配置关闭或请求不符合资格属于正常旁路，不返回错误。
- 代理名冲突使用现有 OpenAI 兼容 400；不动态改名。
- 注入前全局 provider 不可用时失败关闭、不注入；注入后 provider 临时失败继续回灌现有 `web_search_unavailable`/`web_search_failed` tool result。
- 新增能力决策日志只记录 `account_id`、模型、Lite、bridge、显式 Web 工具存在性、tool choice 和安全原因码，不记录查询、结果、body 或凭据。
- usage 累计所有实际 Chat 轮次；Web Search 只按 provider 成功查询数计费。模型未调用、参数失败、provider 失败或空结果不新增搜索费用。

## 10. 兼容与回滚

- 默认关闭，部署后所有存量账号/渠道行为不变。
- 关闭账号或渠道 `codex_web_search_bridge` 可立即停止隐式注入；关闭现有 `web_search_emulation` 或全局 provider 也会停止。
- 不修改数据库 Schema、Ent、migration、Codex 模型清单或客户端配置。
- 不改变显式 typed Web Search、`web.run`、直接模拟、原生 Responses、Anthropic 搜索、生图桥、Lite 工具归一化和 alpha/search。

## 11. 测试矩阵

- 策略单测：账号优先、渠道继承、缺失默认关闭、非法类型关闭、非 OpenAI/APIKey 关闭。
- 资格单测：官方 Codex/ForceCodexCLI、Lite true/false、compact、显式 Web 工具、choice absent/auto/required/none/forced-other、provider readiness。
- fallback 非流式/流式：上游看到内部工具、模型不搜索、单轮/多轮搜索、其它客户端工具、结构化输出。
- 错误：固定名冲突、并行违规、参数错误、provider 失败/不可用、代理 failover、查询/轮次上限、客户端断连。
- 计费与回程：usage 聚合、成功查询计费、`web_search_call`、来源去重/上限、Unicode annotation。
- 前端：账号 extra 读写、渠道 features_config 保留其它键、默认关闭、组件禁用/文案、ChannelsView 保存回填、EditAccountModal 更新。
- 回归：typed Web Search、`web.run`、直接模拟、Lite tools、生图桥、alpha/search、相关 apicompat 与完整后端/前端检查。
