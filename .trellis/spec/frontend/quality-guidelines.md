# Quality Guidelines

> 前端代码质量、测试和评审标准。

---

## Overview

前端质量基线来自 `frontend/package.json`、`frontend/tsconfig.json` 和 `frontend/vitest.config.ts`。

常用命令：

```bash
cd frontend
pnpm install
pnpm lint:check
pnpm typecheck
pnpm test:run
pnpm build
```

`pnpm lint` 会带 `--fix`，只在准备接受格式化修改时使用。CI 或检查优先用 `pnpm lint:check`。

---

## Forbidden Patterns

- 不要使用 npm/yarn 安装依赖或更新 lock。项目要求 pnpm。
- 不要提交 `package.json` 变更但遗漏 `pnpm-lock.yaml`。
- 不要绕过 `apiClient` 创建业务 axios 请求。
- 不要在组件中硬编码新增用户可见文案。
- 不要用 `any` 或类型断言掩盖后端字段不一致。
- 不要把密钥、token、完整 Authorization、支付凭据写进 console、toast 或 DOM。
- 不要在测试中只 mock 自己刚写的逻辑，导致删除真实实现后测试仍通过。

---

## Required Patterns

- 新增视图、组件、store、composable 之前先搜索是否有同类模式。
- 用户可见文案进入 i18n。
- API 交互进入 `src/api/`，组件调用 API 函数。
- 跨页面状态进入 Pinia，局部状态留在组件。
- 复杂或复用逻辑抽到 composable/utils，并补对应测试。
- UI 需要同时考虑 mobile、desktop 和 dark mode。
- 表格、筛选、分页、弹窗等后台控件优先复用现有组件和 class。

---

## Testing Requirements

测试使用 Vitest + jsdom + Vue Test Utils。配置见 `frontend/vitest.config.ts`：

- 测试入口匹配 `src/**/*.{test,spec}.{js,ts,jsx,tsx}`。
- setup 文件是 `src/__tests__/setup.ts`。
- coverage provider 是 v8，阈值全局 80%。

测试放置约定：

- 组件测试放 `src/components/**/__tests__/*.spec.ts`。
- view 测试放 `src/views/**/__tests__/*.spec.ts`。
- store 测试放 `src/stores/__tests__/*.spec.ts`。
- API client 和 API 模块测试放 `src/api/__tests__/*.spec.ts`。
- utils/composables 测试放各自 `__tests__`。

改动认证、支付、路由守卫、API 拦截器、表格筛选、国际化时应优先补测试。

---

## Code Review Checklist

评审时检查：

- 是否使用 pnpm，并同步 lock 文件。
- `pnpm typecheck` 是否能覆盖新增类型。
- API 字段是否与后端 JSON tag 对齐。
- i18n key 是否中英文都存在。
- loading、empty、error、disabled 状态是否完整。
- mobile/dark mode 是否可用。
- 请求是否支持取消或避免重复提交。
- 测试是否覆盖用户可见行为，而不只是实现细节。

---

## Common Mistakes

- `apiClient` 已经解包成功响应，业务代码不要再访问 `response.data.data`。
- `apiClient` 的标准错误 reject 不是完整 AxiosError，组件里不要只读 `error.response.data.message`。
- `tsconfig.json` 排除了 spec/test 文件；测试类型问题仍需要通过 Vitest 暴露。
- Vite dev server 默认端口来自 `VITE_DEV_PORT` 或 3000，API proxy 默认后端是 `http://localhost:8080`。
