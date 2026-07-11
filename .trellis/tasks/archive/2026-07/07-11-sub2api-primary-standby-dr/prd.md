# Sub2API 主备容灾部署

## Goal

在不影响两台服务器现有 Sub2API 单机部署的前提下，建立“A 主、B 备”的半自动容灾能力：A 正常承载业务，B 持续接收 PostgreSQL 与 Redis 复制；确认 A 已停止后，由人工执行提升流程，使 B 在 5 至 15 分钟内接管服务；A 修复后通过统一恢复入口从 B 重新同步数据，并在受控回切后恢复“A 主、B 备”。

## Background

- A 当前通过 Docker Compose 运行 Sub2API、PostgreSQL 18.1 和 Redis 8，现网业务持续使用中。
- A 当前 PostgreSQL 数据约 2.8 GB，真实数据目录位于 PostgreSQL 18 自动创建的匿名卷；初始建设阶段不得重建现有 PostgreSQL 容器。
- A 当前 PostgreSQL 已在线启用复制配置和物理复制槽 `sub2api_b_standby`；B PostgreSQL处于恢复状态，WAL receiver 为 `streaming`，最近验证无 LSN积压。
- A 当前 Redis 为主库，AOF `everysec` 已启用，数据约 59 MB；B Redis为从库，主链路为 `up`，最近验证 offset一致且未处于同步过程。
- B 当前已经有一套独立使用中的 Sub2API 单机部署，路径为 `/root/sub2api`，占用现有 `sub2api*` 容器名、卷名和宿主机 `8080` 端口；该部署不可修改、停止或重建。
- B 为 Debian 12、x86-64，约 7.5 GiB 内存和 61 GiB 可用磁盘，具备运行额外容灾栈的资源。
- B 的 `/root/sub2api-dr` 容灾 PostgreSQL和 Redis已常驻运行，备用应用保持停止，`18080` 空闲；B 原 `/root/sub2api` 部署未被修改。
- 用户选择公网固定 IP 直连复制，不使用 Tailscale；复制入口已通过 A 上独立转发容器提供，建设过程未重建 A 现有数据容器。
- 用户接受 PostgreSQL 异步复制的秒级 RPO，并接受人工确认 A 已停止后的半自动提升，不引入第三节点或自动 fencing。
- 用户确认 A、B 两端都保留统一操作脚本，并采用“单脚本、多个明确阶段、破坏性阶段分别确认”的方式，不要求单个命令无停顿执行完整回切。
- A 会频繁更新 Sub2API，既可能复用同一个可变标签，也可能临时指定其它标签。容灾侧不能把标签文本当作版本身份，必须跟随 A 实际运行镜像的可拉取 `仓库@sha256`，避免数据库迁移已复制到 B、但故障时启动旧应用镜像。
- B 当前未报告 NTP同步，最近时钟偏差约 1.2 秒；实际提升演练前必须先恢复或确认时间同步，复制健康判断以 LSN和 Redis offset为主。

## Requirements

### R1. 现有部署隔离

- B 上现有 `/root/sub2api`、Compose 项目 `sub2api`、现有容器、网络、卷和 `8080` 端口必须保持不变。
- 新容灾栈固定使用目录 `/root/sub2api-dr`、Compose 项目名 `sub2api-dr` 和 `sub2api-dr-*` 资源前缀。
- 新 PostgreSQL、Redis不发布宿主机端口；备用应用仅在提升后使用宿主机 `18080`。
- 第一阶段不得启动备用 Sub2API，不得占用 `18080`。

### R2. B-only 准备阶段

- 在 B 创建隔离的 Compose定义、环境变量模板、初始化脚本、提升脚本和验证脚本。
- 固定使用 A 当前运行的可拉取 `仓库@sha256` 镜像引用，不使用浮动 `build-latest` 作为容灾应用运行依据。
- PostgreSQL固定使用 `postgres:18-alpine`，并使用独立命名卷和明确的数据目录布局。
- Redis固定使用 `redis:8-alpine`，使用独立命名卷并预留从库配置。
- PostgreSQL、Redis和备用应用均通过显式 profile 启动；不带 profile 的默认 Compose 操作不得启动任何容灾服务。
- 环境变量模板不得包含 A 或 B 的真实数据库密码、Redis密码、Token 或其他凭据。
- B-only 阶段允许拉取镜像、创建隔离网络和空卷，但不得连接或修改 A。

### R3. A 在线复制出口

- A 侧后续通过独立 Compose 项目新增 PostgreSQL、Redis TCP 转发容器，并加入 A 现有 Sub2API Docker网络。
- 转发层应将独立宿主机端口映射到现有 `postgres:5432` 和 `redis:6379`，不得修改或重建现有 PostgreSQL、Redis、Sub2API容器。
- 转发监听器只接受 B 固定公网地址来源；PostgreSQL HBA按转发容器所在的 A Docker网络授权专用复制用户。
- PostgreSQL复制用户、访问规则、`wal_keep_size`、`max_slot_wal_keep_size` 和物理复制槽应通过在线 SQL、配置 reload 或等价方式生效，不安排数据库重启。
- `wal_keep_size` 设为 2 GiB，物理复制槽最大保留 WAL 设为 8 GiB；超过上限导致槽失效时，优先重新初始化 B，不能让 A 因无限槽位积压写满磁盘。

### R4. PostgreSQL异步流复制

- B 使用 `pg_basebackup` 从 A 在线初始化，不停止 A 的业务写入。
- B 启动后必须保持 `pg_is_in_recovery() = true`，A 的 `pg_stat_replication` 必须显示 B 为 `streaming`。
- 使用物理复制槽降低短时断线造成 WAL 缺失的风险，并监控复制延迟和槽位积压。

### R5. Redis主从复制

- B Redis应从 A 执行初次同步并持续复制。
- 正常状态下 B 必须显示从库角色且主链路为 `up`，A 必须显示一个已连接从库。
- 提升后 B Redis必须切换为主库，并采用不会在容器重启后自动恢复为旧主从关系的持久化启动配置。

### R6. 备用应用与本地数据

- B 备用 Sub2API使用 B 的容灾 PostgreSQL、Redis和固定镜像 digest。
- 建立复制阶段必须先盘点 A 的 `/app/data`，同步其中全部非临时业务文件；至少包括 `config.yaml`、`.installed`、`pages/` 及实际使用的自定义模板、本地资源，日志不属于同步范围。
- 以 A 当前部署环境为源生成 B 容灾 `.env`，保留数据库/Redis凭据、JWT/TOTP密钥及业务环境参数，只覆盖 B 的内部服务地址、Compose 资源名和备用应用端口；真实凭据不得写入本任务文件或版本库。
- 正常状态下备用应用保持停止，避免连接只读 PostgreSQL备库。

### R7. 半自动提升

- B 应保留统一模式控制脚本，支持查询状态、收敛到安全备库状态、进入启用流程以及在回切前冻结写入；命令接口为 `status`、`standby`、`enable`、`freeze`。
- `standby` 只能在 PostgreSQL仍为备库且 Redis仍为从库时关闭备用应用并复核复制，不能把已经提升并可能产生写入的 B 原地降回备库。
- `enable` 必须复用提升脚本的人工确认和顺序保护，不能绕过“A 已停止且不会继续写入”的确认口令。
- `freeze` 只能停止 B 容灾应用并确认写入入口已冻结，不得降级 PostgreSQL或 Redis；冻结后的 B 数据库继续作为当前主库，供 A 完成最后追平。
- 提升脚本必须先显示 PostgreSQL最后接收、回放位置和延迟，并要求人工明确确认 A 已停止。
- 提升顺序必须为 PostgreSQL提升、Redis提升、启动备用应用、健康验证、切换公共入口。
- 提升操作应具备重复执行保护，任一步失败时停止后续步骤并输出可人工处理的状态。
- 实际公共 API 域名和入口方式在入口脚本定稿前确认；该决策不阻塞 B-only 准备，也不得在未确认时自动切换入口。
- A 恢复后不得直接启动旧主服务；必须先以 B 为主重新初始化 A，再安排计划内回切。

### R8. 分阶段授权

- 容灾脚本、Compose覆盖文件、无凭据模板、文档及 `--dry-run` 验证可以提前准备。
- 实际提升 B、停止现网写入、重建 A 或 B 数据卷、改变复制方向、启动回切后的应用以及切换公共入口，均须在对应维护阶段再次取得明确授权。

### R9. A 修复与受控回切

- B 已提升并承载写入后，A 不得使用故障前的旧 PostgreSQL、Redis数据直接启动应用或恢复写入。
- A 应提供统一模式控制脚本，命令接口为 `status`、`prepare-from-b`、`cutback-to-a`、`restore-b-standby`。
- `prepare-from-b` 应验证 B 仍是当前启用节点，停止并隔离 A 的旧服务，使用新的命名卷从 B 全量初始化 A PostgreSQL、Redis和非临时应用数据，并保持 A 应用停止。
- `cutback-to-a` 应先调用或要求执行 B 的 `freeze`，确认 A 已完全追平，再提升 A PostgreSQL、将 A Redis切为主库、启动 A 应用并等待人工切换公共入口。
- `restore-b-standby` 应在 A 已稳定为主且 B 写入已冻结后，从新的 A 主节点重新初始化 B PostgreSQL和 Redis，并使 B 容灾应用保持停止。
- A 的 PostgreSQL应从 B 做全量基础备份，并在重建时修正 PostgreSQL 18 匿名卷布局；A Redis应先作为 B 的从库完成同步。
- A 重建默认创建新的恢复卷并保留故障前旧卷，不直接删除旧数据；只有明确指定的重新初始化目标卷才允许在确认后清理。
- A 回切前必须冻结 B 的业务写入，确认 A 的 PostgreSQL与 Redis均已追平，再提升 A 并启动 A 应用。
- A 提升会产生新时间线；恢复最终“A 主、B 备”拓扑时，B 必须从新的 A 主节点重新初始化或经过经验证的等价恢复流程，不能把旧 B 主库直接声明为备库。
- 所有会停止业务、清理数据卷、提升数据库或改变复制方向的阶段必须具备状态校验、重复执行保护和人工确认。
- 公共入口切回 A 仍由人工执行或在入口实现确认后接入脚本；入口未知时，脚本不得声称回切已全部完成。

### R10. A 更新后的容灾应用版本同步

- A 统一入口应新增 `sync-release [--dry-run]`，用于 A 完成日常应用更新并通过健康检查后，同步当前实际运行镜像到容灾配置；该命令不替代或接管 A 现有更新流程。
- 无论 A 使用可变标签还是指定标签，脚本都必须从运行中的 `sub2api` 容器解析真实镜像 ID，并选择与当前镜像仓库匹配的可拉取 `仓库@sha256`。不能直接把标签写入容灾 `.env`，也不能默认选择不匹配仓库的第一个 `RepoDigest`。
- 当前运行镜像没有可拉取 RepoDigest、RepoDigest与运行镜像不一致、A 应用不健康、B 不是安全 `standby` 或 B 复制不健康时，版本同步必须失败且不修改任何 `.env`。
- 实际同步必须先让 B 拉取并验证精确 digest，再只原子替换 B `/root/sub2api-dr/.env` 与 A `/root/sub2api-ha-export/.env` 中的 `SUB2API_IMAGE`；不得覆盖 B 的数据库、复制、端口或其它容灾参数。
- 版本同步不得停止或重启 A/B PostgreSQL、Redis，不得启动 B 容灾应用，也不得改变主从方向。`--dry-run` 不拉取镜像、不写 `.env` 或状态文件。
- A/B 应记录最近成功同步的 digest、来源标签、前一 digest 和同步时间；`status` 应显示 A 当前运行 digest、A 恢复配置 digest、B 配置 digest、B 本地缓存状态及 `image_sync=ok|drift|unknown`。
- B `enable` 在数据库提升前必须验证容灾镜像是固定 digest、已在 B 本地缓存，并与最近成功同步记录一致。发现版本漂移时必须拒绝提升，不能在故障窗口临时按可变标签拉取。
- B 已接管后，B 记录的活动 digest 成为 A 恢复时的应用版本依据；A 从 B 重建和回切前必须确保 A 已缓存并使用该 digest。
- 同步命令应保留前一 digest 供排障，但数据库迁移可能不向后兼容，不能把“旧镜像仍存在”描述为可保证成功的应用回滚。

## Acceptance Criteria

- [ ] B 上现有 Sub2API三个容器的 ID、启动时间、卷和端口在 B-only 阶段前后保持不变。
- [ ] `/root/sub2api-dr` 中存在独立 Compose定义和脚本，`docker compose config` 校验通过。
- [ ] 新资源名称均使用 `sub2api-dr` 前缀，不与 B 现有容器、网络、卷冲突。
- [ ] 不带 profile 的 Compose 操作不会启动 PostgreSQL、Redis或备用应用，环境变量模板不包含真实凭据。
- [ ] B-only 阶段结束时没有新的容灾应用进程监听 `18080`，现有 `8080` 服务正常。
- [ ] 容灾应用镜像固定为 A 当前运行的可拉取 `仓库@sha256` 引用。
- [ ] A 侧后续新增复制出口时，现有三个 Sub2API容器均未重启或重建。
- [ ] A 的 PostgreSQL、Redis转发端口仅接受 B 固定公网来源，B 可完成协议级连通检查。
- [ ] A PostgreSQL在线生效 `wal_keep_size=2GiB`、`max_slot_wal_keep_size=8GiB`，复制用户、HBA规则和物理复制槽均可查询，`pending_restart=false`。
- [ ] PostgreSQL初始化完成后，B 为只读备库、A 显示 `streaming`，复制延迟可查询。
- [ ] Redis初始化完成后，B 为从库且复制链路正常。
- [ ] B 模式控制脚本支持 `status`、`standby`、`enable`、`freeze`；`enable --dry-run` 不提升数据库或启动应用，已提升状态下执行 `standby` 会拒绝原地降级，`freeze` 不改变数据库角色。
- [ ] 人工提升演练能在 5 至 15 分钟内使 B 提供健康服务，并保留完整操作记录。
- [ ] 回切演练前 A 先从 B 完成重新同步，不允许旧主直接恢复写入。
- [ ] A 统一恢复入口能识别旧主隔离、A 作为 B 备库追平、A 已回切为主、B 已恢复为备等状态，并在状态矛盾时停止。
- [ ] A 从 B 重新初始化后，PostgreSQL使用正确的 PostgreSQL 18 命名卷布局，Redis显示为 B 的从库，且 A 应用在回切前保持停止。
- [ ] 冻结 B 写入并确认 A 追平后，受控回切可使 A 恢复主库角色和应用服务；随后 B 从新的 A 主节点重新建立为只读备库和 Redis从库。
- [ ] 任一破坏性阶段在未输入明确确认口令时不得清理数据、提升数据库、改变复制方向或启动写入服务。
- [ ] A 模式控制脚本支持 `status`、`prepare-from-b`、`cutback-to-a`、`restore-b-standby`，所有变更命令均支持 `--dry-run`，并能与 B 脚本配合完成完整回切周期。
- [ ] A 使用相同标签更新到不同镜像内容后，`sync-release --dry-run` 能报告新旧 digest 差异且不产生状态变化，实际 `sync-release` 能让 B 缓存精确 digest并只更新双端 `SUB2API_IMAGE`。
- [ ] A、B `status --machine` 能稳定输出版本同步字段；正常态显示 `image_sync=ok`，任一配置、缓存或同步记录不一致时显示 `drift` 或 `unknown`。
- [ ] B `enable --dry-run` 和实际 `enable` 在镜像未固定、未缓存或与同步记录不一致时均在提升 PostgreSQL前失败；版本同步成功后保持原有 fencing确认和提升顺序。
- [ ] B 已接管时，A 的恢复准备能以 B 的活动 digest为准校验或同步 A 恢复镜像，避免回切启动旧版本应用。

## Out Of Scope

- 三节点仲裁、自动 fencing、全自动故障提升。
- A、B 双活流量分发。
- 修改 B 现有 Sub2API单机部署。
- 本任务中的 SSH、防火墙、账号权限等通用安全加固。
- 在 B 接管前在线修复 A 当前 PostgreSQL匿名卷布局；该问题只在 B 已成为权威主节点后的计划重建阶段处理。
- 替换 A 现有应用更新命令、自动选择生产版本、在更新失败时自动回滚数据库或应用，以及在故障提升现场按可变标签临时拉取镜像。
