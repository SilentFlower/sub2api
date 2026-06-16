# Backend Development Guidelines

> 本项目后端开发规范索引。

---

## Overview

本目录记录后端真实代码约定，供 Trellis implement/check 和新加入的开发者使用。正文使用中文；标题和文件名保留英文，方便与模板和工具约定对齐。

---

## Guidelines Index

| Guide | Description | Status |
|-------|-------------|--------|
| [Directory Structure](./directory-structure.md) | 模块组织和文件布局 | Filled |
| [Database Guidelines](./database-guidelines.md) | Ent、SQL、迁移、事务约定 | Filled |
| [Error Handling](./error-handling.md) | 错误类型、传播和 API 响应格式 | Filled |
| [Quality Guidelines](./quality-guidelines.md) | lint、测试、评审和禁用模式 | Filled |
| [Logging Guidelines](./logging-guidelines.md) | zap 日志、结构化字段和脱敏要求 | Filled |

---

## How to Use These Guidelines

写后端代码前先读与改动相关的文件：

1. 涉及目录、分层、命名：读 `directory-structure.md`
2. 涉及 DB、Ent、Redis、migration：读 `database-guidelines.md`
3. 涉及 handler、service 错误、API 响应：读 `error-handling.md`
4. 涉及日志或敏感信息输出：读 `logging-guidelines.md`
5. 提交前或大改动后：读 `quality-guidelines.md`

这些规范描述当前代码实际模式，不是理想化重构目标。

---

**Language**: 正文使用中文；标题可以保持英文。
