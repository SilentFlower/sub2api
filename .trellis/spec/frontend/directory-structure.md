# 目录结构

> 前端代码的组织方式和命名规范。

---

## 概述

本项目前端使用 **Vue 3** + **TypeScript** + **Vite**，UI 使用 **Tailwind CSS**，状态管理使用 **Pinia**，国际化使用 **vue-i18n**，包管理使用 **pnpm**。

构建产物输出到 `../backend/internal/web/dist`，嵌入后端一体化部署。

---

## 目录布局

```
frontend/
├── index.html                    # HTML 入口
├── package.json                  # 依赖配置
├── pnpm-lock.yaml               # pnpm 锁文件（必须提交）
├── tsconfig.json                 # TypeScript 配置
├── vite.config.ts                # Vite 构建配置
├── tailwind.config.js            # Tailwind 配置（自定义主题色、动画）
├── .eslintrc.cjs                 # ESLint 配置
├── vitest.config.ts              # 测试配置
├── postcss.config.js             # PostCSS 配置
├── public/                       # 静态资源（不经过构建）
└── src/
    ├── main.ts                   # 应用入口
    ├── App.vue                   # 根组件
    ├── style.css                 # 全局样式（Tailwind + 自定义 CSS 类）
    ├── api/                      # API 调用层
    │   ├── client.ts             # Axios 实例（拦截器、Token 刷新）
    │   ├── auth.ts               # 认证 API
    │   ├── keys.ts               # API Key 管理
    │   ├── index.ts              # 统一导出
    │   ├── admin/                # 管理端 API（按功能模块分文件）
    │   │   ├── accounts.ts
    │   │   ├── users.ts
    │   │   ├── index.ts          # adminAPI 对象汇总（18 个子模块）
    │   │   └── ...
    │   └── __tests__/            # API 模块测试
    ├── components/               # Vue 组件（按功能领域分目录）
    │   ├── common/               # 通用 UI 组件
    │   │   ├── BaseDialog.vue
    │   │   ├── Input.vue
    │   │   ├── DataTable.vue
    │   │   ├── Pagination.vue
    │   │   ├── types.ts          # 组件共享类型
    │   │   └── index.ts          # 桶文件统一导出
    │   ├── account/              # 账号管理相关
    │   ├── admin/                # 管理端组件（按子领域再分目录）
    │   ├── auth/                 # 认证相关
    │   ├── charts/               # 图表组件
    │   ├── icons/                # 图标组件
    │   ├── keys/                 # API Key 相关
    │   ├── layout/               # 布局组件（侧边栏、头部）
    │   ├── user/                 # 用户端组件
    │   ├── sora/                 # Sora 功能
    │   ├── Guide/                # 引导/教程
    │   └── __tests__/            # 组件测试
    ├── composables/              # Vue 3 Composables（即 Hooks）
    │   ├── useForm.ts            # 表单提交（loading、错误捕获、Toast）
    │   ├── useTableLoader.ts     # 表格数据加载（分页、筛选、防抖）
    │   ├── useClipboard.ts       # 剪贴板复制
    │   ├── useModelWhitelist.ts  # 模型白名单管理
    │   ├── useNavigationLoading.ts # 路由导航加载状态
    │   ├── useAccountOAuth.ts    # 账号 OAuth 流程
    │   └── __tests__/            # Composable 测试
    ├── i18n/                     # 国际化
    │   ├── index.ts              # vue-i18n 配置（延迟加载、语言检测）
    │   └── locales/              # 语言包
    │       ├── en.ts             # 英语
    │       └── zh.ts             # 中文
    ├── router/                   # 路由配置
    │   ├── index.ts              # 路由定义 + 导航守卫
    │   ├── title.ts              # 页面标题生成逻辑
    │   └── meta.d.ts             # 路由元数据类型扩充
    ├── stores/                   # Pinia 状态管理
    │   ├── auth.ts               # 认证状态
    │   ├── app.ts                # 全局 UI 状态
    │   ├── subscriptions.ts      # 订阅数据
    │   ├── adminSettings.ts      # 管理员设置
    │   ├── onboarding.ts         # 引导流程
    │   └── index.ts              # 统一导出
    ├── types/                    # TypeScript 类型定义
    │   ├── index.ts              # 核心类型（约 1500 行，按领域分区块）
    │   └── global.d.ts           # 全局类型声明
    ├── utils/                    # 工具函数
    │   ├── format.ts             # 格式化工具
    │   └── sanitize.ts           # 数据清洗
    ├── views/                    # 页面视图（按功能分目录）
    │   ├── auth/                 # 认证页面（登录、注册等）
    │   ├── user/                 # 用户端页面
    │   ├── admin/                # 管理端页面
    │   └── setup/                # 初始设置页面
    └── __tests__/                # 集成测试
```

---

## 模块组织

### 新增功能模块的标准步骤

1. **类型**: 在 `src/types/index.ts` 中添加相关类型定义
2. **API**: 在 `src/api/` 中创建 API 模块，导出函数对象
3. **组件**: 在 `src/components/{功能}/` 中创建组件
4. **页面**: 在 `src/views/{区域}/` 中创建页面视图
5. **路由**: 在 `src/router/index.ts` 中注册路由
6. **i18n**: 在 `src/i18n/locales/en.ts` 和 `zh.ts` 中添加翻译

### 管理端 vs 用户端分离

| 位置 | 管理端 | 用户端 |
|------|--------|--------|
| API | `src/api/admin/*.ts` | `src/api/*.ts` |
| 组件 | `src/components/admin/` | `src/components/user/` |
| 页面 | `src/views/admin/` | `src/views/user/` |
| 路由前缀 | `/admin/*` | `/dashboard`, `/keys` 等 |

---

## 命名规范

| 类别 | 规则 | 示例 |
|------|------|------|
| 组件文件 | PascalCase `.vue` | `BaseDialog.vue`, `CreateAccountModal.vue` |
| 页面视图 | PascalCase + `View` 后缀 | `DashboardView.vue`, `LoginView.vue` |
| Composable | `use` 前缀 camelCase | `useForm.ts`, `useTableLoader.ts` |
| API 模块 | 小写 camelCase | `auth.ts`, `accounts.ts` |
| Store 文件 | 小写 camelCase | `auth.ts`, `app.ts` |
| 工具函数 | 小写 camelCase | `format.ts`, `sanitize.ts` |
| 测试文件 | `*.spec.ts` | `auth.spec.ts` |
| 组件目录 | 小写按功能命名 | `common/`, `account/`, `admin/` |

### 组件后缀命名约定

| 后缀 | 含义 | 示例 |
|------|------|------|
| `Modal` | 弹窗组件 | `CreateAccountModal.vue` |
| `Cell` | 表格单元格组件 | `AccountUsageCell.vue` |
| `Filters` | 筛选器组件 | `AccountTableFilters.vue` |
| `Layout` | 布局组件 | `TablePageLayout.vue` |
| `View` | 页面级视图 | `DashboardView.vue` |

---

## 桶文件导出模式

各功能目录通过 `index.ts` 统一导出：

```typescript
// src/components/common/index.ts
export { default as DataTable } from './DataTable.vue'
export { default as Pagination } from './Pagination.vue'
export { default as BaseDialog } from './BaseDialog.vue'
export type { Column } from './types'
```

**注意**: `src/utils/` 目录没有桶文件，工具函数按路径直接导入。
