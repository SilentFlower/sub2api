# 合并 main 0.1.178 到 build 并保护多语言 - 技术设计

## 1. 合并策略

在 `build` 上执行 `git merge --no-commit --no-ff origin/main`，以 `main` 的最新公共结构
为基底，将 `build` 的独立行为接回对应薄接入点。先解决硬冲突，再用全量测试发现文本
无冲突但调用顺序、默认值或映射语义发生变化的问题。

## 2. 硬冲突处理

| 文件 | 处理方法 |
| --- | --- |
| `README_CN.md` | 接受 `main` 删除过期 Sora 文档，避免恢复失效说明。 |
| `openai_codex_models_service.go` | 使用 canonical client version 和主线身份规则，同时保留 `ForceCodexCLI` 下的统一 UA。 |
| `openai_codex_models_service_test.go` | 合并 canonical version、header 和 UA 三组断言。 |
| `openai_gateway_cc_pipeline.go` | 保留带 `context.Context` 的 fallback 目标解析以支持 Grok OAuth token，同时使用主线 OpenAI protocol API key 读取。 |
| `openai_gateway_forward.go` | 同时执行主线 Codex beta features 与 build Responses Lite Header 收口。 |
| `openai_gateway_messages.go` | 主线原生 Anthropic 分支优先，其后保留 build raw Chat fallback。 |
| `openai_gateway_passthrough.go` | 同时保留 Codex beta features、Responses Lite Header、fingerprint 和 Fast Policy；模型映射按账号类型与 compact 路径拆分。 |
| `openai_gateway_responses_chat_fallback_test.go` | 合并 GLM/DeepSeek 回归和主线账号映射回归。 |
| `openai_gateway_service.go` | 同时保留 build Web Search executor 与主线 Codex turn state 存储。 |
| `CreateAccountModal.vue` | 保留自定义 UA 字段与写入，同时采用主线 fingerprint mode 默认关闭和显式持久化。 |
| `EditAccountModal.vue` | 保留自定义 UA 回填/reset，同时采用主线 fingerprint mode 读取和持久化。 |

## 3. Responses 透传模型契约

模型处理不能用一个通用映射覆盖所有透传路径：

```text
原始请求模型
  -> OAuth 普通 Responses：model_mapping + 上游归一化
  -> API Key 原生 Responses：保持原始 body/model
  -> Responses compact：直接按原始模型查询 compact_model_mapping
  -> Responses 不受支持：转 Chat Completions 后应用 model_mapping
```

这样既满足 OAuth 账号别名和 Responses Lite 策略基于最终模型判断，也维持 API Key
透传的字节级请求体契约，并避免普通映射先改名后导致 compact 映射无法命中。

## 4. CN Provider 与 Messages 路由

- Anthropic protocol 账号优先走主线原生 Anthropic 实现。
- Chat Completions protocol 的 Kimi、DeepSeek、Zhipu 等 CN provider 继续走 raw Chat
  fallback，不进入 OpenAI 原生 Responses 端点。
- Grok OAuth fallback 保留上下文 token 获取能力。
- 路由辅助函数增加 Kimi Chat、DeepSeek Responses、Zhipu Anthropic 覆盖，防止新增平台
  只在单一入口可用。

## 5. i18n 保留策略

- `main` 修改的 `accounts/channels/overview/resources/settings/dashboard` 中英文主文件
  作为公共文案基线。
- `build` 的 `accountsCodexCustomClients`、`accountsOpenAICompatibility`、
  `settingsResponsesLite`、Web Search、生图等中英文扩展继续由各自 index 导入并展开。
- 对主线本次新增键与扩展键执行碰撞扫描；同键时必须明确唯一 owner，不依赖对象展开顺序
  偶然覆盖。
- 运行 locale 编译、无键冲突、build extension 和 OpenAI Fast Policy locale 专项测试，
  再用前端全量测试验证组件实际取值。

## 6. 回滚与提交

- merge commit 前可使用 `git merge --abort` 回到 `build@442ad2e20`；不得使用破坏性 reset。
- merge commit 后应通过明确 revert 或合并前保护引用回滚，不改写远端历史。
- 提交前由 `trellis-push` 核对 staged 文件集合、两个父提交和推送目标；推送必须为普通
  `build -> origin/build`，且需要独立确认。
