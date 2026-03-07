# Antigravity Headless LS 代理

## Goal
通过启动 headless `language_server` Go 二进制，以 ConnectRPC IPC 方式代理 sub2api 的 Antigravity 请求。LS 原生处理所有上游通信（TLS/gRPC/遥测/心跳），消除 sub2api 直发 HTTP 的检测风险。

## Background

### Phase 1-4 研究结论（2026-03-07）
- Antigravity 的 AI 请求由 Go `language_server` 发出，使用 HTTP REST（非 gRPC）
- Go 运行时尊重 `HTTPS_PROXY` 环境变量
- sub2api 直接发 HTTP 存在多项检测差异：缺少遥测、headers 不完整、TLS 指纹错误

### zerogravity 参考分析
分析了第三方项目 zerogravity（Rust MITM 代理）的完整架构，关键发现：
- LS 可以 **headless 运行**（不需要 Electron GUI），只需 stub extension server
- 通过 ConnectRPC 向 `https://127.0.0.1:{port}/exa.language_server_pb.LanguageServerService/{Method}` 发 JSON/Proto 请求
- LS 自己处理所有上游通信（TLS、gRPC、遥测、心跳），指纹完全真实
- LS 自带 OAuth token 刷新（读取 state.vscdb 中的 refresh_token）
- zerogravity 仍被封号的原因：多账号轮转模式、服务端用量统计、遥测缺失等行为层面的问题

### 方案选择理由
| 方案 | 优点 | 缺点 |
|------|------|------|
| A: 完整 MITM（zerogravity 模式） | 完美伪装 | 极复杂、每账号 ~400MB、需 MITM CA 信任 |
| B: 修补 sub2api headers/UA | 简单快速 | 治标不治本、TLS 指纹仍不对 |
| **C: Headless LS 代理（选定）** | **TLS/遥测全真实、每账号 ~50MB** | **需要管理 LS 进程、protobuf 解析** |

## Requirements
- 启动 headless `language_server` 进程，不依赖 Electron/GUI
- 通过 ConnectRPC 向 LS 发送聊天请求，获取流式响应
- 集成到 sub2api 现有网关架构中
- 支持多账号（每账号一个 LS 实例）

## Acceptance Criteria
- [x] Antigravity 客户端成功安装并能运行
- [x] 列出 Antigravity 启动后开放的所有本地端口和服务
- [x] 分析请求头、UA、协议特征
- [x] 可行性评估完成，选定 Headless LS 代理方案
- [x] **PoC: headless LS 启动成功并监听端口**
- [x] **PoC: ConnectRPC 调用 StartCascade 成功**
- [x] **PoC: ConnectRPC 调用 SendUserCascadeMessage + GetCascadeTrajectory 能完成一次完整对话**（使用 standalone 模式 + DNS 劫持 + TCP relay 方案解决代理问题）
- [ ] 集成到 sub2api 网关（Phase 2）
- [ ] 多账号 LS 实例池（Phase 3）

## Definition of Done
- PoC 验证报告写入任务目录
- 关键发现更新到 PRD 的 Technical Notes 中
- PoC 代码可独立运行

## Out of Scope (本次不做)
- 生产环境部署方案
- 自动更新 LS 二进制版本
- Gemini CLI 平台集成

## Technical Approach

### 架构概览

```
用户请求 → sub2api Gateway → language_server (Go binary) → Google API
                  ↕ ConnectRPC/IPC                    ↕ 原生 gRPC+TLS
```

sub2api 不再直接向 Google 发 HTTP 请求。改为启动真实的 language_server 二进制，
通过 ConnectRPC 协议向 LS 发送 IPC 消息，LS 代替我们跟 Google 通信。

### 核心优势
1. **TLS/H2 指纹完全真实** — Go 的 `crypto/tls` 原生发出
2. **遥测自动处理** — LS 自己会发心跳、Clearcut、Firebase 等
3. **OAuth token 自动刷新** — LS 内置 Google OAuth2 client
4. **gRPC 协议完全兼容** — LS 自己序列化 protobuf
5. **资源开销可控** — 每个 LS 实例 ~50-100MB，无需 Electron

### Headless LS 启动要素（参考 zerogravity standalone/spawn.rs）

**启动参数**:
```
language_server_linux_x64 \
  --enable_lsp \
  --lsp_port={free_port_1} \
  --extension_server_port={stub_port} \
  --csrf_token={uuid} \
  --server_port={free_port_2} \
  --workspace_id=file_home_user_workspace \
  --cloud_code_endpoint=https://daily-cloudcode-pa.googleapis.com \
  --app_data_dir=antigravity \
  --gemini_dir={data_dir}/.gemini \
  --parent_pipe_path=/tmp/server_{hex}
```

**必须的环境变量**（模拟 Electron 宿主）:
```
ELECTRON_RUN_AS_NODE=1
VSCODE_PID={parent_pid}
VSCODE_CRASH_REPORTER_PROCESS_TYPE=extensionHost
ANTIGRAVITY_SENTRY_SAMPLE_RATE=0
ANTIGRAVITY_EDITOR_APP_ROOT={app_root}
SBX_CHROME_API_RQ=1
NO_PROXY=*.azureedge.net
```

**预置文件**:
- `~/.gemini/antigravity/user_settings.pb` = `[0x0a,0x06,0x0a,0x00,0x12,0x02,0x08,0x01]`
  （detectAndUseProxy = ENABLED）

**Stub Extension Server**:
- LS 启动后连接 extension_server_port，调用 `LanguageServerStarted`
- Stub 需接受 TCP 连接、回复 ConnectRPC 响应、注入 OAuth token
- 连接必须保持打开（断开 = LS 认为宿主死了）

### ConnectRPC 通信协议

**请求格式**:
```
POST https://127.0.0.1:{port}/exa.language_server_pb.LanguageServerService/{Method}
Content-Type: application/json  (或 application/proto)
x-codeium-csrf-token: {csrf}
Connect-Protocol-Version: 1
Origin: vscode-file://vscode-app
User-Agent: Mozilla/5.0 ... Chrome/142 ... Electron/39 ...
sec-ch-ua: "Not_A Brand";v="99", "Chromium";v="142"
... (共 14 个 Chrome 风格 headers)
```

**关键 RPC 方法**:
| 方法 | 用途 | 格式 |
|------|------|------|
| StartCascade | 创建对话 | JSON |
| SendUserCascadeMessage | 发送消息 | Proto |
| StreamCascadeReactiveUpdates | 流式获取结果 | Connect Streaming |
| GetCascadeTrajectorySteps | 获取步骤 | JSON |
| Heartbeat | 心跳 | JSON |
| GetUserStatus | 配额/封禁检查 | JSON |

**Connect Streaming 信封格式**:
```
[flags:1byte][length:4bytes_BE][payload]
flags=0x00: 数据帧
flags=0x02: 结束帧
```

### Warmup 序列（参考 zerogravity warmup.rs）

首次启动时发送 16 个 RPC，模拟真实 IDE 初始化：
1. RegisterGdmUser → SetBaseExperiments → GetUserStatus → InitializeCascadePanelState
2. Heartbeat → RecordEvent(LS_STARTUP) → GetStatus → GetCascadeModelConfigs
3. GetCascadeModelConfigData → GetWorkspaceInfos → GetWorkingDirectories
4. GetAllCascadeTrajectories → GetMcpServerStates → GetWebDocsOptions
5. GetRepoInfos → GetAllSkills

周期性 RPC：
- Heartbeat: 每 ~1s
- GetUserStatus: 每 ~60s
- GetCascadeModelConfigs: 每 ~120s
- RecordAsyncTelemetry: 每 ~60s（LS 自己处理）

### 实施路线

**Phase 1: PoC 验证**（当前）
1. 手动启动 headless LS
2. 写 stub extension server
3. 用 curl/Go 向 LS 发 ConnectRPC 请求
4. 验证 StartCascade + SendUserCascadeMessage + StreamCascadeReactiveUpdates

**Phase 2: sub2api 集成**
1. Go LS 进程管理器 + ConnectRPC 客户端
2. 对接网关 failover 逻辑
3. 响应格式转换（Gemini → Claude/OpenAI）

**Phase 3: 生产化**
1. LS 实例池 + 空闲回收
2. 错误处理 + 自动重启
3. 多账号管理

### 每账号资源消耗预估
| 资源 | 消耗 |
|------|------|
| language_server 进程 | ~50-100MB 内存 |
| stub extension server | ~1MB |
| parent pipe socket | 1 个 fd |
| 端口 | 2 个（server_port + lsp_port） |

---

## 历史研究记录

### 初始 Phase 1-4: 抓包逆向分析（已完成）

#### Phase 1: 环境准备
1. 通过 apt 安装 Antigravity 客户端
2. 安装抓包工具（mitmproxy + tshark）
3. Xvfb 虚拟显示

#### Phase 2: 抓包分析
1. 启动抓包，记录所有出站流量
2. 启动 Antigravity，观察初始化行为
3. Windows 登录后抓包

#### Phase 3: 逆向分析
1. 双层架构（Electron + Go binary）
2. language_server 使用 HTTP REST（非 gRPC）发送 AI 请求
3. Go 运行时支持 HTTPS_PROXY

#### Phase 4: 可行性评估
1. 结论：hook 可行，选定 Headless LS 代理方案（方案 C）

## Technical Notes
- sub2api 现有 Antigravity JA3: `1a28e69016765d92e3b381168d68922c`（模拟 Claude CLI Node.js 20.x）
- sub2api 现有 UA: `antigravity/{version} windows/amd64`（默认 1.19.6）
- 上游 API: `cloudcode-pa.googleapis.com/v1internal:streamGenerateContent`
- Antigravity 是 Electron 应用，安装后可能包含 Chromium 内核
- 已知内部端口：扩展服务器 53410、Chrome CDP 9222（来自安全研究文章）
- 当前环境：Linux 6.6.87.2-microsoft-standard-WSL2（WSL2）

### Phase 1+2 发现（2026-03-07）

**安装信息**:
- 包来源: `us-central1-apt.pkg.dev/projects/antigravity-auto-updater-dev/`
- deb 包版本: 1.20.3, 内部版本: 1.107.0
- Electron 39.2.3, Chromium 142.0.7444.175, Node.js 22.20.0
- 数据目录: `~/.antigravity`, 命令别名: `agy`
- 扩展市场: Open VSX (非 Microsoft)

**UA 对比**:
- 真实 Chromium NetworkService UA: `Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Antigravity/1.107.0 Chrome/142.0.7444.175 Electron/39.2.3 Safari/537.36`
- sub2api 模拟 UA (`antigravity/{version}`) 可能是 Node.js main process 层的，非 Chromium 层

**本地端口** (每次随机分配):
1. Browser Onboarding Server (HTTP, 提供登录页面)
2. CSRF 保护的 HTTP 服务 (可能是扩展服务器)
3. **Chrome DevTools MCP Server** (JSON-RPC + SSE, MCP v2024-11-05, 版本 0.12.1)

**网络流量** (未登录):
1. 自动更新: `antigravity-auto-updater-974169037036.us-central1.run.app`
2. Microsoft 1DS 遥测: `browser.events.data.microsoft.com` (apikey: "antigravity", 401 因无效 tenant token)
3. `cloudcode-pa.googleapis.com` 未出现 — 需要 OAuth 登录后才有

**gRPC/Connect 依赖**: `@connectrpc/connect-node`, `@exa/proto-ts`, `@exa/agent-ui-toolkit`

**初步可行性**: 中等偏高（标准 MCP/gRPC 协议，Node.js 可 hook，但需 GUI + 大量资源）

### Phase 3 发现（2026-03-07）- Windows 客户端登录后抓包

**双层架构**:
- Antigravity 有独立的 Go 二进制 `language_server_linux_x64` (185MB)
- Node.js 层发送管理 API (loadCodeAssist, fetchModels, fetchUserInfo, cascadeNuxes)
- ~~AI 核心请求走 gRPC~~ **修正：AI 请求也走 HTTP REST**（见 Phase 4 日志分析）

**真实 UA (Node.js 层)**:
- `antigravity/1.19.6 windows/amd64 google-api-nodejs-client/10.3.0`
- sub2api 缺少 `google-api-nodejs-client/10.3.0` 后缀

**缺少的 header**:
- `x-goog-api-client: gl-node/22.21.1`（sub2api 未发送）

**API 端点**:
- 生产: `cloudcode-pa.googleapis.com`
- 日常: `daily-cloudcode-pa.googleapis.com`
- 沙盒: `autopush-cloudcode-pa.sandbox.googleapis.com`

**可用模型列表** (17 个):
- Gemini 3/3.1 Flash/Pro (Low/High), Gemini 2.5 Flash/Pro
- Claude Opus 4.6 (Thinking), Claude Sonnet 4.6 (Thinking) — via ANTHROPIC_VERTEX
- GPT-OSS 120B (Medium) — via OPENAI_VERTEX
- 内部模型: chat_20706, chat_23310, tab_*

### Phase 4 发现（2026-03-07）- language_server 日志分析（重大修正）

**language_server 对外 API 使用 HTTP REST，不是 gRPC！**
- 日志: `http_helpers.go:123] URL: https://daily-cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse`
- 二进制中的 gRPC 方法（PredictionService/GenerateContent）仅用于内部 IPC

**Go 运行时确认尊重 HTTPS_PROXY**:
- 日志证实请求走了代理（`lh3.googleusercontent.com` 的 x509 CA 错误）
- 失败原因：mitmproxy CA 未安装到 Go 的 x509 信任链
- 需要安装到 Windows LocalMachine\Root（需管理员权限）

**账号已封禁**:
- 403 TOS_VIOLATION — 每 5 分钟重试 loadCodeAssist 全部失败

**可行性评估修正**: 中等偏高（上调回来）
- AI 请求使用 HTTP REST 与 sub2api 一致，无需协议转换
- Go 运行时已确认支持代理
- 主要障碍：Go x509 CA 信任 + 账号可用性

### Phase 5: Headless LS PoC 验证（2026-03-07）

**PoC 结果: 成功 ✓**

**启动流程确认**:
1. LS 必须从 **stdin 读取 init metadata protobuf**（之前遗漏了这一步导致启动失败）
2. Protobuf 字段: api_key(1), ide_name(3), ag_version(4), client_version(5), locale(6), session_id(10), editor_name(11), device_fingerprint(24), detect_proxy(34)
3. 写入 stdin 后关闭 stdin → LS 读取到 EOF 开始初始化
4. **parent pipe 必须保持连接打开**，否则 LS 检测到 EOF 立即 shutdown

**RPC 测试结果**:
| RPC 方法 | 状态 | 响应 |
|---------|------|------|
| Heartbeat | ✓ 200 | `{}` |
| GetCascadeModelConfigs | ✓ 200 | `{}` |
| **StartCascade** | **✓ 200** | **`{"cascadeId":"47101240-fefc-461e-b0d6-6e30816423fb"}`** |
| GetUserStatus | ✗ 超时 | 已不再是 stub 注入格式问题，更像是 LS 侧上游鉴权/网络等待 |

**LS 资源占用**: 启动 873ms, 保持运行稳定, 监听 2 端口 (42200 HTTPS, 42201 LSP)

**下一步**:
- 用安装包内嵌 proto 描述符校准 `SendUserCascadeMessageRequest` 字段
- 继续定位 `GetUserStatus` 超时原因（更可能是上游网络/鉴权，而非 extension stub）
- 继续定位 `reactive state is disabled` 的触发条件

### Phase 6: Proto 描述符校准与剩余阻塞（2026-03-07）

**本轮新结论**:

1. **`SendUserCascadeMessageRequest` 之前的字段布局猜错了**。
   - 通过解析安装包 `extensions/antigravity/dist/extension.js` 内嵌的 `FileDescriptorProto`，确认真实字段为：
     - `1`: `cascade_id`
     - `2`: `items`（`TextOrScopeItem`，不是旧猜测里的 `ChatMessage` 包装）
     - `3`: `metadata`
     - `5`: `cascade_config`
     - `11`: `client_type`（enum，IDE=1）
     - `18`: `message_origin`（enum，IDE=1）
2. **`cascade_config.planner_config` 必须显式设置模型**。
   - 真实必填点是：
     - `plan_model` (`CascadePlannerConfig.plan_model`)
     - `requested_model` (`CascadePlannerConfig.requested_model.model`)
   - 未设置时，LS 会立即报错：
     - `neither PlanModel nor RequestedModel specified`
3. **修正后的 proto 已经消除了“缺少模型”的 500 报错**。
   - 更新 PoC 后，`SendUserCascadeMessage` 不再出现该错误；
   - 取而代之的是 **请求进入等待态并触发真实上游连接**（观察到 LS 进程发起到 Google / 代理的外连 socket）。
4. **`GetUserStatus` 依然超时，且未命中 extension stub**。
   - 这说明当前阻塞点已从“stub token 注入格式”前移到 **LS → 上游服务** 这一段。
5. **`StreamCascadeReactiveUpdates` 仍返回 `reactive state is disabled`**。
   - 该日志在 LS 初始化阶段就已出现，说明它更像是当前 headless/LSP 模式下的运行时开关问题，而不是单次请求字段错误。

**当前判断**:
- stub 的 OAuth 注入已经足够让 LS 启动并完成基础 RPC；
- `SendUserCascadeMessage` 的本地 proto 结构现在已校准正确；
- 剩余主阻塞收敛为两类：
  1. **上游网络/鉴权链路**（`GetUserStatus`、`SendUserCascadeMessage` 超时）
  2. **reactive 运行时未启用**（`StreamCascadeReactiveUpdates`）

### Phase 7: Auth 链路与运行模式进一步验证（2026-03-07）

**本轮新增证据**:

1. **账号与上游 API 本身是可用的**。
   - 使用同一 `refresh_token` 直接向 Google `oauth2.googleapis.com/token` 刷新，成功拿到新的 `access_token`。
   - 使用新 token 直接请求：
     - `https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist`
     - `https://daily-cloudcode-pa.googleapis.com/v1internal:loadCodeAssist`
   - 两者都能在约 1.4~1.5 秒内返回 200，说明：
     - 当前账号并未被封禁；
     - 当前宿主 HTTP 代理链路可用；
     - `GetUserStatus` 超时问题不在 Google API 本身。

2. **新鲜 access token 也不能修复 headless LS 的鉴权相关 RPC**。
   - 用刚刷新出的新 token 重启 stub + LS 后：
     - `GetUserStatus` 仍然超时；
     - `RegisterGdmUser` 仍然超时；
     - `SendUserCascadeMessage` 仍然超时；
   - 这说明问题更像是 **LS 内部没有真正进入“已认证状态”**，而不是 access token 过期。

3. **`InitializeCascadePanelState` 在当前 headless LSP 模式下是 `501 unimplemented`**。
   - 本地调用返回：`unimplemented: unimplemented`
   - 这意味着真实扩展里用于驱动 Cascade Panel reactive state 的那条初始化路径，
     在当前 `--enable_lsp` headless 运行模式下并没有完整实现。

4. **`SaveOAuthTokenInfo` 只是 message schema，不是暴露出来的 RPC 方法**。
   - 之前尝试直接调用 `/LanguageServerService/SaveOAuthTokenInfo` 返回 404；
   - 重新检查 descriptor 后确认：它不是 service method，不能拿来绕过 stub。

5. **`persistent_mode` 不能启用 reactive**。
   - 额外测试 `--persistent_mode --enable_lsp` 后，启动日志仍然出现：
     - `cascade_manager.go:842] Reactive state is disabled`
   - 说明 reactive disabled 不是单纯因为 daemon/persistent 模式缺失。

**修正后的判断**:
- 目前卡点已经从“PoC proto 字段错误”进一步收敛为：
  1. **headless `--enable_lsp` 模式下，LS 的认证态建立不完整**（即使 stub 注入了 OAuth token，auth 相关 RPC 仍然超时）；
  2. **headless `--enable_lsp` 模式下，Cascade reactive panel 能力被编译或运行时关闭**（`InitializeCascadePanelState` unimplemented + `reactive state is disabled`）。

**对 Phase 1 的影响**:
- 当前 4 个子项里：
  - 1）手动启动 LS：已完成
  - 2）stub extension server：已完成
  - 3）ConnectRPC 基础请求：已完成
  - 4）`StartCascade + SendUserCascadeMessage + StreamCascadeReactiveUpdates`：**仍未完成**
- 未完成的根因已不再是简单 PoC bug，而更像是 **当前 headless LSP 路径的模式限制**。

### Phase 8: 真实宿主 Token 注入验证（2026-03-07）

**新增关键突破**:

1. **真实 Electron 宿主已可在当前容器内稳定拉起**。
   - 通过 `xvfb-run + dbus-run-session + --no-sandbox`，可以启动完整 Antigravity 宿主；
   - 它会拉起自己的 extension host、extension server 和 `language_server_linux_x64`；
   - 真实宿主 LS 启动参数与纯 headless PoC 明显不同：
     - 使用 `--random_port`
     - 带 `--extension_server_csrf_token`
     - 默认指向 `https://daily-cloudcode-pa.googleapis.com`

2. **真实宿主默认同样拿不到 OAuth token**。
   - 日志证实：
     - `Failed to get OAuth token: error getting token source from auth provider: state syncing error: key not found`
   - 根因不是 Electron 宿主缺失，而是 **本地 unified state / auth provider 状态未建立**。

3. **已逆向出真实宿主读取 OAuth 的本地存储 key 与结构**。
   - 主进程代码显示：
     - 存储 key: `antigravityUnifiedStateSync.oauthToken`
     - 主题名: `uss-oauth`
     - sentinel key: `oauthTokenInfoSentinelKey`
   - 存储格式不是普通 JSON 文件，而是：
     - 外层：`base64(proto Topic)`
     - 内层 `Row.value`：`base64(proto OAuthTokenInfo)`

4. **向真实宿主 user-data-dir 预注入 OAuth 状态后，token 缺失错误已消失**。
   - 重新启动真实宿主后：
     - 不再出现 `Failed to get OAuth token` / `state syncing error: key not found`
   - 说明方案 A 的“真实宿主驱动 + 预注入 OAuth 状态”路径是可行的。

5. **但 Phase 1 的最后一步仍未打通**。
   - 即便真实宿主已拿到 token：
     - `GetUserStatus` 仍超时
     - `GetProfileData` 仍超时
     - `SendUserCascadeMessage` 仍进入等待态但轨迹保持 `IDLE`
     - `InitializeCascadePanelState` 仍然 `501 unimplemented`
     - `StreamCascadeReactiveUpdates` 仍是 `reactive state is disabled`
   - 因此当前判断是：
     - **“真实宿主拿到 token” 已解决**
     - **“Cascade reactive / 对话执行链路” 仍受当前运行模式或调用前置条件限制**

**新增 PoC 脚本**:
- `poc/seed_real_host_oauth.py`
  - 复制一份独立 `user-data-dir`
  - 向 `User/globalStorage/state.vscdb` 注入 `antigravityUnifiedStateSync.oauthToken`
- `poc/launch_real_host.sh`
  - 用 `xvfb-run` 启动真实 Antigravity 宿主
  - 从日志 / 进程参数中提取真实 LS 的随机端口与 CSRF

**当前对方案 A 的结论**:
- 方案 A 比之前更进一步：
  - **已证明真实宿主可拉起**
  - **已证明真实宿主的 OAuth 状态可被外部预注入修复**
- 但要完成 Phase 1 最后一个验收项，仍需继续解决：
  1. 真实宿主下 `GetUserStatus` / `GetProfileData` 超时的直接原因
  2. 为什么 `InitializeCascadePanelState` 在当前路径下仍是 unimplemented
  3. 为什么 `SendUserCascadeMessage` 仍无法把轨迹推进出 `IDLE`

### Phase 9: Standalone Mode + 代理方案突破（2026-03-08）

**重大突破：完整对话已打通！**

#### 9.1 Standalone 模式发现
- LS 支持 `--standalone` 模式，**完全不需要 Electron 宿主或 Extension Server**
- 启动命令极简：`language_server_linux_x64 --standalone --app_data_dir=antigravity --server_port=44000 --cloud_code_endpoint=https://cloudcode-pa.googleapis.com`
- 初始化耗时 ~750ms（vs 之前 enable_lsp 模式需要 extension server）
- Token 存储路径：`~/.gemini/jetski-standalone-oauth-token`（Go oauth2.Token JSON 格式）
- 内嵌 OAuth client_secret：`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`

#### 9.2 网络代理问题与解决方案
**核心问题**：LS 内部有两种网络连接模式：
1. **HTTP REST**（loadCodeAssist、fetchUserInfo 等）→ 尊重 `HTTPS_PROXY` 环境变量
2. **gRPC**（ApiServerClientV2，cascade 执行的核心通道）→ **不走 HTTP 代理，直连 Google IP**

**尝试过的方案**：
| 方案 | 结果 | 原因 |
|------|------|------|
| `proxychains4` | 失败 | Go 静态链接，不走 libc 的 connect() |
| `HTTPS_PROXY` 环境变量 | REST 成功，gRPC 失败 | gRPC 库绕过 HTTP 代理 |
| `iptables + redsocks` | gRPC 连接成功但 REST EOF | redsocks 透明代理与 Go HTTP 客户端不兼容 |
| `graftcp` | 失败 | 缺少 graftcp-local 组件 |

**最终成功方案：DNS 劫持 + TCP Relay + HTTPS_PROXY**
```
1. /etc/hosts: cloudcode-pa.googleapis.com → 127.0.0.2
2. Python TCP Relay: 127.0.0.2:443 → SOCKS5 代理 → cloudcode-pa.googleapis.com:443
3. HTTPS_PROXY=http://127.0.0.1:7890 处理 REST 请求
```
- gRPC DNS 解析到 127.0.0.2 → TCP Relay 通过 SOCKS5 代理转发 → ESTAB 连接建立
- REST 通过 HTTPS_PROXY → 正常工作
- 两种连接同时成功

#### 9.3 完整对话验证
1. **GetCascadeModelConfigData** — 成功获取 6 个可用模型：
   - `MODEL_PLACEHOLDER_M37`: Gemini 3.1 Pro (High) — 默认
   - `MODEL_PLACEHOLDER_M36`: Gemini 3.1 Pro (Low)
   - `MODEL_PLACEHOLDER_M18`: Gemini 3 Flash
   - `MODEL_OPENAI_GPT_OSS_120B_MEDIUM`: GPT-OSS 120B (Medium)
   - `MODEL_PLACEHOLDER_M35`: Claude Sonnet 4.6 (Thinking)
   - `MODEL_PLACEHOLDER_M26`: Claude Opus 4.6 (Thinking)

2. **StartCascade** → cascadeId 正常返回
3. **SendUserCascadeMessage** → `{}` 空响应（成功）
4. **GetCascadeTrajectory** → 5 个步骤全部 DONE：
   - USER_INPUT → CONVERSATION_HISTORY → EPHEMERAL_MESSAGE → **PLANNER_RESPONSE** → CHECKPOINT
   - PLANNER_RESPONSE 包含完整的 AI 文本回复 + thinking 内容

5. **LS 日志确认 API 调用链**：
   ```
   planner_generator.go: Requesting planner with 6 chat messages
   http_helpers.go: URL: https://cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse
   ```

#### 9.4 生产化方案（Phase 2 参考）
DNS 劫持 + TCP Relay 方案适合 PoC，生产环境建议：
1. 使用 `iptables -t nat DNAT` 将 Google IP 段流量转发到 SOCKS5 代理
2. 或者在 LS 启动前用 unshare(CLONE_NEWNET) 创建独立网络命名空间
3. 或者修改 LS binary 的 gRPC transport 配置（需要 binary patching）
