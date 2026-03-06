# Antigravity 客户端 Hook 代理 - 可行性研究

## Goal
在 Linux 上安装 Google Antigravity 客户端，通过深度抓包和逆向分析，搞清楚它的内部通信机制（TLS 指纹、请求模式、内部端口/API），评估是否可以 hook 它来代理 sub2api 的 Antigravity 请求，从而降低被 Google 识别和封锁的风险。

## Requirements
- 在当前 Linux 服务器上安装 Antigravity 客户端
- 使用抓包工具（mitmproxy/tcpdump/tshark）捕获 Antigravity 的全部网络流量
- 分析 TLS 握手、JA3/JA4 指纹、HTTP/2 帧、请求头等特征
- 发现并记录 Antigravity 的内部端口和本地 API
- 对比 sub2api 现有模拟行为与真实客户端的差异
- 输出可行性评估报告

## Acceptance Criteria
- [ ] Antigravity 客户端成功安装并能运行
- [ ] 完成至少一次完整的请求流程抓包（OAuth 认证 + API 调用）
- [ ] 记录 TLS 指纹（JA3 hash）并与 sub2api 现有指纹对比
- [ ] 列出 Antigravity 启动后开放的所有本地端口和服务
- [ ] 分析请求头、UA、协议特征
- [ ] 输出可行性结论：hook 是否可行、推荐方案、预估工作量

## Definition of Done
- 分析报告写入任务目录
- 关键发现更新到 PRD 的 Technical Notes 中

## Out of Scope (本次不做)
- 实际实现 hook 代码
- 多账户实例管理
- Gemini CLI 平台集成
- fallback 机制设计
- 生产环境部署方案

## Technical Approach

### Phase 1: 环境准备
1. 通过 apt 安装 Antigravity 客户端
2. 安装抓包工具（mitmproxy + tshark）
3. 如需 GUI，安装 Xvfb（虚拟显示）

### Phase 2: 抓包分析
1. 启动抓包，记录所有出站流量
2. 启动 Antigravity，观察初始化行为
3. 如果可以登录，用测试账号完成 OAuth 流程并抓包
4. 触发一次 API 请求，完整记录请求/响应

### Phase 3: 逆向分析
1. 检查安装了哪些二进制文件（file、ldd、strings）
2. 扫描启动后开放的本地端口（ss/netstat）
3. 分析 TLS 指纹（JA3/JA4）
4. 检查是否有 gRPC、WebSocket、IPC 通道
5. 对比 sub2api 现有模拟 vs 真实客户端的差异

### Phase 4: 可行性评估
1. 评估 hook 难度和可行的 hook 点
2. 评估内存/资源开销
3. 评估多账户场景的可行性
4. 输出结论和推荐方案

## Technical Notes
- sub2api 现有 Antigravity JA3: `1a28e69016765d92e3b381168d68922c`（模拟 Claude CLI Node.js 20.x）
- sub2api 现有 UA: `antigravity/{version} windows/amd64`（默认 1.19.6）
- 上游 API: `cloudcode-pa.googleapis.com/v1internal:streamGenerateContent`
- Antigravity 是 Electron 应用，安装后可能包含 Chromium 内核
- 已知内部端口：扩展服务器 53410、Chrome CDP 9222（来自安全研究文章）
- 当前环境：Linux 6.6.87.2-microsoft-standard-WSL2（WSL2）
