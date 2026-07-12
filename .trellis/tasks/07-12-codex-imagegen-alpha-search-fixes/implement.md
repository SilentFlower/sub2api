# 实施计划

## 阶段 1：移植 PR #4063

1. 获取提交 `52071d391b5b2a4e4e0940aea85fc731857c6d07` 并应用到当前 `build`。
2. 核对以下相关定义后再适配：
   - `OpenAIGatewayHandler`
   - `OpenAIGatewayService`
   - endpoint normalization 与 upstream endpoint derivation
   - 账号调度、并发门禁、failover、OAuth/API Key URL 构造
3. 将新增公开 API 注释改为中文，保持现有显式导入和代码格式。
4. 运行 Alpha Search 定向测试。

## 阶段 2：修复 issue #4018

1. 修改 `ensureOpenAIResponsesImageGenerationToolChoiceAuto`：缺失或字符串 `none` 写入 `auto`，其它值保持。
2. 核对 HTTP 与 WebSocket 调用点均受 bridge 门禁控制。
3. 在现有图片桥接测试文件中补充表驱动测试，不复制生产逻辑。
4. 覆盖 bridge enabled/disabled、strip、Spark 和显式选择边界。

## 阶段 3：质量验证

1. 格式化变更文件。
2. 运行：
   ```bash
   cd backend
   go test -tags=unit ./internal/handler ./internal/server/routes ./internal/service \
     -run 'AlphaSearch|NormalizeInboundEndpoint|DeriveUpstreamEndpoint|ImageGenerationToolChoice|ImageGenerationBridge' \
     -count=1
   make test-unit
   ```
3. 按 backend quality spec 运行相关 lint、vet 或范围化静态检查。
4. 执行 Trellis 全面检查，确认 PRD、设计、代码、测试和发布说明一致。

## 阶段 4：提交与 GitHub Actions

1. 通过 `trellis-push` 提交并推送到 `build`；代码提交保持 Alpha Search 与生图修复两个逻辑边界。
2. 监控目标 commit 的 backend CI 和 `Build Image` Action。
3. Action 成功后读取 `build-<短SHA>` 与 `latest`，验证二者 OCI revision 等于完整 commit SHA并在部署时指向同一镜像内容。
4. 记录 Action URL、commit SHA、标签和预期 digest。

## 阶段 5：生产发布

1. 只读采集 A/B 模式、复制、容器 ID、健康和当前 digest。
2. 在 A 将 `/root/sub2api/deploy/docker-compose.yml` 的 `sub2api.image` 设置为 `ghcr.io/silentflower/sub2api:latest`，显式拉取 `latest` 后只重建应用服务。
3. 验证运行容器 OCI revision 等于目标 commit，并解析同仓库唯一 RepoDigest；随后验证 Docker health、`127.0.0.1:8080` HTTP health、基础 Responses、`alpha/search` 和明确携带 `tool_choice: "none"` 的生图桥接场景。
4. 执行：
   ```bash
   /root/sub2api-ha-export/scripts/switch-mode.sh sync-release --dry-run
   /root/sub2api-ha-export/scripts/switch-mode.sh sync-release
   /root/sub2api-ha-export/scripts/switch-mode.sh status
   ```
5. 验证 PostgreSQL/Redis 容器未重启，B 容灾应用未启动，最终 `image_sync=ok`。
6. 将生产验证与回滚点写入任务 research/release 记录。

## 回滚点

- 代码：Alpha Search 和 issue #4018 使用独立提交，可分别回退。
- 应用：A 原 digest 为 `ghcr.io/silentflower/sub2api@sha256:e30acd8618135cc5598b91590c0d27365fe35a94fa0c5f2eb48b78e76bc0b0fa`。
- 数据：本任务无数据库迁移；不得删除 migration 记录或数据卷。
