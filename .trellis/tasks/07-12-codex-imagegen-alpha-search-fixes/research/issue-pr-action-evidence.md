# Issue、PR 与构建链路证据

## GitHub issue #4018

- 状态：open。
- 标题：关于生图失败自动降级为 svg 拼图的问题。
- 根因描述：Codex 请求有时携带 `tool_choice: "none"`；Sub2API 注入 `image_generation` 后没有覆盖该阻断值，上游模型不能调用图片工具。
- 当前代码证据：`ensureOpenAIResponsesImageGenerationToolChoiceAuto` 只在 `tool_choice` 键缺失时写入 `auto`，键存在时直接返回。
- 上游当前没有关联评论、PR 或修复提交。

## GitHub PR #4063

- 标题：`fix: 转发 Codex alpha/search 独立搜索端点`。
- 状态：open，非 draft。
- 提交：`52071d391b5b2a4e4e0940aea85fc731857c6d07`。
- 基线：`main@e316ebf52838a89d57fc790981cce7520f819ac8`，该提交是当前 `build` 的祖先。
- 范围：7 个文件，新增 527 行，删除 3 行；不修改数据库、前端或配置格式。
- 临时合并树：与当前 `build` 可自动合并，无冲突。
- 已验证：
  ```text
  go test -tags=unit ./internal/handler ./internal/server/routes ./internal/service \
    -run 'AlphaSearch|NormalizeInboundEndpoint|DeriveUpstreamEndpoint' -count=1
  ```
  三个包均通过。

## GitHub Actions 构建

- 工作流：`.github/workflows/my-ci.yml`，名称 `Build Image`。
- 触发：任意分支 push；`build` 分支无需 commit message 包含 `[build]`，一定进入镜像构建 job。
- GHCR 标签：
  - `build-<短SHA>`
  - `build-latest`
  - `latest`
- Docker build args：`VERSION=${{ github.sha }}`、`COMMIT=${{ github.sha }}`。
- 生产发布只接受与目标 commit revision 一致的 `ghcr.io/silentflower/sub2api@sha256:<digest>`。

## 当前生产基线

- A 当前应用 digest：`ghcr.io/silentflower/sub2api@sha256:e30acd8618135cc5598b91590c0d27365fe35a94fa0c5f2eb48b78e76bc0b0fa`。
- A 模式：`legacy-active`。
- A/B 发布镜像状态：`image_sync=ok`。
- B 容灾应用必须继续停止；只允许缓存新镜像并更新容灾发布记录。

## 最终发布结果

### 提交与 CI 修复

- `d390f057`：移植 PR #4063 的 Alpha Search 独立转发。
- `b5a51fd0`：修复 `tool_choice: "none"` 并补充 HTTP/WebSocket 回归测试和协议规范。
- `91348728`：把 `settingValuesRepoStub` 移到无 build tag 的共享测试文件，修复 integration 与全量 lint 的测试编译阻塞。
- `53fea533`：修复全量 lint 报告的既有 unused/staticcheck 阻塞；该提交是最终生产镜像 revision。
- 最终 Actions：
  - CI：`https://github.com/SilentFlower/sub2api/actions/runs/29180987966`，success。
  - Security Scan：`https://github.com/SilentFlower/sub2api/actions/runs/29180987983`，success。
  - Build Image：`https://github.com/SilentFlower/sub2api/actions/runs/29180987962`，success。
- 本地等价门禁：完整 unit、完整 integration、`go vet`、清空缓存后的 `golangci-lint v2.9`、gofmt、`git diff --check` 和 Trellis check-all 全部通过。

### 镜像身份

- 完整 revision：`53fea5336ad7cf9f35fea817d7084168ddceaf28`。
- `build-53fea53`、`build-latest`、`latest` 的 OCI revision 均等于该提交。
- 三个标签解析为同一 digest：`ghcr.io/silentflower/sub2api@sha256:74f4f8c88729918b4ec21fbe9f236b740149e923a47f0ee57da7a93a1097e674`。
- 旧生产 digest `e30acd86...b0fa` 保留为回滚点，A Compose 备份为 `/root/sub2api/deploy/docker-compose.yml.pre-53fea53-20260712`。

### A 发布验证

- `/root/sub2api/deploy/docker-compose.yml` 的应用镜像已改为 `ghcr.io/silentflower/sub2api:latest`。
- 只执行应用服务 `--no-deps --force-recreate`；PostgreSQL 容器 ID `bb1cc6f...`、Redis 容器 ID `13324bbe...` 及其启动时间保持不变。
- 运行容器镜像 ID 为 `sha256:74f4f8c8...1097e674`，OCI revision 为 `53fea533...af28`，Docker health 为 `healthy`。
- `http://127.0.0.1:8080/health` 返回 `{"status":"ok"}`。
- 未携带密钥调用 `/v1/alpha/search` 返回预期 `401 API_KEY_REQUIRED`，证明新路由已注册并进入鉴权链路。
- 新容器上的现有 `/v1/responses` 生产流量持续返回 `200`，日志确认 Codex 生图桥接正在注入 `image_generation` 工具和桥接 instructions。

### B 容灾同步验证

- A 执行 `sync-release --dry-run` 后再执行实际同步，双端发布记录均更新为新固定 digest，最终 `image_sync=ok`。
- B 模式仍为 `standby`，PostgreSQL `postgres_recovery=t` 且 `postgres_streaming=streaming`。
- B Redis `role=slave`、`link=up`，主从 offset 一致。
- B 容灾应用仍为 `app_container=absent`，新镜像已缓存，revision/digest 与 A 一致。
- B 原 `/root/sub2api` 单机部署容器保持原 uptime，未被本次同步重启或修改。
