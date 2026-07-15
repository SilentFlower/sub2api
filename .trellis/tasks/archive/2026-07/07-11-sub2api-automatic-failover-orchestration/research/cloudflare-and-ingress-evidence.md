# Cloudflare 与入口证据

## 结论

- 不需要用户自有服务器 C；Cloudflare Worker + SQLite Durable Object 可以提供独立于 A/B 的强一致逻辑租约。
- 免费试运行应把 Worker/DO 限定为控制面，业务请求通过 Cloudflare Tunnel，不逐请求调用 Worker。
- A/B 需要独立 HA Tunnel；A 现有指向 `3000` 的 Tunnel 必须保持不变。
- `api.havefun.eu.cc` 作为新的 HA 公共入口，观察和演练通过后再迁移生产客户端。

## 官方平台证据

核对日期：2026-07-11。

### Workers

来源：`https://developers.cloudflare.com/workers/platform/pricing/`

- Workers Free：每天 100,000 次请求，每次调用最多 10 ms CPU。
- Workers Paid：最低 5 美元/月，包含每月 10,000,000 次请求和 30,000,000 CPU ms。
- Worker 出站数据传输和带宽不额外收费。

### Durable Objects

来源：`https://developers.cloudflare.com/durable-objects/platform/pricing/`

- Durable Objects 同时支持 Workers Free 和 Workers Paid。
- Free 只能使用 SQLite 存储后端。
- Free：每天 100,000 次请求、13,000 GB-s duration、5,000,000 行读取和 100,000 行写入。
- Free 超出任一日限额后，对应操作会失败，不是自动产生超额费用。

### Cloudflare 代理端口

来源：`https://developers.cloudflare.com/fundamentals/reference/network-ports/`

- Cloudflare 默认 HTTP 代理端口包含 `80`、`8080`、`8880`、`2052`、`2082`、`2086`、`2095`。
- 默认 HTTPS 代理端口包含 `443`、`2053`、`2083`、`2087`、`2096`、`8443`。
- B 容灾应用端口 `18080` 不在支持列表中，不能只靠普通代理 A 记录直连。

### Cloudflare Tunnel

来源：`https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/`

- `cloudflared` 从服务器发起出站连接，不要求源站开放新的公网监听端口。
- Tunnel 可以把公共 Web/API 流量转发到本机 HTTP 服务。
- 多个 Tunnel 可以在同一服务器并存，并指向不同本地端口。

## DNS 证据

通过 Cloudflare DNS over HTTPS 查询：

- `havefun.eu.cc` NS：`alla.ns.cloudflare.com`、`glen.ns.cloudflare.com`。
- `api.havefun.eu.cc` 在规划时返回 NXDOMAIN，未被占用。
- `ha.havefun.eu.cc` 在规划时返回 NXDOMAIN，未被占用。

## 现有部署证据

- 旧任务记录 A 应用健康入口为 `127.0.0.1:8080`。
- 旧任务记录 B 原单机应用使用 `8080`，B 容灾应用提升后使用 `18080`。
- 用户确认 A 已有 Cloudflare Tunnel 指向本机 `3000`。
- 2026-07-11 对 A 进行只读检查：`cloudflared` 二进制存在且 systemd 服务为 `active`；未修改服务或配置。
- 当前执行环境没有 A 的 SSH 公钥授权，只读检查使用用户已授权的现有登录方式；检查后已退出会话。

## 设计约束

- A 新 HA Tunnel：`sub2api-ha-a` -> `127.0.0.1:8080`。
- B 新 HA Tunnel：`sub2api-ha-b` -> `127.0.0.1:18080`。
- `api.havefun.eu.cc` 的代理 CNAME 只指向当前租约持有者的 Tunnel UUID 目标。
- 两个 HA Tunnel 使用单服务 catch-all ingress，避免在两个 Tunnel 中同时声明冲突的公共 hostname。
- 切换顺序必须是目标应用健康后再更新公共 CNAME；入口切换不能先于数据库和应用就绪。
- A 现有指向 `3000` 的 Tunnel 不属于本任务资源，任何部署、状态检查、回滚和清理都必须排除它。

## 生产准备执行记录

核对日期：2026-07-12。

- A/B 独立 HA Tunnel connector 已作为独立 systemd 服务常驻，A 原有 `cloudflared.service` 保持 `active` 和 `enabled`。
- `api.havefun.eu.cc` 已创建为 proxied CNAME，初始指向 A HA Tunnel；公网 `/health` 返回 `200`。
- B 已安装并启用 `systemd-timesyncd`，机器状态报告 `ntp_synchronized=yes`；PostgreSQL WAL receiver 为 `streaming`，Redis 链路为 `up` 且同步完成。
- B 容灾 Compose 的应用 restart policy 已持久化为 `no`，`sub2api-dr-app` 保持不存在；B 原单机 `sub2api` 容器 ID、启动时间和 restart policy 未变化。
- A Compose 与运行态应用 restart policy 已在线收敛为 `no`；A 应用容器 ID和启动时间未变化，PostgreSQL、Redis 容器未重启。
- A/B HA agent 已启用且无 systemd 重启；控制面为 `owner=A`、`state=A_ACTIVE`、`mode=observe`，A 租约每 5 秒稳定续期。
- 本轮 24 小时观察起点记为 `2026-07-12 00:27:54 Asia/Shanghai`；最早完成时间为 `2026-07-13 00:27:54 Asia/Shanghai`。期间任何破坏性动作、租约异常、误判或配置修复都会使观察窗口重新计时。
- 首次 bootstrap 后，因 Agent 启动验证超过 30 秒，初始租约曾在 A 常驻 Agent 启动前过期。observe 模式未停止应用、未提升 B、未切换 DNS。恢复时先停止 A/B Agent，再执行 `pause -> resume(owner=A, state=A_ACTIVE, mode=observe)`，随后先启动 A 确认续租，再启动 B。
- 上述时序已回写 README：bootstrap 必须放在全部节点准备完成之后，并在 10 秒内先启动 A Agent；A 稳定续租后才能启动 B Agent。
