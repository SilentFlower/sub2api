# Sub2API 双节点自动容灾产物说明

本目录是 A/B 人工主备方案之上的自动编排层。Cloudflare Durable Object 维护强一致租约，A/B agent 执行 self-fencing 和现有容灾脚本。当前产物用于本地审查、观察模式和后续受控演练，不代表已经部署到生产。

## 资源边界

- A HA 目录固定为 `/root/sub2api-ha-export`，应用为 `sub2api`，新 Tunnel 指向 `127.0.0.1:8080`。
- B HA 目录固定为 `/root/sub2api-dr`，应用为 `sub2api-dr-app`，新 Tunnel 指向 `127.0.0.1:18080`。
- A 现有指向 `3000` 的 Tunnel、`www.havefun.eu.cc` 保留且不纳入 HA 修改范围。
- B 原 `/root/sub2api`、原应用端口 `8080` 和 `biz.havefun.eu.cc` 不参与 HA。
- HA 公共入口固定为 `api.havefun.eu.cc`，只有完成观察和演练后才迁移生产客户端。

这些路径和资源名是当前部署契约，不是任意路径可迁移的通用安装器。配置模板中的 Worker URL、Token 文件和部分命令可配置，但修改 A/B 根目录、原 Compose 路径或容器名时，必须同步修改旧容灾脚本、预检、systemd 和本文档。

## 目录

```text
artifacts/
├── cloudflare-worker/  Worker、Durable Object、Tunnel 管理脚本和测试
├── automation/         Python HA agent、配置模板、安装脚本和 systemd unit
└── managed-shell/      A/B 需要部署到既有容灾目录的受管 switch-mode.sh
```

## 1. 本地验证

```bash
cd cloudflare-worker
npm ci
npm run check
npm test
npm run deploy:dry-run

cd ../automation
python3 -m compileall sub2api_ha test
python3 -m unittest discover -s test -v

bash -n bin/*.sh ../cloudflare-worker/bin/*.sh ../managed-shell/*/*.sh
```

有 `shellcheck` 时再执行：

```bash
shellcheck -S warning bin/*.sh ../cloudflare-worker/bin/*.sh ../managed-shell/*/*.sh
```

## 2. 创建两个独立 HA Tunnel

准备 `curl`、`jq` 和一个权限为 `0600` 的 Cloudflare API Token 文件。Token 至少需要目标账户的 Cloudflare Tunnel 编辑权限；不要把 Token 放入仓库。

```bash
cloudflare-worker/bin/manage-ha-tunnel.sh create \
  --node A \
  --account-id '<ACCOUNT_ID>' \
  --api-token-file /root/.cloudflare/ha-api-token \
  --token-output /root/.cloudflare/sub2api-ha-a.token \
  --dry-run

cloudflare-worker/bin/manage-ha-tunnel.sh create \
  --node B \
  --account-id '<ACCOUNT_ID>' \
  --api-token-file /root/.cloudflare/ha-api-token \
  --token-output /root/.cloudflare/sub2api-ha-b.token \
  --dry-run
```

先确认输出中的名称和本地端口，再去掉 `--dry-run`。每个 Tunnel 只接受 `api.havefun.eu.cc` hostname，最后使用 `http_status:404` 拒绝其它 hostname。脚本只创建 `sub2api-ha-a`、`sub2api-ha-b`，不会复用、修改或删除任何已有 Tunnel。创建后用 `status` 只读核验：

```bash
cloudflare-worker/bin/manage-ha-tunnel.sh status \
  --node A \
  --account-id '<ACCOUNT_ID>' \
  --api-token-file /root/.cloudflare/ha-api-token \
  --tunnel-id '<A_TUNNEL_UUID>'
```

B 同理使用 `--node B`。记录两个 `<UUID>.cfargotunnel.com`，但不要记录 Tunnel Token。

## 3. 准备 Worker

`wrangler.jsonc` 已固定 `api.havefun.eu.cc`、45 秒 TTL 和免费版每日 100,000 次请求估算。部署前需要设置：

```text
NODE_A_SECRET
NODE_B_SECRET
ADMIN_TOKEN
CLOUDFLARE_API_TOKEN
CLOUDFLARE_ZONE_ID
CLOUDFLARE_DNS_RECORD_ID
A_TUNNEL_TARGET=<A_UUID>.cfargotunnel.com
B_TUNNEL_TARGET=<B_UUID>.cfargotunnel.com
DINGTALK_WEBHOOK_TOKEN
```

节点密钥至少 32 字符，A/B 必须不同。`CLOUDFLARE_API_TOKEN` 只允许读取目标 zone 并修改 `api.havefun.eu.cc` 这一条 DNS；`CLOUDFLARE_DNS_RECORD_ID` 必须是该记录 ID。所有真实值通过 `wrangler secret put <NAME>` 设置，不写入 JSON 或 README。

首次部署后只执行一次 bootstrap，确认 A 当前是唯一写入主节点：

```bash
curl -fsS -X POST 'https://<WORKER_DOMAIN>/v1/bootstrap' \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H 'Content-Type: application/json' \
  --data '{"transitionId":"bootstrap-production-a"}'
```

bootstrap 固定进入 `observe`，不要直接切到 `automatic`。

## 4. 管理员控制命令

把 Worker `ADMIN_TOKEN` 单独保存为权限 `0600` 的文件。以下命令都使用 agent 配置中的 Worker URL，但不会读取节点 HMAC 密钥：

```bash
python3 -m sub2api_ha.cli observe --config /etc/sub2api-ha/agent.json \
  --admin-token-file /etc/sub2api-ha/admin-token

python3 -m sub2api_ha.cli pause --config /etc/sub2api-ha/agent.json \
  --admin-token-file /etc/sub2api-ha/admin-token \
  --reason '人工维护暂停'

python3 -m sub2api_ha.cli emergency-freeze --config /etc/sub2api-ha/agent.json \
  --admin-token-file /etc/sub2api-ha/admin-token \
  --reason '检测到双主风险'
```

`emergency-freeze` 会清除 owner，两个节点都应 fail-closed。恢复前必须人工核对数据库角色、应用状态和公共入口，再显式声明当前 epoch、唯一 owner 和真实状态：

```bash
python3 -m sub2api_ha.cli resume --config /etc/sub2api-ha/agent.json \
  --admin-token-file /etc/sub2api-ha/admin-token \
  --expected-epoch 42 \
  --owner A \
  --resume-state A_ACTIVE \
  --resume-mode observe \
  --reason '已确认 A 是唯一主节点且 B 应用停止'
```

resume 会递增 epoch，默认回到 `observe`。只有 24 小时观察和完整演练通过后，才能执行 `automatic` 命令。活动 owner 会在自动模式下纠正 `api` 入口的 Tunnel 漂移。

## 5. 安装受管 shell 入口

先只做 dry-run：

```bash
automation/bin/install-managed-shell.sh --node A --dry-run
automation/bin/install-managed-shell.sh --node B --dry-run
```

实际安装只替换各自容灾目录中的 `scripts/switch-mode.sh`，会保留时间戳备份，不会复制数据库脚本或修改 Compose。安装后只运行：

```bash
/root/sub2api-ha-export/scripts/switch-mode.sh status --machine
/root/sub2api-dr/scripts/switch-mode.sh status --machine
```

生产提升、冻结、重建和回切仍必须放在单独维护窗口。

自动化调用不再只信任 `HA_EPOCH` 等环境变量。`switch-mode.sh` 会调用 `/usr/local/libexec/sub2api-ha-verify-action`，通过节点 HMAC 身份实时复核 Worker 中的 owner、epoch、state、transition ID 和租约有效期；验证失败时不会越过确认门禁。

## 6. 持久化 A/B 应用启动门禁

本步骤需要 `mikefarah/yq v4`。先分别运行 dry-run：

```bash
automation/bin/apply-app-gate.sh --node A --dry-run
automation/bin/apply-app-gate.sh --node B --dry-run
```

确认 Compose 路径、当前 restart policy、容器 ID 和启动时间后，再执行实际命令。脚本会：

1. 备份 A `/root/sub2api/deploy/docker-compose.yml` 或 B `/root/sub2api-dr/compose.yaml`。
2. 结构化设置 A `services.sub2api.restart` 或 B `services.app-dr.restart` 为 `"no"`。
3. 运行 `docker compose config` 验证。
4. 在线执行 `docker update --restart=no sub2api`。
5. 确认容器 ID 和启动时间没有变化。

PostgreSQL、Redis 和 B 原单机部署不会被修改。这些步骤尚未在生产服务器执行，执行前仍需分别确认。

## 7. 安装 Agent

从 `config/agent-a.example.json`、`config/agent-b.example.json` 生成真实配置，权限设为 `0600`。Worker 地址必须使用 HTTPS；节点 HMAC 密钥单独保存到 `/etc/sub2api-ha/node-secret`，权限同样为 `0600`。

`public_health_url` 必须是 `https://api.havefun.eu.cc/health`，`public_health_timeout_seconds` 默认 90 秒。提升流程会先验证本地应用，再切换 Tunnel，最后从公共域名验证健康；公共验证超时会进入 `PAUSED_NEEDS_OPERATOR`，不会标记为 ACTIVE。

```bash
automation/bin/install-agent.sh \
  --node A \
  --source automation \
  --config /root/agent-a.json \
  --dry-run
```

B 使用自己的配置重复执行。实际安装不会启用服务，也不会修改应用 restart policy。先手工执行状态采集和单轮观察，再安装独立 Tunnel unit，最后才允许启动 `sub2api-ha-agent.service`。

安装器还会部署 `/usr/local/libexec/sub2api-ha-verify-action`。长时间数据库动作执行期间，Agent 每 10 秒续租或复核委托 owner；授权变化时会终止动作进程组并进入人工暂停。`run` 和 `once` 共用 `/run/sub2api-ha-agent.lock` 非阻塞进程锁，禁止同一节点并发执行两个编排循环。

A 安装器还会安装但不启用 `sub2api-ha-release-sync.timer`。该 timer 每 60 秒检查 A 当前运行容器的精确 digest；只有 A 为活动模式、`image_sync` 不为 `ok` 且容器已稳定至少 120 秒时，才依次执行 `sync-release --dry-run` 和 `sync-release`。它不会部署到 B，也不会让 B 跟随可变标签。Agent 稳定续租后单独启用：

```bash
systemctl enable --now sub2api-ha-release-sync.timer
systemctl list-timers sub2api-ha-release-sync.timer
```

每次成功心跳都会原子更新 `/var/lib/sub2api-ha-agent/checkpoint.json`。Agent 重启且 Worker 暂时不可达时，只把该文件用于 fail-closed：上次 `automatic`/`paused` 租约已过期或本节点不是 owner 时停止应用，不会凭 checkpoint 自行认主。Worker 不可达期间的关键告警保存在 `/var/lib/sub2api-ha-agent/pending-alerts.json`，权限为 `0600`，恢复后自动补发。

`observe` 模式仍使用完整健康门禁决定是否续租，但只记录拟执行的拓扑动作、原始状态和门禁证据，不停止应用或发送“已完成隔离”的误导性告警。精确发布镜像调和是唯一例外：它可以拉取镜像并更新 A/B 容灾镜像配置和发布记录，但不能启动应用或修改数据库、Redis、卷、Tunnel、DNS 和租约 owner。

正常循环把状态报告和 owner 续租合并为一次心跳，A/B 理论基线约 17,280 次 Worker 请求/天；Agent 仅在启动或恢复状态时额外调用一次 `status`。10 秒间隔在 45 秒租约内保留约四次完整重试机会；代价是故障确认最多比原 30 秒租约增加约 15 秒。

Tunnel Token 放在 `/etc/sub2api-ha/tunnel-a.env` 或 `tunnel-b.env`：

```text
TUNNEL_TOKEN=<真实 Tunnel Token>
```

文件权限必须为 `0600`。A/B 使用不同 unit：`cloudflared-sub2api-ha-a.service`、`cloudflared-sub2api-ha-b.service`。新 unit 只在固定系统 PATH 中查找 `cloudflared`；启动前运行 `command -v cloudflared` 确认二进制存在，不能修改现有 A Tunnel 服务。

## 8. 启用顺序

1. 修复或确认 B 的 NTP 同步。
2. 创建 Worker、A/B 独立 HA Tunnel、`api` DNS 和 Worker Secrets，但暂不 bootstrap。
3. B 安装受管入口、应用启动门禁和 agent，保持容灾应用停止；A 安装受管入口、在线收敛应用启动门禁并安装 agent。此时两个 agent 服务仍保持停止。
4. 核验 A 应用、数据库、Redis、镜像和 Tunnel 健康，B 为健康 `standby`、NTP 已同步、容灾应用停止；确认两个 agent 的本地 `status` 均符合门禁。
5. Worker bootstrap 到 `owner=A`、`state=A_ACTIVE`、`mode=observe`。bootstrap 创建的初始租约为 45 秒，必须在 15 秒内先启动 A agent，并确认租约剩余时间连续保持在 30 秒以上。
6. A 已稳定续租后再启动 B agent，确认 B 不记录模拟接管、容灾应用仍停止且公共入口仍指向 A；随后在 A 启用 `sub2api-ha-release-sync.timer`。
7. 连续观察至少 24 小时；期间不得修改 owner、DNS、数据库角色、卷或应用状态。
8. 重新检查免费额度、误判、复制、镜像 digest、Tunnel 和钉钉告警。
9. 单独申请维护窗口，完成 A 故障、B 接管、A 重建、回切和 B 重新入备的完整演练。
10. 演练通过后，显式切换到 `automatic`，再逐项迁移生产客户端到 `api.havefun.eu.cc`。

## 9. 故障处理

- Worker/DO 不可达超过 TTL：当前 owner 应停止应用，B 不得无租约提升。
- 任一受控动作失败：状态进入 `PAUSED_NEEDS_OPERATOR`，不要直接重复不可逆命令。
- B 已提升：B 数据成为权威，A 必须从 B 新卷重建。
- A 已提升：不得自动恢复 B 写入，先修复 A，再从新 A 全量重建 B。
- 回滚 agent 前先暂停控制面并人工确认唯一主节点；不能只停 agent 后恢复应用 `unless-stopped`。
- bootstrap 后若 A agent 未在初始 45 秒租约内开始续租，不要让运行中的 agent 直接进入 `paused`。先停止 A/B agent，管理员执行 `pause`，再以已确认的唯一 A 主节点执行 `resume` 回到 `observe`；随后先启动 A 并确认续租，最后启动 B。

真实 Cloudflare 资源创建、钉钉测试、A restart policy 在线收敛、生产演练和 `automatic` 启用都需要分别确认。
