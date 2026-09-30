# 技术设计：main 0.2.11 合入 build

## 输入与边界

- 提交、统计和完整逐项方案以 `prd.md` 与 `research/approved-preview.md` 为准；使用 origin/main 的固定 SHA，保留 build 的历史和定制代码。
- 采用标准双父提交 merge 的工作树结果；实施阶段先创建 `backup/build-before-main-0211-9c914278c`，再执行未提交 merge。
- 只修改冲突、跨侧契约适配及发现的功能回归；定制策略继续由独立领域文件拥有，共享入口保持薄接入。
- main 本轮没有 migration、Ent schema/生成代码、Go 和 pnpm 依赖变更；无需迁移或形式化 Ent 重新生成。Wire 以构造器变化决定是否生成。

## 冲突处置

下表路径均相对 `backend/internal/`；详细冲突依据和验证场景见原预览。

| 文件 | 合并决定 |
| --- | --- |
| `handler/openai_gateway_handler.go` | 保留 main 预占、错误返回、defer 释放，再使用 build 粘性会话 helper。 |
| `pkg/apicompat/anthropic_to_responses_response.go` | 搜索独立累计输入；普通 function_call 引入 PendingToolInput，真实 delta 替代首事件待定值。 |
| `pkg/apicompat/chatcompletions_anthropic_bridge.go` | 依据实际上游协议投影 disabled；原生 OpenAI 采用 none，兼容上游保留 thinking=disabled 并清理互斥 effort。 |
| `pkg/openai/constants.go` | 新 GPT-6.1 Sol metadata 与 GPT-5.6、Astra 提示词、旧型号和未知型号回退共存。 |
| `service/gateway_forward_as_responses_test.go` | 保留 build 搜索 fixture/测试与 main 终态 usage 测试，保持独立函数边界。 |
| `service/openai_alpha_search.go` | 解析契约同时表达文本、结果、searched、error，同步所有调用方与测试。 |
| `service/openai_compat_model.go` | 保留派生 effort 返回契约，接入 GPT-6.1 后缀，显式 disabled 不被派生值覆盖。 |
| `service/openai_gateway_chat_completions_raw.go` | 新 GPT-6.1 限制与旧 GPT-6/Grok/Lite 策略按实际映射模型并存。 |
| `service/openai_gateway_messages_chat_fallback.go` | 保留 xai/openai import、新模型校验、build 直连 Chat 及流式折叠；补接现有 Ollama helper。 |
| `service/openai_gateway_responses_chat_fallback.go` | 接入新校验，删除多余的提前 effort 提取，保留最终出站提取和混合工具/搜索/reasoning 缓存/Ollama 路径。 |

## 关键契约与数据流

### 搜索

- 解析器使用明确的搜索证据与完成错误两个独立结果；优先四个返回值，不为单次使用增加复杂抽象。
- 独立 Alpha Search 转换及 `openai_alpha_search_responses_bridge.go` 都消费完整契约；后者原先第三返回值用于 `if searched`，必须同步。
- 只有有效成功完成事件才能成功返回；200、delta、DONE 不足以确认成功。没有实际搜索证据但完成成功时，按当前账号与本地模拟能力决定回退。
- 失败或截断流沿现有错误路径处理；无 URL 的有效模拟结果继续保留。计费和客户端最终事件由实际成功流程拥有。

### 推理、模型与出站参数

- 读取 `AnthropicRequest`、`ChatThinking`、模型 helper 及各入口的真实定义，再实现模型归一化与供应商投影；公共转换层不隐式选择账号策略。
- 原生 OpenAI 使用 disabled -> effort=none；GPT-6.1 Sol 的非法 none/minimal 按 main 契约拒绝。
- 需要 thinking 对象的兼容上游使用 disabled，清理互斥 effort；模型后缀和派生 effort 均不得重新开启显式 disabled。
- 模型映射、供应商策略、token 裁剪完成之后，从最终出站 body 提取日志和计费 effort/tier，避免中途值覆盖最终值。
- Messages -> Chat 补接 `clampOllamaCloudUpstreamMaxTokens`，复用实际账号和最终 body；不增加账号开关或复制裁剪算法。

### 预占、配置与生成代码

- HTTP、WS、内部搜索轮次、切号与异步计费共同检查 reservation 生命周期；请求结束不应提前释放仍有计费任务持有的引用，丢弃和 panic 也必须释放。
- `wrapUsageRecordTaskContext` 等返回契约跨生产调用与测试同步；`GetUserUsageTrend` 的 metric 参数、Wire 构造器及 stub/mock 同步。
- 新设置按表单 -> API 类型 -> DTO -> service 解析/保存 -> repository/cache -> gateway 的方向追踪，同时反查现有定制字段是否被整体覆盖。
- Claude 查询/重置保留上游权限、确认和幂等流程，隔离验证不使用真实额度。

## 自动合并复核与兼容性

- `research/shared-files-audit.md` 列出全部 71 个共同修改文件，冲突和自动合并分别记录实际处置及测试证据；预览检查不能当作合并后的通过证据。
- 领域场景由原预览第 4 节拥有，覆盖 Lite、HTTP/WS 路由、搜索、Schema/DeepSeek、图片/超时、计费、风控、重置、账号和国际化。
- 独立文件内容保留采用文件模式及 blob 比较；必要契约适配应记录为何偏离预览中的 197/716 个独立文件原值。
- locale 按最终导出对象验收，包括后置 spread/override，不仅比较主语言文件的源码 key。

## 验证与回退

- 优先复用现有单元、handler、组件和隔离 harness；按风险补失败分支与实际出站断言，再运行完整构建和检查。
- Redis 预占、API Key 限制及重置幂等在 mock、miniredis 或项目隔离 harness 中验证；不能接入业务数据库或真实供应商。
- 重任务串行执行并明确设置资源预算；本机 `wsl-safe-run` 当前仅透传命令，不附加资源限制。
- 未提交结果可在明确放弃合并、确认没有用户新增改动时正常 merge abort。提交后的回退采用 merge revert，不改写历史。
- 完成 Check-All 后展示通过、失败及未覆盖证据；提交、推送和部署由后续流程单独处理。
