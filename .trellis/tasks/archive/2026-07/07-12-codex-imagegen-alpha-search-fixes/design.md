# Codex 生图桥接与 Alpha Search 兼容修复设计

## 1. 变更边界

本任务包含两个互相独立但共同发布的后端协议修复：

1. 移植上游 PR #4063，新增 Codex Responses Lite standalone `alpha/search` 转发链路。
2. 修复 Codex 生图桥接对 `tool_choice: "none"` 的归一化。

两项改动使用独立代码提交，最终由同一次 `build` 分支 push 触发 GitHub Actions 构建一个镜像。任务不修改数据库、前端、HA 控制面或 B 原单机部署。

## 2. Alpha Search 架构

```text
Codex
  -> POST /v1/alpha/search | /alpha/search | /backend-api/codex/alpha/search
  -> API Key 鉴权与 OpenAI 分组门禁
  -> body/model 校验、channel model mapping、session sticky
  -> 用户与账号并发门禁
  -> OpenAI 账号调度与 failover
  -> OAuth: https://chatgpt.com/backend-api/codex/alpha/search
     APIKey: <validated base URL>/v1/alpha/search
  -> 原样返回状态、JSON body 与允许的响应头
```

请求 JSON schema 仍在演进，handler/service 只读取 `model` 与调度用 `id`，除模型映射外不重建请求体。query 参数必须保留。

PR 中新增的公开 handler/service 方法注释需要按项目规则改为中文；导入、字段和调用签名必须以当前 `build` 实际定义为准。

## 3. 生图 Tool Choice 归一化

共享函数维持单一职责：只归一化已经具备图片工具的 Responses 请求。

```text
无 tool_choice           -> 写入 "auto"
字符串 "none"           -> 写入 "auto"
字符串 " NONE "         -> 写入 "auto"
"auto" / "required"    -> 保持
明确工具选择对象          -> 保持
非图片请求                -> 保持
Codex Spark              -> 保持现有禁用路径
```

HTTP 与 WebSocket 都通过现有 `ensureOpenAIResponsesImageGenerationToolChoiceAuto` 进入相同行为。调用点继续由 `codexImageGenerationBridgeEnabled` 门禁控制，因此账号“关闭注入”和“完全阻断”不会被绕过。

只处理字符串 `none`。不猜测或改写未知对象结构，避免破坏客户端明确选择其它工具的语义。

## 4. 提交与上游关系

- 第一提交移植 `52071d391b5b2a4e4e0940aea85fc731857c6d07`，若当前 `build` 的局部上下文不同，只做必要冲突适配和中文注释调整。
- 第二提交修复 issue #4018 并补充回归测试。
- 不在本任务中向上游仓库提交或合并 PR。

## 5. GitHub Actions 与镜像身份

```text
Trellis push 到 build
  -> backend-ci / 其它 CI
  -> Build Image
  -> ghcr.io/silentflower/sub2api:build-<short-sha> / latest
  -> 校验 latest 与 commit 标签的 OCI revision == pushed commit SHA
  -> A 使用 latest 拉取并重建 app
  -> A health 通过
  -> 从 A 实际运行镜像解析 RepoDigest
  -> sync-release --dry-run
  -> sync-release
  -> B 缓存相同 digest，image_sync=ok
```

`latest` 是 A 原部署的日常拉取来源，但不能作为容灾版本身份。A 更新前必须把 `latest` 与本任务 commit 绑定校验；A 更新后由 `sync-release` 从实际运行容器解析 digest，并只把不可变引用写入 A 恢复配置与 B 容灾配置。若 Action 失败、revision 不匹配或 digest 不唯一，停止发布。

## 6. 发布与回滚

- 更新 A 前记录 Compose、运行 digest、容器启动时间、PostgreSQL/Redis 容器 ID 和 HA 状态。
- 只执行 `docker compose up -d --no-deps --force-recreate sub2api` 等价的应用更新，不触碰数据库与 Redis。
- A 健康失败时，把主 Compose 临时恢复为原 digest `e30acd86...b0fa` 并仅重建应用；同时保留错误日志和新镜像供排查。恢复后再决定是否把主 Compose 改回 `latest`。
- A 健康但 B 同步失败时保持 A 服务，不启动 B；修复后幂等重跑 `sync-release`。
- 最终验证 B 仍为 standby、B 应用停止、A/B `image_sync=ok`。

## 7. 风险

- Alpha Search 是新独立端点，主要风险是路由归一化、账号能力筛选或 failover 写响应顺序错误。
- `none -> auto` 会允许桥接图片工具参与模型选择，但仅在管理员已启用生图桥接的路径生效；其它显式选择保持不变。
- GitHub Actions 的 `latest` 可能在并发构建时移动，必须在 A 拉取前通过 commit 标签、digest 与 OCI revision 双重绑定；拉取后还要复核运行镜像 revision。
- 应用更新会造成短暂连接中断，因此发布前先完成全部本地与 CI 验证。
