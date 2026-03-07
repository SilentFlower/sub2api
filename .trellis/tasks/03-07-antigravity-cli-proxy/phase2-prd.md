# Phase 2: LS 代理集成到 sub2api 网关

## Goal
将 PoC 验证通过的 headless Language Server 代理方案集成到 sub2api 网关，作为新的 Platform 类型与现有 Antigravity HTTP 直连路径并行共存。

## 架构决策（已确认）
1. **并行共存** — LS 代理作为新 Platform 类型 `antigravity_ls`，通过节点配置选择
2. **轮询转流式** — 后端轮询 `GetCascadeTrajectory`，实时以 SSE 推送给用户
3. **网络代理** — 保持 DNS 劫持 + TCP Relay 方案，由 Go 代码管理

## 系统架构

```
用户请求 (Claude API)
    ↓
[Gateway Handler] — Platform 分流
    ├── platform=antigravity     → AntigravityGatewayService (现有, HTTP 直连)
    └── platform=antigravity_ls  → AntigravityLSGatewayService (新增, LS 代理)
                                       ↓
                                   [LS Manager]
                                   获取/启动 LS 实例
                                       ↓
                                   [ConnectRPC Client]
                                   StartCascade → SendMessage → 轮询 Trajectory
                                       ↓
                                   [Trajectory → Claude SSE 转换]
                                   轮询结果实时转为 Claude 流式事件
                                       ↓
                                   返回用户
```

## 需要新增的组件

### 1. LS 进程管理器 (`backend/internal/pkg/antigravityls/manager.go`)
- 启动 standalone LS 进程（`--standalone --server_port=PORT --app_data_dir=DIR`）
- 每账号一个 LS 实例，隔离数据目录
- OAuth token 文件注入（写入 `{data_dir}/.gemini/jetski-standalone-oauth-token`）
- 健康检查（Heartbeat RPC）
- 进程崩溃检测和自动重启

### 2. TCP Relay（Go 版）(`backend/internal/pkg/antigravityls/relay.go`)
- Go 实现的 TCP Relay，替代 PoC 的 Python 脚本
- 监听 127.0.0.2:443，通过 SOCKS5 代理转发到 cloudcode-pa.googleapis.com
- 全局共享一个 relay 实例

### 3. ConnectRPC 客户端 (`backend/internal/pkg/antigravityls/client.go`)
- StartCascade → 获取 cascadeId
- SendUserCascadeMessage → 发送用户消息（含模型配置）
- GetCascadeTrajectory → 轮询对话步骤
- GetCascadeModelConfigData → 获取可用模型列表
- GetUserStatus → 检查配额/封禁状态
- 自签名 TLS 证书处理（LS 使用 localhost HTTPS）

### 4. 响应格式转换 (`backend/internal/pkg/antigravityls/transformer.go`)
- Cascade Trajectory Steps → Claude Messages API 格式
- 步骤映射：
  - PLANNER_RESPONSE.text → Claude text content block
  - PLANNER_RESPONSE.thinking → Claude thinking content block
  - PLANNER_RESPONSE.toolCalls → Claude tool_use content block
- 轮询间隔内增量检测 → SSE delta 事件

### 5. AntigravityLSGatewayService (`backend/internal/service/antigravity_ls_gateway_service.go`)
- 实现 Forward() 方法，对接 Handler 层
- 流程：接收 Claude 请求 → 获取 LS 实例 → ConnectRPC 调用 → 轮询转 SSE → 返回
- 复用现有的 TokenProvider（获取 refresh_token/access_token）
- 复用现有的模型映射逻辑（Claude 模型名 → LS 模型 key）

### 6. Platform 注册
- `domain/constants.go`: 新增 `PlatformAntigravityLS = "antigravity_ls"`
- `handler/gateway_handler.go`: 添加 LS 路径分流
- `service/wire.go`: 注册 AntigravityLSGatewayService
- Ent schema: Account/Group 的 Platform 枚举新增值

## 模型映射

| Claude 请求模型 | LS 模型 Key |
|----------------|------------|
| claude-opus-4-6-* | MODEL_PLACEHOLDER_M26 |
| claude-sonnet-4-6-* | MODEL_PLACEHOLDER_M35 |
| gemini-3.1-pro | MODEL_PLACEHOLDER_M37 |
| gemini-3-flash | MODEL_PLACEHOLDER_M18 |

## Acceptance Criteria
- [ ] 新增 `antigravity_ls` Platform 类型，可在管理面板选择
- [ ] LS Manager 能启动/停止/健康检查 LS 实例
- [ ] ConnectRPC 客户端能完成完整对话（Start → Send → Trajectory）
- [ ] 轮询转流式能正确输出 Claude SSE 事件
- [ ] Go TCP Relay 替代 Python 脚本
- [ ] 通过 Claude API (`/v1/messages`) 完成一次 LS 代理对话
- [ ] 编译通过、无 lint 错误

## Out of Scope (Phase 3)
- 多账号 LS 实例池管理
- LS 实例空闲回收
- LS 二进制自动更新
- Gemini API 格式入口的 LS 支持（先只支持 Claude API）
- OpenAI API 格式入口的 LS 支持

## 技术约束
- LS 二进制路径: `/usr/share/antigravity/resources/app/extensions/antigravity/bin/language_server_linux_x64`
- 每实例内存: ~150MB
- Token 文件路径: `{data_dir}/.gemini/jetski-standalone-oauth-token`
- LS 端口: 动态分配（避免冲突）
- DNS 劫持: 需要 root 权限修改 /etc/hosts（或使用 iptables）
- gRPC 轮询间隔: 建议 200-500ms（平衡延迟和负载）
