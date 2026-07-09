# 新增 Grok 套餐额度进度条 - Implement

## Checklist

1. 后端 DTO 与解析
   - 新增 Grok CLI Billing payload 类型和 snapshot 类型。
   - 实现 cents、percent、period、product usage 的归一化解析。
   - 实现 weekly/monthly 两个 billing 响应合并逻辑。
   - 单测覆盖字段缺失、周/月合并、产品用量、按量付费、非法 payload。

2. 后端服务与缓存
   - 在 Grok billing 独立服务中使用 `GrokTokenProvider.GetAccessToken` 获取 OAuth token。
   - 如果账号已绑定代理，则沿用该代理请求 Grok CLI billing URL；未绑定代理则直连。
   - 成功后写入 `extra.grok_billing_snapshot`。
   - `AccountUsageService.getGrokUsage` 只读取该独立 snapshot 并追加到 UsageInfo，不改旧 Grok quota snapshot 逻辑。
   - 单测确认 `grok_usage_snapshot` 不被覆盖。

3. 后端 API
   - 新增 `GET /api/v1/admin/grok/accounts/:id/billing-quota` 或等价独立刷新接口。
   - handler 使用统一 response envelope 和 `response.ErrorFrom`。
   - 单测覆盖成功、非 Grok、非 OAuth、上游失败脱敏。

4. 前端 API 与类型
   - `frontend/src/api/admin/grok.ts` 增加 billing quota 类型和刷新 API。
   - `frontend/src/types/index.ts` 为 `AccountUsageInfo` 增加 `grok_billing_quota`。
   - i18n 增加“Grok 套餐额度”、月额度、周额度、产品用量、按量付费、刷新、过期、失败等文案。

5. 前端 UI
   - 在 Grok 账号分支中新增独立“Grok 套餐额度”区块。
   - 月 credits 为主进度条，周 credits 有数据时显示附加小进度条，产品用量和按量付费状态与 CLIProxyAPI Management Center 对齐展示。
   - 新增刷新图标，支持主动刷新当前账号。
   - 增加前端懒刷新队列和 TTL，避免列表打开时并发请求大量账号。
   - 失败只影响该区块，不影响现有请求/Token 进度条。

6. 验证
   - 后端：运行 Grok billing / quota / account usage 相关单测。
   - 前端：运行 `AccountUsageCell` 或新增组件测试、typecheck。
   - 人工检查移动端、桌面端和 dark mode。

## Validation Commands

```bash
cd backend && go test -tags=unit ./internal/pkg/xai ./internal/service ./internal/handler/admin -run 'Grok.*Billing|Grok.*Quota|AccountUsage'
cd frontend && pnpm vitest run src/components/account/__tests__/AccountUsageCell.spec.ts
cd frontend && pnpm typecheck
```

## Rollback Points

- 后端新增字段为可选字段，回滚前端展示不会影响现有 usage API。
- 新增 `extra.grok_billing_snapshot` 与旧 `extra.grok_usage_snapshot` 隔离，删除新增字段即可停止展示。
- 新增刷新接口独立于现有 `/admin/grok/accounts/:id/quota`，可单独下线。
