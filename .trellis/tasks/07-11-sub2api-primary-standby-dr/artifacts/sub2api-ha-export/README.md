# Sub2API 主备容灾快速使用

两台服务器各有一个统一操作脚本：

- A：`/root/sub2api-ha-export/scripts/switch-mode.sh`
- B：`/root/sub2api-dr/scripts/switch-mode.sh`

平时只使用这两个脚本，不要手工操作数据库容器或数据卷。

## 1. 日常检查

在 A 执行：

```bash
cd /root/sub2api-ha-export
./scripts/switch-mode.sh status
```

在 B 执行：

```bash
cd /root/sub2api-dr
./scripts/switch-mode.sh status
```

正常状态：

- A 为 `legacy-active` 或 `active-recovered`。
- B 为 `standby`。
- B PostgreSQL为 `recovery=t`。
- B Redis为 `role=slave`、`link=up`、`sync_in_progress=0`。
- B 容灾应用没有运行，`18080` 没有监听。

## 2. A 故障，切换到 B

重要：只有确认 A 已关机或已经完全停止写入时才能继续。网络不通不代表 A 已停机；无法确认时，先在云厂商控制台关闭 A。

在 B 执行：

```bash
cd /root/sub2api-dr
./scripts/switch-mode.sh status
./scripts/switch-mode.sh enable --dry-run
./scripts/switch-mode.sh enable
```

按脚本提示输入确认口令。脚本会依次提升 PostgreSQL、Redis并启动 B 容灾应用。

检查 B：

```bash
curl -fsS http://127.0.0.1:18080/health
./scripts/switch-mode.sh status
```

B 应显示 `active`。最后人工把公共域名或入口切换到 B 的 `18080`。

## 3. A 修复后，切回 A

前提：B 正在提供服务。不要手工启动 A 故障前的旧数据库和应用。

以下命令全部在 A 执行。

### 第一步：从 B 重建 A

```bash
cd /root/sub2api-ha-export
./scripts/switch-mode.sh status
./scripts/switch-mode.sh prepare-from-b --dry-run
./scripts/switch-mode.sh prepare-from-b
```

完成后 A 应显示 `standby-from-b`，A 应用保持停止。

### 第二步：冻结 B，提升 A

```bash
./scripts/switch-mode.sh cutback-to-a --dry-run
./scripts/switch-mode.sh cutback-to-a
```

脚本会停止 B 容灾应用，等待 A 数据完全追平，再提升 A 并启动 A 的 `8080` 服务。

检查 A：

```bash
curl -fsS http://127.0.0.1:8080/health
./scripts/switch-mode.sh status
```

A 应显示 `active-recovered`，B 应显示 `active-stopped`。确认业务正常后，人工把公共域名或入口切回 A 的 `8080`。

### 第三步：恢复 B 为备库

```bash
./scripts/switch-mode.sh restore-b-standby --dry-run
./scripts/switch-mode.sh restore-b-standby
```

该步骤会从新的 A 主库重新初始化 B 容灾数据库。它只操作 B 的 `sub2api-dr-*` 容灾资源，不会修改 B 原有 `/root/sub2api` 单机部署。

最后再次运行 A、B 的 `status`，结果应为 A 主、B `standby`。

## 4. 常见状态

| 节点 | 状态 | 含义 |
|---|---|---|
| A | `legacy-active` | A 原部署正在提供服务 |
| A | `standby-from-b` | A 正从 B 追平，应用停止 |
| A | `active-recovered` | A 已完成回切并提供服务 |
| B | `standby` | B 正在复制 A，应用停止 |
| B | `active` | B 已接管并提供服务 |
| B | `active-stopped` | B 应用已冻结，等待回切 |
| 任一端 | `inconsistent` | 状态矛盾，立即停止操作 |

## 5. 禁止事项

- 所有实际变更都先运行一次相同命令的 `--dry-run`。
- 不要手工执行 `docker compose down`、删除容器或删除数据卷。
- 不要手工启动 B 的 `sub2api-dr-app`。
- B 接管后，不要直接启动 A 故障前的旧服务。
- 不要让 A、B 两边同时提供写入。
- 任何一端出现 `inconsistent` 时，不要继续执行下一步。
- 脚本不会自动切换域名或公共入口。

## 6. 目录说明

- `README.md`：本说明。
- `scripts/switch-mode.sh`：唯一推荐的操作入口。
- `scripts/` 其它文件：辅助脚本，不建议单独执行。
- `compose*.yaml`：容灾配置，不建议手工操作。
- B 的 `.env` 和 A 的 `secrets.env`：运行参数与复制凭据，不要输出或修改。
- `state/`：脚本执行状态，排查问题时不要删除。

如果执行报错，先保存错误信息，再分别运行 A、B 的 `status`。不要连续重跑实际命令，也不要自行清理容器或数据卷。
