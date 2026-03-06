# 类型安全

> TypeScript 类型定义、组织方式和使用规范。

---

## 概述

- **TypeScript 配置**: `strict: true`（完整严格模式）
- **额外严格检查**: `noUnusedLocals`, `noUnusedParameters`, `noFallthroughCasesInSwitch`
- **路径别名**: `@/*` → `./src/*`

---

## 类型组织

### 核心类型文件

所有业务类型集中定义在 `src/types/index.ts`（约 1500 行），按领域用注释区块分隔：

| 区块 | 包含类型 |
|------|---------|
| Common Types | `SelectOption`, `BasePaginationResponse`, `FetchOptions` |
| User & Auth Types | `User`, `AdminUser`, `LoginRequest`, `AuthResponse` |
| Subscription Types | `Subscription`, `CreateSubscriptionRequest` |
| Announcement Types | `Announcement`, `AnnouncementTargeting` |
| API Response Types | `ApiResponse<T>`, `ApiError`, `PaginatedResponse<T>` |
| UI State Types | `Toast`, `ToastType`, `AppState` |
| API Key & Group Types | `Group`, `AdminGroup`, `ApiKey` |
| Account & Proxy Types | `Account`, `Proxy`, 请求/响应类型 |
| Usage & Redeem Types | `UsageLog`, `RedeemCode` |
| Dashboard Types | `DashboardStats`, `TrendDataPoint` |
| User Attribute Types | `UserAttributeDefinition` |
| TOTP (2FA) Types | `TotpStatus`, `TotpSetupResponse` |
| Scheduled Test Types | `ScheduledTestPlan`, `ScheduledTestResult` |

### 辅助类型文件

| 文件 | 用途 |
|------|------|
| `src/types/global.d.ts` | 全局类型声明（如 `Window` 接口扩展） |
| `src/router/meta.d.ts` | 路由元数据类型扩充 |
| `src/components/common/types.ts` | 组件共享类型（如 `Column`） |

---

## 命名规范

| 类型类别 | 命名规则 | 示例 |
|---------|---------|------|
| Request 类型 | 以 `Request` 结尾 | `CreateAccountRequest`, `LoginRequest` |
| Response 类型 | 以 `Response` 结尾 | `AuthResponse`, `PaginatedResponse<T>` |
| 管理员扩展类型 | `Admin` 前缀 | `AdminUser extends User` |
| 分页响应 | 使用泛型 | `PaginatedResponse<T>` |
| API 统一响应 | 使用泛型 | `ApiResponse<T>` |
| 枚举类联合类型 | 字符串字面量联合 | `type ToastType = 'success' \| 'error' \| 'warning'` |

---

## 泛型使用模式

### API 统一响应

```typescript
interface ApiResponse<T> {
  code: number
  message: string
  data?: T
}

interface PaginatedResponse<T> {
  items: T[]
  total: number
  page: number
  pageSize: number
}
```

### API 函数类型标注

```typescript
// API 函数使用泛型标注请求和响应
async function getAccounts(page: number, pageSize: number): Promise<PaginatedResponse<Account>> {
  const response = await apiClient.get<ApiResponse<PaginatedResponse<Account>>>('/admin/accounts', {
    params: { page, pageSize }
  })
  return response.data.data!
}
```

---

## 全局类型声明

```typescript
// src/types/global.d.ts
declare global {
  interface Window {
    __APP_CONFIG__?: PublicSettings
  }
}
```

### 路由元数据类型

```typescript
// src/router/meta.d.ts
import 'vue-router'

declare module 'vue-router' {
  interface RouteMeta {
    requiresAuth?: boolean
    requiresAdmin?: boolean
    title?: string
    titleKey?: string
    descriptionKey?: string
    breadcrumbs?: Breadcrumb[]
    icon?: string
    hideInMenu?: boolean
  }
}
```

---

## 组件类型

```typescript
// src/components/common/types.ts
export interface Column {
  key: string
  label: string
  sortable?: boolean
  formatter?: (value: any, row: any) => string
}
```

---

## 常用模式

### 类型导入

```typescript
// 从统一入口导入
import type { User, Account, ApiResponse } from '@/types'

// 组件类型从组件目录导入
import type { Column } from '@/components/common'
```

### Props 类型定义

```typescript
// 在组件内部定义 Props 接口（不从 types/ 导入）
interface Props {
  account: Account  // Account 类型从 @/types 导入
  editable?: boolean
}
```

---

## ESLint 对 `any` 的处理

项目 ESLint 配置中 `@typescript-eslint/no-explicit-any` 设为 `"off"`，即**允许使用 `any`**。但应尽量减少使用：

```typescript
// 可接受的 any 使用场景
formatter?: (value: any, row: any) => string  // 通用格式化器
field.JSON("data", map[string]any{})           // 对应后端 JSON 字段

// 应避免的 any 使用
const data: any = fetchData()  // ❌ 应该标注具体类型
```

---

## 禁止的模式

| 模式 | 原因 | 替代方案 |
|------|------|---------|
| 大量使用 `as` 类型断言 | 绕过类型检查 | 正确标注类型或使用类型守卫 |
| `// @ts-ignore` 滥用 | 隐藏类型错误 | 修复根本的类型问题 |
| 不导入 `type` | 增加 bundle 大小 | 使用 `import type { X }` |
| 在 types/ 中定义组件 Props | 耦合过紧 | Props 接口在组件文件中定义 |
