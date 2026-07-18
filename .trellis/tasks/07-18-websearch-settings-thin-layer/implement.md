# Implement Plan: 抽离 Web Search Emulation 设置薄层

## 1. Preparation

- 读取 `implement.jsonl` 中的规范。
- 运行 `trellis-before-dev`，确认前端组件、目录结构和 build 私有功能隔离规范。
- 检查 `SettingsView.vue` 当前 Web Search 模板、状态、函数和保存调用，避免遗漏。

## 2. Implementation Checklist

1. 在 `frontend/src/features/webSearch/` 新增 `WebSearchEmulationSettings.vue`。
   - 迁移 `SettingsView.vue` 的 Web Search Emulation 模板。
   - 迁移 `webSearchConfig`、`expandedProviders`、`apiKeyVisible`、test dialog、proxy 列表和 provider helper。
   - 复用 `AnySearchAPIKeyHint`、`anySearchProviderOption`、`hasRequiredWebSearchProviderAPIKey`、`webSearchProviderAPIKeyPlaceholderKey`。
   - 使用 typed props/emits 或 `defineExpose`，不使用未收窄的 `any`。

2. 调整 `frontend/src/views/admin/SettingsView.vue`。
   - 替换原 Web Search Emulation 卡片为 `WebSearchEmulationSettings`。
   - 删除页面内 Web Search 状态与函数。
   - 删除 `loadSettings()` 中 `await loadWebSearchConfig()`。
   - 在 `saveSettings()` 中调用组件 `save()`，保持现有成功提示条件。
   - 清理不再需要的 import 和类型。

3. 测试迁移。
   - 根据新组件位置调整 `SettingsView.anySearch.spec.ts` 和 harness stub。
   - 新增 `frontend/src/features/webSearch/__tests__/WebSearchEmulationSettings.spec.ts` 或等效测试，覆盖 PRD acceptance 中的行为。
   - 保持已有 AnySearch helper 测试不降级。

4. 自查。
   - `rg "webSearchConfig|saveWebSearchConfig|loadWebSearchConfig|wsTest|expandedProviders|apiKeyVisible" frontend/src/views/admin/SettingsView.vue` 应无旧逻辑残留。
   - `rg "@/features/webSearch" frontend/src/views/admin/SettingsView.vue` 只保留新组件和必要类型。

## 3. Validation Commands

优先执行：

```bash
cd frontend && pnpm vitest run src/features/webSearch src/views/admin/__tests__/SettingsView.anySearch.spec.ts
cd frontend && pnpm typecheck
cd frontend && pnpm lint
git diff --check
```

如验证失败，先修复与本任务相关的失败；如失败来自环境或既有问题，记录证据和剩余风险。

## 4. Risk Points

- `SettingsView.saveSettings()` 当前在普通 settings 保存成功后才保存 Web Search 配置；迁移时不能改变顺序。
- `WebSearchEmulationSettings.vue` 自加载可能与主页面 loading 独立；测试需确保配置加载完成后再断言。
- AnySearch 空 API Key 是 build 专属规则，必须继续由 `features/webSearch/anySearch.ts` 拥有，不要把规则重新写回 `SettingsView`。
- Proxy 下拉依赖 `adminAPI.proxies.list()`；组件迁移后仍要处理失败回退为空列表。
- API Key 复制依赖浏览器剪贴板，测试中不要引入不可控真实剪贴板副作用。

## 5. Done Definition

- 代码迁移完成，`SettingsView` 对 Web Search Emulation 只做薄装配。
- 相关测试和类型检查通过或记录明确环境限制。
- PRD acceptance 全部满足。
