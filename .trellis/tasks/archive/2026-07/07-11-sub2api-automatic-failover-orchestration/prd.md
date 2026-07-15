# Sub2API 自动故障切换与自动回切

## Goal

在不新增用户自有服务器 C、也不依赖云厂商 fencing 的前提下，为现有 A 主、B 备容灾部署增加自动故障检测、自动提升 B、A 恢复后的自动重建与受控自动回切能力，并保证任意时刻最多只有一个容灾节点接受业务写入。

## Background

- A 是默认主节点，现有 Sub2API、PostgreSQL 和 Redis 由 Docker Compose 部署。
- B 同时运行一套不可触碰的 `/root/sub2api` 单机部署，以及隔离的 `/root/sub2api-dr` 容灾栈。
- A 到 B 的 PostgreSQL 异步物理复制、Redis 主从复制、应用数据同步、固定镜像 digest 同步和人工切换脚本已经建立。
- 当前正常状态为 A `legacy-active`、B `standby`、`image_sync=ok`，B 容灾应用停止。
- PostgreSQL 和 Redis 均为异步复制，用户继续接受故障瞬间可能损失最近数秒写入的 RPO 边界。
- 现有半自动方案没有云厂商 fencing，不能仅凭 ping、SSH 或健康检查失败证明 A 已停止写入。
- `havefun.eu.cc` 已委派到 Cloudflare；`api.havefun.eu.cc` 当前未占用并被选为新的 HA 公共入口。
- A 已有一套指向本机 `3000` 的 Cloudflare Tunnel，该现有 Tunnel 必须保持不变。
- A 当前应用使用宿主机 `8080`，B 容灾应用提升后使用宿主机 `18080`。

## Constraints

- A、B 两节点无法只靠彼此安全区分对端宕机和节点间网络分区，可靠自动切换必须使用独立强一致逻辑仲裁者。
- 使用 Cloudflare Worker + SQLite Durable Object 作为外部强一致租约服务；Cloudflare KV 不得用于实现租约或锁。
- 不新增用户自有服务器 C。
- 使用逻辑 self-fencing：只有持有有效租约的节点允许应用接受写入；租约失效时节点必须停止应用。
- 协调服务不可达时采用 fail-closed，宁可中断也不得继续无租约写入。
- A 当前应用的自动重启策略必须纳入启动门禁，防止 A 重启后在确认权威主节点前直接启动旧应用。
- B 原 `/root/sub2api`、`biz.havefun.eu.cc`、现有容器、网络、卷和端口不得被 HA 自动化修改、停止或重建。
- 所有真实服务器密码、Cloudflare Token、Tunnel Token 和钉钉 Webhook Token 只能存在于受限密钥配置，不得进入任务文件、日志或版本库。

## Requirements

### R1. 外部强一致租约

- Durable Object 保存 `owner`、`epoch`、`lease_until`、`state`、`transition_id`、`updated_at` 和失败信息。
- 获取、续租、转交、暂停和失效判定必须在同一 Durable Object 内原子执行。
- 租约 TTL 固定为 45 秒，节点每 10 秒续租或观察；`epoch` 只能单调递增。
- 租约续期必须先执行不依赖对端 SSH 的本地关键探测；A 的跨节点镜像与 B 状态检查不得阻塞 A 心跳。
- 详细探测在合并心跳之后执行；详细探测超时只能跳过本轮状态编排，不能撤销已通过本地写入门禁的健康 owner 租约。
- A/B 只有在本地角色、复制、镜像和应用状态与 Durable Object 权威状态一致时才允许续租。
- B 只有在 A 租约到期且本地全部提升门禁通过后，才能原子取得新 `epoch` 并进入 `B_PROMOTING`。
- 节点不得根据本地旧状态自行认主，也不得使用最终一致存储模拟租约。

### R2. 节点 self-fencing 与启动门禁

- A/B 各自运行独立 watchdog，持续读取权威租约并验证本地状态。
- 当前写入节点连续无法联系 Durable Object 且 45 秒租约到期时，必须停止 Sub2API 应用；数据库现场可以保留，但不得继续对外写入。
- 租约到期后的 self-fencing 路径不得再执行状态脚本、对端 SSH 或其它详细探测；缺少最新本地状态时也必须直接尝试停止应用。
- B 无法取得新租约时不得提升，即使 A 的 SSH、ping、应用健康检查和 Tunnel 同时失败。
- 节点重启后默认没有写入资格，必须确认有效租约和匹配 `epoch` 后才能启动应用。
- A 的 Sub2API 容器不得继续依赖 `restart: unless-stopped` 绕过门禁；应用启动和停止必须由 HA agent 编排，PostgreSQL 和 Redis 的正常重启策略不受影响。
- watchdog 检测到应用重启策略漂移、无租约应用运行或本地角色矛盾时必须立即 fail-closed，并发送 `CRITICAL` 告警。
- 协调服务恢复后，节点按 Durable Object 中的 `owner`、`epoch` 和状态机阶段恢复，不自动猜测或重置状态。

### R2.1 A 发布镜像自动调和

- A 作为活动 owner 时，应用、数据库角色、Redis 角色、restart policy 和 HA Tunnel 健康决定其写入租约；B 尚未缓存 A 最新镜像只表示故障接管暂不可用，不得单独导致健康 A 丢失租约或 self-fencing。
- B 申请故障接管时仍必须要求发布镜像精确同步、已缓存并与最近同步记录一致；镜像漂移期间 B 不得取得租约或提升数据库。
- A 检测到运行容器 digest 与 A/B 容灾配置不一致后，等待新容器持续运行至少 120 秒，再复用现有 `sync-release --dry-run` 和 `sync-release` 自动调和。
- 自动调和必须以 A 当前运行容器的不可变 `仓库@sha256` 为唯一来源，禁止让 B 独立跟随 `latest`、`build-latest` 或其它可变标签。
- 自动调和只能拉取精确镜像并更新 A/B 容灾 `SUB2API_IMAGE` 与发布状态文件；不得启动 B 容灾应用、重启 A 应用、修改数据库或 Redis 角色、数据卷、Tunnel 或 DNS，也不得触碰 B 原单机部署。
- 自动调和失败时保持 A 继续服务并续租，把 B 标记为不可接管，按退避周期重试并记录告警；不得为了恢复 HA 就绪而回退到旧镜像。
- `observe` 模式允许执行上述受限发布调和，因为它不改变写入拓扑；其它 Docker、数据库、卷、租约 owner 和 DNS 变更仍只记录拟执行动作。

### R3. A 故障后自动提升 B

- A 租约到期后，B 先复核 PostgreSQL 恢复状态和 WAL receiver、Redis 从库与同步状态、镜像 digest、应用停止状态、HA Tunnel 状态和现有脚本机器状态。
- B 原子取得新 `epoch` 后，按 PostgreSQL 提升、Redis 持久化提升、启动容灾应用、应用健康验证、切换 `api.havefun.eu.cc` Tunnel 路由的顺序执行。
- 从 A 租约确认失效到 B 应用健康且入口切换完成的正常 RTO 目标为 2 至 5 分钟。
- RTO 不得覆盖安全门禁；任一门禁失败时进入 `PAUSED_NEEDS_OPERATOR`，不得为了达标强行提升。
- 任一步失败必须保存失败阶段和实际角色，禁止盲目重跑不可逆操作。

### R4. A 恢复后的自动重建

- A 恢复时不得直接启动故障前旧应用或旧数据库写入。
- A 识别 B 当前持有租约后，复用现有 `prepare-from-b` 能力，使用新的恢复卷从 B 全量重建 PostgreSQL、Redis 和非临时应用数据。
- A 必须以 B 当前活动镜像 digest 为权威，并在停止旧服务前确保该镜像已缓存。
- A 重建完成后保持应用停止，作为 B 的备节点持续追平 PostgreSQL LSN 和 Redis offset。
- 故障前旧卷继续保留，不得被自动删除。

### R5. 受控自动回切 A

- A 重建完成后，PostgreSQL、Redis、镜像和应用数据必须连续健康并保持追平 30 分钟；任一门禁失败后稳定计时清零。
- 自动回切只能在每天 `04:00–05:00 Asia/Shanghai` 开始；窗口外保持 B 为唯一活动主节点。
- 回切采用两阶段 handoff：B 持有租约时先冻结应用写入，等待 A 达到 B 的 PostgreSQL LSN 和 Redis offset 冻结点。
- A 完全追平后，Durable Object 原子转交新 `epoch` 给 A；A 才能提升 PostgreSQL、持久化提升 Redis、启动应用并切换公共入口。
- 租约转交前失败时允许恢复 B 应用；A PostgreSQL 已提升后不得自动回退到 B，必须以 A 为权威并在无法继续时进入 `PAUSED_NEEDS_OPERATOR`。
- A 回切成功后，B 必须从新的 A 主节点重新执行 PostgreSQL 基础备份和 Redis 全量同步，不能原地把旧 B 主库声明为备库。
- 维护窗口只限制计划性回切，不限制 A 故障后的 B 自动接管。

### R6. 状态机、重试与人工控制

- 状态机至少覆盖 `A_ACTIVE`、`FAILOVER_WAIT`、`B_PROMOTING`、`B_ACTIVE`、`A_REBUILDING`、`FAILBACK_WAIT`、`B_FREEZING`、`A_PROMOTING`、`A_ACTIVE`、`B_REBUILDING` 和 `PAUSED_NEEDS_OPERATOR`。
- 每次迁移记录节点、`epoch`、`transition_id`、触发原因、开始时间、结果和失败位置。
- A/B 提供机器可读状态，能区分租约、应用写入资格、数据库角色、复制健康、Tunnel 和公共入口归属。
- 只读检查和幂等 API 调用允许有限次数退避重试；每次重试必须记录原因和结果。
- PostgreSQL 提升、Redis 改变复制方向、删除或重建数据卷、租约转交等不可逆步骤不得盲目自动重跑；重试前必须读取实际角色和阶段标记。
- 提供 `status`、`observe`、`automatic`、`pause`、`resume` 和 `emergency-freeze` 人工控制；默认模式为 `observe`。
- 人工控制不能绕过 Durable Object 的 `epoch` 和唯一写入约束。

### R7. Cloudflare 免费控制面与 HA Tunnel

- 免费试运行阶段 Worker/DO 只作为控制面，正常 Sub2API 业务请求不逐请求经过 Worker。
- A 新增独立 `sub2api-ha-a` Tunnel，指向 `127.0.0.1:8080`；不得复用、重启或改写现有指向 `3000` 的 Tunnel。
- B 新增独立 `sub2api-ha-b` Tunnel，指向容灾应用 `127.0.0.1:18080`；不得修改 B 原 Nginx 或原单机栈。
- 两个 HA Tunnel 使用独立 Tunnel ID、Token、服务单元、日志和状态。
- `api.havefun.eu.cc` 仅路由到当前租约持有者的 HA Tunnel；路由切换必须发生在目标应用健康之后。
- Workers Free 和 SQLite Durable Objects Free 的当前请求、duration 和存储限额必须被记录和监控；接近限额时发送告警。
- 两节点每 10 秒一次控制面请求的理论基线约为 17,280 次/天，重试和附加检查必须有上限。
- 免费额度不足时暂停自动切换并告警，不自动开通 Workers Paid；后续升级单独授权。

### R8. 分阶段启用与演练

- 初始部署后先运行至少连续 24 小时观察模式；观察模式执行真实检查、租约判断和状态机计算，但不得停止应用、提升数据库、改变复制方向、重建数据卷或修改公共入口。
- 观察模式记录每次拟执行动作及完整触发证据。
- 观察期无阻断问题后，在单独维护窗口完成一次受控全周期演练：B 接管、A 从 B 重建、测试用稳定窗口后回切 A、B 从新 A 重建为备。
- 演练允许缩短测试用稳定窗口，正式自动模式仍固定为连续健康 30 分钟。
- 只有观察和全周期演练均通过，才允许显式切换为 `automatic`；部署不得默认开启全自动。

### R9. 钉钉告警

- 使用钉钉群自定义机器人 Webhook 发送 Markdown 告警；Webhook Token 不得出现在日志和版本库。
- 告警包含级别、权威节点、`epoch`、状态机阶段、原因、结果、人工动作和 `Asia/Shanghai` 时间。
- 同一 `epoch` 和状态迁移使用稳定事件 ID 去重；发送失败允许有限重试，但不得阻塞 self-fencing 或状态迁移。
- `INFO` 用于观察模式、正常重建进度和正常回切完成，不 @任何人。
- `WARNING` 用于租约续期抖动、复制延迟和免费额度接近上限，不 @所有人。
- `CRITICAL` 用于 fail-closed、B 接管开始或失败、`PAUSED_NEEDS_OPERATOR` 和双主风险，必须 @所有人。
- Markdown 使用真实换行和有效段落格式，避免消息内容挤在一行。

### R10. 公共入口迁移与兼容

- 观察和演练期间现有客户端入口保持不变。
- 演练通过后，生产客户端统一迁移到 `api.havefun.eu.cc`，只有该域名承诺自动主备切换。
- `www.havefun.eu.cc` 保留为 A 的旧入口，不跳转、不删除，但明确不具备自动容灾能力；A 被 self-fence 或 B 接管期间该入口允许不可用。
- `biz.havefun.eu.cc` 和 B 原单机部署保持原用途，不参与 HA 流量。
- 迁移后必须盘点并验证所有生产客户端已使用 `api.havefun.eu.cc`，不能把仍访问 `www` 的客户端计入 HA 验收。

## Acceptance Criteria

- [ ] 不部署用户自有服务器 C，Durable Object 并发处理 A/B 获取租约时只有一个节点成功，`epoch` 单调递增。
- [ ] A 与 B 互相网络不可达时，最多只有持有有效租约的一方能够保持应用写入。
- [ ] Durable Object 不可达超过 45 秒时，当前应用自动停止且 B 不提升；恢复后不会出现两个节点同时恢复写入。
- [ ] A 到 B SSH 或详细状态脚本连续卡满 20 秒时，A 仍以 5 秒内完成的本地关键门禁正常续租；系统只跳过本轮编排，不模拟故障接管。
- [ ] A 重启时旧应用不会在确认租约前自动启动；检测到重启策略漂移时会 fail-closed。
- [ ] A 使用相同标签更新到新 digest 后继续持有健康租约，120 秒稳定窗口后自动把精确 digest 同步到 B，最终恢复 `image_sync=ok`，且 A/B 业务容器均不因同步重启。
- [ ] 镜像同步失败期间 B 不能取得租约或提升，但健康 A 不因 B 发布就绪失败而停止服务；修复后自动重试可幂等收敛。
- [ ] 正常门禁全部通过时，从 A 租约确认失效到 B 应用健康且入口切换完成控制在 2 至 5 分钟。
- [ ] 任一复制、镜像、角色或 Tunnel 门禁失败时安全暂停，不强行提升。
- [ ] A 恢复后从 B 使用新卷重建，故障前旧卷保留，应用在回切前保持停止。
- [ ] A 连续健康 30 分钟且进入 `04:00–05:00 Asia/Shanghai` 后，系统完成冻结 B、转交租约、提升 A 和切换入口。
- [ ] A 回切成功后，B 从新 A 全量重建为 PostgreSQL 备库和 Redis 从库，B 容灾应用停止。
- [ ] A 现有指向 `3000` 的 Tunnel、B 原 `/root/sub2api`、`biz.havefun.eu.cc` 和原有容器基线保持不变。
- [ ] 正常业务请求不经过 Worker；控制面请求可监控并保持在免费额度内，接近限额时告警。
- [ ] 观察模式连续运行 24 小时且不产生服务、数据库、数据卷、租约所有权或入口变更。
- [ ] 受控演练完成 B 接管、A 重建、A 回切和 B 重新入备的完整周期后，才允许开启全自动。
- [ ] 钉钉能够收到分级、去重的观察、故障、暂停、回切和额度告警，`CRITICAL` 正确 @所有人。
- [ ] 演练通过后生产客户端迁移到 `api.havefun.eu.cc`；`www` 保留但不作为 HA 验收入口。

## Out Of Scope

- A/B 双活写入或多主数据库冲突合并。
- 仅依靠 ping、SSH、DNS TTL、Cloudflare KV 或最终一致存储实现自动仲裁。
- 修改 B 原单机部署、`biz.havefun.eu.cc` 或 A 现有指向 `3000` 的 Tunnel。
- 自动购买或升级 Cloudflare 付费计划。
- 自动删除故障前旧数据卷。
- 在规划确认前修改生产服务器、创建 Cloudflare 资源、发送钉钉消息或执行真实提升和回切。
