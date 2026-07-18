# 抽离 Web Search Emulation 设置薄层

## Goal

将 `SettingsView` 中的 Web Search Emulation 设置 UI、局部状态和 API 编排迁入 `frontend/src/features/webSearch/` 的领域组件，让 `SettingsView` 只保留稳定薄接入，同时保持现有 Web Search/AnySearch 配置功能、保存语义和测试行为不变。

## Background

- 用户要求按代码审计建议继续薄层治理，并明确要求保证功能正常。
- 当前 Web Search Emulation 设置卡片仍直接位于 `frontend/src/views/admin/SettingsView.vue:4688`，provider 列表、API Key 显隐/复制、quota、proxy、test 和 reset usage UI 均在大页面内。
- 当前 Web Search Emulation 状态与动作集中在 `frontend/src/views/admin/SettingsView.vue:8616` 起，包括 `webSearchConfig`、`expandedProviders`、`apiKeyVisible`、`loadWebSearchConfig()` 和 `saveWebSearchConfig()`。
- 当前 Settings 主保存流程在 `frontend/src/views/admin/SettingsView.vue:9998` 调用 `saveWebSearchConfig()`，因此 Web Search 配置需要继续跟随整页保存按钮提交，不能改成 provider 编辑后立即保存。
- API 合约已在 `frontend/src/api/admin/settings.ts:1361` 定义：`WebSearchProviderConfig`、`WebSearchEmulationConfig`、`WebSearchTestResult`，并暴露 `get/update/test/reset` 四个端点。
- `frontend/src/features/webSearch/` 已有 AnySearch 规则、渠道开关和账号级开关组件，但还没有完整的后台设置组件。

## Requirements

- R1. 新增 Web Search Emulation 设置领域组件，拥有现有设置卡片的 UI、provider 增删、展开折叠、API Key 显隐/复制、quota/subscribed_at/proxy/test/reset usage 等交互。
- R2. `SettingsView` 只保留组件渲染、组件 ref 和主保存流程中的一次 `save()` 调用；不得继续直接维护 Web Search provider 列表、API Key 状态、test dialog 或 provider 校验细节。
- R3. 保存语义保持不变：用户点击 Settings 主表单保存后，先保存普通 settings，再保存 Web Search Emulation 配置；如果 Web Search 配置校验失败或保存失败，不显示整页保存成功提示。
- R4. AnySearch 行为保持不变：AnySearch provider 允许 API Key 留空；Brave/Tavily 仍要求新 key 或已配置标记。
- R5. API payload 保持后端 snake_case 字段，不新增后端字段、不改后端端点、不改变 `WebSearchEmulationConfig` / `WebSearchProviderConfig` 类型。
- R6. 保留现有用户可见文案 key，不新增硬编码中英文文案；新增文案如有必要必须同步中英文 locale。
- R7. 迁移后相关测试继续覆盖 AnySearch 空 key 提交、provider 配置保存、Web Search feature helper 和 SettingsView 主保存链路。

## Acceptance Criteria

- [ ] `SettingsView.vue` 中 Web Search Emulation 卡片大块模板和 `load/save/test/reset/provider` 逻辑已迁出到 `features/webSearch`，页面仅保留薄装配。
- [ ] Web Search 配置仍通过 Settings 主保存按钮提交；保存 payload 与当前实现等价，`quota_limit <= 0` 仍归一为 `null`。
- [ ] AnySearch 空 API Key 场景仍能保存；Brave/Tavily 缺 key 时仍阻止保存并显示原错误。
- [ ] 已配置 API Key 的 provider 仍显示已配置标记，新增 key 可显隐和复制，reset usage 和 test search 行为不变。
- [ ] `SettingsView.anySearch.spec.ts`、`features/webSearch` 相关测试通过，并新增或调整测试覆盖新组件暴露的 `save()` 链路。
- [ ] 前端 typecheck/lint 或项目认可的定向替代验证通过；无法执行时必须记录原因和剩余风险。

## Out of Scope

- 不修改后端 Web Search Emulation API、服务、数据库或配置存储格式。
- 不治理 `CreateAccountModal.vue` / `EditAccountModal.vue` 的账号级 Web Search/Compatibility extra 编排。
- 不重做 SettingsView 其它大块设置页签。
- 不改变任何 provider 计费、quota、代理或搜索执行语义。

## Open Questions

无阻塞问题。用户已确认按建议优先治理 `SettingsView` 的 Web Search Emulation 设置薄层。
