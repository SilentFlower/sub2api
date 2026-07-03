# Implement — OpenAI 生图对话主模型与思考预算可配置化

## Step 1: 后端 Setting key 与读取方法

- 在 `backend/internal/service/domain_constants.go` 新增：
  - `SettingKeyOpenAIImageGenerationMainModel = "openai.image_generation.main_model"`
  - `SettingKeyOpenAIImageGenerationReasoningEffort = "openai.image_generation.reasoning_effort"`
- 在 `backend/internal/service/settings_view.go` 的 `SystemSettings` 增加两个 string 字段。
- 在 `backend/internal/service/setting_service.go`：
  - 默认设置 map 增加两个默认值。
  - `buildSettingsUpdates` / settings 写入路径增加两个字段。
  - `GetAllSettings` 映射返回两个字段。
  - 新增 `GetOpenAIImageGenerationMainModel(ctx)` 与 `GetOpenAIImageGenerationReasoningEffort(ctx)`。
  - 新增 effort normalize helper，合法值只允许 `low/medium/high/xhigh`。
  - `UpdateSettings` 后同步缓存或失效缓存。

## Step 2: 后端 admin settings API

- 在 `backend/internal/handler/admin/setting_handler.go`：
  - `UpdateSettingsRequest` 增加两个 `*string` 字段。
  - GET settings 响应 DTO 映射增加两个字段。
  - PUT 请求归一化时 trim 主模型；effort 调用 service normalize。
  - PUT 后响应映射增加两个字段。
  - `diffSettings` 增加两个 key 的变更审计。

## Step 3: Responses Images 请求构造

- 在 `backend/internal/service/openai_images_responses.go`：
  - 新增请求 options 结构。
  - `buildOpenAIImagesResponsesRequest` 增加 options 参数，内部空值回退默认。
  - 用 options 主模型设置 `model`。
  - 用 options effort 设置 `reasoning.effort`。
- 在 `ForwardImages` 调用处从 `s.settingService` 读取主模型和 effort；`s` 或 `settingService` 为 nil 时使用默认。
- 更新直接调用该函数的测试和测试辅助。

## Step 4: Codex transform 主模型

- 在 `backend/internal/service/openai_codex_transform.go`：
  - `codexOAuthTransformOptions` 增加 `ImageGenerationMainModel string`。
  - `normalizeOpenAIResponsesImageOnlyModel` 增加 `mainModel string` 参数，空值回退常量。
  - `applyCodexOAuthTransformWithOptions` 传入 options 字段。
- 在 `backend/internal/service/openai_gateway_service.go` 的 OAuth transform 调用前读取主模型，并传入 options。
- 保留 `applyCodexOAuthTransform(reqBody, isCodexCLI, isCompact)` 包装函数默认行为，降低测试改动范围。

## Step 5: 前端系统设置

- 在 `frontend/src/api/admin/settings.ts` 的 `SystemSettings` / `UpdateSettingsRequest` 增加：
  - `openai_image_generation_main_model`
  - `openai_image_generation_reasoning_effort`
- 在 `frontend/src/types/index.ts` 同步类型字段。
- 在 `frontend/src/views/admin/SettingsView.vue`：
  - `SettingsForm` 默认值增加主模型空字符串和 effort `medium`。
  - Gateway Forwarding card 增加主模型输入和 effort `Select`。
  - 保存 payload trim 主模型；effort 提交枚举值。
- 在 `frontend/src/i18n/locales/zh.ts` 与 `en.ts` 增加文案。

## Step 6: 测试

- 后端单测：
  - `setting_service_update_test.go`：验证默认值、写入值、非法 effort 回退。
  - `openai_images_test.go`：验证 OAuth Images 上游 body 的 `model` 与 `reasoning.effort`。
  - `openai_codex_transform_test.go`：验证 image-only model normalize 使用配置主模型。
  - `openai_gateway_service_hotpath_test.go`：验证网关热路径读取 setting 后生效。
- 前端单测：
  - `SettingsView.spec.ts`：基础响应和保存 payload 覆盖新字段。

## Validation Commands

```bash
cd backend
go test -tags=unit ./internal/service
go test -tags=unit ./internal/handler/admin
```

```bash
cd frontend
pnpm typecheck
pnpm test:run src/views/admin/__tests__/SettingsView.spec.ts
```

## Rollback Points

- 若设置链路出现问题，可先保留 UI/API 字段但让运行时读取 helper 始终回退默认值，快速恢复旧行为。
- 若 Codex transform 测试扩散过大，优先保持 wrapper 旧签名，并只在 service 调用 options 入口传配置。
