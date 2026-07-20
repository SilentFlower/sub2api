# Implement Plan: 反重力渠道 GIF 多帧兼容

## 1. Preparation

- 读取 `implement.jsonl` 中列出的规范。
- 运行 `trellis-before-dev`，重新确认后端协议适配、错误处理、前端类型和私有功能隔离要求。
- 复核相关实体和签名：`AntigravityGatewayService`、`SettingService`、`ClaudeRequest`、`GeminiPart`、`GeminiInlineData`、设置 DTO、管理路由与前端 settings API。
- 记录现有 Claude 三处 `TransformClaudeToGeminiWithOptions` 调用和 Gemini `cleanGeminiRequest` / `wrapV1InternalRequest` 顺序，避免只覆盖首次请求。

## 2. Backend Domain Implementation

1. 在 `backend/internal/pkg/antigravity/` 新增 GIF compatibility 领域文件。
   - 定义转换选项、资源限制常量和可分类错误。
   - 实现请求结构扫描，支持 camelCase、snake_case、根请求和 `request` 包装。
   - 保证无 GIF 时返回原字节，非 GIF part 不被重建或改写。
   - 实现纯 base64、`data:image/gif;base64,` 与单层 `base64:data:image/gif;base64,` 输入兼容、GIF 结构预检、帧数发现和资源限制。
   - 在 PNG 编码过程中限制累计 base64 输出，并将最终 Gemini JSON 请求体限制为 20 MiB。
   - 实现总预算 16 的公平分配与稳定均匀采样。
   - 实现 disposal 正确的完整画布合成和 PNG 编码。

2. 新增 `backend/internal/pkg/antigravity/gif_compat_test.go`。
   - 使用程序化构造的小尺寸 GIF fixture，避免提交大二进制测试资源。
   - 覆盖像素级合成、采样、预算、结构形态、无改写和错误边界。

## 3. Global Setting and Admin API

1. 新增独立的 `backend/internal/service/antigravity_gif_settings.go`，定义 `AntigravityGIFCompatibilitySettings` 与默认函数。
   - 字段为 `Enabled` 和 `MaxFramesPerGIF`。
   - 新增 `SettingKeyAntigravityGIFCompatSettings`。
   - 实现读取默认、损坏值回退、越界归一和更新校验。

2. 在独立的 GIF 设置 DTO 与 handler 文件中新增 GET/PUT 契约。
   - API 路径为 `/admin/settings/antigravity-gif`。
   - 更新请求只接受 `max_frames_per_gif` 的 1..16 范围。
   - handler 使用现有 response envelope 和错误映射。

3. 新增 service 与 handler 定向测试。
   - 覆盖默认启用、默认 8、合法更新、越界拒绝、仓储错误和 JSON 损坏回退。

## 4. Gateway Integration

1. 新增 `backend/internal/service/antigravity_gif_compat.go`。
   - 快速检测无 GIF 请求并直接返回原字节。
   - 仅候选请求读取全局设置；设置读取失败使用默认启用和 8。
   - 设置关闭时直接返回原字节。
   - 调用 `internal/pkg/antigravity` 纯转换器并保留领域错误类别。

2. 调整 Claude 兼容入口。
   - 保留 `AccountTypeUpstream -> ForwardUpstream` 分支不变。
   - 抽取统一 helper 串联 `TransformClaudeToGeminiWithOptions` 与 GIF conversion。
   - 替换初始转换、signature 修复重试和 budget 修复重试的全部调用点。
   - 转换错误用 `writeClaudeError(..., 400, "invalid_request_error", ...)` 返回，不进入上游重试。

3. 调整 Gemini 原生入口。
   - 在 `cleanGeminiRequest` 和 `wrapV1InternalRequest` 后转换最终 `wrappedBody`，确保请求大小检查覆盖完整上游 JSON。
   - 转换错误用 `writeGoogleError(..., 400, ...)` 返回，不进入上游重试。

4. 扩展网关测试。
   - 捕获上游请求体，断言两条入口只发送 PNG part。
   - 覆盖开关关闭透传、错误不调用上游、Claude 重试仍转换和直通账号不变。

## 5. Frontend Settings

1. 新增 `frontend/src/features/antigravityGif/api.ts`。
   - 定义 `AntigravityGIFCompatibilitySettings`。
   - 使用共享 `apiClient` 封装 typed GET/PUT，不修改公共 `api/admin/settings.ts`。

2. 新增 `frontend/src/features/antigravityGif/AntigravityGIFSettings.vue`。
   - 组件自行加载和保存设置。
   - 使用现有开关控件与数字输入，限制范围 1..16。
   - 处理 loading、saving、校验和 toast，不向 `SettingsView` 泄露领域状态。

3. 薄接入 `frontend/src/views/admin/SettingsView.vue`。
   - 在网关设置 tab 中挂载组件。
   - 只增加组件 import 和模板节点，不增加该功能状态或 API 调用。

4. 新增中英文功能 locale 扩展和组件测试。
   - 覆盖默认值、加载失败、合法保存、越界阻止和保存失败。

## 6. Validation Commands

优先执行：

```bash
cd backend && go test -tags=unit ./internal/pkg/antigravity
cd backend && go test -tags=unit ./internal/service -run 'Test.*Antigravity.*GIF|Test.*GIF'
cd backend && go test -tags=unit ./internal/handler/admin -run 'Test.*Antigravity.*GIF|Test.*GIF'
cd frontend && pnpm test:run -- src/features/antigravityGif
cd frontend && pnpm typecheck
git diff --check
```

再执行相关回归：

```bash
cd backend && go test -tags=unit ./internal/pkg/antigravity ./internal/service ./internal/handler/admin
cd frontend && pnpm lint:check
```

若全量包测试存在与本任务无关的既有失败，保留命令、失败用例和定向测试通过证据。

## 7. Risk Points

- Claude 入口存在多次重新转换请求的重试路径，遗漏任一调用会让 GIF 在重试时重新出现。
- `gif.DecodeAll` 会为全部帧分配图像，必须先完成结构和累计像素预检。
- PNG/base64 输出必须边编码边扣减预算，不能先缓存全部大帧再检查 20 MiB 请求上限。
- disposal 处理顺序错误会导致透明或局部帧输出残缺，必须做像素级断言。
- 通用 JSON 替换可能改变未知字段或非 GIF part；无候选和关闭开关必须返回原字节。
- 多 GIF 预算分配必须稳定，不能依赖 map 遍历顺序。
- 设置默认开启要求读取失败 fail-open，但转换失败必须 fail-closed 为 400，二者不能混淆。
- `SettingsView.vue` 已较大，新功能必须留在 `features/antigravityGif`，避免继续堆积状态与保存逻辑。

## 8. Done Definition

- PRD 中两条反重力内部转换入口、全局配置、8 帧默认值、16 帧总预算和直通账号边界全部满足。
- GIF 资源限制、disposal 合成、错误映射和重试一致性有自动化测试。
- 前端可读取和更新全局设置，类型检查与定向组件测试通过。
- 相关后端回归、前端 lint/typecheck 和 `git diff --check` 通过，或记录明确的既有失败证据。
