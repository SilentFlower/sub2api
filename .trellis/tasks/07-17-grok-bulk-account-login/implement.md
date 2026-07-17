# Grok 批量登录 Violentmonkey 脚本实施计划

## 实施步骤

- [x] 在 `tools/grok-login-userscript/` 创建独立领域目录，加入用户脚本、测试和中文 README。
- [x] 实现纯函数核心：输入解析、邮箱规范化、脱敏、状态迁移、Device Flow 错误分类和有限重试策略。
- [x] 实现 Violentmonkey API 适配层，统一共享值、跨域请求、标签控制、Cookie 管理和剪贴板错误处理。
- [x] 实现 `www.havefun.eu.cc` closed Shadow DOM 控制台、Web Locks 独占批次及控制命令。
- [x] 实现 xAI/Grok 隐藏登录驱动：页面分类、语义选择器、表单事件、Cloudflare/未知页面暂停和授权推进。
- [x] 实现官方 Device Flow 创建、轮询、取消和 refresh token 结果投影。
- [x] 实现带 `tab_marker`/`cleanup_id` 的逐域站点存储清理、domain Cookie 全量枚举、二次校验和清理失败阻断。
- [x] 实现停止、跳过、失败重试、标签手工关闭和过期共享任务的恢复/清理路径。
- [x] 修复停止/跳过、页面导航或 challenge 切换与 150ms 延迟提交之间的竞态：共享任务先移除密码并标记取消，驱动取消定时器且执行前重新校验任务归属、调度 URL 和非 challenge 页面状态。
- [x] 修复延迟守卫拒绝时提前消耗动作次数的问题，并覆盖同 URL challenge 完成后恢复提交且只执行一次的浏览器状态机测试。
- [x] 编写 Violentmonkey 安装、HttpOnly 权限、当前 Profile 退出风险、控制台协议风险和手工验收说明。
- [x] 添加纯逻辑、GM API 与 Node VM 浏览器状态机测试；所有夹具使用虚构邮箱、密码和 Token。
- [x] 允许 `http://www.havefun.eu.cc/*` 控制台运行，保留 xAI/Grok 驱动和 Device Flow 的 HTTPS 门禁。
- [x] 为 HTTP/HTTPS 增加跨协议 Violentmonkey 共享租约锁，HTTPS 叠加 Web Locks，并覆盖竞争失败、过期接管、续租、跨协议互斥和 owner 限定释放测试。
- [x] 在 HTTP 控制台、README 和手工验收中增加不可消除的网络注入与凭据泄露风险确认。
- [x] 将清空操作纳入控制台锁，页面卸载只清理自身 `run_id`，避免空闲标签破坏其它活动批次。
- [x] 修复 Violentmonkey 对 `http://www.havefun.eu.cc:8080/*` 的匹配问题，增加精确 `@include` 并补回归测试。
- [x] 修复 xAI 登录入口与 Device Flow 验证页混用问题：先打开 `accounts.x.ai` 登录入口，登录完成后再跳转官方 `verification_uri`；授权路径兼容当前 `/oauth2/device` 基础路径。
- [x] 修复清理标签与登录标签 marker 混用问题：清理标签改用 `#grok-bulk-cleanup=...`，登录/授权标签保留 `#grok-bulk-login=...`，并补清理请求拒绝登录 marker 的回归测试。
- [x] 修复 `auth.x.ai` 根路径清理承载页 404 问题：改用 `https://auth.x.ai/oauth2/authorize#grok-bulk-cleanup=...`，并补禁止 `https://auth.x.ai/` 进入 `storageUrls` 的回归测试。
- [x] 修复邮箱登录入口选择问题：登录标签打开 `https://accounts.x.ai/sign-in`，优先点击 `Login with email`，未提交密码前误入 `/oauth2/device` 时回到邮箱登录入口，不再提前填写设备码。
- [x] 保留密码提交后的 Device Flow 授权能力：密码提交并删除共享密码后，才允许跳转官方验证页、填写 `user_code` 或点击授权。
- [x] 控制台 UI 改为默认右下角悬浮球，点击展开完整面板，标题栏可收起，避免长期遮挡 `www.havefun.eu.cc` 页面。

## 重点风险

- xAI 登录 DOM 无法从当前环境稳定抓取，选择器必须采用语义评分和未知页面停机，不能硬编码单一 class。
- Violentmonkey 共享值是持久存储，当前账号密码必须最小化停留时间并在所有终止路径删除。
- HttpOnly Cookie 清理依赖用户同时开启全局和脚本级权限；清理验证失败必须阻断队列。
- 控制台站点当前 TLS 与 nginx 配置异常；HTTP 模式可绕过证书阻塞，但无法防止网络中间人或页面篡改窃取凭据。
- Violentmonkey 共享值不提供原子 compare-and-swap；租约锁通过竞争确认延迟、短周期心跳和 owner 限定释放降低并发风险，HTTPS 额外叠加 Web Locks 提供同源原子锁。
- 自动登录和账号频率可能触发上游风控，MVP 固定串行并设置账号间冷却，不增加并发。

## 验证命令

```bash
node --check tools/grok-login-userscript/grok-bulk-login.user.js
node --test tools/grok-login-userscript/*.test.cjs
git diff --check
```

如新增本目录专用 `package.json`，仅允许增加无外部依赖的测试脚本，不引入运行时依赖。

## 手工验收

- 在 Violentmonkey 开启全局和脚本级 HttpOnly Cookie 权限。
- 分别打开 `http://www.havefun.eu.cc/`、`http://www.havefun.eu.cc:8080/admin/accounts` 与证书有效的 `https://www.havefun.eu.cc/`，都应先显示右下角 “Grok” 悬浮球；点击后展开控制台，HTTP 页面必须显示额外风险提示并要求确认。
- 同时打开 HTTP/HTTP、HTTPS/HTTPS 和 HTTP/HTTPS 控制台，第二个批次必须被控制台锁拒绝；关闭首个页面并等待租约过期后可以恢复。
- 使用虚构/专用测试账号先跑单号，确认控制台显示、自动填表、CF 暂停和 Token 轮询状态。
- 确认登录标签首先进入 `https://accounts.x.ai/sign-in` 并选择 `Login with email`；若未提交密码前误入 `accounts.x.ai/oauth2/device`，脚本应回到邮箱登录入口，不应填写设备码。
- 密码提交后若进入 `accounts.x.ai/oauth2/device` 的中文 Device Sign-in 页，脚本才应通过附近文案识别设备码输入框并点击“继续”。
- 验证成功后检查 refresh token 导出格式，并确认控制台、日志和共享值中不存在密码。
- 检查目标域 Cookie 二次枚举为空；故意关闭权限时队列必须在清理阶段停止。
- 使用两个测试账号串行运行，确认第二号不会继承第一号登录态。
- 若看到 `x.ai`、`accounts.x.ai` 或 `grok.com` 根路径后台页，URL 应带 `#grok-bulk-cleanup=...`；`auth.x.ai` 清理后台页应是 `https://auth.x.ai/oauth2/authorize#grok-bulk-cleanup=...`，不应再是 `https://auth.x.ai/#grok-bulk-cleanup=...`。带 `#grok-bulk-login=...` 的才是登录/授权标签。
- 手工关闭登录标签、点击停止、制造 Token 超时，确认轮询取消和敏感共享值删除。

## 回滚

- 禁用或删除 Violentmonkey 脚本即可回滚。
- 若脚本已开始批次，先点击停止并清空敏感数据，再禁用脚本。
- 本任务不修改数据库、Sub2API 配置或现网容器，无数据迁移回滚项。
