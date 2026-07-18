# Design: 抽离 Web Search Emulation 设置薄层

## 1. Architecture

新增 `frontend/src/features/webSearch/WebSearchEmulationSettings.vue` 作为 Web Search Emulation 后台设置领域 owner。

```text
SettingsView.vue
  ├─ 普通 settings form / payload / saveSettings()
  └─ WebSearchEmulationSettings.vue
       ├─ load(): getWebSearchEmulationConfig + proxies.list
       ├─ save(): validate + updateWebSearchEmulationConfig
       ├─ provider UI / API key show-copy / quota / proxy
       ├─ testWebSearchEmulation()
       └─ resetWebSearchUsage()
```

## 2. Component Contract

`WebSearchEmulationSettings.vue` 自己拥有局部状态，并通过 `defineExpose` 暴露：

- `save(): Promise<boolean>`：供 `SettingsView.saveSettings()` 在普通 settings 保存后调用。返回 `true` 表示 Web Search 配置保存成功；返回 `false` 表示已处理校验或保存错误，父页面不得显示整页成功提示。
- 可选 `load(): Promise<void>`：用于测试或将来父页面显式刷新；组件挂载时自行调用，替代当前 `SettingsView.loadSettings()` 末尾的 `loadWebSearchConfig()`。

`SettingsView` 保留：

- `<WebSearchEmulationSettings ref="webSearchEmulationSettingsRef" />`
- `const webSearchEmulationSettingsRef = ref<... | null>(null)`
- `const wsOk = await webSearchEmulationSettingsRef.value?.save() ?? true`

这样能保持整页保存链路不变，同时让 Web Search 状态不再污染 `SettingsView`。

## 3. Data and API Contracts

- 继续复用 `frontend/src/api/admin/settings.ts` 中的 `WebSearchProviderConfig`、`WebSearchEmulationConfig`、`WebSearchTestResult`。
- provider payload 字段保持 snake_case：`api_key_configured`、`quota_limit`、`subscribed_at`、`proxy_id`、`expires_at`。
- 保存前继续把 `quota_limit` 非正数归一为 `null`。
- `formatSubscribedAt()` / `parseSubscribedAt()` 继续按 UTC 日期处理，避免日期重复编辑产生时区漂移。

## 4. UI and Behavior Compatibility

必须保留现有行为：

- 全局启用开关控制 provider 区域显示。
- provider 默认新增为 Brave，默认 quota 为 1000。
- provider 删除后重建 `expandedProviders` / `apiKeyVisible` 索引状态。
- 已配置但未重新输入的 API Key 显示 `api_key_configured` 标记。
- AnySearch 使用 `anySearchProviderOption` 和 `AnySearchAPIKeyHint`，允许空 API Key。
- Brave/Tavily 缺少新 key 且 `api_key_configured=false` 时阻止保存。
- test search 使用默认查询文案；reset usage 仍要求确认。

## 5. Testing Strategy

定向测试优先覆盖行为不变：

- `frontend/src/features/webSearch/__tests__/anySearch.spec.ts`
- `frontend/src/features/webSearch/__tests__/channelFeatures.spec.ts`
- `frontend/src/views/admin/__tests__/SettingsView.anySearch.spec.ts`
- 新增或改造 `WebSearchEmulationSettings` 组件测试，覆盖：
  - AnySearch 空 key 保存；
  - Brave/Tavily 缺 key 阻止保存；
  - quota_limit 清洗；
  - `save()` 返回 false 时父页面不显示成功。

质量验证优先执行：

```bash
cd frontend && pnpm vitest run src/features/webSearch src/views/admin/__tests__/SettingsView.anySearch.spec.ts
cd frontend && pnpm typecheck
cd frontend && pnpm lint
```

如果环境限制导致全量前端验证不能执行，至少执行定向 Vitest 和 TypeScript 编译，并记录限制。

## 6. Rollback

本任务只改前端组件组织和测试，不改后端。若迁移后行为异常，可回滚新增 `WebSearchEmulationSettings.vue` 和 `SettingsView.vue` 对应薄接入改动，恢复原页面内逻辑。
