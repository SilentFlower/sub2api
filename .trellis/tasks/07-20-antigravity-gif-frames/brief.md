# Brief — 反重力渠道 GIF 多帧兼容

## Goal

- 在反重力内部请求边界把 Gemini 不支持的 `image/gif` 转换为按时间排序的多张 PNG，避免 GIF 请求继续以不受支持 MIME 触发上游 500。

## Scope

- 覆盖 Anthropic `/v1/messages` 转 Gemini 和反重力原生 Gemini `/v1beta` 两条内部协议转换链路，包括 Claude 重新转换请求的重试路径。
- 使用 Go 标准库预检、解码和正确合成 GIF 增量帧及 disposal，再均匀抽帧并输出 `image/png` inline data。
- 支持纯 base64 和 `data:image/gif;base64,` 输入；非 GIF part、未知 JSON 字段和内容顺序保持不变。
- 新增默认开启的全局设置；每个 GIF 默认最多 8 帧，可在 1 到 16 范围调整，不支持账号 `extra` 覆盖。
- 单次请求最多生成 16 个 PNG part；多个 GIF 按稳定轮转公平分配，名额允许时保留首尾帧。
- 新增管理 GET/PUT API 和独立前端 feature；设置模型、DTO、handler、API 与组件均由 GIF 领域文件拥有，共享文件只做路由注册、网关调用和页面挂载。
- 非法、损坏或资源超限 GIF 在本地返回协议兼容的 400，并确保不调用上游。

## Non-Goals

- 不改变 `AccountTypeUpstream -> ForwardUpstream` 的原有直通行为。
- 不处理非反重力平台，不增加通用媒体转码。
- 不把 GIF 转成视频，也不保留帧时长和循环播放语义。
- 不增加帧数上限之外的抽帧策略配置。

## Key Context

- Claude 图片当前在 `backend/internal/pkg/antigravity/request_transformer.go` 直接映射为 Gemini `inlineData`；Claude 网关的初始、signature 修复和 budget 修复路径都会重新转换请求，必须统一接入 GIF compatibility helper。
- Gemini 原生入口应在 `cleanGeminiRequest` 后、`wrapV1InternalRequest` 前转换，后续回退与重试复用已转换请求体。
- GIF 算法由 `backend/internal/pkg/antigravity` 领域文件拥有，设置感知由 service helper 拥有，两个网关入口只做薄调用。
- 设置缺失、损坏或读取失败时默认启用并使用 8 帧；管理员关闭时不识别、不校验、不转换，完整保留旧上游行为。
- `gif.DecodeAll` 前必须限制单 GIF 原始数据 20 MiB、画布最大 4096x4096/16,777,216 像素、最多 1000 帧、累计帧矩形最多 134,217,728 像素。
- disposal 合成、预算分配、无 GIF 字节不变和错误不调用上游都需要自动化测试；日志不得记录原始 base64 或完整请求体。

## Acceptance

- 单帧 GIF 输出一个可解码 PNG；多帧 GIF 默认最多输出 8 帧，包含首尾且顺序正确。
- 透明、局部更新、`DisposalBackground` 和 `DisposalPrevious` 的输出像素正确。
- 两条内部协议入口及 Claude 重试发送给上游的请求均不再包含 `image/gif`。
- 多 GIF 生成的 PNG 总数不超过 16，公平分配稳定可重复；超过可承载 GIF 数量时返回 400。
- 全局设置默认开启、默认 8，合法范围 1 到 16；关闭后恢复原透传行为。
- 非 GIF 内容保持数据、MIME 和顺序；非法或超限 GIF 返回稳定 4xx 且上游未被调用。
- `AccountTypeUpstream` 和其它平台行为不变，相关后端与前端测试、类型检查和 diff 检查通过。

## Next Step

- 用户确认 `prd.md`、`design.md`、`implement.md` 和本 brief 后，运行 `task.py start`，再通过 `trellis-before-dev` 与实现路由进入编码和验证。
