# Brief - 支持 DeepSeek 混合工具 Web Search

## Goal

- 让 OpenAI APIKey Chat fallback 账号在 Responses 混合工具请求中安全执行 typed `web_search`，保持模型按需选择、其它客户端工具、Responses 回程、usage 与计费语义，不再因 `tool_choice=auto` 在入口返回 400。

## Scope

- 扩展 typed Web Search 能力决策，增加 `chat_tool_loop`，保留原生 Responses、纯/强制直接模拟和明确 reject 边界。
- 在 service 层为混合可选 typed Web Search 注入固定内部 Chat function 代理，保持 `auto`、`required`、缺失、`none` 和明确选择其它工具的原语义。
- 参数化复用现有 `web.run` 搜索循环，覆盖 provider 调用、同账号续跑、查询/轮次上限、代理 failover、usage 聚合、Responses `web_search_call` 和按实际成功查询数计费。
- 为普通最终文本追加最多 5 个真实、去重来源并生成 rune 索引正确的 `url_citation`；流式与非流式共享来源投影。
- 增加生产复现、其它客户端工具、错误、计费、引用、流式事件和既有路径回归测试。
- 适用于所有满足协议条件且策略允许的 OpenAI APIKey Chat fallback 账号，不做 DeepSeek host/model 特判。

## Non-Goals

- 不让同一 OpenAI 账号动态切换到 Anthropic 端点。
- 不实现网页打开、点击、抓取、截图、浏览器会话、图片搜索或专用天气 API。
- 不新增 provider、配额、代理、计费或配置字段，不修改前端、数据库、Ent 或 migration。
- 不修改 Codex 客户端工具声明方式，不做无关 Responses/Chat 重构。
- 不部署到 `www.havefun.eu.cc`，不更新镜像、重启容器或修改生产配置/数据。

## Key Context

- 生产证据：账号 `3806/deepseek官渠` 配置 `force_chat_completions` 和 `web_search_emulation=enabled`；最近 24 小时 36 次失败均为 10 至 15 个混合工具、`tool_choice=auto`，在本地 `reject`，DeepSeek 与 AnySearch 未被调用。
- 入口：`backend/internal/service/openai_responses_websearch.go` 当前只接管唯一或强制 Web Search。
- Chat fallback：`backend/internal/service/openai_gateway_responses_chat_fallback.go` 负责 Responses -> Chat 路由和现有 `web.run` 循环入口。
- 可复用实现：`backend/internal/service/openai_responses_web_run.go` 已具备受控搜索循环、Responses 搜索项、usage 与计费基础。
- 通用桥：`backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go` 继续默认丢弃服务端工具；typed 搜索代理只在 service 层按条件注入，避免扩大通用转换影响面。
- 固定代理名 `sub2api_web_search` 遇到客户端工具同名时明确 400；启用循环时设置 `parallel_tool_calls=false`，上游仍返回内部搜索与客户端并行调用时明确 502。
- `search_context_size`、domain filters、正数 `max_uses` 保留；无等价高级字段显式拒绝。
- 结构化文本输出不追加来源后缀或 annotation，避免破坏 JSON 合同；真实搜索仍输出 `web_search_call`。
- 日志不得记录查询、结果正文、请求体或凭据。

## Acceptance

- 10 至 15 个混合工具、typed `web_search`、`tool_choice=auto` 不再入口 400。
- 模型未选搜索时不调用 provider、不产生搜索项或费用，并正常回传其它 function/custom/namespace/tool_search。
- 模型选择搜索时真实调用现有 provider、按同一 call ID 回灌并续跑同一账号；内部代理不泄漏。
- 流式与非流式输出合法 `web_search_call`、最终文本和一致的事件索引；普通文本附带真实来源和有效 `url_citation`。
- `none`、明确其它工具、纯/强制搜索、原生 Responses、Anthropic 原生搜索、现有 `web.run` 和结构化输出不回归。
- usage 累计所有模型轮次，只按真实成功查询计费；失败、空结果或未执行搜索不计费。
- 工具冲突、重复搜索声明、并行违规、provider/代理失败、轮次/查询上限、Unicode 引用索引和客户端断连有测试覆盖。
- 定向测试、相关 package 单测、后端完整 unit tests、lint 和 `git diff --check` 通过。

## Next Step

- 用户确认三件套与本 brief 后运行 `task.py start`；任务进入 `in_progress` 后先执行 `trellis-route(implement)`，再按路由结果实施，不直接跳过实现门禁。
