# Implement Plan: Sub2API 自动故障切换与自动回切

## 阶段 0：基线与密钥准备

1. 记录 A/B 当前容器、卷、网络、端口、应用 restart policy、现有 Tunnel 服务和 Cloudflare DNS 基线。
2. 明确排除 A 现有指向 `3000` 的 Tunnel 和 B 原 `/root/sub2api` 全部资源。
3. 在 Cloudflare 创建最小权限 API Token、节点身份密钥和 Worker Secrets；真实值不进入任务目录。
4. 在钉钉群创建自定义机器人，记录 Webhook Token 到受限密钥配置；只在用户再次确认后发送测试消息。
5. 恢复或确认 B NTP 同步；时间未同步不得进入自动模式或生产演练。

验证：

- A/B 基线文件完整且不含密码和 Token。
- Cloudflare Token 只能访问目标 zone、Worker、Durable Object 和 `api.havefun.eu.cc` DNS 记录。
- B 原容器 ID、启动时间、卷和端口与旧任务基线一致。

## 阶段 1：Cloudflare Worker 与 Durable Object

1. 在任务受管产物中创建 Wrangler TypeScript 项目，使用 SQLite Durable Object migration。
2. 实现租约、状态、`epoch`、`transition_id`、模式和事件表。
3. 实现节点认证、时间窗口、nonce、防重放和幂等 request ID。
4. 实现 `/status`、节点报告、续租、获取租约、迁移、handoff、模式、暂停、恢复和紧急冻结 API。
5. 实现状态迁移白名单，拒绝旧 `epoch`、非法 owner 和越级迁移。
6. 实现 Cloudflare DNS CNAME 更新，要求 compare-before-update。
7. 实现钉钉 Markdown 告警、事件去重和失败重试。
8. 实现免费额度估算和 70%/85%/95% 阈值行为。

验证：

- 单元测试覆盖 A/B 同时 acquire 只有一个成功。
- `epoch` 单调递增，旧请求不能续租或推进状态。
- Durable Object 重启后状态和事件可恢复。
- DNS、钉钉和额度逻辑使用 mock，不修改真实外部系统。
- `wrangler deploy --dry-run` 和类型检查通过。

## 阶段 2：A/B HA agent 与观察模式

1. 使用 Python 3 标准库实现共享 agent 核心、节点适配器、HTTP 签名、文件锁、退避和 checkpoint。
2. A 适配器读取 `/root/sub2api-ha-export/scripts/switch-mode.sh status --machine`。
3. B 适配器读取 `/root/sub2api-dr/scripts/switch-mode.sh status --machine`。
4. 实现应用、数据库、复制、镜像、restart policy、Tunnel 和端口门禁。
5. 实现 `status`、`observe`、`automatic`、`pause`、`resume`、`emergency-freeze` 本地控制命令。
6. 实现 observe 模式：计算真实决策但只记录拟执行动作。
7. 创建独立 systemd unit、日志轮转、配置模板和无密钥 README。
8. 为所有状态转换、超时、非法响应和重启恢复添加 fixture 测试。
9. 把探测拆为 5 秒本地租约关键探测和 20 秒详细探测；先发送合并心跳，再按权威状态决定是否执行详细探测。
10. A 稳态 `A_ACTIVE` 禁止调用 B SSH；详细探测失败时保留本轮心跳并跳过状态编排。

验证：

- Python 语法、类型或静态检查通过。
- mock Worker 下覆盖续租、过期、自隔离、并发、暂停和恢复。
- observe 模式不执行任何 Docker、数据库、卷、Tunnel 或 DNS 变更。
- agent 重启后从 Durable Object 对账，不使用旧 checkpoint 自行认主。
- mock 详细探测超时后 A 仍完成单次 report，且不触发 self-fencing、B acquire 或任何状态机动作。
- mock 租约关键探测失败且缓存租约过期时，fail-closed 不调用详细探测并直接尝试停止应用。

## 阶段 3：独立 HA Tunnel

1. 创建 A `sub2api-ha-a` Tunnel，catch-all ingress 指向 `http://127.0.0.1:8080`。
2. 创建 B `sub2api-ha-b` Tunnel，catch-all ingress 指向 `http://127.0.0.1:18080`。
3. 使用独立 token、服务名、日志和 systemd unit，不复用现有 cloudflared 服务。
4. 创建 `api.havefun.eu.cc` proxied CNAME，初始指向 A HA Tunnel。
5. 验证 A Tunnel 公共健康；B 应因容灾应用停止而不能提供业务健康，但 connector 本身可在线。
6. 把 Tunnel ID 和 DNS record ID 写入受限运行配置，不写 Token。

验证：

- A 现有指向 `3000` 的 Tunnel 服务、进程、配置和域名不变。
- B 原 Nginx、`biz.havefun.eu.cc`、`8080` 和原容器不变。
- `api` 通过 A Tunnel 到达 A `8080`。
- DNS mock 和只读状态能区分 A/B Tunnel UUID。

## 阶段 4：A 应用启动门禁

> 本阶段改变 A 应用的重启行为，但设计为在线完成，不主动重启当前应用。

1. 修改 A 应用 Compose 重启策略，使应用不由 Docker 自动认主；PostgreSQL 和 Redis 保持原策略。
2. 对当前 `sub2api` 容器执行在线 restart policy 收敛，不停止或重建容器。
3. 实现 A app gate：有效 A 租约时允许启动，无租约时停止并验证端口。
4. 在 A 日常更新和 `sync-release` 后重新验证 restart policy 和租约。
5. 模拟主机重启顺序，确认 agent 未确认租约前应用不会启动。
6. 部署前后记录 A 容器 ID、启动时间和健康状态，证明本阶段无计划重启。
7. 把 A owner 健康与 B 发布镜像就绪拆分：A 镜像漂移不撤销健康 A 的租约，B 接管仍要求 `image_sync=ok`。
8. 新增 A 独立发布同步 oneshot + timer，每 60 秒检查一次，容器稳定 120 秒后复用 `sync-release --dry-run` 和 `sync-release`。
9. timer 只允许拉取精确 digest、更新容灾镜像配置和发布状态；不部署到 B，不启动应用或修改数据库、Redis、卷、Tunnel、DNS。

验证：

- A 当前应用在策略更新前后容器 ID 和启动时间不变。
- Docker daemon 重启模拟中应用不会绕过 HA agent 自动启动。
- restart policy 漂移会触发 fail-closed 和 `CRITICAL` 告警。
- 原 A 更新流程能够在租约门禁下完成更新和 `sync-release`。
- 同标签更新产生新 digest 时，A 继续稳定续租，B 暂时不可接管；timer 自动同步后恢复 `image_sync=ok`。
- 自动同步失败时 A 继续服务，B 接管门禁保持关闭，后续周期可幂等重试。

## 阶段 5：现有主备脚本自动化适配

1. 为 A/B 现有脚本增加内部 orchestrated 入口，验证 `epoch`、`transition_id`、owner 和 state。
2. 保留现有人工命令、确认口令和 `--dry-run` 语义。
3. B 自动提升复用现有 PostgreSQL、Redis、镜像和状态门禁，不复制数据库命令。
4. A 自动重建复用 `prepare-from-b`，只使用新恢复卷并保留旧卷。
5. 自动回切复用 `freeze`、追平检查、`cutback-to-a` 和 `restore-b-standby`。
6. 在每个不可逆动作前后把真实状态写回 Durable Object。
7. 无法证明幂等的中间状态进入 `PAUSED_NEEDS_OPERATOR`。

验证：

- 所有 shell 脚本通过 `bash -n` 和 `shellcheck -S warning`。
- 人工命令接口和现有 `status --machine` 字段保持兼容。
- 旧 `epoch`、错误 owner、错误状态和重复不可逆动作全部被拒绝。
- fixture 覆盖 PostgreSQL 已提升、Redis 已提升、应用启动失败、DNS 失败和 agent 重启。

## 阶段 6：钉钉告警与人工控制

1. 完成 `INFO`、`WARNING`、`CRITICAL` 消息模板。
2. `CRITICAL` 文本包含钉钉 @所有人所需标识，其它级别不 @。
3. 使用稳定事件 ID 做幂等去重，限制发送重试次数。
4. 实现本地待发送队列，Worker 恢复后补发关键事件。
5. 完成 pause、resume 和 emergency-freeze 的审计记录。
6. 在用户确认群和消息内容后发送一次真实测试消息。

验证：

- Markdown 使用真实换行并按段落显示。
- 同一事件重试只产生一条通知。
- 钉钉不可达不阻塞 self-fencing、租约或状态迁移。
- `CRITICAL` 正确 @所有人，`INFO`/`WARNING` 不 @。

## 阶段 7：隔离测试与观察模式部署

1. 使用 mock Docker、PostgreSQL、Redis、Cloudflare API 和钉钉运行状态机故障矩阵。
2. 在 B 先部署 agent 和 HA Tunnel，保持 `observe`、B 应用停止。
3. 在 A 部署 agent 和 HA Tunnel，保持现有应用运行并进入 `observe`。
4. 连续运行至少 24 小时，收集拟执行动作、租约续期、网络抖动、复制和额度数据。
5. 观察期间除精确发布镜像调和外，不启用任何自动资源变更，不迁移客户端。
6. 对发现的误判和状态漂移修复后重新开始完整 24 小时观察。

验证：

- 24 小时内没有 Docker、数据库、卷、DNS 和租约 owner 变更。
- 控制面请求量低于免费额度并保留足够余量。
- A/B 原服务和现有 Tunnel 基线不变。
- 所有拟执行动作都有可解释的触发证据。

## 阶段 8：受控全周期演练

> 本阶段需要单独维护窗口和用户明确确认。

1. 使用演练模式缩短稳定窗口，但不缩短 45 秒租约 TTL 和安全门禁。
2. 触发 A 失去租约并验证 A 自动停止应用。
3. 验证 B 在 2 至 5 分钟目标内取得租约、提升、启动应用并切换 `api`。
4. 恢复 A，验证其不启动旧应用并从 B 使用新卷重建。
5. 验证 A 追平后冻结 B、原子 handoff、提升 A 并切回 `api`。
6. 从新 A 全量重建 B，恢复 A_ACTIVE/B standby。
7. 核对每个阶段的 Durable Object 事件、节点 checkpoint、DNS、Tunnel 和钉钉消息。
8. 任何不可解释状态立即暂停，不在同一窗口盲目重复。

验收：

- 完整周期满足 PRD 所有安全不变量。
- A/B 数据库时间线、复制槽和 Redis 角色符合现有容灾规范。
- B 原单机部署和 A 现有 `3000` Tunnel 未变化。
- 演练结果形成独立 research 记录。

## 阶段 9：正式启用与客户端迁移

1. 用户审核观察和演练结果后，显式把模式改为 `automatic`。
2. 正式恢复 30 分钟稳定窗口和 `04:00–05:00 Asia/Shanghai` 回切窗口。
3. 分批把生产客户端 Base URL 迁移到 `api.havefun.eu.cc`。
4. 盘点残留 `www` 客户端；`www` 保留但不承诺 HA。
5. 保持 `biz` 原用途不变。
6. 启用免费额度、租约、复制、Tunnel 和钉钉告警日常检查。

## 验证命令与测试类别

本地静态与单元测试：

```bash
bash -n <A/B changed scripts>
shellcheck -S warning <A/B changed scripts>
python3 -m compileall <automation directories>
npm test -- <worker tests>
npx wrangler deploy --dry-run
```

服务器只读检查：

```bash
/root/sub2api-ha-export/scripts/switch-mode.sh status --machine
/root/sub2api-dr/scripts/switch-mode.sh status --machine
systemctl status sub2api-ha-agent
systemctl status <dedicated HA tunnel service>
docker inspect <app> --format '{{json .HostConfig.RestartPolicy}}'
curl -fsS https://api.havefun.eu.cc/health
```

故障矩阵至少覆盖：

- A/B 同时申请租约。
- A 到 Cloudflare 网络分区。
- B 到 Cloudflare 网络分区。
- Durable Object 暂时不可达。
- A 应用不健康但 agent 存活。
- A Tunnel 失败但应用健康。
- B 复制延迟、镜像漂移或应用端口占用。
- A 使用相同标签更新到新 digest、稳定窗口未满足、发布同步成功、发布同步失败和幂等重试。
- B PostgreSQL 已提升后 agent 重启。
- 回切冻结后 A 未追平。
- A PostgreSQL 提升后应用启动失败。
- DNS API 和钉钉 API 失败。
- 免费额度达到 70%、85%、95%。
- A/B 主机重启和 Docker daemon 重启。

## 风险与回滚点

- A restart policy 是最大的启动门禁风险，必须先用 fixture 和无重启在线变更验证。
- 自动提升 B 后不能回到 A 旧数据，A 必须重建。
- A PostgreSQL 提升后不能自动恢复 B 写入。
- Cloudflare 免费额度耗尽会触发 fail-closed，必须在接近额度前告警。
- HA agent、Tunnel 和现有人工脚本的资源名必须完全隔离。
- 任意生产演练前先确认 B NTP、复制、镜像和应用数据健康。
- 回滚自动化不等于恢复 `unless-stopped`；必须先确定唯一主节点和权威数据，再按人工容灾流程降级。
