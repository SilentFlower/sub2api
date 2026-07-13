# Brief — Sub2API 自动故障切换与自动回切

## Goal

- 在不新增用户自有服务器 C、没有云厂商 fencing 的前提下，使用 Cloudflare Durable Object 强一致租约和 A/B self-fencing，实现 A 故障后自动接管、A 恢复后自动重建与受控回切，并保证任意时刻最多一个容灾节点写入。

## Scope

- 创建 Cloudflare Worker + SQLite Durable Object 控制面，维护 `owner`、`epoch`、45 秒租约、状态机、事件和人工控制。
- 在 A/B 部署独立 Python HA agent 与 systemd 服务，复用现有 `switch-mode.sh status --machine` 和主备切换脚本。
- 把 Agent 探测拆为 5 秒本地租约关键探测和 20 秒详细探测；A 稳态续租不得访问 B，详细探测失败时跳过本轮编排。
- 把 A 应用启动纳入租约门禁，在线收敛 `restart: unless-stopped`，避免 A 重启后旧应用抢跑。
- A 新增 `sub2api-ha-a` Tunnel -> `127.0.0.1:8080`；B 新增 `sub2api-ha-b` Tunnel -> `127.0.0.1:18080`。
- 使用 `api.havefun.eu.cc` 作为唯一承诺 HA 的入口，控制面按权威租约切换 Tunnel CNAME。
- 实现 B 自动提升、A 从 B 新卷重建、30 分钟稳定观察、每天 `04:00–05:00 Asia/Shanghai` 两阶段回切和 B 全量重新入备。
- 接入钉钉群机器人 Webhook，按 `INFO`、`WARNING`、`CRITICAL` 分级，只有 `CRITICAL` @所有人。
- 先运行 24 小时观察模式，再完成一次受控全周期演练，最后显式启用自动模式并迁移生产客户端到 `api`。

## Non-Goals

- 不做 A/B 双活、多主冲突合并或依靠 ping、SSH、DNS TTL、Cloudflare KV 实现仲裁。
- 不新增服务器 C，不自动购买 Cloudflare 付费计划。
- 不修改 B 原 `/root/sub2api`、`biz.havefun.eu.cc` 或 A 现有指向 `3000` 的 Tunnel。
- 不自动删除故障前旧卷。
- `www.havefun.eu.cc` 保留为 A 旧入口，但不承诺 HA。

## Key Context

- 当前正常状态：A `legacy-active`、B `standby`、`image_sync=ok`；B 容灾应用停止。
- PostgreSQL/Redis 为异步复制，继续接受故障瞬间最近数秒写入可能丢失的 RPO。
- 租约 TTL 45 秒、节点间隔 10 秒、fail-closed；Cloudflare 不可达超过 TTL 时当前应用停止，B 无租约不得提升。
- A 的租约关键命令固定为本地 `verify-cutback.sh --machine`；包含 B SSH 的完整状态只用于非稳态详细编排，不能阻塞合并心跳。
- 缓存租约过期后禁止再次探测；即使本地状态不可用，`automatic` 也直接尝试停止应用。
- 正常故障接管 RTO 目标 2 至 5 分钟，任何复制、镜像、角色或 Tunnel 门禁失败时进入 `PAUSED_NEEDS_OPERATOR`。
- A 回切前必须连续健康并追平 30 分钟，只在每日维护窗口执行；A PostgreSQL 提升后不得自动切回 B。
- 免费版只承担控制面，理论基线约 17,280 次请求/天；业务流量通过 Tunnel，不逐请求经过 Worker。
- A/B 自动化必须复用旧任务的数据库角色、PostgreSQL 18 卷布局、镜像 digest 和资源身份契约。
- B NTP 在旧任务中尚未同步，进入自动模式或生产演练前必须处理或确认。
- 真实 Cloudflare、Tunnel、节点和钉钉 Token 只放受限密钥配置，不进入仓库。

## Acceptance

- Durable Object 并发处理 A/B 获取租约时只有一个成功，`epoch` 单调递增。
- 网络分区、Cloudflare 不可达、节点重启和 Docker 重启均不会形成两个应用写入节点。
- A 故障时 B 在门禁通过后自动接管；A 恢复后不启动旧应用，而是从 B 新卷重建。
- 回切完成后 A 恢复唯一主节点，B 从新 A 全量重建为 PostgreSQL 备库和 Redis 从库。
- A 现有 `3000` Tunnel、B 原单机栈、`biz`、原容器、卷和端口基线保持不变。
- 观察模式连续 24 小时无破坏性变更，全周期演练通过后才允许自动模式。
- A 到 B SSH 或详细状态脚本卡满 20 秒时，健康 A 仍完成本地门禁和单次 report，不触发 self-fencing、B acquire 或其它状态机动作。
- 钉钉告警分级、去重正确；免费额度接近上限时告警并安全暂停新编排。
- 演练通过后生产客户端迁移到 `api.havefun.eu.cc`；仍使用 `www` 的客户端不计入 HA 验收。

## Next Step

- 完成全面检查后，单独确认线上部署与租约恢复方案；先部署 Agent 和真实配置，再以新 epoch 恢复 `observe` 并重新开始连续 24 小时观察。实际生产演练仍需要独立维护窗口和再次确认。
