# 发布操作

## 结论

存在发布操作。本任务新增全局系统设置项，代码部署后可按需在管理后台调整 OpenAI 生图对话主模型与 reasoning effort。

## 已检查证据

- task.json：当前任务为 OpenAI 生图对话主模型与思考预算可配置化。
- prd.md：明确新增 `openai.image_generation.main_model` 与 `openai.image_generation.reasoning_effort` 两个 Setting 键。
- design.md / implement.md / implement.jsonl / check.jsonl：实现范围覆盖 SettingService、admin settings API、OpenAI OAuth Images 请求构造、Codex transform 与前端设置页。
- release.md：原任务目录中缺失。
- git commits / changed files：`524b9b7a` 修改后端设置链路、OpenAI 网关链路、前端设置页与相关测试；`90fae584` 仅更新任务 push snapshot。

## 漂移检查

原任务目录缺失 release.md。本文件按任务要求、实现计划、检查记录与已推送提交补齐，当前未发现文档漂移。

## SQL 变更

无。系统设置沿用现有 Setting 键值链路，不新增表结构或 migration。

## 配置变更

新增两个全局系统设置项：

- `openai.image_generation.main_model`：OpenAI OAuth 生图 Responses 请求的对话主模型，默认值保持为 `gpt-5.4-mini`。
- `openai.image_generation.reasoning_effort`：OpenAI OAuth 生图 Responses 请求的 reasoning effort，合法值为 `low`、`medium`、`high`、`xhigh`，默认值保持为 `medium`。

代码部署后，如需切换主模型或思考预算，可在管理后台系统设置页调整；不配置时保持旧行为。

## 批处理 / 部署脚本 / 数据修复

无。无需一次性脚本、数据修复、定时任务触发或后台任务重跑。

## 外部系统 / 依赖平台

无需要协同发布的外部平台。运行时仍调用既有 OpenAI Responses API 上游；上线后需验证实际请求体中的 `model` 与 `reasoning.effort` 符合配置。

## 发布顺序

1. 部署包含 `524b9b7a` 的应用代码。
2. 如需改变默认行为，在管理后台系统设置页调整 OpenAI 生图主模型与 reasoning effort。
3. 执行发布后验证。

## 回滚说明

如需回滚，可回滚应用代码到上线前版本。若仅需恢复旧行为，可将两个设置清空或改回默认值：`openai.image_generation.main_model = gpt-5.4-mini`，`openai.image_generation.reasoning_effort = medium`。

## 发布后验证

- 管理后台系统设置页能读取并保存 OpenAI 生图主模型与 reasoning effort。
- OpenAI OAuth Images 上游 Responses 请求体中的 `model` 使用配置值或默认 `gpt-5.4-mini`。
- OpenAI OAuth Images 上游 Responses 请求体中的 `reasoning.effort` 使用配置值；非法或空值回退 `medium`。
- Codex image-only transform 路径的对话主模型使用配置值或默认 `gpt-5.4-mini`。
