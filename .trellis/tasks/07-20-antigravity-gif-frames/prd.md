# 反重力渠道 GIF 多帧兼容

## Goal

解决反重力渠道账号向 Gemini 上游发送 `image/gif` 时被拒绝的问题。网关在反重力请求边界将 GIF 解码为多张 PNG 帧，使模型能够理解动图中的关键变化，同时避免将上游不支持的 MIME 类型继续透传为 500 错误。

## Background

- Gemini 官方图片输入格式仅列出 PNG、JPEG、WEBP、HEIC 和 HEIF，不包含 GIF。
- 当前 Claude 兼容入口在 `backend/internal/pkg/antigravity/request_transformer.go` 中把图片 `media_type` 与 base64 数据直接转换为 Gemini `inlineData`。
- 当前原生 Gemini 入口在 `backend/internal/service/antigravity_gateway_gemini.go` 中注入身份提示词并包装请求，未处理不受支持的 GIF。
- 本任务仅处理反重力内部协议转换链路，不能改变其它平台账号的媒体输入行为。
- `AccountTypeUpstream` 当前直接调用 `ForwardUpstream`，继续保持原有直通行为，不增加 GIF 识别或转换。

## Requirements

- 识别反重力请求中的 `image/gif` 内联数据，并在请求发送至上游前完成转换。
- 解码 GIF 动画并从完整时间序列中均匀抽取多帧，至少保留首帧和末帧。
- 正确合成 GIF 的增量帧与 disposal 行为，不能直接把局部帧当成完整画面。
- 将每个抽取结果编码为独立的 `image/png` Gemini `inlineData` part，并保持原内容块的相对位置。
- 同时覆盖 Anthropic `/v1/messages` 转反重力 Gemini 的路径，以及反重力原生 Gemini `/v1beta` 路径。
- 非 GIF 图片和不含图片的请求必须保持现有输出不变。
- GIF base64 非法、数据损坏、尺寸或解码资源超限时，在本地返回明确的客户端错误，不向上游发送请求，也不暴露为通用 500。
- 不引入 FFmpeg、ImageMagick 或新的运行时服务依赖；优先使用 Go 标准库完成 GIF 解码和 PNG 编码。
- 提供反重力 GIF 多帧兼容的全局设置，默认开启；该设置统一作用于所有走反重力内部协议转换的账号，不读取账号 `extra` 覆盖。
- 管理员关闭全局兼容开关时，网关不识别、不转换也不本地拒绝 GIF，按修改前的现有链路原样透传。
- 每个 GIF 默认最多输出 8 帧；管理员可以在 1 到 16 范围内调整该上限，未配置或持久化值无效时回退为 8。
- 单次请求由 GIF 转换生成的 PNG part 总数最多为 16；多个 GIF 共享该预算。
- 多个 GIF 共享预算时应公平分配：每个 GIF 优先保留首帧和末帧，剩余额度再按原始时间序列均匀补充中间帧。

## Acceptance Criteria

- [ ] 单帧 GIF 转换为一个 `image/png` part，像素内容可解码。
- [ ] 多帧 GIF 默认最多均匀抽取 8 帧，包含首帧和末帧，输出顺序与时间顺序一致。
- [ ] 管理员调整帧数上限后，新请求按配置值抽帧；缺失或无效配置回退为 8。
- [ ] 新安装和存量升级在未持久化配置时均默认启用反重力 GIF 多帧兼容。
- [ ] 管理员关闭全局兼容开关后，所有走反重力内部协议转换的账号统一停止 GIF 抽帧转换，GIF 请求保持修改前的透传与上游响应行为。
- [ ] 单个请求由 GIF 转换生成的 PNG part 总数不超过 16；多个 GIF 时每个 GIF 在预算允许时至少保留首尾帧，且分配结果稳定可重复。
- [ ] 带透明、局部更新和 disposal 的 GIF 输出为正确合成后的完整 PNG 帧。
- [ ] Claude 兼容入口和原生 Gemini 入口均不再向反重力上游发送 `image/gif`。
- [ ] 多个 GIF、GIF 与普通图片混合时，各内容块顺序稳定，普通图片保持原数据与 MIME 类型。
- [ ] 非法或超限 GIF 返回稳定、可识别的 4xx 错误，测试验证上游未被调用。
- [ ] `AccountTypeUpstream` 账号继续走 `ForwardUpstream`，请求体和现有上游响应行为不变。
- [ ] 相关单元测试通过，且现有反重力请求转换与 Gemini 网关测试无回归。

## Out of Scope

- 将 GIF 转换为 MP4 或其它视频格式。
- 为非反重力渠道增加通用媒体转码。
- 完整保留 GIF 的帧时长、无限循环等播放语义；模型接收的是按时间排序的静态帧序列。
- 增加除帧数上限之外的抽帧策略配置。

## Notes

- 官方资料：<https://ai.google.dev/gemini-api/docs/image-understanding>
- 该任务涉及双入口请求转换、GIF 帧合成与资源限制，属于复杂任务；开始实现前需要补齐 `design.md` 与 `implement.md`。
