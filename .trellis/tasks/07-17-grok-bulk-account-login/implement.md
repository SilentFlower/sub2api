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
- [x] 修复邮箱登录入口选择问题：曾在 0.2.5 改为登录标签打开 `https://accounts.x.ai/sign-in` 并优先点击 `Login with email`；0.2.13/0.2.14 尝试改为先打开 `verification_uri_complete` 并前置提交设备码，但真实页面反馈表明该顺序仍会卡在授权链路。
- [x] 保留密码提交后的 Device Flow 授权能力：密码提交并删除共享密码后，才允许跳转官方验证页、填写 `user_code` 或点击授权。
- [x] 控制台 UI 改为默认右下角悬浮球，点击展开完整面板，标题栏可收起，避免长期遮挡 `www.havefun.eu.cc` 页面。
- [x] 修复 Cloudflare 成功后的等待竞态：看到 Turnstile/Cloudflare “成功”后先等待稳定窗口，再点击登录，避免验证结果尚未写入时卡住或过早提交。
- [x] 修复 Cloudflare 成功后登录按钮延迟就绪问题：稳定窗口扩大为约 5 秒，按钮仍 disabled 时不派发 Enter、不消耗动作次数，并按有限间隔复扫直到按钮可用后点击。
- [x] 修复密码已消费后页面仍停在登录表单的问题：如果 DOM 密码框仍有值且存在高置信度“登录”按钮，驱动会重按登录而不是进入未知页。
- [x] 修复 Cloudflare 消失后的页面恢复延迟问题：最近检测到 challenge 后，登录/授权控件尚未恢复时保留约 60 秒后置等待窗口，避免 12 秒普通未知页超时误报。
- [x] 修复登录成功后落到 `accounts.x.ai/account` 的中间态：删除共享密码并跳转官方 Device Flow 验证页，不点击账户页的 Email 设置按钮。
- [x] 修复 xAI 自然跳转链路的计时与状态错位：URL 变化时重置页面计时，`/account` 先等待自然跳转后兜底，密码提交后控制台进入“进入授权页”，设备码填入后进入“等待授权结果”。
- [x] 保留已有 Device Flow 标签兼容路径：任务已经携带 `user_code` 且标签位于可信 `/oauth2/device` 时，识别设备码输入框、URL 参数或页面预填码，并点击“继续”；低置信度页面仍回到 `sign-in` 或等待人工，不猜测点击。
- [x] 保留登录态 Device Flow 兼容路径：若 Device 页显示“退出登录”等登录态文案但共享密码仍残留，先删除密码、写入登录态确认并推进授权状态。
- [x] 保留 Device Flow 已预填设备码末步：当页面已经显示当前设备码时，即使没有可编辑输入框也能点击“继续”。
- [x] 修复 0.2.15 实测主流程：登录标签先打开 `https://accounts.x.ai/sign-in` 完成邮箱密码登录；驱动确认 `/account` 或登录态后写入 `authenticated_at`；控制台随后创建 Device Flow 并写入 `device_ready_at`；登录标签再跳转官方设备码页、点击“继续”和 Grok Build 授权页“允许”。已取消或过期任务不得在登录后继续创建 Device Flow。
- [x] 修复 0.2.16 `/account` 后不继续授权的问题：登录标签首次读到 `#grok-bulk-login=...` 时把随机 `tab_marker` 写入同标签 `sessionStorage`，xAI 跳转到 `https://accounts.x.ai/account` 后即使 URL hash / `window.name` 丢失，驱动仍能识别当前任务并写入 `authenticated_at`，触发控制台后续 Device Flow。
- [x] 修复 0.2.17 最终设备授权后过快清理的问题：设备码提交或 Grok Build 授权提交后，Token 成功时按最近提交时间补足约 5 秒稳定窗口；没有捕获提交事件时从 Token 成功后等待约 5 秒，再关闭登录标签和清理 Session。

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
- 点击 Cloudflare 后若页面显示“成功!”但仍停留在登录表单，确认脚本会等待约 5 秒后自动点击“登录”；如果 Cloudflare 消失后页面短暂空白或登录控件尚未恢复，脚本应继续等待约 60 秒并复扫，不应立刻显示“无法识别当前 xAI 页面”；如果第一次点击后仍停在已填写密码的登录表单，脚本应重按“登录”。
- 确认登录标签首先进入 `https://accounts.x.ai/sign-in`，而不是先进入 `verification_uri_complete`；脚本应自动填写邮箱和密码，Cloudflare/验证码仍由人工处理。
- 密码提交后如果页面进入 `https://accounts.x.ai/account`，脚本应显示“进入授权页”，不点击账户页 “Email” 设置按钮；即使地址栏已不带 `#grok-bulk-login=...`、`window.name` 也为空，脚本仍应通过同标签 `sessionStorage` 识别当前登录标签。此时控制台才创建 Device Flow 并把设备码写回共享任务。
- 控制台写入 Device Flow 后，登录标签应跳转到 `accounts.x.ai/oauth2/device` 或 xAI 返回的等价验证页；若显示设备码输入框，脚本应填入当前 `user_code` 并点击“继续”；若页面已预填当前设备码，脚本应直接点击“继续”。
- 若后续出现 “授权 Grok Build” / “Authorize Grok Build” 页面，脚本应点击“允许”/“Allow”，随后控制台进入 Token 轮询；如果 RT 很快返回，控制台应显示授权稳定等待并保留页面约 5 秒后再进入成功和清理。
- 验证成功后检查 refresh token 导出格式，并确认控制台、日志和共享值中不存在密码。
- 检查目标域 Cookie 二次枚举为空；故意关闭权限时队列必须在清理阶段停止。
- 使用两个测试账号串行运行，确认第二号不会继承第一号登录态。
- 若看到 `x.ai`、`accounts.x.ai` 或 `grok.com` 根路径后台页，URL 应带 `#grok-bulk-cleanup=...`；`auth.x.ai` 清理后台页应是 `https://auth.x.ai/oauth2/authorize#grok-bulk-cleanup=...`，不应再是 `https://auth.x.ai/#grok-bulk-cleanup=...`。带 `#grok-bulk-login=...` 的才是登录/授权标签。
- 手工关闭登录标签、点击停止、制造 Token 超时，确认轮询取消和敏感共享值删除。

## 回滚

- 禁用或删除 Violentmonkey 脚本即可回滚。
- 若脚本已开始批次，先点击停止并清空敏感数据，再禁用脚本。
- 本任务不修改数据库、Sub2API 配置或现网容器，无数据迁移回滚项。
