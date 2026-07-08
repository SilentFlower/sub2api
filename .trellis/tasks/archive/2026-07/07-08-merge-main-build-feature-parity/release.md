# Release Operations

## Conclusion
Needs human review.

本任务未引入新的 SQL 文件或部署脚本，但合并范围涉及 OpenAI/Messages/Gateway、设置读写、Codex 管理端能力和前端 i18n。任务文档同时记录了历史 migration 编号重复风险，发布前需要人工复核迁移顺序和实际环境状态。

## Evidence Checked
- task.json
- prd.md
- design.md
- implement.md
- implement.jsonl
- check.jsonl
- git commit: ac3dc0dd merge(main): 同步 main 并保留 build 功能
- git commit: 7e3b32af fix(frontend): 修复 Codex 邀请弹窗中文文案
- git commit: 74d2b819 test(backend): 修复 Codex 导入测试构造参数
- git diff --name-only

## Drift Check
Missing release.md. 已基于任务文档和提交文件范围补充发布操作说明。

## SQL Changes
本任务没有新增或修改 SQL migration 文件。

PRD 和设计文档记录了历史 migration 编号 158 与 160 存在重复风险。本轮按已确认策略不重命名、不修改已发布 migration。发布前需要人工确认目标环境的 migration 历史、文件排序和执行状态，避免把编号重复误判为本轮新增变更。

## Configuration Changes
未发现需要额外配置环境变量、密钥或部署配置文件的变更。

OpenAI 生图主模型、reasoning effort、Codex custom User-Agent allowlist 等均通过应用内设置和代码默认链路发布。发布后需要确认管理端设置读取、保存和回显正常。

## Batch / Deployment Scripts / Data Repair
未发现一次性批处理、数据修复、后台任务重跑或部署脚本变更。

## External Systems / Dependent Platforms
OpenAI-compatible 上游、ChatGPT Web/Codex 相关上游和 Anthropic `/v1/messages` 到 Chat Completions fallback 行为受本轮合并影响。

发布后需要对真实或预发上游做冒烟验证，重点覆盖 raw Chat fallback、工具调用顺序稳定性、Codex reset 邀请弹窗和账号 custom User-Agent 配置。

## Release Order
代码发布即可。无需本任务专属 SQL migration 步骤。

建议发布顺序：
- 先部署后端代码。
- 再部署前端代码。
- 发布后执行管理端设置和网关请求冒烟验证。

## Rollback Notes
回滚代码即可。本任务没有数据库回滚步骤。

如果发布后发现上游兼容性问题，优先回滚本次 build 分支代码到发布前版本；同时保留 migration 历史不变，不做回滚 SQL。

## Post-release Verification
- 验证 `/v1/messages` raw Chat fallback 使用 `forwardAnthropicViaRawChatCompletions` 后仍能稳定处理 typed content、tool_use、tool_result 和 disabled thinking。
- 验证 Codex 邀请 / reset 弹窗在中文环境展示中文文案。
- 验证 Codex 导入、账号列表和设置相关接口在 CI 或预发环境通过。
- 验证 OpenAI 生图主模型、reasoning effort 和 Codex custom User-Agent 设置可以保存、读取和回显。
- 验证前端账号创建、编辑和批量编辑弹窗的 Codex 与 Anthropic 相关字段没有回归。
