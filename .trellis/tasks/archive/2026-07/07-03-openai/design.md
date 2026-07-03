# Design — OpenAI 生图对话主模型与思考预算可配置化

## Architecture

本任务沿用现有系统设置链路，不新增表结构：

1. `backend/internal/service/domain_constants.go` 新增两个 Setting key。
2. `backend/internal/service/setting_service.go` 在默认值、`SystemSettings` 读取/写入和缓存失效路径中纳入两个字段。
3. `backend/internal/handler/admin/setting_handler.go` 在 admin settings GET/PUT DTO 和响应中透传两个字段。
4. `frontend/src/api/admin/settings.ts`、`frontend/src/types/index.ts`、`frontend/src/views/admin/SettingsView.vue`、`frontend/src/i18n/locales/{zh,en}.ts` 暴露管理后台表单。
5. OpenAI OAuth 生图路径从 `OpenAIGatewayService.settingService` 读取配置后注入 Responses 请求。

## Backend Data Flow

### Setting keys

- `openai.image_generation.main_model`
  - 默认值：`openAIImagesResponsesMainModel` 当前常量值 `gpt-5.4-mini`
  - 校验：`strings.TrimSpace` 后非空即使用；空值回退常量
- `openai.image_generation.reasoning_effort`
  - 默认值：`medium`
  - 合法值：`low`、`medium`、`high`、`xhigh`
  - 校验：空值回退 `medium`；非法值回退 `medium` 并记录 warn 日志

### SettingService

新增轻量读取方法，保持热路径调用集中：

- `GetOpenAIImageGenerationMainModel(ctx context.Context) string`
- `GetOpenAIImageGenerationReasoningEffort(ctx context.Context) string`

读取策略参考 `GetOpenAICodexUserAgent`：

- `s == nil` 或 `settingRepo == nil` 时使用默认值。
- DB 读取失败且不是 `ErrSettingNotFound` 时使用默认值并 warn。
- 两个配置可用小型缓存结构缓存 60s，或先按现有单值读取模式实现；如果加缓存，`UpdateSettings` 后必须主动刷新/失效缓存。

### Responses Images

当前 `buildOpenAIImagesResponsesRequest(parsed, toolModel)` 是纯构造函数，内部硬编码：

- 请求骨架中 `reasoning.effort = "medium"`
- 随后 `model = openAIImagesResponsesMainModel`

设计调整：

- 定义小型 options 结构，例如 `openAIImagesResponsesRequestOptions{MainModel string; ReasoningEffort string}`。
- `buildOpenAIImagesResponsesRequest(parsed, toolModel, opts)` 只消费已解析好的字符串，不直接读 `SettingService`。
- `ForwardImages` 中在 `s.settingService` 存在时读取两个 setting；不存在时使用默认值，保持测试兼容。
- `reasoning.effort` 只影响 OAuth Images -> Responses API 生成请求。

### Codex transform

当前 `normalizeOpenAIResponsesImageOnlyModel(reqBody)` 在 image-only model 进入 Responses + image_generation tool 时强制：

```go
reqBody["model"] = openAIImagesResponsesMainModel
```

设计调整：

- 让 `normalizeOpenAIResponsesImageOnlyModel` 接收 `mainModel string`，内部空值回退常量。
- `codexOAuthTransformOptions` 增加 `ImageGenerationMainModel string`。
- `applyCodexOAuthTransformWithOptions` 调用 normalize 时传入该字段。
- `OpenAIGatewayService.Forward` 调用 `applyCodexOAuthTransform...` 前从 `s.settingService` 读取主模型；无配置时保持常量。
- 不改 Codex transform 路径的 `reasoning.effort`，继续按客户端请求透传。

## Frontend / API Contract

后台设置 API 增加两个字段：

- `openai_image_generation_main_model: string`
- `openai_image_generation_reasoning_effort: "low" | "medium" | "high" | "xhigh" | string`

前端落点：

- `frontend/src/api/admin/settings.ts`
  - `SystemSettings` 增加两个必填 string 字段。
  - `UpdateSettingsRequest` 增加两个可选 string 字段。
- `frontend/src/types/index.ts`
  - 同步系统设置类型字段。
- `frontend/src/views/admin/SettingsView.vue`
  - 在 Gateway tab 的 Gateway Forwarding card 中，放在 OpenAI Codex UA 相邻区域。
  - 主模型使用文本输入，placeholder `gpt-5.4-mini`。
  - effort 使用现有 `Select` 组件，选项为 `low/medium/high/xhigh`。
  - 保存时 trim 主模型；effort 只提交枚举值。
- `frontend/src/i18n/locales/zh.ts` 与 `en.ts`
  - 新增 label、placeholder、hint 和 effort 选项文案。

## Compatibility

- 不配置时，主模型仍为 `gpt-5.4-mini`，effort 仍为 `medium`。
- APIKey 生图路径不受影响。
- Codex transform 只改 image-only model 归一化后的对话主模型，不改客户端传入的 `reasoning.effort`。
- 常量 `openAIImagesResponsesMainModel` 保留，现有断言可继续围绕默认值成立。

## Validation

后端：

- `openai_images_test.go`：覆盖默认、配置主模型、配置 effort、非法 effort 回退。
- `openai_codex_transform_test.go`：覆盖 image-only model normalize 使用配置主模型，空值回退常量。
- `openai_gateway_service_hotpath_test.go` 或相关 gateway 测试：覆盖 `OpenAIGatewayService` 读取 setting 后注入上游请求。
- `setting_service_update_test.go`：覆盖两个 setting 的读取、写入、空值/非法值归一化。

前端：

- `SettingsView.spec.ts`：基础响应包含新字段；保存 payload 包含主模型和 effort；effort 下拉渲染枚举。

## Risks

- 热路径读取 Setting 可能引入 DB 调用；如不加缓存，需确认调用频率可接受。推荐沿用现有 60s 单值缓存模式。
- `codexOAuthTransformOptions` 调整会影响大量测试调用；默认空值必须保留旧行为，避免批量测试需要传入新参数。
- `xhigh` 是项目内已存在合法 effort，但上游实际支持取决于所选主模型；本任务不做模型白名单匹配。
