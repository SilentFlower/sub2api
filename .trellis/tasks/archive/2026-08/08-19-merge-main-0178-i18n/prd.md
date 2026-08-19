# 合并 main 0.1.178 到 build 并保护多语言

## Goal

将最新 `origin/main@359fd12b2`（版本 `0.1.178`）合并到
`build@442ad2e20`，吸收主线新增功能、修复、迁移和生成代码，同时保留
`build` 已有的 OpenAI/Codex 私有能力及中英文多语言扩展。最终结果必须是可测试、
可审计的双父合并，不得通过整文件选择导致任一侧有效语义静默丢失。

本任务为用户要求的回溯建档。真实合并、冲突处理和验证先于任务创建发生；任务文档
必须如实记录实际操作与证据，不将其伪装成按正常 Phase 1 顺序执行。

## Background

- 合并前 `build` 为 `442ad2e20`，目标 `origin/main` 为 `359fd12b2`，共同基线为
  `fbfdcef81`；双方独有提交数为 `241 / 124`。
- 使用 `git merge --no-commit --no-ff origin/main` 保留真实合并状态；目标版本为
  `0.1.178`。
- 合并结果涉及 353 个文件，约 24355 行新增、1689 行删除。
- 真实合并产生 11 个硬冲突，集中在 README、OpenAI Codex 模型发现、Responses/
  Chat fallback、Gateway 请求头与 Messages 路由、服务结构体，以及账号创建/编辑表单。
- `main` 同时修改 12 个中英文主 locale 文件；`build` 的功能文案主要通过成对的
  locale extension 文件注入，因此必须检查最终导入、展开顺序和键冲突，而不能只确认
  文件仍存在。
- 全量测试额外发现两类无冲突标记语义回归：Grok 表单测试依赖旧源码形态，以及
  Responses 透传模型映射把 OAuth、API Key、compact 三种契约混在一起。

## Requirements

- R1. 以普通 merge 方式引入 `origin/main@359fd12b2`，不 squash、不 rebase、不改写
  `build` 既有历史。
- R2. 所有 11 个硬冲突必须按领域语义组合处理；禁止为快速清除冲突而对共享大文件
  直接整体采用 ours 或 theirs。
- R3. Codex 模型发现同时保留 `main` 的 canonical 版本/身份规则和 `build` 的
  `ForceCodexCLI`、自定义 User-Agent 兼容行为。
- R4. OpenAI Gateway 同时保留 `main` 的 Codex beta features、CN provider 原生
  Anthropic/Chat fallback、Codex turn state，以及 `build` 的 Responses Lite Header、
  Web Search、Grok/DeepSeek/GLM 兼容逻辑。
- R5. Responses 透传模型契约必须明确分流：OAuth 普通 `/responses` 应用普通账号
  映射；API Key 原生 `/responses` 保持请求体模型不变；`/responses/compact` 仅应用
  `compact_model_mapping`；Responses 不受支持而降级到 Chat 时应用普通账号映射。
- R6. Create/Edit Account 表单同时保留 `build` 的 Codex 自定义 User-Agent 字段与
  `main` 的 fingerprint mode；默认值、编辑回填、reset 和 payload 写入不得互相覆盖。
- R7. `main` 新增或修改的中英文文案必须成对保留，`build` 的 locale extension 也必须
  成对保留；最终 locale 树不得出现扩展键覆盖主线新键、编译失败或只在单一语言存在的
  功能文案。
- R8. README 中已经由主线删除的过期 Sora 文档不得因冲突选择重新引入。
- R9. 自动合并成功的热点仍需通过聚焦测试和全量测试检查无标记语义回归，不能以
  `git ls-files -u` 为空作为完成标准。
- R10. 合并提交与推送必须遵循 `trellis-push` 的精确文件范围和确认门禁；禁止 force
  push，禁止推送到 `main` 或其它分支。

## Acceptance Criteria

- [x] `MERGE_HEAD` 精确指向 `359fd12b2`，`backend/cmd/server/VERSION` 为 `0.1.178`。
- [x] 11 个硬冲突均已解决，`git ls-files -u` 为空，`git diff --cached --check` 通过。
- [x] Codex 模型发现、CC fallback、Gateway 请求头、Messages 路由、服务结构体和账号
  表单的双方有效行为均已保留并有聚焦测试覆盖。
- [x] OAuth、API Key、compact、Responses-to-Chat fallback 四种模型映射契约均有
  回归用例通过。
- [x] 中英文主 locale 与 `build` locale extension 均成对存在；locale 编译、键冲突、
  build 扩展和 Responses Lite 文案专项测试通过。
- [x] 前端 `pnpm typecheck`、`pnpm lint:check`、生产构建和全量 Vitest 通过，共
  248 个测试文件、1709 个测试。
- [x] 后端完整 unit、integration 测试与 `golangci-lint v2.9` 通过；lint 为
  `0 issues`。
- [x] 创建双父 merge commit，并验证两个父提交分别为合并前 `build@442ad2e20` 与
  `origin/main@359fd12b2`。
- [x] 经用户确认后普通推送 `build -> origin/build`；未使用 force push。

## Out Of Scope

- 不新增 `main` 与 `build` 均不存在的产品能力。
- 不顺带重构与本次冲突和语义回归无关的模块。
- 不修改 `main` 分支，不改写远端历史。
- 不把其他两个既有 `in_progress` Trellis 任务并入或改为本任务子任务。
