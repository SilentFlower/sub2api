# Component Guidelines

> 本项目 Vue 组件编写约定。

---

## Overview

组件使用 Vue 3 SFC，优先 `<script setup lang="ts">`。样式主要使用 Tailwind utility class，主题色和 dark mode 在 `frontend/tailwind.config.js` 中定义。

现有组件风格偏后台管理系统：布局紧凑、表格和筛选器较多、按钮通常带图标、深色模式通过 `dark:` 类支持。新增页面应遵循这个产品形态，不要做营销页式大 hero 或装饰性布局。

---

## Component Structure

推荐结构：

```vue
<template>
  <!-- 模板 -->
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  selected: string
}>()

const emit = defineEmits<{
  select: [type: string]
}>()
</script>
```

示例依据：`frontend/src/components/payment/PaymentMethodSelector.vue`。

当组件状态较复杂时，仍保持同文件内的局部 computed/function。只有当逻辑被多个组件复用，才抽到 `src/composables/` 或 `src/utils/`。

---

## Props Conventions

- props 使用 TypeScript 类型声明，不使用运行时 `props: {}` 风格。
- 有默认值时使用 `withDefaults(defineProps<...>(), defaults)`，示例见 `frontend/src/components/icons/Icon.vue`。
- emits 使用 typed tuple，例如 `select: [type: string]`。
- 双向绑定遵循 Vue 标准 `modelValue` / `update:modelValue`，除非已有组件采用特定事件名。
- API 返回的 snake_case 字段在类型和组件中保持一致，避免临时转换导致跨层漂移。

---

## Styling Patterns

样式以 Tailwind class 为主：

- 使用 `primary`、`accent`、`dark` 等 Tailwind 扩展色。
- dark mode 通过 `dark:` class 维护。
- 表单、按钮、表格复用现有全局 class，例如 `btn`、`btn-primary`、`btn-secondary`、`input`。
- 响应式使用 Tailwind 断点，例如 `w-full sm:w-32`、`hidden md:inline`。
- 固定交互控件应设置稳定尺寸，避免 loading、图标、文案切换导致布局跳动。

图标优先使用项目已有 `frontend/src/components/icons/Icon.vue` 或业务已有 SVG asset。支付方式图标来自 `frontend/src/assets/icons/`，示例见 `PaymentMethodSelector.vue`。

---

## Accessibility

现有代码大量使用原生 `button`、`input`、`label` 和 `title`。新增组件应保持：

- 可点击元素使用 `<button type="button">`，不要用 div 模拟按钮。
- 仅图标按钮需要 `title` 或可访问标签。
- 表单 label 与输入含义要清晰。
- 禁用态使用 `disabled`，同时保留视觉态。
- 弹窗、下拉和菜单要考虑键盘关闭、焦点和点击外部关闭，参考现有组件后再新增。

---

## I18n

用户可见文案使用 `vue-i18n`：

```ts
const { t } = useI18n()
```

模板中使用 `t('key.path')`。新增文案必须同步更新：

- `frontend/src/i18n/locales/en.ts`
- `frontend/src/i18n/locales/zh.ts`

不要在组件中硬编码新的用户可见中文或英文，除非该内容是外部数据或调试信息。

---

## Common Mistakes

- 不要在组件中直接写重复请求逻辑；请求封装应在 `src/api/`，可复用加载逻辑进 composable。
- 不要把后端错误对象当成固定 `error.response.data`。项目 `apiClient` 会把标准错误 reject 成扁平对象。
- 不要新增单一大组件承载完整业务页面和所有弹窗逻辑；页面可保留编排，复用块抽到 `components/<domain>/`。
- 不要用未检查的 `any` 绕过 props 或 API 类型，除非是在处理未知外部 payload，并且局部收窄。
