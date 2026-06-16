# Frontend Development Guidelines

> 本项目前端开发规范索引。

---

## Overview

本目录记录前端真实代码约定，供 Trellis implement/check 和新加入的开发者使用。正文使用中文；标题和文件名保留英文，方便与模板和工具约定对齐。

---

## Guidelines Index

| Guide | Description | Status |
|-------|-------------|--------|
| [Directory Structure](./directory-structure.md) | Vue/Vite 项目目录、模块落点 | Filled |
| [Component Guidelines](./component-guidelines.md) | SFC、props、emits、Tailwind、i18n | Filled |
| [Hook Guidelines](./hook-guidelines.md) | composable 命名、数据请求和清理 | Filled |
| [State Management](./state-management.md) | Pinia、局部状态、服务端数据缓存 | Filled |
| [Quality Guidelines](./quality-guidelines.md) | pnpm、lint、typecheck、Vitest | Filled |
| [Type Safety](./type-safety.md) | TS strict、API 类型、runtime 收窄 | Filled |

---

## How to Use These Guidelines

写前端代码前先读与改动相关的文件：

1. 涉及文件落点和模块边界：读 `directory-structure.md`
2. 涉及 Vue 组件、样式、i18n：读 `component-guidelines.md`
3. 涉及 composable 或请求复用：读 `hook-guidelines.md`
4. 涉及 Pinia 或持久化：读 `state-management.md`
5. 涉及 API 字段和 TS 类型：读 `type-safety.md`
6. 提交前或大改动后：读 `quality-guidelines.md`

这些规范描述当前代码实际模式，不是理想化重构目标。

---

**Language**: 正文使用中文；标题可以保持英文。
