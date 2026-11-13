# Brief — 合并远程 main 到 build 并验证双方功能完整性

## Goal

- 将 main 的更新合入 build，保障双方功能按项目契约完整运行。

## Scope

- 处理 32 个文本冲突文件，复核另外 68 个双方修改且自动合并的文件。
- 检查单边新增功能接线、配置部署、生成代码、迁移与依赖兼容。
- 回归 build 的协议适配、搜索引用、Responses Lite、Fast、超时分档、流式停调、账号和渠道配置，以及 main 的模型、计费、资源约束和管理能力。

## Non-Goals

- 生产发布、生产数据库操作、真实付费调用和无关重构。
- 提交与推送授权由独立 Git 精确计划处理。

## Technical Overview

- 固定 main 目标 `530c5e3a4`，使用普通 merge 保留双方历史；按共同基线和功能归属解冲突，逐项审查自动合并区域，再执行前后端验证与 Check-All。

## Context & Decisions

- 当前 build 为 `e63f900ef`，共同基线为 `8f2cbb53d`；上游变更 813 个文件。
- cyber 遵循现有项目契约，保留 build 的“明确会话 ID + API Key”v2 隔离规则。其余互斥且规范未裁决的行为单独提交决定。
- Flower 0.6.13-beta.13 的 19 个升级文件作为独立范围保留。

## Risks

- 协议缓冲、模型 effort、计费、WS 与 locale 覆盖存在静默语义回归风险。
- main 要求 Go 1.27.3，当前默认 Go 为 1.22.2，验证前需要准备工具链。
- 外部依赖场景按实际可用环境记录验证范围。

## Acceptance

- 32 个冲突和 68 个自动合并交集逐项形成处理结论；双方关键行为有回归证据。
- 后端编译、unit、lint 与前端 lint、typecheck、Vitest、构建及 locale 契约测试通过。
- 配置与生成物接线完整；检查报告列明真实结果和未验证项。

## Next Step

- 启动任务，经 trellis-route 进入实施。
