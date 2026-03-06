# 质量规范

> 前端代码质量标准、Linter 配置和测试要求。

---

## 概述

- **包管理**: pnpm（**禁止使用 npm**）
- **构建工具**: Vite 5
- **类型检查**: vue-tsc + TypeScript strict mode
- **Linter**: ESLint + Vue/TypeScript 插件
- **测试框架**: Vitest + @vue/test-utils
- **覆盖率**: @vitest/coverage-v8

---

## 脚本命令

| 命令 | 用途 |
|------|------|
| `pnpm dev` | 启动开发服务器 |
| `pnpm build` | 类型检查 + 构建（`vue-tsc -b && vite build`） |
| `pnpm lint` | ESLint 检查并自动修复 |
| `pnpm lint:check` | ESLint 仅检查（不修复） |
| `pnpm typecheck` | TypeScript 类型检查（`vue-tsc --noEmit`） |
| `pnpm test` | Vitest watch 模式 |
| `pnpm test:run` | Vitest 一次性执行 |
| `pnpm test:coverage` | Vitest + 覆盖率报告 |

---

## ESLint 配置

配置文件：`frontend/.eslintrc.cjs`

### 继承规则

- `eslint:recommended`
- `plugin:vue/vue3-essential`
- `plugin:@typescript-eslint/recommended`

### 关键规则调整

| 规则 | 值 | 说明 |
|------|-----|------|
| `@typescript-eslint/no-explicit-any` | `off` | 允许 `any` 类型 |
| `@typescript-eslint/ban-ts-comment` | `off` | 允许 `@ts-ignore` |
| `vue/multi-word-component-names` | `off` | 允许单词组件名 |
| `@typescript-eslint/no-unused-vars` | `warn` | 仅警告（`_` 前缀忽略） |

---

## TypeScript 配置

配置文件：`frontend/tsconfig.json`

| 配置项 | 值 | 说明 |
|--------|-----|------|
| `strict` | `true` | 完整严格模式 |
| `noUnusedLocals` | `true` | 禁止未使用的局部变量 |
| `noUnusedParameters` | `true` | 禁止未使用的函数参数 |
| `noFallthroughCasesInSwitch` | `true` | 禁止 switch 穿透 |
| `target` | `ES2020` | 编译目标 |
| `module` | `ESNext` | 模块系统 |
| `moduleResolution` | `bundler` | Bundler 解析策略 |
| `paths` | `@/* → ./src/*` | 路径别名 |

---

## 禁止的模式

| 模式 | 原因 | 替代方案 |
|------|------|---------|
| 使用 `npm install` | 与 CI 的 pnpm 冲突 | 使用 `pnpm install` |
| 提交未更新的 `pnpm-lock.yaml` | CI `--frozen-lockfile` 失败 | 修改 package.json 后运行 `pnpm install` 并提交 lock 文件 |
| Options API | 项目统一使用 Composition API | `<script setup lang="ts">` |
| Vuex | 已被 Pinia 替代 | 使用 Pinia Store |
| CSS-in-JS / Scoped Style | 项目使用 Tailwind | 使用 Tailwind 工具类 |

---

## 必须的模式

| 模式 | 说明 |
|------|------|
| `<script setup lang="ts">` | 所有组件使用 Composition API + TypeScript |
| `import type` | 类型导入必须使用 `import type` |
| Props 使用 TypeScript 接口 | `defineProps<Props>()` 而非运行时声明 |
| 深色模式支持 | 所有样式必须添加 `dark:` 变体 |
| 路由懒加载 | Views 使用 `() => import('...')` |
| i18n 翻译 | 新增文案需同时添加 `en.ts` 和 `zh.ts` |

---

## 测试要求

### 测试文件位置

- 各模块的 `__tests__/` 子目录
- 命名格式：`*.spec.ts`

### 测试工具

| 工具 | 用途 |
|------|------|
| Vitest | 测试运行器 |
| @vue/test-utils | Vue 组件测试辅助 |
| jsdom | DOM 模拟环境 |
| @vitest/coverage-v8 | 覆盖率统计 |

### 示例

```typescript
// src/api/__tests__/auth.spec.ts
import { describe, it, expect, vi } from 'vitest'

describe('authAPI', () => {
  it('login 应返回 token', async () => {
    // ...
  })
})
```

---

## 构建配置

### 输出目录

构建产物输出到 `../backend/internal/web/dist`，由后端嵌入一体化部署。

### 分包策略

Vite 配置了手动分包（`vite.config.ts`）：

| Chunk 名 | 包含内容 |
|----------|---------|
| `vendor-vue` | Vue 核心（vue, vue-router, pinia） |
| `vendor-ui` | UI 相关（headlessui） |
| `vendor-chart` | 图表（chart.js） |
| `vendor-i18n` | 国际化（vue-i18n） |
| `vendor-misc` | 其他第三方库 |

### TypeScript 检查集成

Vite 使用 `vite-plugin-checker` 在开发时实时检查 TypeScript + Vue TSC 错误。

---

## PR 提交前检查清单

- [ ] `pnpm lint:check` 无错误
- [ ] `pnpm typecheck` 类型检查通过
- [ ] `pnpm test:run` 测试通过
- [ ] `pnpm-lock.yaml` 已同步（如果改了 package.json）
- [ ] 新增文案已添加 `en.ts` 和 `zh.ts` 翻译
- [ ] 深色模式样式已添加
