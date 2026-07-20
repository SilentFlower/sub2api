# Design: 反重力渠道 GIF 多帧兼容

## 1. Architecture

GIF 兼容逻辑由反重力领域拥有，网关入口只做薄调用：

```text
Claude /v1/messages
  -> TransformClaudeToGeminiWithOptions
  -> Antigravity GIF compatibility
  -> antigravityRetryLoop

Gemini /v1beta
  -> injectIdentityPatchToGeminiRequest
  -> cleanGeminiRequest
  -> wrapV1InternalRequest
  -> Antigravity GIF compatibility
  -> antigravityRetryLoop
```

建议拆分：

- `backend/internal/pkg/antigravity/gif_compat.go`：纯请求体扫描、预算分配、GIF 解码合成与 PNG 编码，不访问数据库。
- `backend/internal/service/antigravity_gif_compat.go`：读取全局设置、执行快速候选检测、调用纯转换器并把领域错误映射到协议错误。
- `backend/internal/service/antigravity_gif_settings.go`、`backend/internal/handler/dto/antigravity_gif_settings.go`、`backend/internal/handler/admin/setting_handler_antigravity_gif.go`：分别拥有设置模型/持久化、DTO 和管理 API，避免把 build 私有逻辑继续堆入共享设置大文件。
- `antigravity_gateway_claude.go` 与 `antigravity_gateway_gemini.go`：只在既有转换点调用 service helper，不承载 GIF 算法。
- `frontend/src/features/antigravityGif/`：独立拥有设置 API、类型、组件、测试；`SettingsView.vue` 只负责挂载。

`AccountTypeUpstream` 在 Claude 入口最前面直接调用 `ForwardUpstream`，该分支保持不变，不进入 GIF compatibility helper。

## 2. Global Setting Contract

新增全局 JSON 设置：

```json
{
  "enabled": true,
  "max_frames_per_gif": 8
}
```

- 设置 key 使用 `antigravity_gif_compat_settings`。
- `enabled` 缺省为 `true`。
- `max_frames_per_gif` 缺省为 `8`，合法范围为 `1..16`。
- 设置不存在、空值或 JSON 无法解析时返回完整默认值。
- 持久化帧数越界时读取侧回退为 `8`；管理 API 更新越界时返回 400，不写入。
- 热路径读取设置失败时 fail-open 为默认启用和 8 帧，避免临时存储故障重新暴露 GIF 上游 500。
- 设置仅为全局配置，不读取或写入账号 `extra`。

管理 API：

- `GET /api/v1/admin/settings/antigravity-gif`
- `PUT /api/v1/admin/settings/antigravity-gif`

请求与响应字段保持 snake_case，后端 DTO、前端类型与 service 类型一一对应。

## 3. Request Detection and Preservation

service helper 先对请求体做不分配大对象的 `image/gif` 候选检查：

- 没有候选时直接返回原 `[]byte`，避免数据库查询和 JSON 重编码。
- 有候选后才读取全局设置。
- 设置关闭时直接返回原 `[]byte`，不校验 base64、不解码、不本地拒绝，完整保留旧行为。
- 设置开启时才解析 JSON 并执行转换。

纯转换器同时支持：

- Gemini `inlineData.mimeType` / `inlineData.data`。
- 兼容请求中的 `inline_data.mime_type` / `inline_data.data`。
- 根请求 `contents[].parts[]`。
- 已包装请求 `request.contents[].parts[]`。

扫描和替换使用通用 JSON 结构，保留未知字段。每个 GIF part 被连续的 PNG part 替换；非 GIF part 的内容和相对顺序不变。MIME 比较忽略大小写并清理首尾空白，但只匹配精确的 `image/gif`。

GIF 数据优先按纯 base64 解码，同时兼容常见的 `data:image/gif;base64,` 前缀；前缀中的 MIME 必须同样为 `image/gif`。转换后的 `data` 始终写入不带 data URI 前缀的纯 PNG base64。

## 4. Frame Budget and Sampling

单次请求由 GIF 转换生成的 PNG part 总预算固定为 16，每个 GIF 的目标帧数为：

```text
min(原始帧数, max_frames_per_gif)
```

采用稳定的两阶段公平分配：

1. 按原请求顺序给每个 GIF 分配 1 个名额，保证每个可转换 GIF 至少保留首帧；GIF 数量超过 16 时返回 400。
2. 继续按请求顺序轮转，每轮给尚未达到目标帧数的 GIF 增加 1 个名额，直到预算耗尽或全部达到目标。

每个 GIF 根据最终名额独立选取帧索引：

- 名额为 1 时选首帧。
- 名额大于 1 时必须包含首帧和末帧。
- 中间索引按完整帧序列均匀取样，去重后保持时间顺序。

该方案在预算不足时让各 GIF 的输出数量差最多为 1；相同输入与配置始终得到相同分配。

## 5. GIF Composition

使用 Go 标准库 `image/gif`、`image/draw`、`image/png`：

1. base64 解码 GIF 数据。
2. 在完整逻辑画布上逐帧按 `draw.Over` 合成局部图像。
3. 当前帧合成完成后，如该帧被选中，复制完整画布并编码为 PNG。
4. 进入下一帧前应用当前帧 disposal：
   - `DisposalNone` / 未指定：保留画布。
   - `DisposalBackground`：清空当前帧矩形。
   - `DisposalPrevious`：恢复绘制当前帧前保存的画布。

测试必须用透明、局部更新、`DisposalBackground` 和 `DisposalPrevious` 样例验证最终像素，而不只验证 PNG 可解码。

## 6. Resource Limits

在 `gif.DecodeAll` 前执行轻量 GIF 结构预检，避免压缩数据导致无界内存分配：

- 单个 GIF base64 解码后的原始数据最多 20 MiB。
- 逻辑画布宽高均不得超过 4096，且画布总像素不得超过 16,777,216。
- 单个 GIF 原始帧数最多 1000。
- 单个 GIF 所有图像描述符矩形的累计像素最多 134,217,728。
- 图像描述符矩形必须位于逻辑画布范围内，块结构必须完整。
- GIF 转换后的完整 Gemini JSON 请求体最多 20 MiB，与 Gemini 内联媒体请求上限一致。
- PNG 编码阶段同步限制累计 base64 输出，不能等全部帧进入内存后才检查最终请求大小。

预检只负责资源边界和结构完整性，实际格式正确性仍由 `gif.DecodeAll` 判定。所有限制使用领域常量集中定义，并由边界测试锁定。

## 7. Error Contract

纯转换器返回可分类的领域错误，不包含原始 base64 或完整请求体。错误类别至少包括：

- GIF base64 非法。
- GIF data URI 前缀或前缀 MIME 非法。
- GIF 数据损坏或结构不完整。
- GIF 尺寸、帧数、累计像素或原始字节数超限。
- 请求中的 GIF 数量超过总预算。
- PNG 编码失败。

入口映射：

- Claude 兼容入口：HTTP 400，`invalid_request_error`。
- Gemini 原生入口：HTTP 400，Google/Gemini 错误结构。
- 错误发生后不得调用 `antigravityRetryLoop`，日志只记录安全的错误类别和限制值。

设置关闭时不进入上述错误路径，继续保留原上游响应。

## 8. Retry Consistency

Claude 入口当前会在初始请求、thinking/signature 修复重试以及 budget 修复重试中重新调用 `TransformClaudeToGeminiWithOptions`。新增一个统一转换 helper，把 Claude -> Gemini 与 GIF compatibility 串联，并替换全部调用点，防止重试请求重新携带 `image/gif`。

Gemini 原生入口在 schema 清理和 `wrapV1InternalRequest` 后转换最终 `wrappedBody`，使 20 MiB 上限覆盖真正发送至上游的完整 JSON。后续模型回退和签名重试复用该请求体，因此只需转换一次。

## 9. Admin UI

在网关设置页新增独立功能组件：

- 开关：反重力 GIF 多帧兼容。
- 数字输入：每个 GIF 最大帧数，最小 1、最大 16、默认 8。
- 组件挂载时调用 GET，保存按钮调用 PUT；加载、保存、校验失败沿用现有 toast 风格。
- 前端 API 类型保持 snake_case，不使用 `any` 或双重断言。
- 中英文文案分别放入功能专属 locale 扩展文件，再由现有 `admin/settings.ts` 展开合并。
- 功能 API 放在 `features/antigravityGif/api.ts`，不扩张公共 `api/admin/settings.ts`；组件直接依赖该领域 API。
- `SettingsView.vue` 只新增 import 和组件挂载，不承载该功能状态、类型、API 或保存逻辑。

## 10. Testing Strategy

后端纯转换测试：

- 无 GIF 时请求字节保持不变。
- 单帧、多帧、首尾帧、均匀采样和时间顺序。
- 多 GIF 公平分配、总预算 16、超过 16 个 GIF 返回错误。
- camelCase、snake_case、根请求和包装请求。
- GIF 与 PNG/JPEG/文本混合时非 GIF 内容保持不变。
- 透明、局部帧和三种 disposal 的像素级结果。
- 非法 base64、损坏 GIF 和各资源上限边界。
- PNG/base64 累计输出与最终请求体超过 20 MiB 时返回领域错误。

service / handler 测试：

- 设置缺失、空值、损坏 JSON、越界值均得到默认启用和 8。
- GET/PUT 正常更新，帧数越界返回 400 且不写入。
- 无 GIF 不读取设置；设置关闭时 GIF 原字节透传。
- Claude 与 Gemini 入口发送给上游的请求不含 `image/gif`。
- 转换错误不调用上游；Claude 所有重试请求仍为 PNG。
- `AccountTypeUpstream` 继续原样调用 `ForwardUpstream`。

前端测试：

- 默认加载开启和 8 帧。
- 修改开关和帧数后提交正确 payload。
- 1 与 16 边界可保存，越界值阻止提交并提示。
- GET/PUT 失败展示错误且不产生错误成功提示。

## 11. Rollback

关闭全局开关即可即时恢复修改前的 GIF 透传行为。代码级回滚可移除两个网关薄调用、设置 API 和功能组件；GIF 纯转换文件无其它调用方，不影响其它平台协议转换。
