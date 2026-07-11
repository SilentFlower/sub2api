# PostgreSQL 18 父级卷问题复盘

## 1. 根因分类

- **分类**：E - 隐式假设。
- **具体原因**：初始设计假设把命名卷挂到 `PGDATA=/var/lib/postgresql/data` 即可避免匿名卷，但当前固定 PostgreSQL 18.1 镜像实际声明 `VOLUME /var/lib/postgresql`，并在镜像内提供 `data -> .`。父级卷声明会改变子目录挂载的可见性和空卷复制行为。

## 2. 修复尝试为何失败

1. 首次初始化使用子目录短挂载：Docker把镜像内的 `data -> .` 复制到空命名卷，`pg_basebackup` 在连接 A 前因目标目录非空退出。
2. 第二次只给子目录增加 `volume-nocopy`：镜像声明的父级匿名卷仍遮蔽子挂载，权限辅助容器看不到 `/var/lib/postgresql/data`，同样在连接 A 前退出。
3. 最终修复把命名卷挂到父目录 `/var/lib/postgresql`，启用 `volume-nocopy`，再在卷内创建 `/var/lib/postgresql/data` 作为 `PGDATA`；容器实测只有一个父目录命名卷挂载。

## 3. 预防机制

| 优先级 | 机制 | 具体动作 | 状态 |
|--------|------|----------|------|
| P0 | 架构 | Compose、`pg_basebackup` 和配置辅助容器统一挂载父级命名卷，显式设置子目录 `PGDATA` | 已完成 |
| P0 | 运行时检查 | 初始化前检查卷为空，启动后检查唯一挂载为 `sub2api-dr-postgres-data:/var/lib/postgresql` | 已完成 |
| P1 | 文档 | 更新任务设计、brief 和数据库规范，记录 PostgreSQL 18 父级卷契约 | 已完成 |
| P1 | 评审 | 使用固定镜像前先检查 `docker image inspect .Config.Volumes`，不能按旧版本路径猜测 | 已完成 |

## 4. 系统性扩展

- **相似问题**：所有从 PostgreSQL 17 或更早版本升级到 18 的 Compose部署、备份辅助容器和恢复脚本都可能出现同类父子挂载问题。
- **设计改进**：数据库服务和所有一次性辅助容器共用同一卷挂载契约，避免主服务与初始化路径漂移。
- **流程改进**：涉及固定容器镜像的文件系统布局时，把镜像元数据检查和实际容器挂载检查纳入预检，而不是只做静态 Compose解析。

## 5. 知识沉淀

- [x] 更新 `design.md` 和 `brief.md` 的 PostgreSQL 18 卷布局。
- [x] 更新 `.trellis/spec/backend/database-guidelines.md`。
- [x] 更新 `compose.yaml` 和 `init-postgres-standby.sh`。
- [x] 在阶段 3 结果中记录两次失败均未连接 A、未写入基础备份，并记录最终验证证据。
