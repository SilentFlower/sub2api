# main 0.2.17 与 cyber 修复合入 build 的预演方案

## 1. 结论与固定输入

已获取本仓库 `origin/main`，并把本地 `main` 快进到 0.2.17。当前仍在 `build`，没有实际 merge、cherry-pick、产品代码修改、提交、推送或部署。任务文档为本轮新增的唯一工作区内容。

| 项目 | 固定值 |
| --- | --- |
| build / origin/build | `f03c930bef832df4d49c90de0e895316e7ffc1fd`，版本 0.2.11 |
| main / origin/main | `8f2cbb53d9d1c40e1865b07ac460571f7d4c48de`，版本 0.2.17 |
| 共同基线 | `42bc7f6cffe24bcb471608e48e66b4a0afa1f882` |
| 双方独有提交 | build 285、main 326；含 merge commit |
| main 相对基线净变更 | 648 个文件：后端 447、前端 177、其他 24 |
| 共同修改 | 77 个文件：13 个文本冲突、64 个自动合并 |
| 文本冲突块 | 19 个 |
| main 假想合并树 | `2248d5cc8a5413b8f1b2c6a21905e81807bba0f3` |
| cyber 来源 | [xlplbo/sub2api#1](https://github.com/xlplbo/sub2api/pull/1) |
| cyber 固定增量 | `bf598c1641bf98d1df201d299447a2f378fdf966..14ba46df4fccf60a4f960befcc4c930d8d0cd7b1` |
| 来源 PR head | `f06f93c4e7b71d3e81c8a66d7e1dc4cb1b7d7014`；其相对上述修复提交无文件差异 |
| 叠加 cyber 的假想树 | `4d18d65a3c482a1d06d1a7c79d67c7c85d23becc` |

两棵预演树仍包含 main 合并的 13 个文件冲突标记，不能编译、发布或作为实施完成证据。cyber 补丁通过独立临时索引执行 `git apply --cached --3way`，11 个文件全部应用成功，没有新增冲突；没有使用当前工作区索引，也没有创建合并提交。

## 2. 合并策略

1. 先按固定 main SHA 完成 `main → build` 合并，再叠加 cyber 的 11 文件修复增量；不合入 xlplbo fork 的其他历史或功能。
2. main 统一协议路由、平台 profile、模型退役及新模型契约作为本轮新基础；build 的搜索、Lite、GIF、Schema、Grok、模型提示词、超时和计费定制逐项适配到新入口。
3. 不整文件选择 ours/theirs；同一业务语义只有一个实际 owner，不保留两份同名方法或两条重复路由。
4. 所有共同修改文件和新入口与旧私有 owner 的边界均进入回归矩阵；自动合并、文件保留和行为通过分别验收。
5. 本轮是方案阶段。真实合并完成后仍需构建、测试、静态检查及项目 Check-All，才能对覆盖到的行为给出通过结论。

## 3. 全部文本冲突及建议处理

下列路径均从仓库根目录起算。

| 编号 | 文件 | 块数 | 两侧行为及具体处理 | 验证组 |
| --- | --- | ---: | --- | --- |
| C01 | `backend/internal/handler/endpoint.go` | 1 | 注释冲突。保留 build 中文文档及 Grok 实际端点说明，补入 main 的 Gemini `/v1beta/cachedContents` 端点语义；同时复核已自动合并的端点常量、归一化和路由。 | V03、V09 |
| C02 | `backend/internal/pkg/apicompat/types.go` | 1 | `ResponsesContentPart` 同时保留 main 新增 `Refusal` 和 build 的 `Annotations`，保留缓存断点、图片、文件等既有字段及 JSON tag。拒绝信息和搜索引用均须在 stream/non-stream 转换后可见。 | V02、V04 |
| C03 | `backend/internal/service/account.go` | 2 | `IsModelSupported` 接入 main 的 `supportsOpenAIRequestModel → isModelMappingSupported`，保留 build 映射/透传实际行为；不能把新退役检查覆盖回旧函数。main 回添的搜索三态常量与方法已由 build 的 `account_websearch.go` 独立拥有：保留一个 owner，将 main 的 OAuth 排除及 API Key 范围与现有实现对齐。 | V01、V03、V04 |
| C04 | `backend/internal/service/account_websearch_test.go` | 1 | 保留 build 的 Grok/不支持平台场景，以及 main 的 OpenAI OAuth、Gemini 和 API Key 三态场景，整理为各自完整测试或等价表驱动测试，不删业务断言。 | V04 |
| C05 | `backend/internal/service/antigravity_gateway_gemini.go` | 1 | 先执行 main 的 `ensureGeminiFunctionCallThoughtSignatures`，再调用 build 的 `wrapV1InternalRequestWithGIFCompatibility`。既保留 functionCall/thoughtSignature，也保留 GIF 抽帧、资源上限和包装后请求大小校验。 | V06 |
| C06 | `backend/internal/service/gateway_forward_as_responses.go` | 1 | 保留 build 上游内容块索引到实际数组位置的映射及 citation 累积，接入 main 的负索引防护；对负 upstream index 先拒绝/忽略，再执行映射，不能退回直接使用 upstream index 下标。 | V02、V04 |
| C07 | `backend/internal/service/openai_gateway_chat_completions.go` | 3 | 采用 main 的 provider/profile 统一分流，移除被其替代的 OpenCode 专用旧调用；保留 Grok 专用桥接、Schema 预处理、推理回注和 search 保护。请求形状只判定一次；所有选中 raw Chat 的 Responses 形状请求均须转换，不能只覆盖 adaptive/按模型分流账号。转换复用 main 的 `WithOptions` 保留 reasoning cache，同时保留 build 对无法执行 web_search 的明确拒绝。main 的结构化 input 拒绝检测及安全重试逻辑独立保留，不与 build helper 拼成一个函数。 | V02、V03、V04、V05 |
| C08 | `backend/internal/service/openai_gateway_messages.go` | 1 | 接入 main 的原生 Anthropic / Chat / Responses switch；把 build 的 Grok 强制 Chat 行为以专属薄分流接入，避免被统一路由漏掉。保留 main 新增纯搜索模拟入口、OAuth 不模拟限制和 build 的 Schema/工具清洗及稳定会话链路。 | V03、V04、V05 |
| C09 | `backend/internal/service/openai_gateway_messages_chat_fallback.go` | 1 | 使用 main 的 `AnthropicToChatCompletionsRequestWithMinOutputTokens` 和配置下限；保留 build 直接桥接、显式关闭推理优先级、最终 effort 计费、上游 SSE 折叠和 Ollama token 裁剪。 | V02、V05、V07 |
| C10 | `backend/internal/service/openai_gateway_responses_chat_fallback_test.go` | 2 | imports 取需要项的并集；分别保留 build 的写失败测试 writer/搜索回归与 main 的原始开始时间回归。测试调用同步带 `startTime` 的新签名，避免首 token 延迟从 fallback 时重新起算。 | V02、V04、V07 |
| C11 | `backend/internal/service/openai_image_generation_controls_test.go` | 1 | 保留 build 的 `tool_choice=none → auto` 桥接测试与 main 的非 OpenAI 原生 Responses 不注入 hosted 图片工具测试，两个测试函数分别闭合。 | V08 |
| C12 | `frontend/src/components/account/EditAccountModal.vue` | 3 | 保留 build 的独立搜索字段、Codex 搜索桥及兼容 extra helper；吸收 main 的平台清单、Vertex/API Key、显式缓存与临时不可调度 scope。三态只解析一次；保留 JSON Schema、Lite、Alpha Search、图片 URL、自动重置等原字段的加载/保存/清除。保留 main 对运行态字段禁止回写的规则，跟随原文件空格缩进。 | V10、V11、V12 |
| C13 | `frontend/src/components/account/__tests__/EditAccountModal.spec.ts` | 1 | 保留 build 的 Lite/Alpha Search suites 和 main 的 Gemini 显式缓存/规则 scope suites；逐个完整闭合 describe/it，复核 mock reset 和简单模式 fixture 隔离。 | V10、V11 |

## 4. 无文本冲突处的语义复核重点

77 文件的完整清单及验收分组见 [shared-files-audit.md](shared-files-audit.md)。其中 64 项自动合并也必须在实施时逐项标记验收结果。

| 领域 | 本轮组合风险和保留要求 |
| --- | --- |
| 统一协议路由与平台清单 | main 新增 TypeSafe、Command Code、Cline 及统一 profile；要验证路由、模型目录、映射、额度和 Header Override 全链路。build 的 Grok 强制 Chat、国产原生 Responses 与 Schema 路径不能被旧单一平台判断挡住。 |
| Lite 与模型映射 | main 新增 OAuth 历史 `web_search_call` 工具声明和 configured min-output 归一化，build 已有 Lite 降级、additional_tools、namespace、最终模型阻止列表及原始入站图片桥接限制。HTTP、WS、WS→HTTP 每条路径都要验证，不能因先删 Lite 标记重新开放图片桥。 |
| GPT-5.5 与目录 | main 会按最终目标排除 ChatGPT OAuth/影子账号的退役 GPT-5.5，但 API Key 上游仍可用；build 的 GPT-5.6、Astra 提示词和显式映射应保留。目录、调度、compact 和实际出站一致。 |
| 搜索与拒绝内容 | build 的 Alpha Search→Responses、本地 AnySearch、混合工具、web.run、citation、预算及错误处理仍有效；main 的 OpenAI `/v1/messages` 纯搜索模拟不能重复执行、错误计费或作用于 OAuth。新增 refusal 字段也不能丢失。 |
| 长连接计费与严格客户端 | main 新增后续 turn 使用当前分组倍率、图片输入 token 和 `created_at`/sequence 补齐；与 cyber 单次审计、异步预占释放、build 的最终 effort/tier 统计共同验收。 |
| Gemini / Antigravity | 4 Argon、Haiku 5.5、Vertex API Key、显式缓存、缓存绑定账号/存储计费/并发修改恢复、客户端断开计费及负 input 修复；与 GIF、工具图片、思考签名、模型映射及隔离限流组合验证。 |
| 账号与系统设置 | 新平台配置、缓存 TTL、充值阶梯、省略字段/空数组语义与 build 的 Lite、Schema、搜索、UA、超时、GIF、Grok 等设置共同保存并重读，不回写旧运行态。 |
| 生图下载与对象存储 | main 的异步下载公网限制、代理/重定向及 MIME 检查必须保留；build 的同步图片桥接、下载回填、独立响应头超时仍正常。 |
| 支付、认证和页面状态 | main 独立变更虽然逐 blob 保留，仍需支付赠送/折扣/退款幂等及回调验签、删除 Key 后结算、邮箱验证码原子校验、并发 step-up 和过期请求 UI 回归。 |
| 国际化与构建资产 | 复核最终 locale 聚合与后置 override，而非只检查冲突文件；保留 build CI、品牌 SVG、工程资料和容灾资产，更新 Go/依赖并重新检查资源构建。 |

## 5. cyber 修复的效果和边界

- 修复源移除 transcript 推断和 `transcript_lookup_limit_exceeded` 分支，只按认证 API Key ID + 明确会话请求头派生 v2 哈希。
- 对同来源、不同会话、600 条工具历史的回归测试已存在于补丁中；同一实际命中会话、不同 Key、无 ID、缓存 key、旧记录和存储失败也有定向测试。
- 没有明确会话 ID 时不做跨请求本地封禁，仍由上游审核、拒绝并留痕；同一 WS 连接的已命中阻断保留。
- 共享 Key 的网关须传递各下游会话稳定且不重复的 ID；复用同一明确 ID 的不同下游会话仍会被视为同一会话。
- v2 不继承旧 Redis 封禁，不删除审计或旧记录；全部旧实例替换后才能消除旧实现的误拦。
- 本次仅核对代码、测试内容与补丁应用；没有把来源 PR 自述的测试或部署结果当成本地已通过证据。

## 6. 已执行的预演检查

1. `git merge-tree --write-tree --name-only <build> <main>`：13 文件、19 块冲突；未使用当前索引或工作树。
2. 通过独立临时索引叠加来源 PR 的 11 文件修复，返回码 0、无新增冲突；记录在 `patch-apply.txt`。
3. 按 mode/type/blob 比较独立变更：571 项 main-only 全部与 main 一致；728 项 build-only 在 main 假想树全部与 build 一致；叠加修复后仅 `.gitignore` 按已知补丁变更。见 `independent-retention.json`。
4. 实际加载最终 en/zh locale 导出及全部本地 spread：各 8534 个叶子键，集合差异 0；main 有效键丢失 0；未发现本轮 main 新改文案停留在旧 build 值。cyber 新说明已出现在最终对象。见 `locale-preflight.json`。尚未执行组件渲染/完整翻译调用检查。
5. 定价 JSON 结构比较：main 新增 Haiku 5.5、Gemini 4 Argon 两项，修改 Gemini 3.6 Flash；build 的三项 GPT-5.6 定价也保留，双边偏差均为 0。见 `pricing-preflight.json`。
6. 检查旧函数引用：OpenCode 旧分流调用仅残留在待处理冲突块，需随统一路由改掉；`defaultImageDownloadHTTPClient` 实际已改为 `sync.OnceValue` 返回的可调用变量，不是缺失符号，不作为缺陷。
7. 检查迁移：7 个新增 SQL；旧迁移没有修改。相同数字前缀使用不同完整文件名，不重命名任何上游迁移。

## 7. 实施前必须知悉的变化

- 工具链从 Go 1.27.0 升到 1.27.2，CI lint 使用 v2.14.0；前端有 Axios/Vue 等安全更新，需按新 lock 安装并回归。
- 7 个新增 SQL 涉及订单 bonus、TypeSafe/平台约束、分组 i18n 和 Gemini 缓存资源。实际应用启动可能自动迁移；本轮不启动应用连接业务数据库。
- 新增输出 token 下限配置默认 128、最小 16；要验证自定义值在所有转换入口生效，且不覆盖 Ollama 的最终上限。
- GPT-5.5 的 OAuth/影子账号退役及订阅长上下文计费变化是 main 的明确行为变化，不能为了旧断言继续放行。
- 仅在所有回归门槛通过后才评估交付；真实供应商、生产并发和数据库上线后的行为不能由静态文件保留保证。

## 8. 后续执行和回退

用户确认方案后，重新核对固定 refs、产品代码和索引，创建原 build 的恢复引用，执行未提交 main 合并，逐块适配并应用限定 cyber 补丁。完成 77 文件检查、定向/全量前后端检查和隔离迁移验证后，展示实际结果，再按交付流程处理提交/推送。

未提交时以正常 merge abort 回退，前提是没有夹带用户新增改动；提交后用正常 revert 保留历史。数据库只允许 forward migration，代码回退不能被描述成数据库已回退。推送 build 会触发仓库镜像构建发布流程，不属于本轮方案授权。
