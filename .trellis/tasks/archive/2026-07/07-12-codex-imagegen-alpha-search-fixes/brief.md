# Brief — Codex 生图桥接与 Alpha Search 兼容修复

## Goal

- 修复 Codex 生图桥接在 `tool_choice: "none"` 下无法调用 `image_generation` 的问题，并合入 PR #4063 的独立 `alpha/search` 转发能力；验证后由 GitHub Actions 构建镜像，更新 A 并把实际运行 digest 同步到 B。

## Scope

- 移植上游提交 `52071d391b5b2a4e4e0940aea85fc731857c6d07`，支持 `/v1/alpha/search`、`/alpha/search`、`/backend-api/codex/alpha/search`。
- 对已启用生图桥接且非 Spark 的图片工具请求，将缺失或字符串 `none` 的 `tool_choice` 归一化为 `auto`；其它明确选择保持不变。
- 补齐 Alpha Search 与 HTTP/WebSocket 生图桥接回归测试，执行后端质量门和 Trellis 全面检查。
- 推送 `build` 后由 GitHub Actions 生成 GHCR 镜像；A 使用 `latest` 拉取并只重建应用，随后从运行容器解析不可变 digest，通过 `sync-release` 同步 A 恢复配置和 B 容灾配置。
- 记录 Action、commit、digest、生产验证和回滚结果。

## Non-Goals

- 不修改数据库、migration、HA 自动切换策略、Cloudflare Tunnel、DNS、租约参数或复制拓扑。
- 不修改 B 原 `/root/sub2api` 单机部署，不启动 B 容灾应用。
- 不替上游维护者合并 PR、关闭 issue，且不进行无关重构。

## Key Context

- issue #4018 的现有缺口位于 `ensureOpenAIResponsesImageGenerationToolChoiceAuto`：只在键缺失时写入 `auto`，未处理字符串 `none`。
- PR #4063 基于当前 `build` 已包含的 `e316ebf5`，临时合并树无冲突，Alpha Search 定向测试已通过。
- 新增公开 API 注释必须使用中文；相关实体、handler、service、调度、failover 和 URL 构造签名必须按当前代码核对，不得猜测。
- GitHub Actions `Build Image` 在 `build` push 后生成 `build-<短SHA>`、`build-latest`、`latest`；A 拉取前必须确认 `latest` 的 OCI revision 等于目标 commit。
- A 当前回滚 digest 为 `ghcr.io/silentflower/sub2api@sha256:e30acd8618135cc5598b91590c0d27365fe35a94fa0c5f2eb48b78e76bc0b0fa`，当前 A/B `image_sync=ok`。
- A 可以使用浮动 `latest` 作为日常来源，但容灾侧只能记录和运行 A 实际容器解析出的固定 digest。
- 自动循环路由：实现 `inline`，检查 `check-all-inline`；profile 为 `commit-only`，不会自动 push 或发布。

## Acceptance

- PR #4063 功能干净合入，三个 Alpha Search 入口、OAuth/API Key 转发、模型映射、query/body 透传、非 OpenAI 拒绝和 failover 测试通过。
- `tool_choice` 缺失或为大小写/空白归一化后的 `none` 时变为 `auto`；`auto`、`required`、明确工具对象、关闭注入、完全阻断和 Spark 行为保持。
- HTTP 与 WebSocket 生图桥接测试、后端质量门和 Trellis 全面检查通过。
- GitHub Actions 构建成功，A 运行容器 revision 与目标 commit 一致，Docker/HTTP/Responses/Alpha Search/生图验证通过，PostgreSQL 与 Redis 未重启。
- A 实际运行 digest 已同步到 B，B 应用保持停止，最终 `image_sync=ok`，发布与回滚证据已记录。

## Next Step

- 确认本 brief 后启动任务；auto-loop 进入 inline 实现，随后执行 check-all-inline、规格更新和本地 commit。
