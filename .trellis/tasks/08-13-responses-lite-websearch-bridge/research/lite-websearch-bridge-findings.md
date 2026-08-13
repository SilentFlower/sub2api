# Responses Lite Web Search 桥接研究结论

## 1. 生产复现形态

- Codex CLI `0.147.0` 使用模型清单 `use_responses_lite=true` 时，请求携带 `X-OpenAI-Internal-Codex-Responses-Lite: true`。
- 本地抓包确认该 `/v1/responses` body 包含 Lite 的 `input.additional_tools` namespace，但不包含 typed `web_search`，也不包含 `namespace=web,name=run`。
- 即使 provider 声明 `supports_standalone_web_search=true`，该 Responses body 仍不会自动出现搜索工具；standalone `/v1/alpha/search` 是客户端独立执行路径，不是模型在当前 Responses turn 中可见的 hosted tool。

## 2. Lite 与 Hosted Tool 边界

- 标准 Responses hosted 搜索使用顶层 `tools: [{"type":"web_search"}]`。
- 项目 Lite 工具归一化只接受 `function`、`custom`、`tool_search`，namespace 会迁移到 `input.additional_tools`。
- 因此不能照搬 hosted `image_generation` 的原始 Responses 注入方式；向 Lite body 注入 hosted `web_search` 会违反当前 Lite 契约。
- 可行路径是在 Responses 已转换为 Chat Completions 后，向 Chat tools 注入内部 function，由网关消费 function call 并回投标准 Responses 搜索项。

## 3. 现有可复用能力

- `openai_gateway_responses_chat_fallback.go` 已完成 Responses -> Chat 转换，并能把 custom、namespace 与 `tool_search` 回投为原 Responses 类型。
- `openai_responses_web_run.go` 已有共享内部 Web 工具循环，支持：
  - 固定内部工具 `sub2api_web_search`；
  - 同账号、同模型多轮 Chat 续跑；
  - 每轮最多 4 个查询、最多 2 轮；
  - provider、代理、配额、失败回灌与 failover；
  - Responses `web_search_call`、流式事件、usage 聚合；
  - 按真实成功查询数计费，以及最多 5 个来源和 `url_citation`。
- 现有 typed Web Search 混合工具已经使用上述循环，本任务只需增加一个“未显式声明搜索工具但策略授权”的配置来源，不能复制执行链。

## 4. 配置现状

- 搜索执行授权现有三层条件：
  - 账号 `extra.web_search_emulation=default|enabled|disabled`；
  - 渠道 `features_config.web_search_emulation.openai`；
  - 全局 Web Search 配置开启且存在可用 provider/manager。
- 生图桥已有账号布尔覆盖、渠道平台布尔覆盖和功能 UI，可借用装配模式，但搜索桥需要独立键，避免启用普通 Web Search 模拟后自动扩大到所有 Codex Lite 请求。
- 最终决策：新增 `codex_web_search_bridge`，账号布尔值优先于渠道；两处均缺失时关闭。它与现有搜索授权是合取关系，不替代任何现有开关。

## 5. 建议资格矩阵

只有同时满足以下条件才隐式注入：

1. HTTP `/v1/responses`，不是 `/responses/compact`，不是 WebSocket。
2. 入站 Lite Header 为真，且请求来自官方 Codex 客户端或启用了 `ForceCodexCLI`。
3. 最终账号是 OpenAI APIKey，并通过 `ShouldUseResponsesAPI(account.Extra)==false` 进入 Chat fallback。
4. `codex_web_search_bridge` 账号/渠道有效值为真。
5. 现有 `web_search_emulation` 策略允许，且全局配置、provider manager 可用。
6. 请求没有显式 typed `web_search` 或 `web.run`。
7. `tool_choice` 缺失或为 `auto`；为 `required` 时还需至少保留一个客户端可执行工具，避免隐式搜索成为唯一强制工具。`none` 或明确选择其它工具时不注入。

## 6. 风险与边界

- 模型看到工具不等于一定调用；本任务保证工具可见、可执行和回程正确，不根据提示词强制搜索。
- 桥接启用会让首轮 Chat 在网关内缓冲，以便消费潜在内部工具调用；模型未搜索时仍只发一次 Chat 请求，不调用 provider、不产生搜索费用。
- 全局 provider 在注入前不可用时应失败关闭、不注入；注入后 provider 临时失败则复用现有稳定 tool result，让模型生成可诊断回答。
- Responses WebSocket 和 compact 协议需要独立设计，本轮不扩展。
