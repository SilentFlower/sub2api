# Brief — OpenAI 生图对话主模型与思考预算可配置化

## Goal

- 将 OpenAI OAuth 生图路径中硬编码的对话主模型与 `reasoning.effort` 改为可通过全局系统设置动态配置，默认行为保持 `gpt-5.4-mini` + `medium`。

## Scope

- 新增两个全局 Setting 键：`openai.image_generation.main_model` 与 `openai.image_generation.reasoning_effort`。
- 后端 `SettingService` 负责读取、默认值、trim、effort 合法值校验与非法回退。
- OAuth Images -> Responses API 请求构造使用配置后的主模型和 effort。
- Codex transform 的 image-only model 归一化使用配置后的主模型，但不改写 Codex transform 路径的 `reasoning.effort`。
- 管理后台系统设置页新增主模型输入与 effort 枚举选择，并同步 admin settings API 类型、响应、保存 payload 和中英文 i18n。
- 增加后端和前端单测覆盖配置生效、默认回退、非法 effort 回退、两条主模型路径和设置页保存。

## Non-Goals

- 不做按账号或账号组的差异化配置。
- 不做 JSON 容器式扩展。
- 不改 APIKey 生图路径。
- 不改 Codex transform 路径的 `reasoning.effort` 透传语义。
- 不做主模型白名单或按模型能力自动约束 `xhigh`。

## Key Context

- 当前硬编码点：
  - `backend/internal/service/openai_images_responses.go:357-358`
  - `backend/internal/service/openai_codex_transform.go:891-894`
- `OpenAIGatewayService` 已持有 `settingService`，运行时读取应在 service 调用处完成，再把已解析配置传给纯构造/transform helper。
- 系统设置链路已有落点：
  - `backend/internal/service/domain_constants.go`
  - `backend/internal/service/settings_view.go`
  - `backend/internal/service/setting_service.go`
  - `backend/internal/handler/admin/setting_handler.go`
  - `frontend/src/api/admin/settings.ts`
  - `frontend/src/types/index.ts`
  - `frontend/src/views/admin/SettingsView.vue`
  - `frontend/src/i18n/locales/zh.ts`
  - `frontend/src/i18n/locales/en.ts`
- `implement.jsonl` 与 `check.jsonl` 已补齐真实 spec 条目，任务可进入启动 review。

## Acceptance

- 新增 Setting 键和默认值，不配置时行为完全等同当前实现。
- OAuth Images 请求上游 body 的 `model` 与 `reasoning.effort` 可由 Setting 控制。
- Codex transform image-only model 路径的主模型可由 Setting 控制。
- effort 只接受 `low/medium/high/xhigh`；非法值回退 `medium` 并记录 warn。
- 管理后台可编辑两个字段，用户可见文案走 i18n。
- 后端和前端新增测试覆盖关键分支，现有相关测试保持通过。

## Next Step

- 用户确认 planning artifacts 与本 brief 后，执行 `python3 ./.trellis/scripts/task.py start .trellis/tasks/07-03-openai`，随后进入 Phase 2.1，并按 workflow 先走 `trellis-route(implement)`。
