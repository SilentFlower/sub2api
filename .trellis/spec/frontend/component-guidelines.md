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

- `frontend/src/i18n/locales/en/**`
- `frontend/src/i18n/locales/zh/**`

文案应放到业务归属模块中，例如账号管理文案放
`frontend/src/i18n/locales/{en,zh}/admin/accounts.ts`。从旧大文件迁移或合并
i18n 时，必须逐个确认组件使用的 `t('...')` key 已在中英文同名路径下存在；
关键弹窗或菜单 key 应补 `frontend/src/i18n/__tests__/localesNoKeyCollision.spec.ts`
或相邻组件测试，防止缺失后回退到英文或 key path。

不要在组件中硬编码新的用户可见中文或英文，除非该内容是外部数据或调试信息。

### 可合并的 locale 扩展

当 build 需要覆盖 main 已有文案，并且该 key 所在区块仍会被上游修改时，必须让
main 原 key 相对共同基线保持零 diff。build 文案放入按功能命名的独立模块，再在
原业务对象的稳定末尾展开；不要删除、移动或包裹 main 原 key，否则后续
`main -> build` 会形成 delete/modify 或同区块冲突。

```typescript
import imageOverrides from './accountsOpenAIImageGenerationOverrides'

export default {
  accounts: {
    openai: {
      // main 拥有并继续修改的原 key 保持原样
      codexImageToolDesc: 'main text',

      // build 只在稳定末尾覆盖最终有效值
      ...(imageOverrides as Record<string, string>)
    }
  }
}
```

使用 `Record<string, string>` 收窄是为了避免 TypeScript 对“直接属性随后被已知对象
spread 覆盖”报告 `TS2783`；最终 key path 和运行时覆盖顺序保持不变。嵌套业务域
必须在 `accounts.ts`、`settings.ts`、`channels.ts` 内部展开，不能依赖 admin index
的顶层浅合并。

错误做法：

```typescript
// 删除 main 正在修改的 key 后只从新文件展开，会制造 delete/modify 冲突。
const openai = {
  ...imageOverrides
}
```

此类迁移至少验证：中英文扩展 key 结构一致、最终有效 key path 文案正确、locale
编译通过、无 key collision，并使用包含工作区改动的临时索引执行 `git merge-tree`
确认上游热点不再冲突。

> **注意**：对象末尾的 build override 会稳定覆盖 main 同名 key。main 后续修改原 key
> 时，Git 可以 0 冲突自动合并，但最终页面仍显示旧 override。同步 main 时必须同时读取
> main 新值和独立 override，把上游新增语义合入 override；不能只检查 marker、文件 diff
> 或 locale 是否可编译。相邻测试应断言最终有效文案或关键语义词，避免只断言 key 存在。

---

## Common Mistakes

- 不要在组件中直接写重复请求逻辑；请求封装应在 `src/api/`，可复用加载逻辑进 composable。
- 不要把后端错误对象当成固定 `error.response.data`。项目 `apiClient` 会把标准错误 reject 成扁平对象。
- 不要新增单一大组件承载完整业务页面和所有弹窗逻辑；页面可保留编排，复用块抽到 `components/<domain>/`。
- 不要用未检查的 `any` 绕过 props 或 API 类型，除非是在处理未知外部 payload，并且局部收窄。
