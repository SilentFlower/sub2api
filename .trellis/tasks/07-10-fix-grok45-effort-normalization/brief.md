# Brief — 修复 Grok 4.5 effort 归一化

## Goal

- 为 `grok-4.5` 增加与 GLM 机制一致的 provider-specific effort 归一化，并让 Grok/GLM usage log 始终记录最终上游请求体实际发送的 effort。

## Scope

- Grok 4.5 映射：`none/minimal/low -> low`、`medium -> medium`、`high/xhigh/extra high/max/ultracode -> high`。
- GLM 保持 `low/medium/high -> high`、`xhigh/extra high/max/ultracode -> max`，并规范化保留 `none/minimal`。
- 共用嵌套 `reasoning.effort` / 扁平 `reasoning_effort` 的字段定位、大小写和分隔符识别机制；未知值原样透传，缺失值不注入。
- 覆盖 Grok Responses、Chat Completions、Messages 两种路由、WebSocket HTTP bridge，以及 GLM 原生 Chat、Messages 转 Chat、Responses 转 Chat fallback。
- 从完成全部改写后的最终上游 body 提取 `OpenAIForwardResult.ReasoningEffort`，沿现有链路写入 usage log。
- 前端把实际字段 `none/minimal` 显示为 `None/Minimal`。

## Non-Goals

- 不统一 `/v1/messages` 默认 Responses 与强制 Chat 路由的缺省 effort 行为。
- 不保存客户端原始 effort，不展示归一化前后双值。
- 不新增数据库 migration、usage log 字段、DTO 或前端 API 类型。
- 不改变 Grok、GLM 之外模型的 provider-specific effort 语义。

## Key Context

- Grok 4.5 官网只支持 `low/medium/high`，默认 `high` 且不能关闭；GLM-5.2 官网支持 `none/minimal/low/medium/high/xhigh/max`。
- Grok 规则必须精确匹配最终模型 `grok-4.5`，不能影响支持 `xhigh` 的 `grok-4.20-multi-agent`。
- 请求体是日志真值源；最终 body 没有 effort 时 usage log 必须为空，不根据厂商默认值或 thinking 状态推断。
- 未知值保持原样并沿用上游校验、错误透传和 failover 语义。
- `patchGrokResponsesBody` 是 Grok Responses、Messages Responses 和 WebSocket HTTP bridge 的共享入口；raw Chat 和 Responses 转 Chat 需要调用统一 provider 分派 helper。
- 主要风险是提取时机和模型 guard：必须在最后一次 body 改写后提取，并对 Grok 使用精确模型匹配。

## Acceptance

- Grok/GLM 已知档位按 PRD 映射，大小写和分隔符变体可识别，未知值原样透传，缺失值不注入。
- 所有列出的 HTTP、Messages 和 WebSocket 路径使用一致规则，非 `grok-4.5` 模型不受影响。
- 最终上游 body、`OpenAIForwardResult.ReasoningEffort` 和持久化 usage log 三者一致。
- `none/minimal` 在前端显示为 `None/Minimal`。
- 无数据库变更，现有 Grok 字段/工具清理、配额快照、错误处理测试继续通过。
- 后端 service 单测、前端 formatter Vitest、typecheck、lint 和 `git diff --check` 通过。

## Next Step

- 用户确认本 brief 和 planning artifacts 后运行 `task.py start`；进入 `in_progress` 后先执行 `trellis-route(implement)`，再按实施计划编码和验证。
