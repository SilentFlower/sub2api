# State Management

> 本项目前端状态管理约定。

---

## Overview

全局状态使用 Pinia，store 位于 `frontend/src/stores/`。项目采用 setup-style store：

```ts
export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const isAuthenticated = computed(() => !!token.value && !!user.value)
  return { user, isAuthenticated }
})
```

示例依据：

- `frontend/src/stores/auth.ts`：认证、token、pending auth session、自动刷新。
- `frontend/src/stores/app.ts`：侧边栏、全局 loading、toast、公开设置和版本缓存。
- `frontend/src/stores/subscriptions.ts`：订阅相关状态。

---

## State Categories

- 组件局部状态：弹窗开关、当前输入、临时筛选、按钮 loading，优先留在组件。
- 可复用交互状态：分页大小、表格加载、防抖搜索、复制等，放 composable。
- 全局 UI 状态：侧边栏、toast、全局 loading、站点公开设置，放 `app` store。
- 认证状态：token、refresh token、当前用户、权限、pending auth session，放 `auth` store。
- 服务端数据：默认由 API 调用按需加载，不引入独立 server-state 库；需要跨页面缓存时放对应 store 并提供 refresh/force 参数。
- URL 状态：路由参数、query、页面标题放 router 相关模块，不复制进 store，除非需要持久化。

---

## When to Use Global State

只有满足以下条件之一才新增或扩展 store：

- 多个无父子关系页面需要共享同一状态。
- 状态必须跨路由保留，例如登录态、公开设置、站点版本。
- 状态变化会影响全局 UI，例如 toast、侧边栏、loading。
- 多个组件需要统一刷新和缓存同一组后端数据。

单个页面内的筛选条件、表格列开关、弹窗表单不应默认进入全局 store。当前项目已有 `utils/tablePreferences.ts` 等工具处理持久化偏好，先搜索复用。

---

## Persistence

项目使用 `localStorage` 和 `sessionStorage` 保存少量状态：

- `auth.ts` 保存 `auth_token`、`refresh_token`、`auth_user`、`token_expires_at`、`pending_auth_session`。
- `i18n/index.ts` 保存 `sub2api_locale`。
- 部分表格偏好由 `utils/tablePreferences.ts` 管理。

读写 storage 时要处理 JSON parse 失败和浏览器限制。认证相关清理必须覆盖 token、refresh token、用户缓存和过期时间。

---

## Server State

API client 已处理：

- 自动添加 Authorization。
- GET 请求附带 timezone。
- 标准 envelope 解包。
- 401 refresh token 队列。
- 特定 423 / ops disabled 行为。

因此 store 或组件里不要重复实现这些横切逻辑。需要取消请求时，API 函数应支持 `signal?: AbortSignal`，例如 `admin/users.ts` 的 `list`。

---

## Common Mistakes

- 不要把 API 原始响应 envelope 存进 store；`apiClient` 返回的已经是 `data`。
- 不要在多个 store 中复制同一认证字段。
- 不要让 store 直接依赖组件实例或 DOM。
- 不要在 store 初始化时无条件发大量请求；参考 `app.ts` 的 cache flag 和 loading flag 防重复。
