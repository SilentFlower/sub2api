# 合并 main 并配置 Responses Lite Header 模型策略

## Goal

在不丢失 `build` 私有能力和 `main` 新增功能的前提下完成 `main -> build` 合并，逐项记录并解决 12 个冲突文件；同时替换旧的“Responses Lite Header 永不透传”策略，新增系统级模型列表，使网关根据最终上游模型决定是否转发 `X-OpenAI-Internal-Codex-Responses-Lite` 及其 WebSocket 等价 metadata。

## Background

- 2026-07-16 已将 `origin/main` 从 `d515c304` 更新到 `b960ec19`，本地 `main` 已快进；当前 `build` 基线为 `934982ae`。
- 已创建回退分支 `backup/build-before-main-0157-934982ae`，并执行 `git merge --no-commit --no-ff main`；合并尚未提交。
- 合并产生 12 个冲突文件，涉及 Wire 依赖注入、Grok quota、OpenAI 生图/Responses Lite/WebSocket、测试和账号创建/编辑弹窗。
- 在用户要求先确认方案前，工作区曾手工移除 11 个冲突文件的 marker，但未 `git add`、未提交；`backend/cmd/server/wire_gen.go` 仍处于冲突状态。任务实施前必须重新核对这些工作区改动，不得把它们视为已批准结果。
- 旧任务 `07-15-fix-codex-responses-lite-image-bridge` 基于当时的 CPA 行为和 `gpt-5.4-mini` 报错，决定所有 OpenAI HTTP 上游都剥离 Lite Header，并将该结论写入协议规范。
- 本轮核对官方 `openai/codex` 最新源码后确认：`use_responses_lite=true` 是模型能力，官方客户端会改变请求布局，并在 HTTP/Compact 请求发送 Lite Header；WebSocket 使用 `client_metadata.ws_request_header_x_openai_internal_codex_responses_lite=true` 表达同一请求级协议模式。
- 官方当前模型目录中 `gpt-5.6-sol/terra/luna` 为 `use_responses_lite=true`，`gpt-5.5`、`gpt-5.4`、`gpt-5.4-mini` 为 `false`。因此“永不透传”和“无条件透传”都不正确。
- PR `#4380` / commit `56650d6a` 只补齐 Lite 请求的 `reasoning.context=all_turns`，没有新增模型能力判断，也没有改变 Header 透传；其 HTTP 测试使用 Lite-capable 的 `gpt-5.6-terra`。
- `/root/project/CLIProxyAPI` 已快进到 `09da52ad`。其 HTTP executor 只用 Header/metadata 阻止重复注入 hosted `image_generation`，没有把入站 Lite Header 加入 OpenAI 上游 Header；直连 WebSocket 会保留 metadata。

## Confirmed Technical Constraints

- 策略判断必须使用完成账号模型映射和上游归一化后的最终模型，不能只看客户端请求模型。
- HTTP managed、HTTP passthrough、WebSocket 直连和 WebSocket HTTP bridge 必须使用同一模型策略；WS metadata 是 Lite Header 的请求级等价物，不能遗漏。
- 配置读取位于网关热路径，必须使用现有 `SettingService` 进程内 TTL 缓存和 singleflight 模式，不能逐请求查库。
- 系统设置已有 JSON 字符串数组持久化模式；模型规则已有精确匹配和末尾 `*` 前缀通配符语义，可复用 `matchModelPattern`。
- 客户端 `image_gen` 与 hosted `image_generation` 属于不同执行域；本任务不得重新引入重复工具、覆盖客户端明确 `tool_choice` 或绕过 group/账号/compact/Spark 门禁。
- 只有 Lite 请求才受该列表影响；普通请求不得因为模型命中而新增或删除无关 Header/body 字段。
- 删除 Lite Header/metadata 不天然等于完整的非 Lite 协议转换。已确认采用有限兼容降级：最终模型命中阻止列表时，只删除 HTTP Header/WS metadata，并跳过网关对 `reasoning.context=all_turns` 的强制补齐；不改写客户端已有的 developer message、`input.additional_tools`、`parallel_tool_calls` 或其它请求体字段，不承诺完整 Lite -> 标准 Responses 转换。

## Requirements

### R1. 合并与冲突决策

- 保留 `main` 新增的安全审计、异步生图、计费、网关、Grok 上游配置、前端设置和其它无冲突更新。
- 对 12 个冲突文件逐项记录 ours、theirs、最终选择和理由，不使用整文件 `ours/theirs` 覆盖真实业务冲突。
- 复核冲突周边的自动合并文件和构造器调用链，避免只清除 `UU` 却静默丢失 `main` 或 `build` 的跨文件能力。
- `wire_gen.go` 必须由最终 ProviderSet 和构造器签名重新生成，不手工拼接生成文件。
- 冲突解决后不自动创建 merge commit；先完成任务规定的质量检查和用户确认。

### R2. 系统配置

- 新增系统设置 `openai_responses_lite_header_blocked_models`，API 类型为 `string[]`，持久化为 JSON 数组。
- 设置键缺失时使用非空默认列表：`gpt-5.4`、`gpt-5.4-mini`、`gpt-5.5`。默认值仅使用精确模型名，避免未来支持 Lite 的同前缀模型被通配符误拦截；管理员仍可显式配置末尾 `*` 通配符或保存空数组。
- 更新请求未携带该字段时保留现值；显式提交空数组时必须持久化为空，不能回退默认列表。非法存储 JSON 在运行时记录告警并回退默认列表。
- 后台设置页提供模型规则列表编辑入口，使用中英文 i18n，不在组件中硬编码用户可见文案。
- 每条规则 trim 后不能为空；去重并保持稳定顺序；支持精确模型名和仅末尾 `*` 的前缀通配符。
- 设置保存后按现有缓存失效/TTL 机制生效，不需要重启服务。

### R3. 运行时策略

- 默认行为从 build 的“全部剥离”改为：最终上游模型未命中阻止列表时，Lite 请求向 OpenAI 上游透传 Header/metadata。
- 最终上游模型命中阻止列表时，不转发 HTTP Lite Header；WebSocket 直连不得继续发送等价 metadata；WS HTTP bridge 不得从 metadata 重建 Header。
- 最终上游模型命中阻止列表时，跳过 PR `#4380` 引入的 `reasoning.context=all_turns` 强制补齐；若客户端原始请求已经显式携带该字段，则保持客户端值，不主动删除或改写。
- 最终上游模型未命中时，保留 `main` 和官方 Codex 的 Lite 协议标记行为，并继续应用 PR `#4380` 的 `reasoning.context=all_turns` 归一化。
- 阻止列表只控制 Lite 标记传播和网关的 Lite 专属 `all_turns` 补齐，不承担 developer message、`input.additional_tools`、`parallel_tool_calls` 等字段的标准 Responses 降级；这些客户端原始字段保持原样。
- Grok 不得因为 OpenAI Lite 策略而收到该内部 Header；现有 Grok 平台门禁保持。
- 模型映射、failover 或 WS 会话内模型切换后，必须按该次 attempt/turn 的最终模型重新判断，不能复用陈旧的请求模型结论。

### R4. 生图与工具边界

- 保留 build 已验证的客户端 `image_gen` 可执行 namespace/扁平 function 识别。
- 已有客户端 `image_gen` 时不注入 hosted `image_generation`、不追加 hosted bridge 提示、不修改客户端明确的 `tool_choice`。
- 无任何图片工具且 hosted bridge 有效时，继续遵守 group、全局/频道/账号、`strip`、compact、Spark 和 passthrough 契约。
- 配置项不得改变独立 `/v1/images/generations`、`/v1/images/edits` 和批量图片 API。

### R5. 验证与记录

- 后端覆盖设置解析/更新/缓存、精确与通配符匹配、最终模型判断、HTTP managed/passthrough、WS 直连、WS HTTP bridge、模型切换和 Grok 排除。
- 前端覆盖设置加载、编辑、保存、空项校验和中英文 key 完整性。
- 回归覆盖 PR `#4380` reasoning context、客户端 `image_gen`、hosted 生图桥接、图片权限和计费。
- 更新 `.trellis/spec/backend/protocol-adapter-guidelines.md`，用本轮按最终模型配置的策略替代旧的“Header 永不透传”契约。
- 在 `design.md` 记录官方 Codex、PR `#4380`、CLIProxyAPI、旧 build 决策和本轮替代决策；在 `implement.md` 记录冲突文件解决顺序、验证命令和回滚点。

## Acceptance Criteria

- [ ] `main` 更新已合入 `build`，不存在未解决冲突或 conflict marker，且 `wire_gen.go` 与 Wire 源定义一致。
- [ ] 每个原始冲突文件都有明确的最终决策记录，工作区中先前未确认的手工结果已重新审查。
- [ ] 系统设置 API 和后台设置页可以读取、修改并保存阻止模型列表。
- [ ] 新安装或设置键缺失时返回 `gpt-5.4`、`gpt-5.4-mini`、`gpt-5.5` 三个精确默认项；管理员可保存空数组以显式取消所有默认阻止规则。
- [ ] 空列表时，Lite-capable 模型的 HTTP Header 和 WS metadata 默认透传。
- [ ] 最终模型精确命中或命中末尾 `*` 规则时，HTTP Header、WS metadata 和 WS->HTTP Header 均不进入上游。
- [ ] 最终模型命中阻止列表时，网关不强制新增 `reasoning.context=all_turns`；客户端显式提供的值及其它 Lite 请求体字段不被配置项改写。
- [ ] 使用 `gpt-5.6-terra` 的 Lite 请求保留 Header、`reasoning.context=all_turns` 和 Lite 工具布局。
- [ ] 使用配置中阻止的 `gpt-5.4-mini` 时不向上游发送 Lite 标记，且不会因该标记触发 `unsupported_value`。
- [ ] 模型映射和 WS turn 切换测试证明判断使用最终上游模型，而不是原始模型。
- [ ] 客户端 `image_gen`、hosted `image_generation`、group/账号/compact/Spark/passthrough 行为无回退。
- [ ] 后端定向测试、完整相关单测、前端 Vitest/typecheck/lint、Wire 生成检查和 `git diff --check` 通过；无法运行项有明确原因。
- [ ] 未在用户确认前创建 merge commit 或推送 `build`。

## Out Of Scope

- 不自动从 OpenAI 远端模型目录更新阻止列表。
- 不修改 CLIProxyAPI 或 OpenAI Codex 仓库。
- 不为客户端 `image_gen` 托管执行环境，也不保证其二次请求经过 sub2api。
- 不修改与本次 merge 冲突和 Lite 策略无关的 main 代码。
