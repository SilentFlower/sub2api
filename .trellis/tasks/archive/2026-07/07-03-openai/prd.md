# OpenAI 生图对话主模型与思考预算可配置化

## Goal

将 OpenAI OAuth 生图路径中两个当前硬编码的"对话主模型"与"思考预算(reasoning.effort)"改为可通过全局系统设置(Setting)动态配置，保留常量作为默认回退，旧行为不破坏。

## Background

当前实现里，OAuth 账户走 Responses API 生图时，请求骨架的 `model` 与 `reasoning.effort` 都是硬编码：

- `backend/internal/service/openai_images_responses.go:357` —— 请求骨架写死
  `{"reasoning":{"effort":"medium","summary":"auto"}, "model":"", ...}`，随后 line 358 把 `model` 强制设为常量 `openAIImagesResponsesMainModel = "gpt-5.4-mini"`。
- `backend/internal/service/openai_codex_transform.go:894` —— Codex 变换路径同样把 `reqBody["model"]` 强制覆盖为 `openAIImagesResponsesMainModel`。
- 客户端传入的 `gpt-image-2` 只会进 `tools[].model`(图模工具)，**对话主模型与思考预算对客户端完全不透明、不可调**。

需求来源：希望能在不改源码的前提下，把对话主模型(如换成 `gpt-5.4`)与思考预算(如 `low/medium/high`)动态调整。

## Decision (已与用户确认)

- **配置层级**：全局系统设置(Setting 表，键值对)，不做按账号/按组区分。
- **存储形态**：单字符串字段(不做 JSON 容器)，先满足当前两个可调项，遵循 YAGNI。
- **回退策略**：常量 `openAIImagesResponsesMainModel` 保留为默认值，配置为空时回落到常量；思考预算默认 `medium`。
- **任务类型**：本任务横跨后端设置链路、OpenAI 网关热路径、前端系统设置页和测试，按复杂任务处理，需 `design.md` 与 `implement.md` 后再启动。

## Requirements

### R1 新增两个 Setting 键
- `openai.image_generation.main_model` —— 对话主模型，默认 `gpt-5.4-mini`(与现有常量一致)。
- `openai.image_generation.reasoning_effort` —— 思考预算，合法值 `low/medium/high/xhigh`，默认 `medium`。
  - `xhigh` 已在项目内被认可：`openai_compat_model.go:86` 会输出 `reasoningEffort = "xhigh"`；定价元数据 `resources/model-pricing/...json` 也有 `supports_xhigh_reasoning_effort` 标志。
- 参考现有 `SettingService` 的键定义与读写范式(`backend/internal/service/setting_service.go`，如 `GetOpenAICodexUserAgent`、`GetAntigravityUserAgentVersion` 等)。

### R2 服务层读取并注入
- 在 `buildOpenAIImagesResponsesRequest`(`openai_images_responses.go`)中：
  - `model` 字段：读取 Setting → 为空回落 `openAIImagesResponsesMainModel`。
  - `reasoning.effort`：读取 Setting → 为空回落 `"medium"`，并校验合法值。
- 在 `openai_codex_transform.go:894` 的 `reqBody["model"]` 覆盖处，同样改为读 Setting → 回落常量。
- 注意：`openai_codex_transform.go` 路径里 `reasoning.effort` 是从客户端请求透传的，**本次不强制改写**该路径的 effort(仅改主模型)；只改 Responses 生图路径的 effort。需在设计阶段确认这条边界。

### R3 校验
- 主模型：非空校验(允许任意字符串，因为上游 baseURL 可定制，不强行白名单；但空值回落默认)。
- effort：限定 `low/medium/high/xhigh`，非法值回落 `medium` 并记日志。

### R4 管理后台可配置
- 在系统设置前端页面暴露这两个配置项(参考现有 OpenAI 相关设置项的 UI 范式)。
- 中文 i18n 文案(项目约定所有面向用户文案为中文)。
- `reasoning_effort` 使用枚举控件，选项为 `low/medium/high/xhigh`，避免管理员输入非法值。

### R5 不破坏现有行为与测试
- 不配置时行为与现在完全一致(`gpt-5.4-mini` + `medium`)。
- 现有测试(`openai_codex_transform_test.go:823/860`、`openai_images_test.go:649`、`openai_gateway_service_hotpath_test.go:309/573`)断言的是 `openAIImagesResponsesMainModel` 符号，常量保留即不挂。
- 新增测试覆盖：配置生效、空值回落、effort 非法值回落、两条路径(Responses / Codex transform)主模型均受控。

## Acceptance Criteria

- [ ] 新增 Setting 键 `openai.image_generation.main_model` 与 `openai.image_generation.reasoning_effort`，带默认值。
- [ ] `buildOpenAIImagesResponsesRequest` 的 `model` 与 `reasoning.effort` 改为读 Setting，空值/非法值正确回落。
- [ ] `openai_codex_transform.go` 主模型覆盖改为读 Setting 回落常量。
- [ ] 管理后台可编辑这两个配置项，含中文文案。
- [ ] 不配置时行为与当前一致；现有测试全绿。
- [ ] 新增单元测试覆盖配置生效、回落、effort 校验、两条路径。
- [ ] 中文 Javadoc/注释(项目约定)。

## Out of Scope

- 不做按账号/按账号组的差异化配置(已确认走全局)。
- 不做 JSON 容器式扩展(单字符串即可，YAGNI)。
- 不改 APIKey 路径(该路径主模型由 `account.GetMappedModel` 决定，已可配)。
- 不改 Codex transform 路径的 `reasoning.effort`(透传即可，避免越界)。

## Technical Notes

- `OpenAIGatewayService` 已持有 `settingService`，OAuth Images 调用 `buildOpenAIImagesResponsesRequest` 前可从 `s.settingService` 读取主模型与 effort，并通过参数传入请求构造函数；构造函数不直接依赖 service，便于单元测试。
- Codex transform 仍保持纯 map 变换，给 `codexOAuthTransformOptions` 或 `normalizeOpenAIResponsesImageOnlyModel` 增加主模型参数即可；未传时继续回退 `openAIImagesResponsesMainModel`。
- 系统设置已有 key-value 默认值、`SystemSettings`、admin setting handler、前端 `SettingsView.vue`/`settings.ts` 类型链路；新增两个字符串字段沿现有 Gateway Forwarding 配置扩展，不需要数据库 migration。
- effort 合法值限定在 `low/medium/high/xhigh`；非法值在服务层回退 `medium` 并记录 warn 日志。
