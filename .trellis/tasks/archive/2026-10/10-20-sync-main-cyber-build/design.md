# 同步 main 0.2.17 与 cyber 会话隔离的设计

## 固定范围与依据

产品目标和验收由 `prd.md` 拥有。固定 Git 输入、13 文件/19 块冲突及处理表由 [预演报告](research/merge-preview.md) 拥有；77 个共同修改文件由 [逐文件清单](research/shared-files-audit.md) 记录。

本次来源为 `origin/main@8f2cbb53d` 和 `xlplbo/sub2api#1` 的限定增量。后者修复提交为 `14ba46df4`，最后一个 CI 触发提交没有文件差异。仅提取该修复的 11 个文件，避免把第三方 fork 的其他定制并入 build。

## 合并边界与顺序

1. 保持当前 build 历史，在用户审阅后以固定 main SHA 做未提交合并。
2. 先处理 main 的 13 文件冲突，并适配自动合并处的调用契约。
3. 在主线合并结果上应用固定 cyber 增量，保留本仓库白名单与其他定制。
4. 验证最终功能树与双方来源的差异。源码完成、验证通过、提交推送分别推进，不提前创建交付结果。

规划仅使用 Git 对象、假想合并树、临时索引和源码归档，不在产品工作树解冲突。

## 协议与账号契约

- main 的 `resolveUpstreamProtocolFor` / provider profile 作为多协议路由的统一基础；新 TypeSafe、Command Code、Cline、OpenCode 按 main 的平台/能力契约接入。
- Grok 的 Responses/Chat 桥、Free 工具缓存、显式强制 Chat 仍由 build 的 Grok owner 处理；Messages 的强制 Chat 必须在统一路由前接入，或者以等价薄适配保留，不恢复旧的整套并行分流。
- 发到 Chat URL 的 Responses 形状先正确识别，再按实际上游转换；所有选中 raw Chat 的此类请求均覆盖，不能只限 adaptive/按模型分流。转换复用 main 的 reasoning-cache options，并保留 build 对不可执行搜索工具的保护。
- main 结构化 input 安全回退只在明确可安全重试的拒绝条件下触发；不扩大到已产生 usage/output 的错误或残缺错误体。
- `ResponsesContentPart` 合并 `Refusal`、`Annotations` 等字段。保留 build 的工具、引用与上游非连续索引处理，加入 main 的负索引保护。
- main 可配置最小输出 token 与 build 的供应商推理关闭/后缀派生/Schema/Lite/Ollama 上限按顺序组合，最终计费 effort 必须来自实际出站请求。

## 搜索与图片契约

- `account_websearch.go` 保持搜索三态的唯一 owner，避免 main 回添内容导致重复方法/常量；OpenAI OAuth/setup-token 不进入本地搜索模拟。
- main 新增 Messages 纯搜索模拟与 build Alpha Search→Responses、本地 AnySearch、web.run/混合工具分别沿用各自入口。完成事件、真实搜索证据、引用与预算规则不能退回旧行为。
- Lite 保留原始入站状态与最终模型的两种用途：main 历史工具声明、additional_tools 与 build Lite 降级/阻止列表一起验证；剥离 Lite 标记不能重新开放本轮图片桥接。
- main 的非 OpenAI 原生 Responses 不注入 hosted 图片规则与 build `none → auto` 仅在桥接有效时生效的规则并存。
- Antigravity native Gemini 先补齐 functionCall thoughtSignature，再由 build GIF helper 抽帧和包装；保留包装后请求大小、错误类型与资源限制。
- 图片转存沿用 main 新下载策略；保持 build 同步桥接、图片响应头 profile 与实际返回格式。

## cyber 身份与状态

入站认证 API Key ID 与六种明确会话请求头别名派生 v2 哈希。读写都使用同一命名空间，仅查一个精确键；不再依赖 transcript、IP、User-Agent、缓存 key 或历史条目数。

原始上游 `cyber_policy` 仍透传、审计和计费；允许名单维持 log-only，HTTP/WS 准入跳过本地封禁，命中后不新增封禁。相同 WS 连接已有的命中后拦截语义保留。

无明确 ID 时不实施跨请求本地屏蔽，交给上游审核。共享 Key 的下游网关应确保不同会话 ID 不复用；该 ID 是会话关联标记，不是用户认证凭据。不增加 response lineage 或其他未包含在源修复中的推断机制。

旧键无法可靠区分缓存标记与真实会话，因此不迁移到 v2，按 TTL 过期。修复不删除日志或改变全局屏蔽开关；所有旧实例更新后才不再执行旧超限分支。

## 自动合并与跨层兼容

- 571 个 main-only 和 728 个 build-only 文件先按 mode/type/blob 验证保留，再追踪到共同入口。静态保留不能替代行为测试。
- 77 项共同文件逐项完成验收，不只关闭 13 项 marker。
- main 每 turn 最新分组倍率、图片 token、响应 created_at/sequence 与 build 异步预占释放、最终 effort/tier 统计共同覆盖。
- main GPT-5.5 退役按实际最终目标限制 ChatGPT OAuth/影子账号，保留 API Key 上游与显式替换映射；build Lite 特例及 GPT-5.6/Astra 能力不被目录修复覆盖。
- 账号创建、编辑、批量编辑、credentials builder、设置 DTO/API/保存/重读一起验证：新平台、缓存、scope、充值阶梯与旧 Lite/Schema/搜索/UA/GIF/超时字段共存。
- 国际化检查最终聚合对象、静态/动态调用和可见文案。已执行的 8534 键对等检查是预检查；实际 Vue 组件的 en/zh 渲染和 key completeness 留待实施。

## 数据、生成代码与工具链

main 增加 7 个 SQL：订单 bonus、TypeSafe/平台约束、分组 i18n、Gemini 缓存及变更租约。旧迁移未修改；同数字前缀的完整文件名保持不变，依现有 migration runner 的文件名顺序和 checksum 契约执行。

合入 Ent/schema/生成代码及新增 Wire provider 后，核对生成结果与源；有适配时用现有生成器重新生成，不手改生成文件。Gemini 缓存恢复循环的启动与关闭一起验收。

使用 main 要求的 Go 1.27.2、匹配的 golangci-lint v2.14.0 及更新后的前端 lock。保留 build 镜像工作流、SVG 品牌、资源上限、Redis 参数与项目治理资产。

## 验证与失败归因

使用 [验证矩阵](research/validation-matrix.md) 覆盖双方能力与新边界，先定向后全量。真实合并树出现失败时，在固定原 build/main 上定向对照，区分新回归、main 明确行为变更导致的旧断言和存量/环境问题。不得删除有效测试或放宽策略来获得绿灯。

数据库/Redis 验证使用现有隔离测试 harness，不连接业务实例。真实供应商、付费工具、兑换与支付不在本地规划或常规回归中执行。

## 回退与交付

实施前保存原 build 恢复引用。未提交合并通过正常 abort 回退；提交后使用 revert 保留历史。迁移不设计回滚 SQL，代码回退不能被表述为数据库回退。

用户已确认规划并批准本地合并实施；该批准不自动授权生产部署、数据库执行、推送触发镜像发布或真实供应商消费；相关交付动作遵循后续明确范围。
