# Antigravity 客户端安装与抓包分析报告

> 日期: 2026-03-07
> 环境: Ubuntu 24.04 LTS (WSL2), amd64
> Antigravity 版本: 1.20.3 (apt package), 内部版本 1.107.0

---

## 一、安装分析

### 1.1 安装方式

- **包名**: `antigravity`
- **来源**: Google Artifact Registry (`us-central1-apt.pkg.dev/projects/antigravity-auto-updater-dev/`)
- **安装大小**: ~786MB (deb 包 161MB)
- **架构**: amd64

### 1.2 文件结构

```
/usr/share/antigravity/
├── antigravity          # 主 Electron 二进制 (191MB, ELF 64-bit)
├── bin/antigravity      # shell 启动脚本
├── chrome-sandbox       # Chrome sandbox (SUID root)
├── chrome_crashpad_handler
├── libEGL.so, libGLESv2.so, libffmpeg.so  # 渲染库
├── libvk_swiftshader.so, libvulkan.so.1    # Vulkan/SwiftShader
├── resources/
│   ├── app/
│   │   ├── package.json      # name: "Antigravity", version: "1.107.0"
│   │   ├── product.json      # 配置文件（VS Code fork）
│   │   ├── out/main.js       # 主入口
│   │   ├── out/cli.js        # CLI 入口
│   │   └── node_modules.asar # Node.js 依赖
│   └── completions/          # Shell 自动补全
├── locales/                   # 多语言
├── icudtl.dat, resources.pak  # Chromium 资源
└── v8_context_snapshot.bin    # V8 快照
```

### 1.3 关键发现

| 项目 | 值 |
|------|-----|
| **本质** | VS Code 分支 (Electron 应用) |
| **Electron 版本** | 39.2.3 |
| **Chromium 版本** | 142.0.7444.175 |
| **Node.js 版本** | 22.20.0 |
| **数据目录** | `~/.antigravity` |
| **命令别名** | `agy` |
| **作者** | Google |
| **许可证** | MIT |
| **扩展市场** | Open VSX (`open-vsx.org`) |
| **核心扩展** | `google.antigravity` |
| **协议** | gRPC/Connect (`@connectrpc/connect-node`) |
| **Proto 包** | `@exa/proto-ts`, `@exa/agent-ui-toolkit` |
| **MCP OAuth** | Figma API 集成 (`clientId: WPjiIBOlI6Snc0EeAjsit7`) |
| **darwinBundleId** | `com.google.antigravity` |
| **URL Protocol** | `antigravity://` |

---

## 二、网络流量分析

### 2.1 抓包方法

由于 WSL2 环境中代理 (`127.0.0.1:7890`) 运行在 Windows 侧，使用 **mitmproxy upstream 模式** 进行中间人解密：

```
Antigravity → mitmproxy (8888) → 系统代理 (7890) → 外部网络
```

- `--ignore-certificate-errors` 绕过 Chromium TLS 证书校验
- `NODE_TLS_REJECT_UNAUTHORIZED=0` 绕过 Node.js TLS 校验

### 2.2 捕获的流量（未登录状态）

#### 请求 1: 自动更新检查

```
GET https://antigravity-auto-updater-974169037036.us-central1.run.app/api/update/linux-x64/stable/{commit_hash}/{package_hash}
```

- **User-Agent**: `Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Antigravity/1.107.0 Chrome/142.0.7444.175 Electron/39.2.3 Safari/537.36`
- **响应**: 204 No Content (已是最新版)
- **服务端**: Google Frontend
- **HTTP 版本**: HTTP/2.0

**关键参数**:
- 平台: `linux-x64`
- 通道: `stable`
- commit hash: `98d96766f768718ad6d450edb79e635b58373f9f`
- package hash: `5364aec18cac98a15b5b0db027b0a8e9689330e7bda8e39ba6f516c8fec385a7`

#### 请求 2: Microsoft 遥测 (1DS)

```
POST https://browser.events.data.microsoft.com/OneCollector/1.0/?cors=true&content-type=application/x-json-stream
```

- **apikey**: `antigravity`（用作 tenant token）
- **client-version**: `1DS-Web-JS-3.2.13`（Microsoft 1DS 遥测 SDK）
- **响应**: 401 Unauthorized (Invalid Tenant Token)
- **说明**: 继承自 VS Code 的遥测系统，Google 没有替换 API key

### 2.3 未登录状态下未观察到的流量

以下端点在未登录时**未出现**：
- `cloudcode-pa.googleapis.com` — AI API 端点
- `oauth2.googleapis.com` — OAuth 认证
- `www.googleapis.com/oauth2/v2/userinfo` — 用户信息
- 任何 gRPC/Connect 通信

---

## 三、本地端口分析

### 3.1 Antigravity 开放的端口

| 端口 | 进程 | 服务 | 详情 |
|------|------|------|------|
| **动态端口A** | 主进程 | Browser Onboarding Server | HTTP 服务，提供登录 onboarding 页面 |
| **动态端口B** | 子进程 | CSRF 保护的 HTTP 服务 | 可能是扩展服务器（PRD 提到的 53410） |
| **动态端口C** | 子进程 | **Chrome DevTools MCP Server** | JSON-RPC + SSE，MCP 协议 v2024-11-05 |

### 3.2 Chrome DevTools MCP Server 详细分析

```json
{
  "protocolVersion": "2024-11-05",
  "capabilities": {
    "logging": {},
    "tools": {"listChanged": true}
  },
  "serverInfo": {
    "name": "chrome_devtools",
    "title": "Chrome DevTools MCP server",
    "version": "0.12.1"
  }
}
```

- 使用 MCP (Model Context Protocol) 标准
- 支持 SSE 流式通信
- 端口每次启动随机分配
- 同一会话只允许一个 SSE 连接

### 3.3 language_server 端口

Antigravity 还启动了一个独立的 `language_server` 进程，开放了 3 个端口：
- 这可能是 AI Agent 的后端通信进程

---

## 四、User-Agent 对比

| 来源 | User-Agent |
|------|------------|
| **Antigravity 真实客户端 (Chromium)** | `Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Antigravity/1.107.0 Chrome/142.0.7444.175 Electron/39.2.3 Safari/537.36` |
| **sub2api 现有模拟** | `antigravity/1.19.6 windows/amd64` |

**关键差异**:
1. 真实客户端的 HTTP 流量使用 **标准 Chromium UA**，不是 `antigravity/x.x.x`
2. sub2api 模拟的 `antigravity/{version} {platform}` 格式可能是 **Node.js 进程内部的 UA**（非 NetworkService）
3. 真实客户端版本已升级到 `1.107.0`（内部）/ `1.20.3`（deb 包），sub2api 默认 `1.19.6`

---

## 五、与 PRD 已知信息的对比

| PRD 信息 | 实际发现 |
|----------|----------|
| JA3: `1a28e69016765d92e3b381168d68922c` | 需要登录后捕获到 googleapis.com 的直连流量才能对比 |
| UA: `antigravity/{version} windows/amd64` | HTTP 层用的是 Chromium UA；`antigravity/x.x.x` 可能在更上层（应用层/gRPC 头） |
| API: `cloudcode-pa.googleapis.com/v1internal:streamGenerateContent` | 未登录时不出现，需要 OAuth 登录后才能捕获 |
| 端口 53410 (扩展服务器) | 未固定端口，每次启动随机分配 |
| 端口 9222 (CDP) | 未检测到，可能需要 `--remote-debugging-port=9222` 显式启用 |
| Electron 应用 | 确认，使用 Electron 39.2.3 + Chromium 142 |

---

## 六、登录后抓包分析（Windows 客户端）

### 6.1 抓包方法

WSL2 上 GUI 渲染失败（黑屏），改用 Windows 侧的 Antigravity 客户端：

```powershell
# PowerShell 启动脚本
$env:NODE_TLS_REJECT_UNAUTHORIZED = "0"
$env:https_proxy = "http://localhost:8888"
$env:http_proxy = "http://localhost:8888"
$env:HTTP_PROXY = "http://localhost:8888"
$env:HTTPS_PROXY = "http://localhost:8888"
& "E:\Antigravity\Antigravity.exe" --proxy-server=http://localhost:8888 --ignore-certificate-errors
```

- env vars 让 Node.js 主进程的 HTTP 请求走代理
- `--proxy-server` 让 Chromium NetworkService 走代理
- `--ignore-certificate-errors` 绕过 Chromium 的证书校验

### 6.2 捕获的 API 流量（登录后）

共捕获 109 个请求，其中与 googleapis 相关的核心请求：

#### 6.2.1 cascadeNuxes — UI 提示信息

```
GET https://daily-cloudcode-pa.googleapis.com/v1internal/cascadeNuxes
```

**请求头**:
- `user-agent: antigravity/1.19.6 windows/amd64 google-api-nodejs-client/10.3.0`
- `x-goog-api-client: gl-node/22.21.1`
- `authorization: Bearer ya29.a0ATkoCc5EDdQ8...`
- `content-type: application/json`

**响应**: 包含 7 条 NUX（新用户体验）消息，包括 Gemini 3.1 Pro 公告、模型选择提示等。

#### 6.2.2 loadCodeAssist — 加载用户订阅信息

```
POST https://daily-cloudcode-pa.googleapis.com/v1internal:loadCodeAssist
```

**请求体**:
```json
{
  "metadata": {
    "ide_type": "ANTIGRAVITY",
    "ide_version": "1.19.6",
    "ide_name": "antigravity"
  }
}
```

**响应（关键字段）**:
```json
{
  "currentTier": {"id": "free-tier", "name": "Antigravity"},
  "allowedTiers": [
    {"id": "free-tier", "isDefault": true},
    {"id": "standard-tier", "userDefinedCloudaicompanionProject": true, "usesGcpTos": true}
  ],
  "cloudaicompanionProject": "driven-shuttle-ck9tz",
  "paidTier": {"id": "g1-ultra-tier", "name": "Google AI Ultra"}
}
```

#### 6.2.3 fetchAvailableModels — 获取可用模型列表

```
POST https://daily-cloudcode-pa.googleapis.com/v1internal:fetchAvailableModels
```

**请求体**: `{"project": "driven-shuttle-ck9tz"}`

**响应**: 共 17 个模型：

| 模型名 | 显示名 | Provider | 特性 |
|--------|--------|----------|------|
| gemini-3-flash | Gemini 3 Flash | GOOGLE_GEMINI | RECOMMENDED, THINKING, IMAGES |
| gemini-3-pro-low | Gemini 3 Pro (Low) | GOOGLE_GEMINI | RECOMMENDED, THINKING, IMAGES |
| gemini-3-pro-high | Gemini 3 Pro (High) | GOOGLE_GEMINI | RECOMMENDED, THINKING, IMAGES |
| gemini-3.1-pro-low | Gemini 3.1 Pro (Low) | GOOGLE_GEMINI | RECOMMENDED, THINKING, IMAGES |
| gemini-3.1-pro-high | Gemini 3.1 Pro (High) | GOOGLE_GEMINI | RECOMMENDED, THINKING, IMAGES |
| gemini-3.1-flash-image | Gemini 3.1 Flash Image | GOOGLE_GEMINI | |
| gemini-2.5-flash | Gemini 2.5 Flash | GOOGLE_GEMINI | RECOMMENDED, THINKING, IMAGES |
| gemini-2.5-flash-thinking | Gemini 2.5 Flash (Thinking) | GOOGLE_GEMINI | RECOMMENDED, THINKING, IMAGES |
| gemini-2.5-flash-lite | Gemini 2.5 Flash Lite | GOOGLE_GEMINI | |
| gemini-2.5-pro | Gemini 2.5 Pro | GOOGLE_GEMINI | RECOMMENDED, THINKING, IMAGES |
| claude-opus-4-6-thinking | Claude Opus 4.6 (Thinking) | ANTHROPIC_VERTEX | RECOMMENDED, THINKING, IMAGES |
| claude-sonnet-4-6 | Claude Sonnet 4.6 (Thinking) | ANTHROPIC_VERTEX | RECOMMENDED, THINKING, IMAGES |
| gpt-oss-120b-medium | GPT-OSS 120B (Medium) | OPENAI_VERTEX | RECOMMENDED, THINKING |
| chat_20706 | (internal) | INTERNAL | |
| chat_23310 | (internal) | INTERNAL | |
| tab_jump_flash_lite_preview | (internal) | GOOGLE_GEMINI | |
| tab_flash_lite_preview | (internal) | GOOGLE_GEMINI | |

#### 6.2.4 fetchUserInfo — 获取用户设置

```
POST https://daily-cloudcode-pa.googleapis.com/v1internal:fetchUserInfo
```
**请求体**: `{"project": "driven-shuttle-ck9tz"}`
**响应**: `{"userSettings": {"telemetryEnabled": true}, "regionCode": "US"}`

#### 6.2.5 OAuth userinfo

```
GET https://www.googleapis.com/oauth2/v2/userinfo
```
- 同样使用 `antigravity/1.19.6 windows/amd64 google-api-nodejs-client/10.3.0` UA
- 返回用户基本信息（email、name、picture）

#### 6.2.6 其他流量

| 类型 | 端点 | 说明 |
|------|------|------|
| 自动更新 | `antigravity-auto-updater-*.run.app` | 204 无更新 |
| 扩展更新 | `open-vsx.org/vscode/gallery/vscode/*/latest` | 15+ 个扩展检查 |
| Microsoft 遥测 | `browser.events.data.microsoft.com` | 401 无效 token |
| Google 遥测 | `play.googleapis.com/log` | 200 成功 |
| Feature Flags | `localhost:55068/proxy/unleash/client/features` | 502 无法连接 |
| 连通性检查 | `msftncsi.com/connecttest.txt` | 200 |
| Google 连通性 | `www.google.com/generate_204` | 204 |

### 6.3 关键缺失：streamGenerateContent 未被捕获！

**用户在 Agent Chat 中发送了消息，但 109 个请求中没有 `streamGenerateContent`。**

**原因分析**：

Antigravity 的 AI 请求不是由 Node.js 主进程发送的，而是由独立的 **`language_server`** 二进制发送：

```
/usr/share/antigravity/resources/app/extensions/antigravity/bin/language_server_linux_x64
```

- **类型**: Go 编译的 ELF 64-bit 二进制 (185MB)
- **通信协议**: 原生 gRPC (非 HTTP REST)
- **不走 HTTP 代理**: Go 的 gRPC 客户端默认不尊重 `https_proxy` 环境变量
- **直连 API**: 通过 Go 原生 HTTP/2 + TLS 直连 `cloudcode-pa.googleapis.com`

---

## 七、language_server 逆向分析

### 7.1 发现的 gRPC 服务和方法

从 `language_server_linux_x64` 二进制中提取的 gRPC 服务：

**AI 核心 API (cloudcode-pa.googleapis.com)**:

| 服务 | 方法 | 说明 |
|------|------|------|
| CloudCode | `GenerateCode` | 代码生成 |
| CloudCode | `CompleteCode` | 代码补全 |
| CloudCode | `GenerateChat` | 聊天生成（关键！sub2api 的目标） |
| CloudCode | `InternalAtomicAgenticChat` | Agent 模式聊天 |
| CloudCode | `SearchSnippets` | 代码片段搜索 |
| CloudCode | `LoadCodeAssist` | 加载 Code Assist |
| CloudCode | `OnboardUser` | 用户 onboarding |
| CloudCode | `RecordClientEvent` | 事件记录 |
| CloudCode | `RecordCodeAssistMetrics` | 指标记录 |
| CloudCode | `ListExperiments` | A/B 实验列表 |
| CloudCode | `ListModelConfigs` | 模型配置 |
| CloudCode | `ListAgents` | Agent 列表 |
| CloudCode | `FetchAdminControls` | 管理控制 |
| CloudCode | `FetchCodeCustomizationState` | 代码定制状态 |
| CloudCode | `ListCloudAICompanionProjects` | 项目列表 |
| CloudCode | `TransformCode` | 代码转换 |
| CloudCode | `MigrateDatabaseCode` | 数据库代码迁移 |
| CloudCode | `ListRemoteRepositories` | 远程仓库列表 |
| CloudCode | `RecordSmartchoicesFeedback` | 反馈记录 |
| CloudCode | `OnboardUserBackgroundTasks` | 后台任务 |
| CloudCode | `GetCodeAssistGlobalUserSetting` | 全局设置 |
| CloudCode | `SetCodeAssistGlobalUserSetting` | 设置全局 |
| **PredictionService** | **`GenerateContent`** | **AI 内容生成（核心！）** |
| PredictionService | `FetchAvailableModels` | 可用模型 |
| PredictionService | `CountTokens` | Token 计数 |
| PredictionService | `RetrieveUserQuota` | 用户配额 |
| JetskiService | `RewriteUri` | URI 重写 |
| JetskiService | `FetchUserInfo` | 用户信息 |
| JetskiService | `ListAgentPlugins` | Agent 插件列表 |
| JetskiService | `ListCascadeNuxes` | NUX 列表 |
| JetskiService | `SetUserSettings` | 用户设置 |
| JetskiService | `GetHealth` | 健康检查 |
| JetskiService | `CheckUrlDenylist` | URL 拒绝列表 |
| JetskiService | `RecordTrajectoryAnalytics` | 轨迹分析 |
| JetskiService | `ListWebDocsOptions` | 文档选项 |

**内部进程间通信 (gRPC)**:

| 服务 | 用途 |
|------|------|
| `LanguageServerService` | Electron 主进程 ↔ language_server 通信 |
| `ExtensionServerService` | 扩展服务器通信 |
| `ChatClientServerService` | 聊天客户端通信（`StartChatClientRequestStream`） |
| `SeatManagementService` | 用户/团队管理 |
| `CascadePluginsService` | 插件管理 |
| `AnalyticsService` | 分析服务 |
| `KnowledgeBaseService` | 知识库服务 |
| `IndexService` | 代码索引 |
| `ApiServerService` | 内部 API 服务器 |
| `UserAnalyticsService` | 用户分析 |
| `ModelManagementService` | 模型管理（`StartInferenceServer`） |

### 7.2 关键发现

1. **AI 请求通过原生 gRPC 发送**，使用 `PredictionService/GenerateContent`（非 REST `streamGenerateContent`）
2. **language_server 是 Go 二进制**，有自己的 TLS 栈，不受 Node.js 代理设置影响
3. **API 端点**：二进制中包含 3 个环境：
   - `https://cloudcode-pa.googleapis.com` (生产)
   - `https://daily-cloudcode-pa.googleapis.com` (日常)
   - `https://autopush-cloudcode-pa.sandbox.googleapis.com` (沙盒)
4. **UA 字符串**: 二进制中包含 `User-Agent` 和 `user-agent` 字段，但具体值需要通过流量捕获确认
5. **代理支持**: 二进制中包含 `http_proxy`、`https_proxy`、`no_proxy`、`HTTP_PROXY`、`HTTPS_PROXY`、`NO_PROXY` 字符串，说明 **Go 运行时支持环境变量代理**
6. **Google Cloud Auth**: 使用 `gl-go/` 前缀的 auth 客户端标识

### 7.3 Antigravity 架构图

```
┌─────────────────────────────────────────────────────────┐
│                    Antigravity Desktop                   │
│                                                         │
│  ┌──────────────────┐    ┌──────────────────────────┐   │
│  │  Electron Main   │    │   Chromium Renderer       │   │
│  │  (Node.js 22.20) │    │   (Chrome 142)            │   │
│  │                  │    │                            │   │
│  │  - out/main.js   │    │  - Agent Chat UI          │   │
│  │  - connect-node  │    │  - Editor                 │   │
│  │  - OAuth 管理     │    │  - 扩展 Host              │   │
│  │                  │    │                            │   │
│  │  HTTP 请求:       │    │  HTTP 请求 (NetworkSvc):  │   │
│  │  ├ loadCodeAssist │    │  ├ 自动更新               │   │
│  │  ├ fetchModels    │    │  ├ 扩展市场 (Open VSX)    │   │
│  │  ├ fetchUserInfo  │    │  └ MS 遥测               │   │
│  │  └ cascadeNuxes   │    │                            │   │
│  │                  │    │                            │   │
│  │  受 https_proxy   │    │  受 --proxy-server        │   │
│  │  环境变量影响 ✓    │    │  参数影响 ✓               │   │
│  └────────┬─────────┘    └──────────────────────────┘   │
│           │ gRPC (IPC)                                   │
│  ┌────────▼─────────────────────────────────────────┐   │
│  │           language_server (Go 二进制, 185MB)       │   │
│  │                                                   │   │
│  │  gRPC 直连 cloudcode-pa.googleapis.com:           │   │
│  │  ├ PredictionService/GenerateContent  ← AI 核心   │   │
│  │  ├ CloudCode/GenerateChat             ← 聊天      │   │
│  │  ├ CloudCode/InternalAtomicAgenticChat ← Agent    │   │
│  │  ├ CloudCode/GenerateCode             ← 代码生成  │   │
│  │  ├ CloudCode/CompleteCode             ← 补全      │   │
│  │  └ (更多 40+ 个 gRPC 方法)                        │   │
│  │                                                   │   │
│  │  受 https_proxy/HTTPS_PROXY 环境变量影响           │   │
│  │  （Go 运行时支持，但需要验证 gRPC 是否配置）       │   │
│  └───────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

---

## 八、language_server 日志分析（重大修正）

> 时间: 2026-03-07 第三次分析
> 日志路径: `AppData/Roaming/Antigravity/logs/20260307T062045/`

### 8.1 关键发现 — language_server 使用 REST 而非 gRPC！

**之前的推测是错误的。** language_server 的 AI 核心请求实际使用的是 **HTTP REST API**，而非原生 gRPC：

```
I0307 06:22:50.827093 31228 http_helpers.go:123] URL: https://daily-cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse Trace: 0x269e294f92a3e294
I0307 06:22:54.099979 31228 http_helpers.go:123] URL: https://daily-cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse Trace: 0x9edff7e00a229e14
```

**这意味着 language_server 使用的 API 路径和 sub2api 完全一致：`/v1internal:streamGenerateContent?alt=sse`**

二进制中发现的 gRPC 方法（`PredictionService/GenerateContent`）可能仅用于内部进程间通信（Electron ↔ language_server），而非对外 API 调用。

### 8.2 Go 运行时确实尊重 HTTPS_PROXY

日志证实 language_server 的 HTTP 请求**确实走了代理**，但 TLS 握手因 CA 证书不信任而失败：

```
Cache(peopleInfo): Singleflight refresh failed: failed to get profile picture:
Get "https://lh3.googleusercontent.com/a/ACg8ocIV1pO8VdHtzSIWg-MR7jcKZ11T2Y5SfSwB1Huf7z7XTRRIoQ=s96-c":
tls: failed to verify certificate: x509: certificate signed by unknown authority
```

**根因**：Go 的 `crypto/x509` 需要 CA 安装到**操作系统系统信任链**中（不是 Windows CurrentUser\Root，而是 LocalMachine\Root），或通过 `SSL_CERT_FILE` 精确指定——但后者对 Go 的 gRPC/HTTP2 传输不一定生效。

### 8.3 抓包期间为何 streamGenerateContent 未被捕获

时间线分析：
- `06:20` — language_server 启动，proxy 生效，但 x509 CA 验证失败
- `06:22` — 用户发送消息，language_server 发起 `streamGenerateContent` 请求
- `06:22` 的请求**可能因为 x509 CA 失败而直接断开**，或者走了代理但 mitmproxy 因为 gRPC/H2 模式问题未记录
- `11:41+` — loadCodeAssist 每 5 分钟重试，全部返回 `403 TOS_VIOLATION`

### 8.4 TLS 内部连接错误

language_server 对内部 gRPC IPC 也有 TLS 握手异常：

```
http: TLS handshake error from 127.0.0.1:59659: read tcp 127.0.0.1:55067->127.0.0.1:59659:
wsarecv: An established connection was aborted by the software in your host machine.
```

这些是 Electron 主进程 ↔ language_server 之间的本地 gRPC 连接被中断（可能因为 Electron 重启或连接超时）。

### 8.5 账号封禁状态

```json
{
  "error": {
    "code": 403,
    "message": "This service has been disabled in this account for violation of Terms of Service. Please submit an appeal to continue using this product.",
    "status": "PERMISSION_DENIED",
    "reason": "TOS_VIOLATION",
    "domain": "cloudcode-pa.googleapis.com"
  }
}
```

当前测试账号已被 Google 因 TOS 违规封禁，每 5 分钟重试 loadCodeAssist 全部返回 403。

---

## 九、sub2api 现有模拟 vs 真实客户端对比（修正版）

### 9.1 已确认的对比

| 特征 | sub2api 现有模拟 | 真实客户端 | 差异 |
|------|-----------------|-----------|------|
| **User-Agent (Node.js 层)** | `antigravity/1.19.6 windows/amd64` | `antigravity/1.19.6 windows/amd64 google-api-nodejs-client/10.3.0` | sub2api 缺少 `google-api-nodejs-client/10.3.0` 后缀 |
| **x-goog-api-client** | 未发送 | `gl-node/22.21.1` | sub2api 缺少此 header |
| **AI 请求协议** | HTTP REST (`/v1internal:streamGenerateContent?alt=sse`) | **也是 HTTP REST** (`/v1internal:streamGenerateContent?alt=sse`) | **一致！**（修正之前的错误推测） |
| **AI 请求发送者** | Node.js (sub2api Go 模拟 Node.js UA) | Go 二进制 (`language_server`) | 运行时不同，UA 可能不同 |
| **TLS 指纹 (JA3)** | `1a28e69016765d92e3b381168d68922c` (Node.js 模拟) | Go 的 crypto/tls 指纹 | 待通过捕获确认 |
| **deb 版本号** | 1.19.6 (默认) | 1.20.3 | 版本滞后 |
| **ide_version** | N/A | `1.19.6`（loadCodeAssist 请求体中） | sub2api 应该在请求体中设置 |

### 9.2 重大修正

**~~sub2api 使用 REST 而真实客户端使用 gRPC~~ — 错误！**

**日志证实：language_server 对外 API 请求也使用 HTTP REST (`/v1internal:streamGenerateContent?alt=sse`)。** sub2api 的协议模拟方向是正确的。

差异主要在于：
1. **UA 和 header 不完整**（缺少 `google-api-nodejs-client/10.3.0` 和 `x-goog-api-client`）
2. **TLS 指纹可能不同**（Go vs Node.js 的 crypto/tls 行为不同）
3. **language_server 的 UA 尚未确认**（需要成功捕获 streamGenerateContent 请求后才能看到完整 headers）

---

## 十、最终可行性评估（修正版）

### Hook 方案可行性: **中等偏高**（从"中等"上调回来）

**有利因素（修正后更多）**:
1. **language_server 使用 HTTP REST 而非 gRPC** — 与 sub2api 协议一致，不需要协议转换
2. language_server 的 Go 运行时**已确认支持 `HTTPS_PROXY`** 环境变量（日志证实）
3. Electron 应用的 Node.js 层已证明可以完全代理
4. 标准 HTTP/HTTPS 代理即可拦截全部流量

**剩余挑战**:
1. **Go x509 CA 信任** — 需要将 mitmproxy CA 正确安装到 Go 信任链（Windows LocalMachine\Root 或 Linux /etc/ssl/certs/）
2. **两层运行时** — 需要同时代理 Node.js (Electron) 和 Go (language_server) 的流量
3. **资源开销大** — 完整 Electron + Go 实例，预计 1-2GB 内存/实例
4. **账号已被封禁** — 当前测试账号不可用，需要新账号

**结论**:

Antigravity 客户端 hook 方案的可行性**比之前评估的更高**，因为 AI 核心请求实际使用 HTTP REST（与 sub2api 一致），而非原生 gRPC。主要障碍是 Go 的 TLS CA 信任问题和账号可用性。

### 推荐方案（优先级排序）

| 方案 | 可行性 | 复杂度 | 说明 |
|------|--------|--------|------|
| **A. 修复 CA 信任 + 完整捕获** | 高 | 低 | 将 mitmproxy CA 安装到系统根信任 (LocalMachine\Root)，重新抓包确认完整 headers |
| **B. sub2api UA/Header 修复** | 高 | 低 | 即使不完全 hook 客户端，先修复已知差异（UA 后缀、x-goog-api-client header） |
| **C. Token 中继** | 中 | 低 | 只从 Antigravity 客户端获取 OAuth token，API 请求仍由 sub2api 发送 |
| **D. 完整客户端 Hook** | 中偏高 | 中 | 在服务器上运行 headless Antigravity，代理所有请求 |

### 下一步建议

1. **修复 Go CA 信任**：以管理员权限将 mitmproxy CA 安装到 Windows `LocalMachine\Root`（而非 `CurrentUser\Root`）
2. **重新捕获 streamGenerateContent**：修复 CA 后，用新账号重新触发 AI 请求，完整捕获 language_server 的 headers/body
3. **修复 sub2api 已知差异**：添加 `google-api-nodejs-client/10.3.0` UA 后缀和 `x-goog-api-client: gl-node/22.21.1` header
4. **对比 TLS 指纹**：捕获 Go language_server 的 JA3 指纹，与 sub2api 现有指纹对比
