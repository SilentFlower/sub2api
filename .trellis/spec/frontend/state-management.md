# 状态管理

> Pinia 状态管理的使用方式和规范。

---

## 概述

- **状态管理库**: Pinia（`^2.1.7`）
- **Store 风格**: 统一使用 **Setup Store**（Composition API 函数式）
- **禁止使用**: Options Store 风格、Vuex

---

## Store 列表

| Store 文件 | Store ID | 职责 |
|-----------|----------|------|
| `stores/auth.ts` | `'auth'` | 认证状态、Token 管理、自动刷新 |
| `stores/app.ts` | `'app'` | 全局 UI 状态（侧边栏、Loading、Toast、公共设置缓存、版本管理） |
| `stores/subscriptions.ts` | `'subscriptions'` | 用户订阅数据、缓存、轮询 |
| `stores/adminSettings.ts` | — | 管理员设置 |
| `stores/onboarding.ts` | — | 引导流程状态 |

统一导出：

```typescript
// stores/index.ts
export { useAuthStore } from './auth'
export { useAppStore } from './app'
export { useAdminSettingsStore } from './adminSettings'
export { useSubscriptionStore } from './subscriptions'
export { useOnboardingStore } from './onboarding'
```

---

## Store 编写模式

### 标准结构

```typescript
// 参考：stores/auth.ts
export const useAuthStore = defineStore('auth', () => {
  // ==================== State ====================
  const user = ref<User | null>(null)
  const token = ref<string | null>(null)

  // ==================== Computed ====================
  const isAuthenticated = computed(() => !!token.value && !!user.value)
  const isAdmin = computed(() => user.value?.role === 'admin')

  // ==================== Actions ====================
  async function login(credentials: LoginRequest): Promise<LoginResponse> {
    const response = await authAPI.login(credentials)
    token.value = response.token
    user.value = response.user
    return response
  }

  function logout() {
    user.value = null
    token.value = null
    localStorage.removeItem('token')
  }

  // ==================== Return Store API ====================
  return {
    // State
    user,
    token,
    // Computed
    isAuthenticated,
    isAdmin,
    // Actions
    login,
    logout,
  }
})
```

### 内部规范

1. 使用注释区块分隔：`// ==================== State/Computed/Actions ====================`
2. 公开函数添加 JSDoc 注释
3. 只读状态使用 `readonly()` 包装导出
4. 持久化使用手动 `localStorage`（非 Pinia 插件）

---

## 状态分类

### 何时使用全局状态（Store）

| 场景 | 使用 Store | 示例 |
|------|-----------|------|
| 多个组件共享的状态 | ✅ | 用户信息、认证 Token |
| 需要持久化的状态 | ✅ | 主题偏好、语言设置 |
| 跨路由保持的状态 | ✅ | 侧边栏展开/收起 |
| 仅单个组件使用 | ❌ 使用组件本地 `ref` | 表单输入值、弹窗开关 |
| 仅父子组件传递 | ❌ 使用 Props/Emit | 列表项的选中状态 |

### 何时使用本地状态

```vue
<script setup lang="ts">
// 组件本地状态 — 不需要放到 Store
const showModal = ref(false)
const formData = reactive({ name: '', email: '' })
</script>
```

### 何时使用 Composable

```typescript
// 可复用的有状态逻辑 — 使用 Composable
const { items, loading, load } = useTableLoader(options)
```

---

## 服务端状态

项目不使用 VueQuery / TanStack Query。服务端数据通过以下方式管理：

1. **Store 内 Action**: 在 Store 的 Action 中调用 API，缓存到 Store State
2. **Composable**: 使用 `useTableLoader` 等 Composable 管理列表数据
3. **组件本地**: 简单的一次性数据获取直接在组件中调用 API

```typescript
// Store 缓存模式（参考 stores/app.ts）
const publicSettings = ref<PublicSettings | null>(null)

async function loadPublicSettings() {
  if (publicSettings.value) return  // 有缓存则跳过
  publicSettings.value = await settingsAPI.getPublic()
}
```

---

## 常见错误

### 1. 使用 Options Store

```typescript
// ❌ 禁止
export const useAuthStore = defineStore('auth', {
  state: () => ({ user: null }),
  actions: { login() { ... } }
})

// ✅ 使用 Setup Store
export const useAuthStore = defineStore('auth', () => {
  const user = ref(null)
  function login() { ... }
  return { user, login }
})
```

### 2. Store 中直接操作 DOM

```typescript
// ❌ Store 不应操作 DOM
function showError(msg: string) {
  document.querySelector('.toast').textContent = msg
}

// ✅ Store 管理状态，组件响应式渲染
const toasts = ref<Toast[]>([])
function addToast(toast: Toast) {
  toasts.value.push(toast)
}
```

### 3. 过度使用全局状态

不要把所有状态都放到 Store。表单输入值、弹窗开关、临时加载状态等应留在组件本地。
