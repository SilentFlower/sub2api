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

### 合并/解冲突后的 i18n 验收契约

#### 1. Scope / Trigger

- 合并分支、rebase 或解决冲突时，只要组件、locale 主模块、领域 locale 扩展、re-export
  或对象 spread 任一侧发生变化，就必须执行本节检查。
- 目标是同时阻止两类静默回归：中文 key 缺失后回退英文，以及中英文都缺失后直接显示
  `key.path`。

#### 2. Signatures

组件调用与 locale 叶子必须形成同名契约：

```typescript
t('admin.settings.gatewayForwarding.someFeature')

// frontend/src/i18n/locales/en/**
someFeature: 'English text'

// frontend/src/i18n/locales/zh/**
someFeature: '中文文案'
```

聚合入口固定为：

```text
frontend/src/i18n/locales/en/index.ts
frontend/src/i18n/locales/zh/index.ts
```

运行时 `fallbackLocale` 是 `en`；因此中文页面显示英文通常是中文最终聚合树缺 key，
不能把 fallback 当成已完成多语言适配。

#### 3. Contracts

- 必须比较聚合后的最终 `en` / `zh` 叶子 key 集合，不能只比较本次冲突文件或领域扩展源文件。
- 每个静态 `t('...')`、`$t('...')`、`i18n.global.t('...')` key 必须同时存在于
  最终中英文树；缺一侧会回退，缺两侧会显示 key path。
- 动态 key（例如 ``t(`status.${status}`)``）必须按实际枚举值检查所有叶子，不能只检查
  `status.` 前缀。
- 中文值允许保留 API、URL、HTTP、模型名和品牌名等语言中立术语；普通标题、按钮、状态、
  提示和完整句子不得直接复制英文值来掩盖未翻译。
- locale spread、re-export 或冲突 resolution 改变顺序后，必须验证最终运行时值；源文件中
  两侧都有 key 不代表最终对象没有被后续 spread 覆盖。
- 解决冲突不得只保留一侧新增的 import、spread、key 或测试注册。中英文领域扩展必须成对
  注册，并保持相同 key 路径。

#### 4. Validation & Error Matrix

| 条件 | 中文运行时表现 | 验收结果 |
| --- | --- | --- |
| `en` 有 key，`zh` 无 key | 回退显示英文 | 失败，补齐中文同路径 key |
| `en` / `zh` 都无静态调用 key | 显示 `key.path` | 失败，补齐两种语言或修正调用路径 |
| 两侧源文件有 key，但中文聚合入口漏 import/spread | 回退显示英文 | 失败，修复聚合注册 |
| 两侧 key 路径不同 | 一侧回退或显示 key path | 失败，统一到组件实际调用路径 |
| 后置 override 覆盖 main 新文案 | 显示旧语义 | 失败，把新语义合入最终 owner |
| 两侧 key 与最终文案都正确 | 按当前 locale 显示 | 通过 |

#### 5. Good/Base/Bad Cases

- Good：组件调用 `admin.groups.claudeMaxSimulation.title`，中英文最终树都在同一路径提供
  各自文案，组件测试分别切换 `en` / `zh` 断言最终文本。
- Good：领域扩展新增中英文文件，并同时在两个主 locale 模块的相同深层对象末尾展开；
  key 结构测试和最终有效文案测试同时覆盖。
- Base：`API Key`、`HTTP`、`DeepSeek` 等语言中立术语在中英文相同，不视为漏翻译。
- Bad：英文 key 位于 `admin.groups.claudeMaxSimulation`，中文却位于
  `admin.groups.modelRouting.claudeMaxSimulation`；中文界面会静默回退英文。
- Bad：冲突解决后 locale 文件可以编译，但组件使用的静态 key 在两侧都不存在；运行时会
  直接显示 key path。

#### 6. Tests Required

- locale 变更或合并后至少运行：

```bash
cd frontend
pnpm exec vitest run \
  src/i18n/__tests__/localesMessageCompile.spec.ts \
  src/i18n/__tests__/localesNoKeyCollision.spec.ts \
  src/i18n/__tests__/buildFeatureLocaleExtensions.spec.ts
```

- 涉及新增/迁移 key 时，测试必须断言最终聚合树的中英文叶子 key 集合一致，并检查所有
  静态翻译调用在两侧都存在；动态 key 必须用真实枚举值展开断言。
- 关键页面或组件必须分别用 `en` / `zh` 渲染并断言最终可见文案，不能只断言 locale
  源对象中存在 key。
- 合并 build 私有 locale 扩展时，继续使用包含工作区改动的临时索引执行
  `git merge-tree`，并在 0 文本冲突后人工复核最终有效值。

#### 7. Wrong vs Correct

错误：中英文使用了不同路径，依赖英文 fallback 掩盖中文缺失。

```typescript
// en
groups: { claudeMaxSimulation: { title: 'Claude Max simulation' } }

// zh
groups: { modelRouting: { claudeMaxSimulation: { title: 'Claude Max 模拟' } } }
```

正确：两种语言保持同名路径，并让组件测试断言最终文案。

```typescript
// en
groups: { claudeMaxSimulation: { title: 'Claude Max simulation' } }

// zh
groups: { claudeMaxSimulation: { title: 'Claude Max 模拟' } }
```

---

## Common Mistakes

- 不要在组件中直接写重复请求逻辑；请求封装应在 `src/api/`，可复用加载逻辑进 composable。
- 不要把后端错误对象当成固定 `error.response.data`。项目 `apiClient` 会把标准错误 reject 成扁平对象。
- 不要新增单一大组件承载完整业务页面和所有弹窗逻辑；页面可保留编排，复用块抽到 `components/<domain>/`。
- 不要用未检查的 `any` 绕过 props 或 API 类型，除非是在处理未知外部 payload，并且局部收窄。
