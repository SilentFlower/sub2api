# Brief — Responses Lite 兼容：chat 桥接 custom 子工具修复 + 原生 Responses 账号级降级开关

## Goal

- Codex 以 Responses Lite 形态请求时，无论账号上游是 Chat Completions 还是原生 Responses，都能拿到完整客户端工具（含 custom `exec`/`apply_patch`）并正确回程。

## Scope

- chat 回退桥接（无条件生效）：namespace 内 custom 子工具降级为 function、历史 `custom_tool_call` 按 namespace 摊平、回程还原为带 namespace 的 `custom_tool_call`（含唯一裸名别名）。
- 原生 Responses 路径共享修复：`FlattenResponsesNamespacesExcept` 接受 custom 子工具，历史改写与流式/非流式还原支持带 namespace 的 `custom_tool_call`。
- 账号级开关 `openai_responses_lite_downgrade`（默认关闭，apikey 且 openai/CN 供应商）：开启后对入站 HTTP Lite 请求提升 `additional_tools`、删除 `reasoning.context`、移除 Lite 头，再走既有客户端工具适配；原生适配条件扩展到 CN 原生与 OpenAI API-key 托管路径。
- 前端：`features/responsesLite/` 新增 extra 助手、Toggle 组件，接入 Edit/Create 账号弹窗，中英文文案扩展。
- 后端 apicompat/service 单测与前端 spec。

## Non-Goals

- Codex 模型目录/清单侧压 `use_responses_lite=false`；new-api 侧改动。
- WebSocket 入站 Lite 的降级；BulkEditAccountModal 批量设置。
- `tool_choice` 指向 namespace 内 custom 子工具的转换；Grok、Anthropic 协议账号。
- 上游 PR #6062 其余无关改动。

## Key Decisions

- chat 桥接修复不受开关控制：丢工具是纯缺陷，任何账号都应始终正确；开关只控制原生路径的 Lite→标准 Responses 降级（上游可能是能吃 Lite 的网关，需可关闭）。
- 通用修复直接改上游共享文件并对齐 PR #6062 第 3 项形态；build 私有的开关与降级逻辑放独立领域文件 `openai_responses_lite_downgrade.go`，共享入口只做一次薄调用。
- 回程沿用既有 namespace 约定：保留摊平名，`custom_tool_call` 带 `namespace`，不对 `functions` 做裸名特殊化；只处理精确摊平名与唯一裸名别名。
- 降级通过删除入站 Lite 头让后续判定统一按非 Lite 处理，避免多处分支。

## Key Context

- apicompat：`chatcompletions_responses_bridge.go`（`:207`、`:218`、`:249`、`:471`、`:1081`、`:1316-1370`、`:1966-2010`、`:2040-2110`、`:2155-2190`）、`responses_namespace.go:23/171`、`responses_client_tools.go:22/218/423/782/853`。
- service：`openai_gateway_forward.go:99`（降级接线点）与 `:151`（原生适配条件）、`openai_gateway_passthrough.go:242`（既有适配）、`gateway_forward_as_responses.go:229`（复用 `liftResponsesAdditionalTools`）、`openai_responses_lite_policy.go:310`。
- 前端：`EditAccountModal.vue:1826`、`CreateAccountModal.vue:3280`、`features/openAICompatibility/extra.ts`（模式参照）、`locales/{zh,en}/admin/accounts.ts:771`（spread 位置）。
- 约束：`protocol-adapter-guidelines.md` L1053-1134/L1224-1312、`quality-guidelines.md`、build 私有隔离指南、frontend component-guidelines；gofmt、`go test -tags=unit`。

## Risks / Deferred

- 模型把摊平名再次变形（`functions.exec`）不在本次范围。
- Kimi/MiniMax 原生端点对降级后工具的接受度未实测，仅在开关开启时生效。
- Phase 3.3 需在 `protocol-adapter-guidelines.md` 新增 Lite 降级与 namespace custom 子工具 scenario。
- 可选：在 Wei-Shaw/sub2api#6653 留言说明真实 Lite 形态。

## Acceptance

- AC1-AC5：chat 桥接请求/历史/回程（非流式、流式、歧义保护）单测。
- AC6：chat 回退服务级 Lite 形态端到端（开关关闭）。
- AC7：原生路径 apicompat 适配、历史改写、流式/非流式还原单测。
- AC8/AC9：DeepSeek 原生 + Lite，开关关闭上游 body 不变；开启后无 `additional_tools`/`reasoning.context`/Lite 头，顶层 tools 含 `functions__exec` 等，回程 `custom_tool_call{exec, functions}`。
- AC10：OpenAI API-key 托管与 passthrough 路径开关开启同 AC9，关闭不变。
- AC11：访问器边界（非 apikey、OAuth、Grok/Anthropic、缺失/非布尔）恒 false。
- AC12：前端 extra 助手、Toggle、弹窗显示条件与保存 payload、中英文 key 成对。
- AC13：gofmt 无差异；apicompat 与 service 单测全量通过；前端相关 spec 与 type-check 通过。

## Next Step

- `task.py start` 后从 implement.md 步骤 1 开始：`NamespacedToolName` 增加 `Custom` 并让 `NamespaceToolNames` 记录 custom 子工具。
