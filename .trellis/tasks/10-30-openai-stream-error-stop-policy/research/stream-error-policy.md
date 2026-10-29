# 流式错误停调缺口研究

下列缺口与代码行号以任务开始时的提交 `db8ff80c68ec0d10c9cf13e29178441ad73d957c` 为基线；修复后的执行证据见 `implement.md`。

## 已核对的定义与行为

- `backend/internal/service/account.go:1168`：`IsCustomErrorCodesEnabled` 仅适用于 API Key 账号与严格布尔开关。
- `backend/internal/service/account.go:1325`：`GetCustomErrorCodes` 从 JSON 反序列化后的数组提取状态码；`ShouldHandleErrorCode` 在开关关闭或列表为空时不筛选。
- `backend/internal/service/ratelimit_service.go:333`：`HandleUpstreamError` 具有池模式、错误码过滤、临时规则及平台错误处理；default 分支对命中的自定义错误码调用现有停用方法。
- `backend/internal/service/ratelimit_service.go:1176`：`handleCustomErrorCode` 通知运行时阻断并调用 `accountRepo.SetError`。
- `backend/internal/service/openai_account_runtime_block_fastpath.go:95`：共用账号错误入口有请求级错误排除与运行时阻断，不能绕开而直接写数据库。
- `backend/internal/repository/account_repo.go:1463`：`SetError` 同时写入 `status=error`、`schedulable=false` 与错误消息，并发布账号变更 outbox、同步调度快照。
- `backend/internal/service/openai_account_runtime_block_fastpath.go:558`：请求调度中的运行时阻断以持久化冷却字段为依据；本次复用其现有契约，后续调度资格仍由错误状态和同步的账号快照排除。

## 缺口与调用链

- `backend/internal/service/openai_gateway_passthrough.go:1419`：结构化余额耗尽当前不会命中 401/403/429 等分类，按 502 兜底。
- `backend/internal/service/openai_gateway_passthrough.go:1626`：裸 `error` 使用保守换号分类器，余额耗尽不在当前正向识别范围。
- `backend/internal/service/openai_gateway_passthrough.go:1655`：终止事件的账号状态 helper 只处理 401、部分 403、429、529，通用 502 被直接返回。
- `backend/internal/service/openai_gateway_response_handling.go:531` 与透传读取器对应分支：首输出前裸错误如果不触发换号，可能完全绕过账号状态 helper。
- `backend/internal/service/openai_gateway_passthrough.go:1848`：非流式 SSE 转 JSON 共用流式错误分类，修改分类后需要回归一致性。
- 裸 `error` 后还可能有 `response.failed` 或成功事件；原生与透传已有 pending/终止收口机制，应复用并测试。

## 现有测试入口

- `openai_account_runtime_block_fastpath_test.go`：账号运行时阻断与既有配额行为。
- `openai_gateway_response_flush_test.go`、`openai_gateway_passthrough_flush_test.go`：错误帧、权威终止、usage 与输出刷新。
- `openai_access_state_failover_test.go`：请求级与凭据级失败区分。
- `openai_nonstreaming_terminal_failure_failover_test.go`：非流式 SSE 转 JSON 的分类一致性。

## 环境和排查证据

- 当前分支 `build`，任务创建前工作区干净，基线提交 `db8ff80c68ec0d10c9cf13e29178441ad73d957c`。
- 线上只读排查对应镜像修订 `2a5053964c5aea3eb114c5338de33b32c7d8ca00`；关键两个流式处理文件与当前 HEAD 无差异。
- 当前错误规则保存为 `[502]`，账号仍 active；账号记录更新时间晚于示例请求，因此不能把当前配置当作请求时的配置历史。
- 系统默认 Go 为 1.22.2，本轮验证使用 Go 1.27.2 与独立构建缓存；原始源码 overlay 已复现漏停，修复后的 59 个定向场景和完整服务层 15,664 个场景已通过。
