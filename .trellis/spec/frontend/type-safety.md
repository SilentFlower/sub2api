# Type Safety

> 本项目 TypeScript 类型安全约定。

---

## Overview

前端启用严格 TypeScript。`frontend/tsconfig.json` 中关键配置：

- `strict: true`
- `noUnusedLocals: true`
- `noUnusedParameters: true`
- `noFallthroughCasesInSwitch: true`
- `isolatedModules: true`
- `noEmit: true`

因此新增代码必须保持显式类型和可推断类型的平衡，不要用 `any` 掩盖错误。

---

## Type Organization

- 全局共享类型放 `frontend/src/types/`，核心类型集中在 `frontend/src/types/index.ts`。
- 单个 API 模块专用的请求/响应类型可以定义在对应 `src/api/*.ts` 文件内。
- 组件内部只使用的 props/emits 类型可以直接写在 `.vue` 文件中。
- 业务常量和联合类型尽量靠近使用处，例如支付 provider 配置放在 `components/payment/providerConfig.ts`。

后端 API 字段保持 snake_case，前端类型也按实际 JSON 字段命名。示例：`User.allowed_groups`、`PublicSettings.table_default_page_size`。

---

## API Types

API client 标准响应类型来自 `ApiResponse<T>`，业务 API 函数应返回解包后的类型：

```ts
const { data } = await apiClient.get<PaginatedResponse<AdminUser>>('/admin/users', { params })
return data
```

新增 API 类型时必须对照后端 DTO、handler response 和 JSON tag，不要凭字段名猜测。跨层改动至少检查：

- 后端 service/handler DTO
- `internal/pkg/response` envelope
- 前端 `src/api/*`
- 前端 `src/types/*`
- 使用该字段的 view/component/store

---

## Validation

当前项目没有统一引入 Zod/Yup 等 runtime validation 库。现有模式是：

- 后端负责 API 输入校验和业务约束。
- 前端用 TypeScript 类型、表单限制和局部 guard 防止明显错误。
- 解析 `localStorage`、外部回调、未知 metadata 时使用 `unknown` 或 `Record<string, unknown>` 并做运行时收窄。

示例：`frontend/src/stores/auth.ts` 的 `getPersistedPendingAuthSession` 会捕获 JSON parse 失败并清理无效 storage。

---

## Common Patterns

- props/emits 使用 Vue typed macros。
- `computed` 的类型优先由表达式推断，复杂返回值可显式标注。
- 外部未知对象使用 `unknown`、`Record<string, unknown>`，不要直接 `any`。
- 常量数组可用 `as const` 保留字面量类型，例如 `availableLocales`。
- 可取消请求使用 `AbortSignal`，类型见 `FetchOptions`。

---

## Forbidden Patterns

- 不要新增裸 `any` 作为长期类型。确需处理未知错误对象时，范围要小，并尽快收窄。
- 不要用类型断言绕过后端字段不存在的问题。
- 不要把后端 snake_case 字段随意改成 camelCase 后再传回 API。
- 不要使用未在 `tsconfig` alias 中定义的路径别名。
- 不要新增未同步到 `en.ts` / `zh.ts` 的 i18n key。
