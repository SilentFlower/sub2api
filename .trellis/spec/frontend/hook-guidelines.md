# Hook（Composable）规范

> Vue 3 Composables 的编写模式和使用规范。

---

## 概述

本项目使用 Vue 3 的 Composition API，将可复用的有状态逻辑封装在 `src/composables/` 目录下的 Composable 函数中。

---

## 现有 Composables

| 文件 | 函数名 | 功能 |
|------|--------|------|
| `useForm.ts` | `useForm()` | 统一表单提交（loading 状态、错误捕获、Toast 通知） |
| `useTableLoader.ts` | `useTableLoader()` | 通用表格数据加载（分页、筛选、搜索防抖、请求取消） |
| `useClipboard.ts` | `useClipboard()` | 剪贴板复制（带 Clipboard API 检测和 fallback） |
| `useModelWhitelist.ts` | `useModelWhitelist()` | 模型白名单管理 |
| `useNavigationLoading.ts` | `useNavigationLoading()` | 路由导航加载状态 |
| `useRoutePrefetch.ts` | `useRoutePrefetch()` | 路由预加载 |
| `useOnboardingTour.ts` | `useOnboardingTour()` | 新手引导流程 |
| `useAccountOAuth.ts` | `useAccountOAuth()` | 账号 OAuth 授权流程 |
| `useGeminiOAuth.ts` | `useGeminiOAuth()` | Gemini OAuth |
| `useAntigravityOAuth.ts` | `useAntigravityOAuth()` | Antigravity OAuth |
| `useOpenAIOAuth.ts` | `useOpenAIOAuth()` | OpenAI OAuth |
| `useKeyedDebouncedSearch.ts` | `useKeyedDebouncedSearch()` | 带 key 的防抖搜索 |

---

## 编写模式

### 标准结构

```typescript
// 参考：src/composables/useTableLoader.ts

// 1. 定义 Options 接口
interface TableLoaderOptions<T, P> {
  fetchFn: (page: number, pageSize: number, params: P, options?: FetchOptions) => Promise<BasePaginationResponse<T>>
  initialParams?: P
  pageSize?: number
  debounceMs?: number
}

// 2. 导出 use* 函数（泛型支持）
export function useTableLoader<T, P extends Record<string, any>>(options: TableLoaderOptions<T, P>) {
  // 3. 内部响应式状态
  const items = ref<T[]>([])
  const loading = ref(false)
  const params = ref(options.initialParams ?? {} as P)
  const pagination = ref({ page: 1, pageSize: options.pageSize ?? 20, total: 0 })

  // 4. 内部方法
  async function load() {
    loading.value = true
    try {
      const result = await options.fetchFn(pagination.value.page, pagination.value.pageSize, params.value)
      items.value = result.items
      pagination.value.total = result.total
    } finally {
      loading.value = false
    }
  }

  function reload() {
    pagination.value.page = 1
    return load()
  }

  // 5. 返回响应式状态和方法
  return {
    items,
    loading,
    params,
    pagination,
    load,
    reload,
  }
}
```

### 关键要点

1. **Options 参数**: 使用接口定义配置对象，支持泛型
2. **内部状态**: 使用 `ref()` / `reactive()` 管理内部状态
3. **返回值**: 返回响应式状态 + 操作方法的对象
4. **类型安全**: 使用泛型确保类型推导正确

---

## 数据获取模式

项目不使用 VueQuery / SWR 等库，而是通过自定义 Composable 封装数据获取逻辑。

### 表格数据：`useTableLoader`

```typescript
const { items, loading, params, pagination, load, reload } = useTableLoader<Account, AccountFilters>({
  fetchFn: (page, pageSize, params) => adminAPI.accounts.list(page, pageSize, params),
  initialParams: { platform: 'openai' },
  pageSize: 20,
  debounceMs: 300,
})

onMounted(() => load())
```

### 表单提交：`useForm`

```typescript
const { submit, loading } = useForm({
  onSubmit: async () => {
    await adminAPI.accounts.create(formData)
  },
  successMessage: '创建成功',
})
```

---

## 命名规范

| 规则 | 说明 |
|------|------|
| 函数名以 `use` 前缀 | `useForm`, `useTableLoader` |
| 文件名与函数名一致 | `useForm.ts` 导出 `useForm()` |
| 返回值命名清晰 | `{ items, loading, load, reload }` |
| 泛型参数有意义 | `<T>` 表示数据类型，`<P>` 表示参数类型 |

---

## 常见错误

### 1. Composable 内未清理副作用

```typescript
// ❌ 定时器/订阅未清理
export function usePoll() {
  const timer = setInterval(fetch, 5000)
  // 组件卸载后 timer 继续运行！
}

// ✅ 使用 onUnmounted 清理
export function usePoll() {
  const timer = setInterval(fetch, 5000)
  onUnmounted(() => clearInterval(timer))
}
```

### 2. 在 Composable 外部调用

```typescript
// ❌ 不在 setup() 上下文中调用
const data = useTableLoader(options) // 在普通函数中调用会失败

// ✅ 在 <script setup> 或 setup() 中调用
// <script setup lang="ts">
const data = useTableLoader(options) // 正确
```

### 3. 重复造轮子

在创建新 Composable 前，先检查 `src/composables/` 目录是否已有类似功能。常见的可复用场景：
- 表格数据加载 → `useTableLoader`
- 表单提交 → `useForm`
- 剪贴板操作 → `useClipboard`
- 防抖搜索 → `useKeyedDebouncedSearch`
