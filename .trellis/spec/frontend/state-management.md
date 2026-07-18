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

## Component-local Server State and Parent Save

当子组件自主管理一组后端配置，但父页面拥有统一保存按钮时，子组件必须显式维护
初始加载状态，并让 `save()` 等待加载完成后再提交。

### 1. Scope / Trigger

- Trigger: 新增或抽离类似 `SettingsView -> FeatureSettings` 的组件，子组件在
  `onMounted()` 中加载远端配置，父组件稍后通过 `ref.save()` 触发保存。
- 风险：父组件的主表单可能早于子组件 API 返回完成。如果 `save()` 直接读取默认空状态，
  会把远端已有配置覆盖成默认值。

### 2. Signatures

子组件暴露给父页面的接口应保持可等待：

```ts
defineExpose({
  load, // () => Promise<void>
  save, // () => Promise<boolean>
})
```

内部至少维护：

```ts
let loaded = false
let loadPromise: Promise<void> | null = null

async function ensureLoaded(): Promise<boolean> {
  if (loaded) return true
  if (loadPromise) await loadPromise
  if (loaded) return true
  await load()
  return loaded
}
```

### 3. Contracts

- `load()` 必须复用正在进行的 `loadPromise`，避免同一配置并发读取时互相覆盖。
- 首次加载成功或“配置不存在但可使用默认值”的 404 场景，可以把 `loaded` 置为 `true`。
- 非预期加载失败不得把 `loaded` 置为 `true`；`save()` 应返回 `false` 并提示错误，不提交默认状态。
- `save()` 的第一步必须 `await ensureLoaded()`；只有加载完成后才执行校验、归一化和 API update。
- 父页面在 `ref` 不存在时可以按业务需要降级为 `true`，但只要子组件已挂载，保存安全由子组件保证。

### 4. Validation & Error Matrix

| 条件 | 行为 |
| --- | --- |
| 初始加载已完成 | `save()` 直接校验并提交当前状态 |
| 初始加载进行中 | `save()` 等待同一个 `loadPromise` 后再提交 |
| 初始加载返回 404 且业务允许默认配置 | 标记 loaded，允许保存默认状态 |
| 初始加载失败且非允许错误 | 不标记 loaded，`save()` 返回 false，不调用 update API |

### 5. Good/Base/Bad Cases

- Good: 用户在页面刚打开时立即点击保存，子组件 `save()` 等待远端配置返回，然后提交远端配置派生出的状态。
- Base: 用户等待页面加载完成后保存，`save()` 不重复请求，直接使用已加载状态。
- Bad: 子组件默认 `enabled=false, providers=[]`，`onMounted(load)` 尚未完成时父页面调用 `save()`，
  直接把空配置 PUT 到后端。

### 6. Tests Required

组件测试必须覆盖：

- API 延迟返回时调用 `save()`，断言 update API 在加载完成前未被调用。
- 加载完成后 `save()` 使用远端返回数据提交，而不是默认空状态。
- 加载失败时 `save()` 返回 false，且不调用 update API。

### 7. Wrong vs Correct

#### Wrong

```ts
onMounted(() => {
  void load()
})

async function save(): Promise<boolean> {
  await updateConfig(localState)
  return true
}
```

问题：父页面保存按钮和子组件加载请求之间存在竞态，可能用默认状态覆盖远端配置。

#### Correct

```ts
onMounted(() => {
  void load()
})

async function save(): Promise<boolean> {
  if (!(await ensureLoaded())) return false
  await updateConfig(localState)
  return true
}
```

`save()` 是子组件的稳定边界，必须保证提交前本地状态已经代表远端当前配置。

---

## Common Mistakes

- 不要把 API 原始响应 envelope 存进 store；`apiClient` 返回的已经是 `data`。
- 不要在多个 store 中复制同一认证字段。
- 不要让 store 直接依赖组件实例或 DOM。
- 不要在 store 初始化时无条件发大量请求；参考 `app.ts` 的 cache flag 和 loading flag 防重复。
