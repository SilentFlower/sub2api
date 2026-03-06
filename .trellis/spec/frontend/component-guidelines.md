# 组件规范

> Vue 3 组件的编写模式、Props 定义和样式约定。

---

## 概述

- **框架**: Vue 3 Composition API
- **SFC 模式**: 统一使用 `<script setup lang="ts">` + `<template>`
- **样式**: Tailwind CSS（`darkMode: 'class'`）
- **禁止使用**: Options API

---

## 组件结构

### 标准 SFC 结构

```vue
<script setup lang="ts">
// 1. 导入
import { ref, computed } from 'vue'
import type { SomeType } from '@/types'

// 2. Props 和 Emit 定义
interface Props {
  title: string
  count?: number
}

interface Emits {
  (e: 'update', value: string): void
}

const props = withDefaults(defineProps<Props>(), {
  count: 0
})

const emit = defineEmits<Emits>()

// 3. 响应式状态
const loading = ref(false)

// 4. 计算属性
const displayTitle = computed(() => `${props.title} (${props.count})`)

// 5. 方法
function handleClick() {
  emit('update', 'new-value')
}

// 6. defineExpose（如需暴露给父组件）
defineExpose({
  reset: () => { /* ... */ }
})
</script>

<template>
  <div>
    <!-- 模板内容 -->
  </div>
</template>
```

---

## Props 规范

### 基本模式

使用 TypeScript 接口 + `withDefaults(defineProps<Props>(), {...})` 定义：

```typescript
// 参考：src/components/common/BaseDialog.vue
type DialogWidth = 'narrow' | 'normal' | 'wide' | 'extra-wide' | 'full'

interface Props {
  show: boolean
  title: string
  width?: DialogWidth
  closeOnEscape?: boolean
  closeOnClickOutside?: boolean
  zIndex?: number
}

const props = withDefaults(defineProps<Props>(), {
  width: 'normal',
  closeOnEscape: true,
  closeOnClickOutside: false,
  zIndex: 50
})
```

### v-model 支持

```typescript
// 使用 modelValue 配合 defineModel
const modelValue = defineModel<string>()

// 或使用传统 emit 方式
const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
}>()
```

---

## Emit 规范

使用 TypeScript 函数签名形式定义：

```typescript
// 参考：src/components/common/Input.vue
const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'change', value: string): void
  (e: 'blur', event: FocusEvent): void
  (e: 'focus', event: FocusEvent): void
  (e: 'enter', event: KeyboardEvent): void
}>()
```

---

## defineExpose 模式

部分组件需要暴露方法给父组件调用：

```typescript
// 参考：src/components/common/Input.vue
const inputRef = ref<HTMLInputElement>()

defineExpose({
  focus: () => inputRef.value?.focus(),
  select: () => inputRef.value?.select()
})
```

---

## 样式模式

### Tailwind CSS

组件内直接使用 Tailwind 工具类：

```vue
<template>
  <div class="flex items-center gap-4 p-4 bg-white dark:bg-dark-800 rounded-lg shadow-card">
    <h2 class="text-lg font-semibold text-gray-900 dark:text-gray-100">
      {{ title }}
    </h2>
  </div>
</template>
```

### 深色模式

通过 `dark:` 前缀控制深色模式样式：

```html
<div class="bg-white dark:bg-dark-800 text-gray-900 dark:text-gray-100">
```

### 自定义主题色

项目定义了自定义颜色方案（`tailwind.config.js`）：

| CSS 变量 | 用途 |
|----------|------|
| `primary-*` | 主色调（Teal/Cyan 青色系，50-950 色阶） |
| `accent-*` | 辅助色（深蓝灰，50-950 色阶） |
| `dark-*` | 深色模式背景色 |

### 全局 CSS 类

在 `src/style.css` 中定义了常用样式组合：

- `modal-overlay` / `modal-content` — 弹窗样式
- `sidebar-link` — 侧边栏链接
- `input` — 输入框基础样式
- `badge` — 徽章/标签

---

## 桶文件导出

每个功能目录通过 `index.ts` 统一导出组件：

```typescript
// src/components/common/index.ts
export { default as DataTable } from './DataTable.vue'
export { default as Pagination } from './Pagination.vue'
export { default as BaseDialog } from './BaseDialog.vue'
export type { Column } from './types'
```

使用时：

```typescript
import { DataTable, BaseDialog } from '@/components/common'
```

---

## 路由懒加载

页面组件（Views）使用动态 `import()` 懒加载：

```typescript
// src/router/index.ts
{
  path: '/admin/dashboard',
  component: () => import('@/views/admin/DashboardView.vue'),
  meta: { requiresAuth: true, requiresAdmin: true }
}
```

---

## 常见错误

### 1. 使用 Options API

```vue
<!-- ❌ 禁止 -->
<script lang="ts">
export default {
  data() { return { ... } },
  methods: { ... }
}
</script>

<!-- ✅ 必须使用 Composition API -->
<script setup lang="ts">
const data = ref(...)
function method() { ... }
</script>
```

### 2. Props 未使用 TypeScript 接口

```typescript
// ❌ 不推荐
const props = defineProps({
  title: { type: String, required: true }
})

// ✅ 推荐
interface Props {
  title: string
}
const props = defineProps<Props>()
```

### 3. 样式未考虑深色模式

```html
<!-- ❌ 缺少 dark: 前缀 -->
<div class="bg-white text-gray-900">

<!-- ✅ 同时支持深色模式 -->
<div class="bg-white dark:bg-dark-800 text-gray-900 dark:text-gray-100">
```
