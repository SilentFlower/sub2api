# Design: Sub2API 自动故障切换与自动回切

## 1. 设计目标

本设计在现有人工主备脚本上增加外部强一致租约和本地 self-fencing。Cloudflare 只负责逻辑仲裁、HA Tunnel 和公共入口，不承担 PostgreSQL/Redis 数据复制，也不替代现有 A/B 恢复脚本。

核心不变量：

1. 只有 Durable Object 当前 `owner` 且持有未过期 `epoch` 的节点可以运行容灾应用。
2. 应用写入资格、数据库角色和公共入口必须指向同一权威节点。
3. 不可逆数据库动作只能按持久化状态机向前执行，不能靠重试猜测结果。
4. B 原单机部署和 A 现有 `3000` Tunnel 不属于 HA 资源。

## 2. 总体架构

```text
                         Cloudflare
        +--------------------------------------------+
        | Worker API                                 |
        |   -> Durable Object: lease/state/epoch     |
        |   -> DNS API: api.havefun.eu.cc CNAME      |
        |   -> DingTalk Webhook                      |
        +----------------------+---------------------+
                               |
            10s heartbeat      |     10s observer
                      +--------+--------+
                      |                 |
                      v                 v
             +----------------+  +----------------+
             | A HA agent     |  | B HA agent     |
             | app gate       |  | app gate       |
             | 8080           |  | 18080          |
             +-------+--------+  +--------+-------+
                     |                    |
       sub2api-ha-a Tunnel      sub2api-ha-b Tunnel
                     \                    /
                      \                  /
                       +-- api.havefun.eu.cc

PostgreSQL/Redis:
A primary <==== async replication ==== B standby
B takeover -> A rebuild from B -> handoff -> B rebuild from new A
```

## 3. 组件边界

### 3.1 Cloudflare Worker

Worker 是受认证的控制面入口，职责如下：

- 把节点请求路由到唯一 Durable Object 实例 `sub2api-production`。
- 校验节点身份、请求时间、nonce 和请求签名。
- 提供租约、状态、观察模式和人工控制 API。
- 在 Durable Object 完成状态提交后调用 Cloudflare DNS API 切换 HA Tunnel CNAME。
- 发送钉钉告警并记录发送结果，但告警失败不回滚租约。
- 输出免费额度使用情况和接近上限告警。

Worker 不代理 Sub2API 业务请求，不保存数据库密码、SSH 私钥或服务器登录凭据。

### 3.2 Durable Object

每套生产主备只使用一个 Durable Object。SQLite 只保存控制状态和事件，不保存业务数据。

建议状态结构：

```text
owner                 A | B | NONE
epoch                 unsigned integer
lease_until           ISO-8601 UTC
state                 state machine value
transition_id         unique transition identifier
transition_step       current durable step
transition_step_at    ISO-8601 UTC when the durable step last changed
mode                  observe | automatic | paused
stable_since          ISO-8601 UTC or empty
entry_tunnel          A | B | NONE
last_error_code       stable machine code or empty
last_error_message    redacted message or empty
updated_at            ISO-8601 UTC
```

事件表记录：

```text
event_id, epoch, state_from, state_to, node, reason,
started_at, completed_at, result, error_code
```

所有写操作在 Durable Object 单线程边界内完成。`epoch` 只有在新节点取得写入资格或执行正式 handoff 时递增。

### 3.3 A/B HA agent

节点 agent 使用 Python 3 标准库实现常驻进程，由独立 systemd 服务管理。选择 Python 而不是扩展 shell 循环的原因是：需要稳定 JSON、HTTP 签名、退避、文件锁、持久化状态和可测试状态机，shell 容易在部分失败时产生歧义。

agent 职责：

- 调用现有 `switch-mode.sh status --machine`，解析稳定字段。
- 检查应用容器、重启策略、数据库角色、复制、镜像和 HA Tunnel 服务。
- B 申请租约前必须确认 PostgreSQL WAL receiver 为 `streaming` 且 NTP 明确同步，不能只检查 recovery 标志。
- 每 10 秒续租或观察权威状态。
- 失去租约时先停止应用，再记录 self-fencing 结果。
- 按 Durable Object 状态调用现有 A/B 辅助脚本。
- 每次不可逆动作前后重新读取真实状态，不只依赖本地阶段文件。
- 长时间动作执行期间每 10 秒续租或复核委托 owner；授权变化时终止动作进程组。
- 保存不含密钥的本地 checkpoint，重启后与 Durable Object 对账。
- Worker 不可达时把关键告警写入 `0600` 本地队列，恢复后按稳定事件 ID 补发。

建议路径：

```text
A: /root/sub2api-ha-export/automation/
B: /root/sub2api-dr/automation/

automation/
├── ha_agent.py
├── ha_config.example.json
├── scripts/
│   ├── app-gate.sh
│   ├── tunnel-status.sh
│   └── reconcile.sh
├── systemd/
│   └── sub2api-ha-agent.service
└── tests/
```

真实配置和密钥使用独立 `0600` 文件，不进入仓库。

### 3.4 现有脚本适配层

现有人工入口继续保留：

- A：`status`、`sync-release`、`prepare-from-b`、`cutback-to-a`、`restore-b-standby`。
- B：`status`、`standby`、`enable`、`freeze`。

自动化不得直接复制这些脚本中的数据库命令。应增加内部 orchestrated 入口，要求传入：

```text
epoch
transition_id
expected_owner
expected_state
```

内部入口在执行前通过节点 HMAC 身份重新查询 Worker，并验证 Durable Object 返回的 `epoch`、`transition_id`、owner、state、mode 和租约有效期与参数一致。人工确认口令继续用于人工模式；自动模式不能通过环境变量静默绕过租约检查。

## 4. 控制 API

建议 Worker API：

```text
GET  /v1/status
POST /v1/node/report
POST /v1/lease/renew
POST /v1/lease/acquire
POST /v1/transition/advance
POST /v1/handoff/prepare
POST /v1/handoff/commit
POST /v1/control/mode
POST /v1/control/pause
POST /v1/control/resume
POST /v1/control/emergency-freeze
```

所有响应至少返回：

```text
owner, epoch, lease_until, state, transition_id, mode, server_time
```

写请求包含节点 ID、当前已知 `epoch`、幂等 request ID、时间戳、nonce 和签名。旧 `epoch`、重复 nonce、过期请求和不允许的状态迁移必须拒绝。

## 5. self-fencing 与启动门禁

### 5.1 A 节点

A 当前应用使用 `restart: unless-stopped`，这是自动方案的首要启动风险。实施时：

1. 把 A 应用服务的 Compose 重启策略改为不自动启动，PostgreSQL 和 Redis 保持原策略。
2. 使用 `docker update --restart=no sub2api` 在线收敛当前容器策略，不要求重启应用。
3. HA agent 只有在 A 持有有效租约时才启动应用。
4. 每次 A 日常更新和 `sync-release` 后重新验证并收敛重启策略。
5. agent 发现策略漂移或无租约应用运行时立即停止应用。

这样可尽量避免首次部署重启 A，同时保证后续主机重启不会抢跑。

### 5.2 B 节点

B 原单机应用完全排除。agent 只控制 `sub2api-dr-app`，正常 standby 时该容器停止且没有自动重启资格。

### 5.3 fail-closed 顺序

```text
租约续期失败
-> 在 lease_until 前继续重试并告警 WARNING
-> lease_until 到期
-> 停止本节点容灾应用
-> 验证应用和业务端口停止
-> 写本地 checkpoint
-> 上报 CRITICAL（若 Cloudflare/钉钉可达）
```

停止应用失败时持续重试并进入最高优先级告警；不得因为告警发送失败而停止 fencing。

### 5.4 主节点健康与备机接管就绪分离

活动 A 的续租门禁只判断 A 自身是否仍可安全写入：本地模式、应用 HTTP 健康、PostgreSQL/Redis 主库角色、restart policy 和 A HA Tunnel。A 当前运行镜像与 B 容灾镜像不一致时，A 仍续租并提供服务，但 Worker 报告中的 `imageSyncHealthy=false` 会使 B 的接管门禁保持关闭。

A 使用独立 oneshot + timer 执行发布调和，避免镜像拉取阻塞 10 秒心跳：

```text
每 60 秒检查 A status --machine
-> image_sync=ok: 直接退出
-> A 不是 legacy-active/active-recovered: 直接退出
-> 当前 A 容器启动不足 120 秒: 等待下一轮
-> 获取非阻塞文件锁
-> sync-release --dry-run
-> sync-release
-> 再次确认 image_sync=ok
```

- timer 只部署在 A，不部署在 B；B 不能独立解析可变标签。
- `sync-release` 继续承担 B standby、PostgreSQL streaming、Redis 同步、应用停止、精确 digest 和资源隔离门禁。
- 该调和动作允许在 `observe` 运行，但只能改变镜像缓存、A/B 容灾配置中的单一镜像键和发布状态文件。
- 执行失败不改变租约、应用状态或入口，systemd 下次按固定周期幂等重试；日志必须保留 dry-run 失败原因。

## 6. 正常状态

```text
Durable Object: owner=A, state=A_ACTIVE
A: PostgreSQL primary, Redis primary, app running, HA Tunnel active
B: PostgreSQL recovery, Redis replica, app stopped, HA Tunnel connector active
api.havefun.eu.cc -> sub2api-ha-a Tunnel
```

A 续租必须同时满足：

- A `status --machine` 为 `legacy-active` 或 `active-recovered`。
- 应用健康；镜像同步状态单独决定 B 是否具备接管资格，不决定健康 A 是否续租。
- 应用 restart policy 符合门禁。
- A HA Tunnel 服务健康。
- B 状态没有报告不一致或已提升。

## 7. A 故障与 B 接管

```text
A renew stops
-> A lease expires and A self-fences
-> B verifies local standby gates
-> B atomically acquires epoch N+1, state=B_PROMOTING
-> promote PostgreSQL
-> persist Redis primary role
-> start sub2api-dr-app
-> verify 18080/health
-> checkpoint service-ready
-> change api CNAME to B tunnel UUID
-> checkpoint entry-switched
-> verify public api health
-> state=B_ACTIVE
```

B 获取租约前不执行数据库提升。获取后若 PostgreSQL 提升失败，保持 `owner=B`、应用停止并进入 `PAUSED_NEEDS_OPERATOR`；A 旧数据不能重新成为权威。

## 8. A 恢复与重建

A agent 启动后先读取 Durable Object。发现 `owner=B` 时：

```text
stop A app and enforce restart=no
-> verify B_ACTIVE
-> ensure B active image digest is cached on A
-> run prepare-from-b with epoch proof
-> verify A PostgreSQL recovery and Redis replica
-> state=A_REBUILDING / then FAILBACK_WAIT
-> start 30-minute stable timer
```

稳定计时要求每次检查均满足复制、镜像、Tunnel agent 和机器状态。任一失败清零。

## 9. 两阶段自动回切

### 9.1 准备阶段

仅在 `04:00–05:00 Asia/Shanghai`：

```text
owner=B, state=FAILBACK_WAIT
-> freeze B app while B keeps lease
-> capture B PostgreSQL LSN and Redis offset
-> wait A reaches both targets
```

此阶段失败可在确认 A 未提升后恢复 B 应用，继续由 B 持有原 `epoch`。

### 9.2 提交阶段

```text
Durable Object handoff B -> A, epoch N+1, state=A_PROMOTING
-> promote A PostgreSQL
-> persist A Redis primary role
-> start A app
-> verify 8080/health
-> checkpoint service-ready
-> change api CNAME to A tunnel UUID
-> checkpoint entry-switched
-> verify public api health
-> state=A_ACTIVE
```

A PostgreSQL 一旦提升，B 不得自动恢复应用。后续失败进入 `PAUSED_NEEDS_OPERATOR`，以 A 为权威继续修复。

### 9.3 B 重新入备

```text
owner=A, state=B_REBUILDING
-> create/verify B replication slot on new A
-> rebuild only sub2api-dr PostgreSQL and Redis volumes
-> verify B recovery/replica roles and app stopped
-> state=A_ACTIVE
```

## 10. HA Tunnel 与入口切换

- A/B 分别使用独立 Tunnel；首条 ingress 仅匹配 `api.havefun.eu.cc` 并指向本地应用端口，末条 catch-all 返回 404。
- `api.havefun.eu.cc` 使用 proxied CNAME，内容为当前 Tunnel 的 `<uuid>.cfargotunnel.com`。
- Worker 使用最小权限 Cloudflare API Token，仅允许读取 zone 和修改 `api.havefun.eu.cc` 记录。
- 切换时先读取当前 DNS 记录 ID 和内容，确认预期旧值，再原子更新到目标 Tunnel。
- DNS 更新后从公共域名验证 `/health`；验证失败不把状态标记为 ACTIVE。
- A 现有 `3000` Tunnel 的 ID、Token、systemd 服务和 DNS 记录都不得出现在 HA 清理范围。

## 11. 钉钉告警

告警由 Worker 优先发送，节点在 Worker 不可达时保留本地待发送事件。消息示例字段：

```text
标题: [CRITICAL] Sub2API HA fail-closed
权威节点: A
epoch: 42
状态: A_ACTIVE -> PAUSED_NEEDS_OPERATOR
原因: lease expired
结果: A app stopped
人工动作: 检查 Cloudflare Worker/DO
时间: 2026-07-11 18:00:00 Asia/Shanghai
```

事件 ID 使用 `epoch + transition_id + event_type`，避免重试产生消息风暴。Worker 对钉钉最多重试 3 次；节点在 Worker 不可达时把关键事件写入本地队列，恢复后通过 HMAC 节点告警 API 补发。只有 `CRITICAL` 使用 @所有人。

## 12. 免费额度控制

- A/B 每 10 秒各一次合并状态报告与续租的心跳，理论基线约 17,280 次/天；`status` 只在 Agent 启动或状态恢复时额外调用。
- 状态查询、DNS 切换、告警和重试共享剩余额度。
- agent 使用指数退避和请求熔断，禁止错误时无上限重试。
- Worker 记录当日请求估算；达到 70% 发送 `WARNING`，达到 85% 禁止非必要观察请求，达到 95% 自动暂停新的故障编排并 `CRITICAL` 告警。
- 当前写入节点仍按 TTL fail-closed，不能为了节省额度延长无租约运行。

## 13. 观察模式与启用

`observe` 模式使用真实状态输入和 Durable Object，但不授予自动执行破坏性动作的权限。唯一例外是 5.4 定义的精确发布镜像调和；该动作不改变写入拓扑。每次其它拟执行动作记录：

- 触发条件和原始状态。
- 预期迁移、命令和目标节点。
- 如果执行会触碰的容器、卷、Tunnel 和 DNS。
- 被哪个门禁允许或拒绝。

观察 24 小时无阻断问题后进行维护窗口演练。演练通过后，必须显式调用控制 API 把模式从 `observe` 改为 `automatic`。

## 14. 客户端迁移

- 演练期间 `www.havefun.eu.cc`、`biz.havefun.eu.cc` 不变。
- 演练通过后逐项迁移生产客户端到 `api.havefun.eu.cc`。
- `www` 保留为 A 的旧入口，但不承诺 HA；A self-fence 或 B 接管时允许不可用。
- 验收以 `api` 为唯一业务入口，仍使用 `www` 的客户端列为未迁移项。

## 15. 失败恢复与回滚

- Worker/DO 未启用自动模式前：删除新控制面资源不影响现有人工主备。
- HA Tunnel 未切换客户端前：停止新 Tunnel 不影响 `www`、`biz` 和 A 现有 `3000` Tunnel。
- self-fencing 门禁部署后：紧急回退必须先把 Durable Object 模式设为 `paused`，确认唯一主节点，再恢复人工脚本；不能简单删除 agent 后恢复 `unless-stopped`。
- B 提升前失败：保持 A 为主或保持全局无写入，根据权威租约处理。
- B 提升后失败：B 数据库成为权威，A 必须从 B 重建。
- A 回切提升前失败：可恢复 B 应用。
- A PostgreSQL 提升后失败：A 成为权威，不自动切回 B。
- 任意卷清理只能作用于现有规范允许的恢复卷和 `sub2api-dr-*` 资源。
