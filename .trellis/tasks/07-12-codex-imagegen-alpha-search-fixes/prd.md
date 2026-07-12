# Codex 生图桥接与 Alpha Search 兼容修复

## Goal

修复 Codex 生图桥接在入站请求携带 `tool_choice: "none"` 时无法调用图片工具的问题，并将上游 PR #4063 的 Codex 独立 `alpha/search` 转发能力合入 `build` 分支。完成代码验证后构建不可变镜像，在不修改数据库和 HA 拓扑的前提下更新 A，并把相同 digest 同步到 B 容灾配置。

## Background

- GitHub issue #4018 指出：Sub2API 为 Codex 注入 `image_generation` 工具时，现有逻辑只在 `tool_choice` 缺失时写入 `auto`；若客户端显式携带 `tool_choice: "none"`，工具虽然存在但被禁止调用，模型会退化为文字、SVG 或本地绘制方案。
- 当前 `build` 中 `ensureOpenAIResponsesImageGenerationToolChoiceAuto` 仍保留“字段存在即不修改”的逻辑，issue 尚未有关联修复 PR 或提交。
- 上游 PR #4063（提交 `52071d391b5b2a4e4e0940aea85fc731857c6d07`）新增 Codex Responses Lite 独立搜索端点 `/alpha/search` 的认证、调度、转发、故障切换和测试。
- PR #4063 基于 `e316ebf52838a89d57fc790981cce7520f819ac8`；该提交已是当前 `build` 的祖先。临时合并树无冲突，带 `unit` build tag 的定向测试已通过。
- A 当前运行固定镜像 `ghcr.io/silentflower/sub2api@sha256:e30acd8618135cc5598b91590c0d27365fe35a94fa0c5f2eb48b78e76bc0b0fa`，A/B 当前 `image_sync=ok`，B 容灾应用必须继续保持停止。

## Requirements

### R1. 合入 Codex Alpha Search

- 将 PR #4063 的功能合入 `build`，保留其独立变更边界，不把无关重构混入该提交。
- 支持以下入站路径：
  - `/v1/alpha/search`
  - `/alpha/search`
  - `/backend-api/codex/alpha/search`
- OpenAI OAuth 请求转发到 ChatGPT Codex standalone search；OpenAI API Key 请求转发到对应 OpenAI-compatible `/v1/alpha/search`。
- 复用现有 API Key 鉴权、用户/账号并发、账号调度、模型映射、故障切换和安全响应头过滤。
- 不改变现有 `/responses`、hosted web search 和 web search emulation 行为。

### R2. 修复 Codex 生图 `tool_choice: "none"`

- 当且仅当请求已经包含 `image_generation` 能力、模型不是 Codex Spark，且生图桥接正在参与请求处理时：
  - `tool_choice` 缺失时写入 `"auto"`；
  - `tool_choice` 为忽略大小写和首尾空白后的字符串 `"none"` 时改写为 `"auto"`。
- 已有 `"auto"`、`"required"`、明确工具对象或其它非阻断选择不得被覆盖。
- 不能把账号“关闭注入”或“完全阻断”策略绕过为可调用生图。
- HTTP Responses 与 WebSocket Responses 必须使用一致的归一化行为，避免协议分支漂移。
- Spark 模型继续保持不支持生图的现有行为。

### R3. 测试与质量门

- 生图单测至少覆盖：缺失、`none`、带空白/大小写的 `none`、`auto`、`required`、明确工具选择、Spark、关闭注入和完全阻断。
- Alpha Search 测试至少覆盖：三类入口、OAuth/API Key 上游地址、请求体与 query 透传、模型映射、非 OpenAI 分组拒绝、可切换错误进入 failover。
- 运行相关后端单测、完整 service 定向回归、格式化、静态检查和 Trellis 全面检查。
- 修复测试夹具或检查命令问题时，只处理本任务真实阻塞，不顺带改造无关测试基础设施。

### R4. 镜像与 A/B 发布

- 代码通过 Trellis 提交流程推送到 `build`，由 `.github/workflows/my-ci.yml` 的 GitHub Actions `Build Image` 构建并推送 GHCR；不得在服务器或本地手工构建生产镜像。
- `build` 推送会生成 `build-<短SHA>`、`build-latest` 和 `latest`。发布必须先等待对应 commit 的 Action 成功，再校验镜像 OCI `org.opencontainers.image.revision` 等于该 commit SHA，并解析为 `仓库@sha256` 不可变引用。
- A 原部署按用户选择使用 `ghcr.io/silentflower/sub2api:latest` 拉取并重建应用；拉取前后必须验证 `latest` 对应的 OCI revision 就是本任务推送的 commit，禁止误部署并发构建产生的其它 revision。
- A 运行成功后，从实际运行容器的镜像 ID 与同仓库 RepoDigest 解析不可变 digest。A 的恢复配置、B 容灾配置、B 镜像缓存和双端发布记录必须使用该精确 digest，不得把 `latest` 写入容灾侧。
- 发布前记录当前 A digest 作为明确回滚点，不删除旧镜像或数据库迁移记录。
- A 仅重建 `sub2api` 应用服务，不重启 PostgreSQL、Redis，不修改数据库结构、复制关系、HA owner、租约模式或 Tunnel。
- A 更新后必须通过 Docker health、HTTP health 和关键 Codex Responses 验证。
- 随后通过 A 的 `sync-release --dry-run` 与 `sync-release` 将相同 digest 同步到 B；B 只缓存镜像并更新容灾发布记录，不启动容灾应用。
- 最终要求 A/B `image_sync=ok`，A 继续为活动节点，B 继续为健康 standby。

## Out Of Scope

- 数据库迁移、反向迁移或删除现有 migration 记录。
- 修改 HA 自动切换策略、Cloudflare Tunnel、DNS、租约参数或数据库复制拓扑。
- 修改 B 原 `/root/sub2api` 单机部署。
- 代替上游维护者合并或关闭 GitHub issue/PR；本任务只保证本仓库 `build` 与生产镜像包含所需功能。

## Acceptance Criteria

- [ ] PR #4063 功能在 `build` 中完成干净合入，定向测试通过。
- [ ] 带 `tool_choice: "none"` 的 Codex 生图桥接请求会改写为 `auto`，模型能够调用 `image_generation`。
- [ ] 非阻断 `tool_choice`、关闭注入、完全阻断和 Spark 行为保持不变。
- [ ] HTTP 与 WebSocket 生图桥接测试均覆盖并通过。
- [ ] `/v1/alpha/search`、`/alpha/search`、`/backend-api/codex/alpha/search` 均进入正确转发链路。
- [ ] 代码质量检查和任务规格检查通过，没有混入现有 HA 任务或其它未跟踪任务文件。
- [ ] A 通过 `latest` 拉取目标构建并运行，运行容器 revision 与目标 commit 一致、实际 RepoDigest 已确认，应用健康且 PostgreSQL/Redis 未重启。
- [ ] 同一 digest 已同步到 B 容灾配置与缓存，B 应用保持停止，最终 `image_sync=ok`。
- [ ] 回滚点和验证结果记录在任务研究或发布文档中。

## Decisions

- 完成全部测试并由 GitHub Actions 成功构建镜像后，立即在当前生产 A 上进行一次仅应用容器的更新；随后同步相同 digest 到 B。
- 生产镜像只通过 GitHub Actions 构建，不使用本地或服务器手工构建产物。
- A 原部署长期使用 `latest` 作为日常拉取来源；容灾发布身份仍以 A 实际运行后的不可变 digest 为准。
