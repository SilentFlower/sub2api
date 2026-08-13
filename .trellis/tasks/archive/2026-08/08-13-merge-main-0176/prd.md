# 合并 main 0.1.176 到 build

## Goal

将 `main` 的 `0.1.176`（`fbfdcef81`）合并到当前 `build`
（`7a6ab3280`），在保留确认需要的 `build` 私有能力的同时，采用 `main`
的最新公共实现和数据契约，完成可验证、可回滚的分支同步。

## Background

- 当前共同基线为 `0b3fe95af`。
- 模拟合并得到 9 个冲突文件、24 个内容冲突块，没有删除、重命名冲突；
  其余 162 个主线变更文件可自动合并。
- 冲突集中在 Anthropic Chat Completions 桥接、Grok 配额测试、账号用量展示、
  Codex 账号创建/编辑/批量编辑以及账号列表套餐徽章。
- 用户已确认：本次范围内的 Grok 改动不再尝试融合双方实现，采用 `main`
  的实现和测试作为最终行为基线。
- 用户进一步确认：“Grok 只保留 main”仅指删除 `build` 独立套餐额度查询与展示
  链路；Grok messages 强制 Chat Completions、Grok 4.5 effort 归一化和批量登录
  用户脚本继续保留。
- `build` 独立套餐额度不会全部形成文本冲突，因此必须显式删除其独立文件、
  接入点、类型、API、文案和测试，不能只在冲突文件中选择 `theirs`。
- 除显式冲突外，`main` 0.1.176 还修改了多个 `build` 私有能力的薄接入点；
  自动合并成功不代表最终功能仍然生效，需要进行私有能力回归审计。

## Requirements

- R1. 以合并提交方式把 `main@fbfdcef81` 引入 `build`，不得改写现有
  `build` 历史。
- R2. Grok 的最终行为范围确认后，相关实现、类型、API、文案和测试必须只有
  一个权威来源，不得保留 main/build 双轨逻辑或重复额度展示。
- R2.1. 删除范围仅包括独立 `/billing-quota` API、独立 Billing service/parser/
  UsageInfo 投影、账号列表独立额度组件与队列、相关共享类型、locale 和专项测试。
- R2.2. 保留 main 的 `/quota`、`grok_billing`、JWT tier、Grok 4.6、长上下文、
  模型级定价和主线账号用量展示。
- R2.3. 保留 build 的 Grok force-chat、Grok 4.5 provider reasoning 归一化和
  批量登录用户脚本。
- R3. 非 Grok 冲突按能力组合处理：Anthropic 桥接同时保留 main 的
  `reasoning_content` 别名支持和 build 的流式回退/稳定工具调用行为；Codex
  表单同时保留 build 自定义 User-Agent 能力与 main 指纹收敛模式。
- R4. 自动合并成功的双边热点文件仍需语义复核，重点覆盖 Billing、OpenAI
  Gateway、Grok quota、DTO/types、Wire/生成文件和前端 locale 最终有效值。
- R5. Wire/Ent 等生成文件必须从源定义重新生成或验证，不得仅手工修正生成结果。
- R6. 合并完成后不得残留冲突标记、失效导入、孤立路由、重复字段、废弃测试或
  只在一侧存在的错误文案覆盖。
- R7. 对 `build` 私有能力执行回归审计。每项能力至少核对：领域主体文件仍存在、
  共享入口仍调用、配置/extra 字段仍能往返、专项测试仍覆盖最终运行时行为。
- R8. 回归审计必须覆盖 Codex 自定义客户端、Codex reset、Codex 身份与 main
  指纹模式、Codex Alpha Search、Responses Lite、JSON Schema 降级、Web Search/
  `web.run`/AnySearch、Codex 生图策略、Anthropic 直连桥、provider reasoning、
  Grok force-chat、Raw Chat 调试快照、Antigravity GIF 兼容、build 设置面板与
  CI/README/Trellis 分支资产。

## Acceptance Criteria

- [ ] `build` 包含 `main@fbfdcef81`，`backend/cmd/server/VERSION` 为 `0.1.176`。
- [ ] `git diff --name-only --diff-filter=U` 为空，仓库中不存在冲突标记。
- [ ] Grok 最终实现符合用户确认的保留范围，相关专项测试证明不存在双轨数据源
  和重复 UI。
- [ ] Anthropic Chat/Responses 桥接专项单元测试通过。
- [ ] Codex 创建、编辑、批量编辑中的自定义 UA 与指纹模式均能正确加载、重置和
  写入 payload。
- [ ] 私有能力回归矩阵逐项完成，未发现因冲突选择、自动合并、调用顺序、locale
  覆盖或 Wire 依赖图变化造成的特性消失；发现的问题在本任务内修复并补测试。
- [ ] 后端受影响单元测试、前端 typecheck、相关 Vitest 以及全量质量门禁通过。
- [ ] 再次执行合并树/双边热点检查，没有遗漏的语义覆盖或新增硬冲突。

## Out Of Scope

- 不在本任务中新增 main 与 build 均不存在的产品能力。
- 不顺带重构与本次分支同步无关的模块。
- 不提交或推送；提交仍需后续独立确认。
- 不清理数据库中历史 `grok_billing_quota_snapshot` 数据；运行时不再读取或展示，
  repository 可继续将该旧 key 视为调度中性字段，避免历史数据影响调度指纹。
