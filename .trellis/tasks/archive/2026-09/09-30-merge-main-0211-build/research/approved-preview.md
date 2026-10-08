# main 0.2.11 合入 build 的预览方案

## 1. 当前结论与边界

已获取 origin 的最新代码，并将本地 main 快进到 0.2.11。当前仍在 build，工作区干净。
本轮只执行 fetch、Git 假想合并和源码分析，没有在 build 执行实际 merge、解冲突、提交、推送或部署。

| 项目 | 固定值 |
| --- | --- |
| 当前 build | `9c914278cef7cfe571120d19a92b7a75e5bfc6f8` |
| 最新 main / origin/main | `42bc7f6cffe24bcb471608e48e66b4a0afa1f882` |
| 共同基线 | `a3eb7ef302961cba716dc78b39b93b60c467db0e` |
| 版本 | build `0.2.8` -> main `0.2.11` |
| 待合入上游提交 | 126 个，包含 merge commit |
| 上游净变更 | 268 个文件，211 个后端、52 个前端、5 个其他文件 |
| 双方共同修改 | 71 个文件，其中 10 个文本冲突、61 个可自动合并 |
| 文本冲突块 | 13 个，全部在后端 |
| 假想合并树 | `f7cdacc5bb71f384f0434df112ef5dd5e2f93ba2`，仍含冲突标记，不能构建或发布 |

origin/build 未发生更新；当前 build 比 origin/build 多 3 个本地工程记录提交，实施时应保留。
上游来源沿用本项目既有的 origin/main；另一个 remote `other` 没有参与本次同步。

## 2. 总体合并原则

- 合入 main 的新增能力和修复，同时保留当前 build 的实际定制行为；不以旧任务清单恢复已经删除的功能。
- 逐块结合共同基线和双方调用链处理，不使用整文件 ours/theirs 代替业务判断。
- build 专属规则继续由独立领域文件拥有，共享入口保持薄接入。
- 新模型能力按 main 的模型契约接入，保留 GPT-5.6 专用提示词及当前 Astra 提示词选择；未知模型维持既有回退。
- 无文本冲突的共享文件同样复核入参、调用顺序、返回值、配置往返、生成代码和测试预期。
- Git 文件保留、编译通过、行为回归通过分别记录，不能相互替代。

## 3. 逐文件冲突处理

| 文件 | 冲突块 | 两侧差异 | 建议处理与验收 |
| --- | ---: | --- | --- |
| `backend/internal/handler/openai_gateway_handler.go` | 1 | main 在 Messages 增加余额在途预占；build 用独立 helper 稳定会话身份 | 保留 main 的预占、错误返回和 defer 释放，再调用 build 的 `resolveOpenAIMessagesSessionHash`。核对请求 context 和异步计费引用，测试成功、错误、切号、断连和粘性会话。 |
| `backend/internal/pkg/apicompat/anthropic_to_responses_response.go` | 1 | build 处理搜索输入 JSON；main 修复工具参数只出现在 content_block_start 时的丢失 | 搜索分支继续独立累计搜索参数；普通 function_call 接入 main 的 PendingToolInput 状态与 delta 优先规则。保留工具参数、搜索事件和引用测试，新增交错场景验证状态不会串用。 |
| `backend/internal/pkg/apicompat/chatcompletions_anthropic_bridge.go` | 1 | build 发送 thinking=disabled 且不发送 reasoning_effort；main 将显式 disabled 映射为 effort=none | 按最终上游协议处理关闭推理：原生 OpenAI 采用 main 的 none 语义；需要 thinking 对象的兼容上游保留 disabled 并清理互斥 effort。GPT-6.1 Sol 按 main 拒绝不支持的 none/minimal。联动归一化 helper，确保后续 model/effort 改写不会重新开启推理。 |
| `backend/internal/pkg/openai/constants.go` | 1 | main 新增 GPT-6.1 Sol 官方 descriptor embed，位置与 build 的回退说明重叠 | 加入新的 embed 和模型能力定义，保留 GPT-5.6、Astra 模板及旧型号/未知型号的回退顺序；核对目录和实际请求的有效提示词。 |
| `backend/internal/service/gateway_forward_as_responses_test.go` | 1 | build 新增搜索保留用例；main 新增终态 usage 归一化用例，位于同一插入位置 | 完整保留两套独立测试和 fixture，不把两段函数体拼进同一个函数；联合验证搜索引用和输入/缓存 token 统计。 |
| `backend/internal/service/openai_alpha_search.go` | 4 | build 的解析器第三返回值是 searched；main 改为 error，并要求有效 response.completed 才成功 | 统一解析契约，同时保留 searched 和 error，更新所有调用者。200、delta 或 DONE 均不能单独视为成功；完成成功但没有真实搜索证据时，保留 build 的本地模拟兜底。截断/失败的上游流不能作为成功搜索计费。 |
| `backend/internal/service/openai_compat_model.go` | 1 | build 的模型归一化返回派生 effort；main 为 GPT-6.1 Sol 增加后缀处理，签名没有返回值 | 保留派生 effort 的调用契约，接入新模型后缀规则，补齐所有 return；显式 output_config 与模型后缀的优先级保持一致，disabled 不得被派生 effort 覆盖。 |
| `backend/internal/service/openai_gateway_chat_completions_raw.go` | 1 | main 增加 GPT-6.1 Sol 的 Chat 工具调用限制；build 已有 GPT-6 Sol/Luna 和 Grok 的处理 | 保留新校验，并保持旧型号、Grok 和 Lite 的规则边界；必须按映射后的实际模型判断。覆盖 tools/functions、无工具、允许/禁止 effort 和映射别名。 |
| `backend/internal/service/openai_gateway_messages_chat_fallback.go` | 1 | build 需要 xai import，main 新增 openai import | 保留两个显式 import。除此之外还要复核自动插入的新模型校验与 build 的直连 Chat、会话粘性、Grok 额度、流式消费及非流式折叠。 |
| `backend/internal/service/openai_gateway_responses_chat_fallback.go` | 1 | main 新增 GPT-6.1 Sol 校验和入站 effort 提取；build 已在最终请求改写后提取 effort | 加入新模型校验；保留从最终出站请求提取计费 effort 的位置，避免同作用域重复声明 reasoningEffort，也避免入站值覆盖降级/改写结果。保留混合工具、本地搜索、reasoning 缓存及 Ollama token 上限。 |

### 搜索解析契约的具体影响

当前 build 专属调用在 `openai_alpha_search_responses_bridge.go` 中把第三返回值用于 `if searched`。
main 还新增了缺失完成事件、完成状态失败时的错误返回。只选 main 会使该调用无法编译；只选 build 会丢掉上游新增的成功完成检查。

建议解析器输出同时携带文本、结果、是否真实搜索和解析错误；适配独立搜索转换、build 的上游 Responses 搜索桥以及相关测试。
需要分别测试：真实搜索成功、成功但忽略搜索工具、本地模拟可用/不可用、上游失败、流截断、无 URL 的模拟搜索结果。

## 4. 无冲突区域的语义复核和验收

以下列出已定位的接入风险和实施时的检查范围，不代表合并后的行为测试已通过。

| 领域 | 复核范围 | 必须通过的组合场景 |
| --- | --- | --- |
| 余额在途预占 | 新增 reservation/cache 与现有 HTTP、WS、异步计费、模型映射和利润控制 | 并发准入、防重复扣费；计费落地后释放；任务丢弃/panic/关停释放；搜索内部多轮与切号；长请求续期；订阅、未定价及 Redis 故障的既有回退。 |
| 新模型与定价 | GPT-6.1 Sol、Sonnet 5.5、Astra Ultrafast；目录、别名、提示词、effort、缓存写入价和长上下文计费 | 客户端目录与实际出站一致；新模型的非法 effort 被拒绝；HTTP/WS 的限制及最终日志一致；Ultrafast 不与普通 Fast 倍率混用，OAuth 能力仍以上游目录为准。 |
| Lite 定制 | `openai_lite_mapped_gpt55.go`、可配置 blocked models、HTTP/WS 接入、原生降级开关 | 映射到 GPT-5.5 后去除 Lite 标记；重试到其他账号不污染入站数据；保留 additional_tools、namespace、工具历史；新模型目录不会绕过 blocked models。 |
| 搜索 | Alpha Search 经上游 Responses、本地 AnySearch、typed web_search、web.run、混合工具、预算耗尽 | 工具内部轮次不会泄漏为客户端工具；真实搜索证据和成功完成事件共同验收；失败/预算耗尽仍能按现有约定完成回答；请求 ID 和计费正确。 |
| 协议桥接 | Anthropic/Chat/Responses，多供应商模型及工具参数恢复 | 关闭推理优先级；首事件参数与后续 delta 不拼成双 JSON；缓存前缀、工具顺序及思维签名保持；stream/non-stream 的最终 usage 一致。 |
| Ollama 存量接入缺口 | 当前 build 的 Messages -> Chat 路径未调用已有 `clampOllamaCloudUpstreamMaxTokens`；不是本次新引入的问题 | 建议在本次合并中补接既有 helper，按账号规则裁剪最终出站 token 上限，并补实际出站断言；与 raw Chat、Responses -> Chat 保持一致。 |
| 图片与非流式超时 | 近期 build 增加的独立 header profile、图片桥接、配置主模型和 effort | 原始非流式请求即使被桥接为上游 SSE，也保持非流式超时 profile；图片请求使用图片 profile；保留后台配置 -> 环境变量 -> 内置值的模型优先级。 |
| JSON Schema 与 DeepSeek | 账号级 Schema 降级、当前 reasoning 回注/占位实现及新增 OpenCode Zen 判定 | Schema 开关生效且不破坏工具 Schema；有真实 reasoning 时不覆盖；仅符合当前上游判定的请求补占位。以当前代码为保留基线，不重建历史已移除的降级开关。 |
| 合成分组、白名单与 WS | 账号模型归属、通配匹配、首帧/后续帧、模型映射、上下文窗口 rollover | 白名单按公开模型准入，出站映射按目标模型；高级/旧调度都限制账号归属；后续帧不绕过准入；窗口切换后不错误延续旧 response 链。 |
| 风控设置 | `cyber_policy_user_allowlist` 的前端、DTO、设置解析/保存、缓存刷新和 HTTP/WS 调用 | 保存 -> 重读不丢用户 ID；白名单只关闭本地风控处置，上游限制仍保留；旧设置与 build 定制字段一起保存不互相覆盖。 |
| Claude 重置与账号配置 | 新增查询/手动兑换、路由、Wire、账号创建/编辑/批量修改、套餐 SKU、白名单和映射 | 查询不消耗；兑换需要明确确认且幂等；新套餐不改写旧 SKU；新建、编辑、批量操作保留 build 的 Schema/UA/搜索/Lite 选项。预览及本地验证不执行真实兑换。 |
| 前端国际化 | 组件调用、主 locale、独立扩展和最终 spread | 中英文最终键集合一致；静态调用不缺键；关键新增界面分别渲染 en/zh；后置 override 不遮蔽上游新语义。 |
| 工程与部署资产 | Wire 源/生成结果、三个 Redis Compose command、build CI、Trellis 和现有容灾资产 | 生成代码与构造器签名一致；Redis argv 保留持久化参数和正确的密码传递；build 定制镜像/资源配置和工程资产保持。 |

## 5. 值得提前知道的上游行为变化

- 余额在途预占默认开启，TTL 默认 900 秒；并发余额准入会改变。应验证它与 build 的映射、搜索、多轮计费一起工作。
- 用户创建 API Key 默认限制为 200 个活动 Key、每小时创建 60 次；这是上游新增业务限制，不应当作冲突消掉。
- 渠道没有显式配置图片价格时，现在继承模型目录价格；需要测试缺省、显式零价和自定义价，防止账单变化被误判。
- 新增 Claude 重置按钮会实际消耗重置次数；保留上游确认、权限和幂等规则。
- 本轮 main 相对共同基线没有修改数据库 migration、Ent schema/生成代码、Go 或 pnpm 依赖文件。没有新增迁移需要在本轮同步中执行。

## 6. 本轮已经执行的预览验证

1. `git merge-tree --write-tree --name-only build <固定 main>`：定位 10 个文件、13 个冲突块；没有修改当前索引或工作树。
2. 按 Git tree 的文件模式和 blob 进行独立变更核对：197 个 main 独立变更文件在假想树中与 main 一致；716 个 build 独立变更文件与 build 一致，差异均为 0。
3. 使用本地 TypeScript 编译器在内存中加载假想树的最终 locale：en/zh 各 8453 个叶子键，键集合一致；main 的有效键没有丢失；本次上游修改的已有文案未发现被旧 build 最终值遮蔽。
4. 追踪关键签名和引用：搜索解析返回值、模型归一化返回值、usage task 包装、用户趋势 metric 参数、Wire 构造器及 Lite/超时接入。
5. 最终 Git 状态确认：仍在 build，工作区干净，HEAD 未变化；本地 main 已更新。

这些结果是文件和部分契约的预检查。尚未运行解冲突后的 Go/Vue 编译、单元测试、lint、运行时场景或真实上游测试。
61 个无冲突共同修改文件仍需在实施过程中逐一完成语义验收，不能仅凭上述文件一致性宣称功能正常。

## 7. 实际合并的执行顺序

1. 固化这份范围和验收清单；重新核对 HEAD、固定 main 和工作区，若任何一侧已变化则刷新预览。
2. 为当前 build 创建 `backup/build-before-main-0211-9c914278c`；采用固定 main SHA 执行未提交 merge。
3. 按第 3 节逐项解冲突；同步调用方、测试、生成代码和必要的领域 helper。
4. 对 71 个共同修改文件逐一记录处置；核对第 4 节跨路径行为，包括 main 独立变更与 build 独立领域文件之间的新契约。
5. 先执行定向测试和构建，修复发现的问题；再运行后端全量 unit/lint/二进制构建，前端全量测试/typecheck/lint/生产构建。
6. 在隔离环境验证新增 Redis 预占、API Key 限制和重置幂等相关数据路径；不触碰业务数据库、不消费真实重置次数。
7. 复核合并树、版本、冲突标记、父提交、文件保留和最终 locale；完成项目 Check-All 和规范收尾。通过后展示实际合并结果与剩余未覆盖场景，再进入提交/推送流程。

### 验证命令

以下仅为合并完成后的执行清单，本轮没有执行这些构建与测试。各代码块分别从仓库根目录执行。

```bash
# 后端：先运行定向用例，之后执行完整检查
cd backend
go test -tags=unit ./...
golangci-lint run ./...
make build
```

```bash
# 前端
cd frontend
pnpm typecheck
pnpm lint:check
pnpm test:run
pnpm build

# 国际化专项
pnpm exec vitest run \
  src/i18n/__tests__/localeKeyCompleteness.spec.ts \
  src/i18n/__tests__/localesMessageCompile.spec.ts \
  src/i18n/__tests__/localesNoKeyCollision.spec.ts \
  src/i18n/__tests__/buildFeatureLocaleExtensions.spec.ts
```

构建与全量测试沿用本机已有的资源限制，重任务串行执行。
若需要调整 Wire 生成结果，使用项目已有生成流程；本轮没有 Ent schema 变更，不为形式重新生成 Ent。
测试失败时应区分新回归、上游有意变更导致的旧断言和存量问题；不能仅删除失败测试来通过检查。

## 8. 回退和交付边界

未提交合并期间的回退基线为备份分支对应的原 build SHA；仅在决定放弃合并且没有用户新增改动时使用正常 merge abort。
提交后的撤销应另行按合并提交进行 revert，不改写分支历史。
本轮不推送，因此不触发 build 的远端镜像发布流程；实际提交、推送或部署以之后确认的范围为准。
