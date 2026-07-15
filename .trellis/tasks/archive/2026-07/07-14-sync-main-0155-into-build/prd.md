# 同步 main 0.1.155 到 build 并保留特性

## Goal

将最新 `origin/main`（当前 `7c717365`，版本 `0.1.155`）合并到 `build`，吸收主线新增功能、修复、迁移和生成代码。main 已等价或更完整覆盖的 build 能力允许由 main 替代；仍有明确独立价值或兼容契约的 build 业务能力、部署资产、Trellis 工作流和定制行为继续保留。合并后输出可复核的冲突处理记录、主线更新摘要和 build 特性处置清单；本任务不自动推送。

## Background

- 当前合并基线是 `a1930ea6`（版本 `0.1.152`）。
- `build...origin/main` 当前分叉为：build 侧 118 个提交，main 侧 112 个提交。
- main 相对共同基线修改约 315 个文件，包含后端、前端、Ent 生成代码、SQL migration、部署脚本和 CI。
- build 包含大量主线不存在的定制能力，包括 OpenAI/Codex/Grok 协议适配、账号管理、HA/DR 自动化、Trellis 工作流及部署资产。
- Grok 配额与 Billing 在 main 0.1.155 已形成统一 `QueryQuota` / `ProbeBilling` / `ProbeUsage` 数据流，覆盖 build 原独立 Billing 查询的大部分目标，并新增单飞探测、免费账号 24 小时估算、付费账号窗口统计和 SSO 导入后探测。
- build 于 2026-07-10 02:50 的提交 `60d36274` 首先补充 GPT-5.6 `max`，来源是用户在 2026-07-09 明确指出项目遗漏该档位。main 随后在同日通过 `80b3d4c1` 和 `c3ae5fc3` 增加模型感知的 `max` 处理与原始模型候选提取；最新 main 已覆盖显式 `reasoning.effort="max"`、`gpt-5.6-sol-max` 后缀推导、上游模型规范化和用量元数据记录，因此 build 的早期通用后缀实现可由 main 替代。
- 之前的 main→build 同步采用备份分支、`git merge --no-commit --no-ff`、逐冲突语义合并和定向测试，不能用全局 `ours`/`theirs` 覆盖。
- 当前工作区有另一个未跟踪 Trellis 任务目录，必须保持原样，不纳入合并提交。

## Requirements

- 合并前创建指向当前 `build` HEAD 的备份分支，名称包含目标版本和短 SHA。
- 重新确认 `origin/main` 最新状态，并记录合并基线、双方提交数和主线更新范围。
- 建立 build 独有特性清单，至少覆盖协议桥接、Codex/Grok、自定义账号能力、HA/DR、部署与 Trellis 元数据。
- 使用非快进、合并前不自动提交的方式执行合并，以便先审查全部冲突和自动合并结果。
- 每个冲突必须基于行为和测试语义解决：优先吸收 main 的修复与新架构，同时保留 build 独有能力；禁止按整文件无差别选择一侧。
- main 已提供同类能力时，默认收敛到 main 实现并删除 build 的重复实现；只有 main 未覆盖且仍有明确用户价值或兼容契约的 build 增量才保留，避免重复逻辑、双重请求和双数据模型。
- Grok 冲突使用 main 的统一探测、Billing、缓存和前端数据流作为基线，不为代码形式上的“保留 build”继续维护独立 `queryBillingQuota` 请求、旧 Billing 快照类型或重复展示组件；若 build 字段能够提供 main 明确缺失且仍需要的用户能力，再以 main 数据模型的增量字段实现。
- GPT-5.6 `max` 使用 main 的模型感知实现和候选模型提取链路，不保留 build 在 `openai_compat_model.go` 与通用 Codex suffix 白名单中的重复旧实现；保留 main 对显式 effort、后缀模型、compact 降级和 usage 元数据的测试覆盖。
- Codex 图片工具使用 main 对顶层 `tools` 与 Responses Lite `additional_tools` 的统一 namespace 识别；同时保留 build 的 `image_gen` namespace 保护：namespace 已存在时不得注入旧 `image_generation` 工具，也不得追加旧图片桥接提示，避免模型被引导到错误工具路径。
- `tool_choice` 按工具类型分流：旧原生 `image_generation` 桥接继续保留 build 的缺失或字符串 `none` 改写为 `auto` 的修复，并保持 `auto`、`required`、明确工具对象、Spark 与无图片工具场景不变；`image_gen` namespace 不得触发该改写，客户端显式 `none` 必须原样保留。
- 保留 build 的 OpenAI OAuth 生图主模型与思考预算配置，包括后台设置 API、管理页面、运行时读取与请求转换；以 main 默认 `gpt-5.4-mini` 作为空配置回退值，但不得让合并后的固定常量路径静默忽略管理员已保存的主模型或思考预算。
- 保持 build 的 Codex reset 邀请重置功能独立：继续保留 `OpenAICodexResetService`、独立 repository client、`/admin/accounts/:id/openai-codex-reset/status|consume|invite` API、账号操作菜单和独立弹窗；同时吸收 main 的 `OpenAIQuotaService.ResetCredit`、`/admin/openai/accounts/:id/reset-quota` 与 `OpenAIQuotaResetCell`。两套链路不得互相替换、拆分或交叉调用。
- SQL migration、Ent schema/生成代码、Wire 生成代码必须保持同一主线版本的一致组合，不得手工拼出不匹配状态。
- 合并完成后生成更新摘要，按用户可理解的领域归类说明 main 从 0.1.152 到 0.1.155 新增或修复了什么。
- 合并完成后生成 build 特性保留报告，列出已保留、被 main 替代、需要调整和存在残余风险的能力。
- 不修改或提交计划外的未跟踪文件，不自动推送远端。

## Acceptance Criteria

- [ ] 存在可恢复的合并前备份分支，准确指向合并前 `build` HEAD。
- [ ] `origin/main` 已合并进本地 `build`，且 `git ls-files -u` 为空。
- [ ] 所有冲突都有文件级处理结论和保留理由，不存在未审查的整文件覆盖。
- [ ] build 独有特性清单逐项标记为保留、主线替代、调整或风险。
- [ ] Grok 重叠能力已收敛到 main 的单一数据流，不存在独立 Billing API 与统一配额 API 并行请求、重复缓存或新旧快照同时维护。
- [ ] 顶层或 `additional_tools` 中存在 `image_gen` namespace 时，请求不新增旧 `image_generation` 工具，且 instructions 不追加旧图片桥接提示；对应回归测试通过。
- [ ] 旧原生图片桥接的 `tool_choice` 缺失或 `none` 会变为 `auto`；namespace 场景的 `tool_choice`（包括 `none`）保持不变，且两类行为均有测试覆盖。
- [ ] 生图主模型和思考预算配置在后台可读写，空值回退 main 默认值，非空配置实际进入 OpenAI OAuth 生图 Responses 请求；现有设置与生图测试通过。
- [ ] build 独立 Codex reset 邀请重置的三个 API、独立弹窗、邀请确认与邮箱限制继续工作；main 的额度重置单元格和 reset API 同时可用，Wire 注入包含两套服务且不存在路由冲突。
- [ ] main 的 migration、Ent 与 Wire 生成产物版本一致，相关生成/编译检查通过。
- [ ] 后端定向测试、前端 typecheck/lint/相关测试及 `git diff --check` 通过；无法运行的检查明确记录原因。
- [ ] 输出 0.1.153～0.1.155 主线更新摘要，覆盖主要后端、前端、运维和协议变化。
- [ ] 未跟踪的 `.trellis/tasks/07-14-deepseek-response-format-json-schema-compat/` 文件保持未修改、未暂存。
- [ ] 合并结果仅保留在本地，等待用户复核和单独的提交/推送确认。

## Out Of Scope

- 不在本任务中重新实现刚刚回滚的 JSON Schema 账号降级或 `request_permissions` 文本恢复。
- 不部署到线上环境，不执行数据库 migration，不发布镜像。
- 不自动推送 `build` 或删除合并前备份分支。
