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
