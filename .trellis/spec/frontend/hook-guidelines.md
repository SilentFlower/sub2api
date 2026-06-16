# Hook Guidelines

> 本项目 composable 编写约定。

---

## Overview

Vue 组合式逻辑放在 `frontend/src/composables/`，命名为 `useXxx.ts`，导出函数名同文件名。当前项目已有：

- `useForm.ts`
- `useClipboard.ts`
- `useAutoRefresh.ts`
- `usePersistedPageSize.ts`
- `useTableLoader.ts`
- `useKeyedDebouncedSearch.ts`
- OAuth 相关 `useOpenAIOAuth.ts`、`useGeminiOAuth.ts`、`useAntigravityOAuth.ts`
- 页面体验相关 `useNavigationLoading.ts`、`useOnboardingTour.ts`

---

## Custom Hook Patterns

composable 应封装可复用状态和行为，返回 `ref`、`computed` 或函数。示例来自 `frontend/src/composables/useForm.ts`：

```ts
export function useForm<T>(options: UseFormOptions<T>) {
  const loading = ref(false)

  const submit = async () => {
    if (loading.value) return
    loading.value = true
    try {
      await submitFn(form)
    } finally {
      loading.value = false
    }
  }

  return { loading, submit }
}
```

约定：

- 使用 Vue 的 `ref`、`computed`、生命周期 API 管理状态。
- 外部传入 API 函数或配置，避免 composable 隐式绑定单一页面。
- 需要访问 store 时在 composable 内调用对应 `useXxxStore`，但不要让 composable 变成全局业务服务。
- 涉及事件监听、定时器、AbortController 时必须清理资源。

---

## Data Fetching

数据请求统一通过 `src/api/` 函数，不直接创建 axios。composable 可以接收 `AbortSignal` 或维护取消逻辑，现有 API 类型里有 `FetchOptions { signal?: AbortSignal }`。

请求状态常见返回：

- `loading`
- `error`
- `data`
- `refresh` / `load` / `submit`

如果只是单个页面的一次性请求，局部写在 view 中即可；当分页、自动刷新、防抖搜索、表格加载等模式重复出现时再抽 composable。

---

## Naming Conventions

- 文件名：`useXxx.ts`
- 导出函数：`useXxx`
- 测试：`src/composables/__tests__/useXxx.spec.ts`
- 返回函数命名使用业务动作，例如 `loadUsers`、`refresh`、`copy`、`submit`，不要只叫 `doIt` 或 `handle`。

---

## Common Mistakes

- 不要把不复用的页面局部变量抽成 composable。
- 不要在 composable 顶层访问浏览器 API；应在函数调用或生命周期内访问，测试环境是 jsdom。
- 不要吞掉错误导致页面无法展示失败态。可在 composable 中 toast，但仍要让调用方有局部处理机会，`useForm` 就会重新抛出错误。
- 不要忘记清理 interval、timeout、event listener。
