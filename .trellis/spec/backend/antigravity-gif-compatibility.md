# Antigravity GIF Compatibility

> 反重力渠道把 Gemini 不支持的 GIF 内联图片转换为有界的 PNG 帧序列；领域模块拥有全部转换规则，共享网关、路由和设置页只保留薄接入。

---

## Scenario: 反重力 GIF 多帧兼容

### 1. Scope / Trigger

- Trigger: 修改反重力 Claude/Gemini 请求转换、GIF 解码与抽帧、全局设置、管理 API 或管理端设置组件时，必须按本节检查。
- 只作用于反重力内部协议转换账号；`AccountTypeUpstream` 继续原样调用 `ForwardUpstream`。
- 领域所有者：
  - 纯转换：`backend/internal/pkg/antigravity/gif_compat.go`
  - 设置与网关编排：`backend/internal/service/antigravity_gif_*.go`
  - 管理 API：独立 DTO、handler 文件和两条路由注册
  - 前端：`frontend/src/features/antigravityGif/`
- 共享 gateway、`SettingsView.vue` 和 locale 主文件只能调用、挂载或展开领域实现，不得承载 GIF 算法、默认值、校验或保存状态。

### 2. Signatures

后端纯转换边界：

```go
func ContainsGIFInlineDataCandidate(body []byte) bool
func TransformGIFInlineData(body []byte, maxFramesPerGIF int) ([]byte, error)
func IsGIFCompatibilityError(err error) bool
func GIFCompatibilityDiagnosticsFromError(err error) (GIFCompatibilityDiagnostics, bool)
```

全局设置边界：

```go
type AntigravityGIFCompatibilitySettings struct {
	Enabled         bool `json:"enabled"`
	MaxFramesPerGIF int  `json:"max_frames_per_gif"`
}

func DefaultAntigravityGIFCompatibilitySettings() *AntigravityGIFCompatibilitySettings
func (s *SettingService) GetAntigravityGIFCompatibilitySettings(ctx context.Context) (*AntigravityGIFCompatibilitySettings, error)
func (s *SettingService) SetAntigravityGIFCompatibilitySettings(ctx context.Context, settings *AntigravityGIFCompatibilitySettings) error
```

前端 API 边界：

```typescript
interface AntigravityGIFCompatibilitySettings {
  enabled: boolean
  max_frames_per_gif: number
}

function getAntigravityGIFCompatibilitySettings(): Promise<AntigravityGIFCompatibilitySettings>
function updateAntigravityGIFCompatibilitySettings(
  settings: AntigravityGIFCompatibilitySettings
): Promise<AntigravityGIFCompatibilitySettings>
```

### 3. Contracts

- 持久化 key 固定为 `antigravity_gif_compat_settings`，JSON 字段固定为 `enabled` 和 `max_frames_per_gif`。
- 默认配置为 `enabled=true`、`max_frames_per_gif=8`；合法帧数范围为 `1..16`。
- 管理 API 为 `GET /api/v1/admin/settings/antigravity-gif` 与 `PUT /api/v1/admin/settings/antigravity-gif`，继续使用统一 `{code,message,reason,data}` envelope。前端 `apiClient` 使用去掉 `/api/v1` 后的相对路径 `/admin/settings/antigravity-gif`。
- 设置缺失、空值或损坏 JSON时返回完整默认值；持久化帧数越界时只把帧数回退为 `8`。管理 API 写入越界值必须返回 `400`，reason 为 `ANTIGRAVITY_GIF_MAX_FRAMES_INVALID`，且不得写入仓储。
- 热路径没有 `image/gif` 候选时必须返回原 `[]byte` 且不读取设置；读取设置失败时按默认开启和 8 帧继续；设置关闭时必须原字节透传，不解析、不拒绝 GIF。
- 纯转换同时支持根 `contents[].parts[]`、包装后的 `request.contents[].parts[]`、`inlineData.mimeType/data` 和 `inline_data.mime_type/data`。MIME 只在 trim 后大小写不敏感地精确匹配 `image/gif`。
- 输入支持纯标准 base64、纯 base64url、`data:image/gif;base64,`，以及部分客户端产生的单层 `base64:data:image/gif;base64,` 包装；解析器最多剥离一层外部 `base64:` 标记，再校验内部 data URI。base64 payload 可接受 URL 转义后的标准 padded/raw base64；反转义后按标准 padded/raw base64 解码，若 payload 含 `-`/`_` 且标准解码失败，再按 padded/raw base64url 解码。不主动移除中间空白（Go 标准解码器自身允许的 CR/LF 除外）。输出必须是按时间顺序排列、无任何前缀的 `image/png` base64 part。非 GIF part、未知字段和相对顺序必须保留。
- 单 GIF 配置上限为 `1..16`，单请求转换生成的 PNG part 总数固定不超过 `16`。多个 GIF 使用稳定轮转公平分配；名额大于 1 时保留首尾帧。
- GIF 合成必须在完整逻辑画布上处理透明局部帧以及 `DisposalNone`、`DisposalBackground`、`DisposalPrevious`，不能直接把局部帧编码为最终 PNG。
- 资源上限固定为：单 GIF 解码后 `20 MiB`、画布单边 `4096`、画布 `16,777,216` 像素、原始帧 `1000`、累计帧矩形 `134,217,728` 像素、最终包装后的 Gemini JSON `20 MiB`。PNG 编码过程中必须同步扣减累计 base64 预算。
- Claude 路径的初始转换、signature 修复重试和 budget 修复重试必须统一经过 `transformClaudeRequestWithGIFCompatibility`。
- Gemini 原生路径的每个上游请求体构建点都必须统一经过 `wrapV1InternalRequestWithGIFCompatibility` 或等价 helper：初始请求、模型 fallback 重试、`thoughtSignature` 清理重试都必须在 `wrapV1InternalRequest` 后立刻应用 GIF 兼容转换，禁止重新包装后直接 `NewAPIRequest` 或 `antigravityRetryLoop`。
- 转换失败时 service helper 必须记录一次结构化日志 `antigravity_gif_transform_failed`，并只输出安全诊断字段：阶段、输入/trim/payload 长度、是否外层 `base64:`、是否 data URI、data URI MIME、是否 base64、是否含 `%` 转义痕迹、是否含 base64url 字符、padding 余数和 base64 解码错误类别。禁止记录原始 base64、完整 data URI 或完整请求体。

### 4. Validation & Error Matrix

| 条件 | 行为 |
| --- | --- |
| 请求不含 `image/gif` 候选 | 原字节返回，不读设置，不重编码 JSON |
| 设置读取失败 | 记录安全告警，使用默认开启和 8 帧 |
| `enabled=false` | 原字节透传，保留原上游响应行为 |
| PUT 缺少或无法绑定请求 | HTTP 400，reason 为 `ANTIGRAVITY_GIF_SETTINGS_INVALID_REQUEST` |
| PUT `max_frames_per_gif` 不在 `1..16` | HTTP 400，reason 为 `ANTIGRAVITY_GIF_MAX_FRAMES_INVALID`，不写仓储 |
| base64、data URI、GIF 结构或 disposal 数据非法 | 返回 `GIFCompatibilityError`；Claude 映射为 `400 invalid_request_error`，Gemini 映射为 Google/Gemini 400 |
| `base64:data:image/gif;base64,...` | 剥离单层外部标记后按 GIF data URI 解码并转换 |
| data URI payload 包含 `%2B/%2F/%3D` 等 URL 转义后的标准 base64 字符 | 仅反转义 payload 后按标准 padded/raw base64 解码并转换 |
| 纯 payload 包含 `-`/`_` 的 base64url GIF 数据 | 标准 base64 失败后按 padded/raw base64url 解码并转换 |
| 输入字节、尺寸、帧数、累计像素或输出请求超限 | 返回安全的 `GIFCompatibilityError`，不包含原始 base64 或完整请求体 |
| 带空格/Tab 等解码器不接受的中间空白，或仍无法按标准/base64url padded/raw 解码的 GIF payload | 本地 400，同时日志包含 `gif_stage=base64_decode` 等安全诊断字段，便于判别是 payload 形态错误、padding 问题还是真实损坏 |
| 单请求 GIF 数量超过 16 | 本地 400，不调用上游 |
| 转换成功 | 上游请求不得再包含 `image/gif`，转换 part 的 MIME 必须为 `image/png` |
| `AccountTypeUpstream` | 不进入兼容 helper，请求体保持原样 |

所有转换错误都必须在 `antigravityRetryLoop` 前终止，测试需断言上游调用次数为 0。

### 5. Good/Base/Bad Cases

- Good: 透明局部更新 GIF 经画布合成后均匀输出 8 张 PNG，包含首帧和末帧，文本和普通图片 part 保持原位置与字段。
- Good: 两个多帧 GIF 在总预算 16 内稳定、公平分配，重复输入得到相同帧索引。
- Good: Gemini 原生请求首次上游返回 model-not-found 或 thought-signature 400 后，fallback / 签名清理重试发送的请求体仍只包含 `image/png`，不重新出现 `image/gif`。
- Good: 管理设置加载失败时控件保持禁用，不能把组件内默认值误写回服务端；保存期间开关和保存按钮均禁用。
- Base: 不含 GIF 的反重力请求不查设置、不解析 JSON，并保持原始字节。
- Base: 管理员关闭开关后，损坏 GIF 也按旧链路透传，不在本地返回 400。
- Bad: 在共享 gateway 中直接扫描 JSON、解码 GIF 或复制默认值，导致重试路径遗漏并增加 `main -> build` 合并冲突。
- Bad: Gemini 原生 signature 或 fallback 重试重新调用 `wrapV1InternalRequest` 后直接发送上游，导致首次请求已转换但二次请求重新携带 `image/gif`。
- Bad: 在全部 PNG 已进入内存后才检查 20 MiB，无法阻止编码阶段的内存膨胀。
- Bad: GET 失败后仍允许保存，会把 UI 的默认开启和 8 帧覆盖未知的服务端状态。

### 6. Tests Required

建议运行：

```bash
cd backend && go test -tags=unit ./internal/pkg/antigravity
cd backend && go test -tags=unit ./internal/service -run 'Test.*Antigravity.*GIF|Test.*GIF' -count=1
cd backend && go test -tags=unit ./internal/handler/admin -run 'Test.*Antigravity.*GIF|Test.*GIF' -count=1
cd frontend && pnpm exec vitest run src/features/antigravityGif
cd frontend && pnpm typecheck
```

断言点：

- 纯转换：无 GIF 字节不变；纯标准 base64、纯 base64url、data URI、单层 `base64:` 包装和 URL 转义后的标准 base64 均可解码；默认 8 帧包含首尾；多 GIF 公平预算；snake/camel 与根/包装结构；未知字段保留；透明和三类 disposal 像素正确；所有资源边界返回可分类错误。
- service：无候选跳过设置；关闭时透传；设置读取失败使用默认值；配置帧数实际控制输出；转换失败时只打安全诊断日志，不泄露原始图片 payload。
- gateway：Claude 初始与两类 rectifier 重试、Gemini 初始包装请求、Gemini model fallback 重试和 Gemini `thoughtSignature` 清理重试都只发送 PNG；转换错误不上游；`AccountTypeUpstream` 保持原 body。
- handler/frontend：默认值、合法边界 1/16、越界拒绝、GET/PUT 失败、加载与保存期间禁用交互。

### 7. Wrong vs Correct

#### Wrong

```go
// antigravity_gateway_gemini.go
if bytes.Contains(wrappedBody, []byte("image/gif")) {
	// 在共享入口实现解析、抽帧、PNG 编码和限制判断。
}
```

问题：共享入口承担领域语义，Claude 重试容易遗漏同一规则，也会扩大上游同步冲突面。

#### Correct

```go
// antigravity_gateway_gemini.go
wrappedBody, err = s.applyAntigravityGIFCompatibility(ctx, wrappedBody)
if err != nil {
	return nil, s.writeGoogleError(c, http.StatusBadRequest, antigravityGIFClientErrorMessage(err, "Invalid request"))
}
```

转换、设置读取、错误分类和资源限制由反重力 GIF 领域文件拥有；共享 gateway 只调用一次并映射协议错误。前端同理，`SettingsView.vue` 只挂载 `AntigravityGIFSettings`。

#### Wrong

```go
// antigravity_gateway_gemini.go
retryWrappedBody, err := s.wrapV1InternalRequest(projectID, mappedModel, cleanedInjectedBody)
retryResult, err := s.antigravityRetryLoop(antigravityRetryLoopParams{body: retryWrappedBody})
```

问题：Gemini 原生重试会重新从 `injectedBody` 或清理后的 body 构建上游 JSON；如果只在首次请求转换 GIF，二次请求会把 `image/gif` 重新送到 Gemini。

#### Correct

```go
// antigravity_gateway_gemini.go
retryWrappedBody, err := s.wrapV1InternalRequestWithGIFCompatibility(ctx, projectID, mappedModel, cleanedInjectedBody)
retryResult, err := s.antigravityRetryLoop(antigravityRetryLoopParams{body: retryWrappedBody})
```

每个重新包装并发送上游的 Gemini 原生请求都必须经过同一个包装兼容 helper；测试应捕获全部上游请求体，而不只断言首次请求。
