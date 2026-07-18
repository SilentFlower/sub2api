# Brief — 抽离 Web Search Emulation 设置薄层

## Goal

- 将 `SettingsView` 中的 Web Search Emulation 设置 UI、局部状态和 API 编排迁入 `frontend/src/features/webSearch/` 的领域组件，让 `SettingsView` 只保留稳定薄接入，同时保持现有 Web Search/AnySearch 配置功能、保存语义和测试行为不变。

## Scope

- 新增 `frontend/src/features/webSearch/WebSearchEmulationSettings.vue`，承载 Web Search Emulation 设置卡片、provider 增删、展开折叠、API Key 显隐/复制、quota/subscribed_at/proxy/test/reset usage 等交互。
- `SettingsView.vue` 替换为 `<WebSearchEmulationSettings ref="webSearchEmulationSettingsRef" />`，删除页面内 Web Search 状态与函数，并在 `saveSettings()` 中调用组件 `save()`。
- 保持主保存链路：普通 settings 保存成功后再保存 Web Search 配置；Web Search 校验或保存失败时不显示整页保存成功。
- 继续复用 `frontend/src/api/admin/settings.ts` 中的 `WebSearchProviderConfig`、`WebSearchEmulationConfig`、`WebSearchTestResult` 和四个现有 API 端点。
- 补充或调整测试，覆盖 AnySearch 空 key、Brave/Tavily 缺 key、quota_limit 清洗、组件 `save()` 返回值和 SettingsView 主保存链路。

## Non-Goals

- 不修改后端 Web Search Emulation API、服务、数据库或配置存储格式。
- 不治理 `CreateAccountModal.vue` / `EditAccountModal.vue` 的账号级 Web Search/Compatibility extra 编排。
- 不重做 SettingsView 其它大块设置页签。
- 不改变 provider 计费、quota、代理或搜索执行语义。

## Key Context

- 当前设置卡片位于 `frontend/src/views/admin/SettingsView.vue:4688`，状态和动作集中在 `frontend/src/views/admin/SettingsView.vue:8616` 起。
- 当前主保存流程在 `frontend/src/views/admin/SettingsView.vue:9998` 调用 `saveWebSearchConfig()`；迁移时必须保留顺序和成功提示条件。
- API 类型和端点位于 `frontend/src/api/admin/settings.ts:1361` 起，payload 字段保持 snake_case。
- AnySearch 空 API Key 规则由 `features/webSearch/anySearch.ts` 拥有，不能重新写回 `SettingsView`。
- 组件建议通过 `defineExpose` 暴露 `save(): Promise<boolean>`，可选暴露 `load(): Promise<void>`；组件挂载时自行加载配置和 proxy 列表。

## Acceptance

- `SettingsView.vue` 中 Web Search Emulation 大块模板和 `load/save/test/reset/provider` 逻辑已迁出到 `features/webSearch`，页面仅保留薄装配。
- Web Search 配置仍通过 Settings 主保存按钮提交，保存 payload 与当前实现等价，`quota_limit <= 0` 仍归一为 `null`。
- AnySearch 空 API Key 场景仍能保存；Brave/Tavily 缺 key 时仍阻止保存并显示原错误。
- 已配置 API Key 的 provider 仍显示已配置标记，新增 key 可显隐和复制，reset usage 和 test search 行为不变。
- `SettingsView.anySearch.spec.ts`、`features/webSearch` 相关测试通过，并新增或调整测试覆盖新组件暴露的 `save()` 链路。
- 前端 typecheck/lint 或项目认可的定向替代验证通过；无法执行时记录原因和剩余风险。

## Next Step

- 用户确认 planning artifacts 和本 brief 后，运行 `task.py start`，随后进入 `trellis-route(implement)`；实现前运行 `trellis-before-dev` 并按 `implement.md` 执行。
