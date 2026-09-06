# build 功能保留建议

本轮为只读评估。依据 build 4e9829519707f0743a7cbcabcf3e73b8dd0beeb0 与 origin/main ab99d56e9626e6cd731592dae8553c9758a0efa2 的当前代码及净差异；尚未实际合并或验证合并后的行为。

## 已确定的处理原则

用户明确 GPT-6 按 main。覆盖模型默认列表、别名识别、上下文、推理档位、能力元数据、计费及 GPT-6 提示词选择，不用旧 build 测试强行保留旧行为。main 当前没有 build 的 GPT-6 专用内嵌提示词，因此 GPT-6 的本地提示词选择回到 main 逻辑；GPT-5.6 专用提示词可独立保留。

build 的通用功能（包括可配置的 Responses Lite Header 模型阻止列表）继续保留；这与 GPT-6 模型自身的默认目录能力是不同层次。

## 建议保留的功能

| 功能 | 保留行为 | 当前代码依据 |
| --- | --- | --- |
| 本地网页搜索 | AnySearch、OpenAI API Key 搜索模拟、Codex web.run/混合工具搜索循环、Lite Chat fallback 的搜索桥接和账号/渠道设置；搜索预算耗尽后继续完成回答 | backend/internal/pkg/websearch/anysearch.go；backend/internal/service/openai_responses_web_run.go；backend/internal/service/openai_responses_websearch.go；backend/internal/service/codex_web_search_bridge.go；frontend/src/features/webSearch/ |
| JSON Schema 兼容 | 账号开关控制 json_schema → json_object，原 Schema 作为尽力遵循约束，保留工具 Schema | backend/internal/pkg/apicompat/json_schema_downgrade.go；backend/internal/service/openai_json_schema_downgrade.go；frontend/src/features/openAICompatibility/ |
| DeepSeek 推理历史修复 | 工具调用历史缺失 reasoning 时自动关闭 thinking，并清理不匹配的 reasoning_effort；保留全局开关 | backend/internal/service/openai_deepseek_missing_reasoning_policy.go；frontend/src/features/deepSeekReasoning/ |
| Anthropic → Chat 兼容增强 | 保留直连桥的稳定消息/工具顺序与缓存前缀、会话粘性、上游 SSE 消费和非流式 JSON 折叠、自定义请求头；补 main 的响应头记录 | backend/internal/pkg/apicompat/chatcompletions_anthropic_bridge.go；backend/internal/service/openai_gateway_messages_chat_fallback.go；backend/internal/handler/openai_gateway_messages_session.go |
| Responses Lite 可配置策略 | 按最终上游模型决定 HTTP Header 与 WebSocket metadata 是否透传；精确或末尾通配规则；防止账号 Header 覆写绕过 | backend/internal/service/openai_responses_lite_policy.go；backend/internal/service/account_header_override.go；frontend/src/features/responsesLite/ |
| 图片生成定制 | 可配置 OAuth 生图对话主模型和思考预算；正确识别客户端 image_gen，避免重复 hosted 工具注入；保留现有 Lite 不自动注入 hosted 图片工具的边界 | backend/internal/service/setting_openai_image_generation.go；backend/internal/service/openai_images_responses.go；backend/internal/service/image_generation_intent.go；frontend/src/features/openAIImageGeneration/ |
| Grok 定制 | 显式强制 Messages 走 Chat Completions、Grok 4.5 effort 适配、额度探测补充请求/token余额和冷却信息、批量登录用户脚本 | backend/internal/service/openai_gateway_grok_force_chat.go；backend/internal/service/openai_provider_reasoning_effort.go；frontend/src/components/account/GrokQuotaProbeCell.vue；tools/grok-login-userscript/ |
| 反重力 GIF 兼容 | GIF 转 PNG 多帧、帧数设置、数据编码兼容和重试链路接入 | backend/internal/pkg/antigravity/gif_compat.go；backend/internal/service/antigravity_gif_compat.go；frontend/src/features/antigravityGif/ |
| Codex 客户端和 GPT-5.6 定制 | 账号/批量自定义 UA 放行、拒绝响应中的 UA 诊断、出站身份一致性、GPT-5.6 专用提示词 | backend/internal/pkg/openai/custom_allowed_client.go；backend/internal/service/account_codex_cli_only_allowed_clients.go；backend/internal/service/openai_client_restriction_response.go；backend/internal/service/openai_codex_base_instructions.go；backend/internal/pkg/openai/instructions_gpt5_6.txt |
| 工程资产 | build 的手动构建流程、Trellis/Flower 配置、领域拆分结构、已有任务与容灾/故障切换脚本资产 | .github/workflows/my-ci.yml；.trellis/；.flower/；.agents/ |

## 与 main 整合的部分

- main 已包含独立 alpha/search 转发；当前业务差异主要是 main 新增 UpstreamHeaders，接续上游实现即可，保留需要的中文说明。
- GLM-5.3 low 保留规则和 Anthropic thinking 适配采用 main 新行为，但整合进 build 已有 provider 领域实现，保留 Grok 4.5 与其它现有兼容分支，避免重复函数或新旧入口分歧。
- 账号弹窗保留 build 兼容设置，同时合入 main 的图片 URL 转 base64 设置、上游请求 ID 配置和能力同步新行为。
- 已移除的旧 Codex 邀请重置不恢复；不能凭历史提交标题把它列为当前应保留功能。

## 验证边界

此清单是合并决策建议，并非所有分支兼容性已验证的结论。实施时需逐条复核接线，处理文本与语义冲突，执行 Go 编译/相关测试、前端类型与功能测试以及最终中英文 locale 检查。
