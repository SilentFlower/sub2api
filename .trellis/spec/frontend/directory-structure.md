# Directory Structure

> 本项目前端代码的组织方式。

---

## Overview

前端位于 `frontend/`，使用 Vue 3、Vite、TypeScript、Pinia、Vue Router、vue-i18n、Tailwind CSS 和 Vitest。包管理器是 pnpm，不能用 npm/yarn 替代。

构建输出配置在 `frontend/vite.config.ts`：生产构建写入 `../backend/internal/web/dist`，由后端内嵌服务。开发模式通过 Vite proxy 转发 `/api`、`/v1`、`/setup` 到后端。

路径别名：

- `@/*` 指向 `frontend/src/*`。
- `vue-i18n` 被别名到 runtime esm bundler 版本，以避免 CSP `unsafe-eval` 问题。

---

## Directory Layout

```text
frontend/
├── package.json          # scripts 和依赖；使用 pnpm
├── pnpm-lock.yaml        # 依赖锁文件，package 变更后必须同步
├── vite.config.ts        # Vite、代理、构建输出、公开配置注入
├── vitest.config.ts      # Vitest + jsdom + coverage
├── tailwind.config.js    # 主题色、dark mode、动画
└── src/
    ├── api/              # API client 和各业务 API 封装
    │   └── admin/        # 管理员 API 子域
    ├── assets/           # 静态资源和支付图标
    ├── components/       # 可复用组件，按业务域分目录
    ├── composables/      # use* 组合式逻辑
    ├── constants/        # 前端常量
    ├── i18n/             # 国际化入口和 locales
    ├── router/           # 路由、守卫、标题
    ├── stores/           # Pinia stores
    ├── styles/           # 额外样式文件
    ├── types/            # 共享 TypeScript 类型
    ├── utils/            # 纯工具函数
    └── views/            # 页面级组件
```

---

## Module Organization

按现有模式放置新代码：

- 页面级路由组件放 `src/views/<domain>/`，例如 `src/views/admin/UsersView.vue`、`src/views/user/PaymentView.vue`。
- 复用 UI 或业务组件放 `src/components/<domain>/`，例如 `src/components/payment/PaymentMethodSelector.vue`。
- API 请求封装放 `src/api/` 或 `src/api/admin/`。组件不直接 import axios，统一 import API 函数。
- 共享类型放 `src/types/`；只在单个 API 模块内部使用的类型可以先放 API 文件内。
- 跨组件状态放 `src/stores/`，局部交互状态留在组件或 composable。
- 可复用组合式逻辑放 `src/composables/use*.ts`。
- 纯函数放 `src/utils/`，对应测试放 `src/utils/__tests__/`。
- 国际化文案放 `src/i18n/locales/en.ts` 和 `src/i18n/locales/zh.ts`。

---

## Naming Conventions

- Vue 页面和组件文件使用 PascalCase，例如 `UsersView.vue`、`PaymentQRDialog.vue`。
- composable 使用 `useXxx.ts`，导出函数也使用 `useXxx`。
- store 文件使用小写业务名，例如 `auth.ts`、`app.ts`、`subscriptions.ts`，导出 `useXxxStore`。
- API 文件按业务名命名，例如 `payment.ts`、`channelMonitor.ts`、`admin/users.ts`。
- 测试文件放在相邻 `__tests__` 目录或对应层级，命名为 `*.spec.ts`。
- API payload 字段保持后端 snake_case，不在请求层随意改成 camelCase。

---

## Examples

API 模块示例：`frontend/src/api/admin/users.ts` 使用 `apiClient` 并返回强类型数据。

store 示例：`frontend/src/stores/auth.ts` 用 setup-style `defineStore`，状态使用 `ref`，派生值使用 `computed`。

组件示例：`frontend/src/components/payment/PaymentMethodSelector.vue` 使用 `<script setup lang="ts">`、typed props、typed emits 和 Tailwind class。

页面示例：`frontend/src/views/admin/UsersView.vue` 组合 layout、筛选条件、DataTable、i18n 和状态加载。

---

## Common Mistakes

- 不要用 npm 安装依赖。`DEV_GUIDE.md` 明确前端使用 pnpm，`pnpm-lock.yaml` 必须同步提交。
- 不要绕过 `src/api/client.ts` 直接在组件里创建 axios 实例。
- 不要把多个业务域的大量逻辑堆在全局 store；优先局部状态或 composable。
- 不要新增硬编码中文/英文 UI 文案，用户可见文本应进入 i18n locales。
- 不要假设 Vite 构建输出在 `frontend/dist`；当前生产输出进入后端内嵌目录。
