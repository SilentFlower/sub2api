# 新增 Grok 套餐额度进度条

## Goal

在管理端账号列表的 Grok 账号用量区域中，新增一个独立的 Grok CLI Billing 额度进度条，用于展示从 CLIProxyAPI 管理面板迁移来的 Grok subscription billing 数据。

该功能必须与 Sub2API 现有 Grok 额度逻辑隔离：现有 Grok rate-limit 响应头采样、主动 probe、`extra.grok_usage_snapshot`、请求/Token 进度条和未知状态提示都保持原行为。

## Confirmed Facts

- 现有 Grok 额度逻辑是被动读取 xAI rate-limit headers，并保存到账号 `extra.grok_usage_snapshot`。
- 现有管理端账号用量 API 会返回 `grok_request_quota`、`grok_token_quota`、`grok_quota_snapshot_state`、`grok_last_quota_probe_at` 等字段。
- 现有前端 `AccountUsageCell.vue` 会基于 `grok_request_quota` 和 `grok_token_quota` 显示请求/Token 两条进度条，并显示主动 probe 组件 `GrokQuotaProbeCell`。
- CLIProxyAPI Management Center 的 Grok CLI Billing 逻辑不是读取 rate-limit headers，而是使用 Grok OAuth access token 请求：
  - `GET https://cli-chat-proxy.grok.com/v1/billing?format=credits`
  - `GET https://cli-chat-proxy.grok.com/v1/billing`
- Grok CLI Billing 响应中会按 `config` 解析周 credits、月 credits、产品用量和按量付费额度等字段。
- Sub2API 已有 `GrokTokenProvider.GetAccessToken` 可为 Grok OAuth 账号获取有效 access token。

## Requirements

- 新增 Grok CLI Billing 查询链路，不能修改或替换现有 Grok rate-limit header 额度逻辑。
- 新增字段必须使用独立命名，不能写入或覆盖 `extra.grok_usage_snapshot`。
- 新增前端展示必须是独立进度条或独立区块，不能把 Grok CLI Billing 数据混入现有请求/Token 进度条。
- 仅 Grok OAuth 账号支持 Grok CLI Billing 查询；非 Grok 或非 OAuth 账号不得触发该查询。
- Grok CLI Billing 查询必须通过现有 token 获取链路发起请求；如果该 Grok 账号已绑定代理，则沿用该代理请求 billing URL，未绑定代理则直连。
- 日志、错误、API 响应和前端展示不得暴露 access token、refresh token、Authorization 或完整敏感上游响应。
- 用户可见文案必须进入 i18n。
- 增量实现应覆盖后端解析/服务测试和前端展示测试。

## Acceptance Criteria

- [ ] 现有 `grok_request_quota` / `grok_token_quota` 进度条、未知提示和主动 probe 行为保持不变。
- [ ] Grok OAuth 账号可以显示独立的 Grok CLI Billing 进度条；查询失败时不影响旧 Grok 额度展示。
- [ ] 后端不会把 Grok CLI Billing 数据写入 `grok_usage_snapshot`，并使用独立 DTO/字段返回给前端。
- [ ] 后端解析覆盖 monthly billing 与 `format=credits` billing 的成功、缺字段、失败状态。
- [ ] 前端在 mobile、desktop、dark mode 下能显示新增进度条的 loading/empty/error/success 状态。
- [ ] 测试覆盖新增 API 字段、解析逻辑和用户可见展示。

## Out of Scope

- 不重构现有 Grok quota snapshot、active probe、自动暂停或调度逻辑。
- 不实现 Grok subscription quota reset。
- 不用 xAI API Management Billing 替代 Grok CLI billing，因为两者计费域不同。

## Decisions

- 新增展示不使用“CPA”作为产品命名；“CPA”仅指 CLIProxyAPI 迁移来源。
- 前端用户可见标题使用“Grok 套餐额度”。
- MVP 展示口径与 CLIProxyAPI Management Center 对齐：独立区块中以月 credits 为主进度条；如果 `?format=credits` 返回有效周数据，则在同一区块内额外显示周 credits 小进度条；如果返回产品用量则显示产品用量；如果返回按量付费 cap，则显示按量付费进度/剩余额度，否则显示按量付费未启用状态。
- 查询触发方式采用“缓存优先 + 低频懒刷新 + 主动刷新”：
  - 账号用量接口优先返回最近一次缓存快照。
  - 后端把最近一次成功的 Grok CLI Billing 查询结果保存到独立字段，例如 `extra.grok_billing_snapshot`。
  - 缓存缺失或超过 TTL 时，前端在账号行进入视口后排队触发后台刷新。
  - 管理员可以点击刷新图标主动刷新当前账号。
  - 刷新失败只影响“Grok 套餐额度”区块，不影响现有 Grok 请求/Token 额度展示。
  - 刷新有全局并发限制和单账号 TTL，避免账号列表打开时同时请求大量 Grok billing。
  - 账号用量接口返回缓存快照时，前端显示更新时间/过期状态。
