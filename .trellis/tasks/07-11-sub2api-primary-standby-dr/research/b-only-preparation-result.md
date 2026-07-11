# B-only 准备阶段执行结果

执行时间：2026-07-11。

## 已完成

- 在 B 创建独立目录 `/root/sub2api-dr`，未修改 `/root/sub2api`。
- 部署 `compose.yaml`、`compose.promoted.yaml`、无真实凭据的 `.env.example`、Redis配置和运维脚本。
- 所有容灾服务均由 `standby` / `promoted` profile 控制；不带 profile 时服务列表为空。
- PostgreSQL、Redis没有宿主机端口映射，备用应用端口固定为 `18080`。
- A 当前运行的 Sub2API固定 `仓库@sha256` 镜像已在 B 成功拉取并校验 RepoDigest。
- `postgres:18-alpine` 和 `redis:8-alpine` 已拉取到 B。
- 未生成真实 `.env`，未写入服务器密码、数据库密码、Redis密码或 Token。

## 验证结果

- `bash -n scripts/*.sh` 通过。
- ShellCheck 全部脚本通过，无 warning/error。
- 普通 Compose 配置不选择任何服务。
- 同时启用 `standby`、`promoted` profile 后，Compose 配置解析通过。
- 提升覆盖配置中不包含 Redis `replicaof` 或 `masterauth`，容器重建后不会自动恢复成旧主从关系。
- B 现有 Sub2API三个容器的 ID、启动时间、挂载、端口和网络在阶段前后完全一致。
- B 现有 `http://127.0.0.1:8080/health` 在阶段前后均返回正常。
- 宿主机 `18080` 未监听。
- 没有创建或启动任何 `sub2api-dr-*` 容器、卷或网络。
- 本地部署包与 B `/root/sub2api-dr` 中全部受管文件的 SHA256逐项一致。
- 缺少真实 `.env` 时，PostgreSQL初始化脚本和提升脚本都会在创建卷、容器或执行提升前失败退出。

## 全面检查修复

- 初版使用 `DR_ENV_FILE` 选择容器环境文件；若直接把 `.env.example` 复制为 `.env`，该变量会继续指向占位模板。
- 已移除 `DR_ENV_FILE` 及调试用路径覆盖，Redis和备用应用固定读取 `/root/sub2api-dr/.env`。
- `.env` 在 B-only 阶段为可选且不存在，因此 Compose仍可完成静态校验；进入复制阶段后，所有运行脚本都强制要求真实 `.env` 存在且关键变量不是占位值。
- 修复后重新通过 Compose配置解析、B 基线比对、端口检查、B 原服务健康检查、B/本地文件哈希比对和 ShellCheck。

## 现有状态备注

- B 原有 `sub2api` 容器在本阶段开始前 Docker health 状态已是 `unhealthy`，但宿主机 `8080/health` 正常返回；阶段结束后状态相同。
- 本阶段没有重启、重建或修改该容器，也未尝试处理其原有 health 状态。

## 下一阶段边界

- 下一步是 A 在线复制出口，需要再次取得用户确认。
- 在该确认前，不连接或修改 A，不创建复制用户、访问规则、复制槽或转发容器。
- PostgreSQL、Redis容灾容器和备用应用继续保持不存在/未运行状态。
