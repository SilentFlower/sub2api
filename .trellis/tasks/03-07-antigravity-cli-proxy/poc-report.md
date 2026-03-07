# Headless LS 代理 PoC 验证报告

## 概要

**结论：PoC 验证成功。** Headless Language Server (standalone 模式) 能完成完整的 AI 对话流程。

## 验证环境

| 项目 | 值 |
|------|-----|
| 平台 | Linux 6.6.87.2 (WSL2) |
| LS 版本 | Antigravity 1.107.0 内置 |
| LS 二进制 | `/usr/share/antigravity/resources/app/extensions/antigravity/bin/language_server_linux_x64` (193MB) |
| 运行模式 | `--standalone` |
| 账号 | silentflower233@gmail.com (standard-tier) |
| 代理方式 | DNS 劫持 + TCP Relay (SOCKS5) + HTTPS_PROXY |

## 验证结果

### 1. LS 启动 (standalone 模式)

```bash
HTTPS_PROXY=http://127.0.0.1:7890 HTTP_PROXY=http://127.0.0.1:7890 NO_PROXY=127.0.0.1,localhost \
/usr/share/antigravity/resources/app/extensions/antigravity/bin/language_server_linux_x64 \
  --standalone --app_data_dir=antigravity --server_port=44000 \
  --cloud_code_endpoint=https://cloudcode-pa.googleapis.com
```

- 初始化耗时: ~750ms
- 无需 Electron 宿主或 Extension Server
- 无需 stdin init protobuf 或 parent pipe
- Token 自动从 `~/.gemini/jetski-standalone-oauth-token` 读取
- 内存占用: ~150MB

### 2. ConnectRPC 调用验证

| 方法 | 状态 | 耗时 | 备注 |
|------|------|------|------|
| Heartbeat | ✓ | <10ms | `{}` |
| GetCascadeModelConfigData | ✓ | ~1s | 返回 6 个模型 |
| StartCascade | ✓ | <100ms | 返回 cascadeId |
| SendUserCascadeMessage | ✓ | <100ms | `{}` |
| GetCascadeTrajectory | ✓ | ~5-12s (首次) | 完整轨迹含 AI 回复 |
| GetUserStatus | ✓ | ~2s | 配额信息 |

### 3. 完整对话流程

```
1. StartCascade → cascadeId
2. SendUserCascadeMessage(cascadeId, model, text) → {}
3. GetCascadeTrajectory(cascadeId) → [轮询]
   → USER_INPUT (DONE)
   → CONVERSATION_HISTORY (DONE)
   → KNOWLEDGE_ARTIFACTS (DONE)
   → EPHEMERAL_MESSAGE (DONE)
   → PLANNER_RESPONSE (DONE) ← 包含 thinking + toolCalls/text
   → (可能有更多步骤: TOOL_RESULT, 继续 PLANNER_RESPONSE...)
4. 如果 PLANNER_RESPONSE 包含 toolCalls → 需要执行工具并继续
5. 如果只包含 text → 对话完成
```

### 4. 可用模型

| 模型 Key | 对应模型 |
|----------|---------|
| MODEL_PLACEHOLDER_M37 | Gemini 3.1 Pro (High) — 默认 |
| MODEL_PLACEHOLDER_M36 | Gemini 3.1 Pro (Low) |
| MODEL_PLACEHOLDER_M18 | Gemini 3 Flash |
| MODEL_OPENAI_GPT_OSS_120B_MEDIUM | GPT-OSS 120B (Medium) |
| MODEL_PLACEHOLDER_M35 | Claude Sonnet 4.6 (Thinking) |
| MODEL_PLACEHOLDER_M26 | Claude Opus 4.6 (Thinking) |

### 5. 代理方案

**问题**: LS 内部 gRPC 连接不走 HTTP 代理，直连 Google IP。

**已尝试方案**:

| 方案 | 结果 | 原因 |
|------|------|------|
| `proxychains4` | ✗ | Go 静态链接，不走 libc |
| `HTTPS_PROXY` 环境变量 | REST ✓ / gRPC ✗ | gRPC 库绕过 HTTP 代理 |
| `iptables + redsocks` | gRPC ✓ / REST ✗ | 透明代理与 Go HTTP 客户端冲突 |
| `graftcp` | ✗ | 缺少 graftcp-local |

**最终方案: DNS 劫持 + TCP Relay + HTTPS_PROXY**

```
┌─────────────────────────────────────────────────────────┐
│                    Language Server                        │
│                                                           │
│  REST 请求 ──→ HTTPS_PROXY (127.0.0.1:7890) ──→ Google  │
│                                                           │
│  gRPC 请求 ──→ DNS 解析 cloudcode-pa → 127.0.0.2        │
│              ──→ TCP Relay (127.0.0.2:443)                │
│              ──→ SOCKS5 代理 (127.0.0.1:7890)            │
│              ──→ Google (真实 IP)                          │
└─────────────────────────────────────────────────────────┘
```

## PoC 脚本

| 脚本 | 用途 |
|------|------|
| `run_poc.sh` | 一键启动: DNS 劫持 + TCP Relay + LS |
| `tcp_relay.py` | SOCKS5 TCP 转发 (gRPC 代理) |
| `test_chat.sh` | 完整对话测试 |

### 使用方法

```bash
# 1. 确保 SOCKS5 代理在 127.0.0.1:7890 运行
# 2. 确保 OAuth token 在 ~/.gemini/jetski-standalone-oauth-token

# 启动 LS
sudo bash run_poc.sh

# 另一终端测试对话
bash test_chat.sh 44000 "Hello, what is 2+2?"
```

## Phase 2 集成要点

### 需要实现的组件

1. **LS 进程管理器** — 启动/停止/健康检查 LS 实例
2. **TCP Relay 管理** — 为每个 LS 实例配置网络代理
3. **ConnectRPC 客户端** — StartCascade + SendMessage + GetTrajectory
4. **响应格式转换** — Trajectory Steps → OpenAI ChatCompletion 格式
5. **OAuth Token 管理** — refresh_token 持久化，access_token 自动刷新

### 关键注意事项

- LS 的 `GetCascadeTrajectory` 是轮询式，不是流式推送
- 每个 cascade 有独立的 trajectoryId
- PLANNER_RESPONSE 可能包含 toolCalls（需要模拟工具执行或忽略）
- LS 会自动发心跳和遥测，无需额外处理
- standalone 模式内置 OAuth client_secret，支持自动 token 刷新

### 资源消耗

| 资源 | 每账号消耗 |
|------|-----------|
| LS 进程 | ~150MB 内存 |
| TCP Relay | ~10MB（共享） |
| 端口 | 1 个 (server_port) |
| /etc/hosts 条目 | 共享 |
