# Brief — 合并 main 0.1.178 到 build 并保护多语言

## Goal

- 将 `origin/main@359fd12b2`（0.1.178）以双父 merge 合入
  `build@442ad2e20`，同时保留 build 的 OpenAI/Codex 私有能力和中英文多语言扩展。

## Scope

- 处理 11 个 README、OpenAI Gateway、Codex 模型发现和账号表单硬冲突。
- 复核并修复自动合并产生的 Grok 测试与 Responses 透传模型映射语义回归。
- 将最终上游模型解析集中到单一领域 owner，并让 Forward、渠道限制、Responses Lite、
  compact/reasoning 共用同一契约。
- 审计 main 主 locale 与 build locale extension 的中英文配对、导入、展开和键冲突。
- 完成前后端聚焦及全量验证，并在确认后创建双父 merge commit。

## Non-Goals

- 不新增双方均不存在的产品能力，不顺带重构无关模块。
- 不修改 main、不改写远端历史、不 force push。
- 不改变或合并其他两个既有 Trellis 任务。

## Key Decisions

- 共享冲突按双方能力组合处理，不对大型共享文件整体选择 ours/theirs。
- Codex 使用 main canonical 版本和身份规则，同时保留 build 的 `ForceCodexCLI`、
  自定义 UA、Responses Lite Header 和 Web Search 等私有接入。
- Messages 先走 main 原生 Anthropic，再走 build raw Chat fallback；Kimi、DeepSeek、
  Zhipu 等 CN provider 按账号协议分流。
- Responses 模型映射按路径拆分：OAuth 普通透传用普通映射，API Key 原生透传保持
  body，compact 只用 compact 映射，降级 Chat 使用普通映射。
- 上述路径统一由 `openai_model_mapping.go` 解析最终上游模型；调度、Lite 策略和
  reasoning 只消费结果，不再复制映射顺序。
- i18n 以 main 中英文主文件为公共基线，同时保留 build 成对扩展；通过碰撞扫描和专项
  测试验证最终值，不依赖对象展开顺序偶然覆盖。

## Key Context

- 合并目标 `359fd12b2`，共同基线 `fbfdcef81`，双方独有提交数 `241 / 124`。
- 双父 merge commit `c50a16338` 已创建并普通推送到 `origin/build`；两个父提交分别为
  `442ad2e20` 与 `359fd12b2`，且 `origin/main` 是该提交祖先。
- `CHK-001` 修复覆盖 12 个后端实现/测试文件；合并后复审确认双方独有文件除 8 个
  明确交互修复点外均与所属父分支逐字一致。
- 任务为用户要求的回溯建档，真实实现和验证先于任务创建发生。
- 详细冲突矩阵与路径契约见 `design.md`，实际完成项和验证命令见 `implement.md`。

## Risks / Deferred

- 未向真实 OpenAI、xAI 或国产供应商上游发起在线请求；本地 unit、integration、lint
  与前端全量验证均已通过。
- task 与规范文件属于本轮复审记录，不应混入业务 merge commit。

## Acceptance

- 11 个硬冲突全部解决，`git ls-files -u` 为空，`git diff --cached --check` 通过。
- OAuth、API Key、compact 和 Responses-to-Chat 四种模型映射契约均通过回归测试。
- OAuth 渠道限制、API Key/compact Lite 与 compact reasoning 组合回归均通过。
- 中英文主 locale 与 build 扩展成对存在，locale 编译、键冲突及专项测试通过。
- 前端 typecheck、lint、生产构建和 1709 个 Vitest 全部通过；后端 unit、integration
  与 `golangci-lint v2.9` 全部通过。
- 最终 merge commit 的两个父提交必须分别为 `442ad2e20` 与 `359fd12b2`。

## Next Step

- 等待用户确认本轮全面审查结果；确认后进入任务收尾与记录提交。
