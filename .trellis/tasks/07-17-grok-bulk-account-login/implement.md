# Grok 批量登录 Violentmonkey 脚本实施计划

## 实施步骤

- [x] 在 `tools/grok-login-userscript/` 创建独立领域目录，加入用户脚本、测试和中文 README。
- [x] 实现纯函数核心：输入解析、邮箱规范化、脱敏、状态迁移、Device Flow 错误分类和有限重试策略。
- [x] 实现 Violentmonkey API 适配层，统一共享值、跨域请求、标签控制、Cookie 管理和剪贴板错误处理。
- [x] 实现仅允许 HTTPS 的 `www.havefun.eu.cc` closed Shadow DOM 控制台、Web Locks 独占批次及控制命令。
- [x] 实现 xAI/Grok 隐藏登录驱动：页面分类、语义选择器、表单事件、Cloudflare/未知页面暂停和授权推进。
- [x] 实现官方 Device Flow 创建、轮询、取消和 refresh token 结果投影。
- [x] 实现带 `tab_marker`/`cleanup_id` 的逐域站点存储清理、domain Cookie 全量枚举、二次校验和清理失败阻断。
- [x] 实现停止、跳过、失败重试、标签手工关闭和过期共享任务的恢复/清理路径。
- [x] 修复停止/跳过、页面导航或 challenge 切换与 150ms 延迟提交之间的竞态：共享任务先移除密码并标记取消，驱动取消定时器且执行前重新校验任务归属、调度 URL 和非 challenge 页面状态。
- [x] 修复延迟守卫拒绝时提前消耗动作次数的问题，并覆盖同 URL challenge 完成后恢复提交且只执行一次的浏览器状态机测试。
- [x] 编写 Violentmonkey 安装、HttpOnly 权限、当前 Profile 退出风险、域名 TLS 前置条件和手工验收说明。
- [x] 添加纯逻辑、GM API 与 Node VM 浏览器状态机测试；所有夹具使用虚构邮箱、密码和 Token。

## 重点风险

- xAI 登录 DOM 无法从当前环境稳定抓取，选择器必须采用语义评分和未知页面停机，不能硬编码单一 class。
- Violentmonkey 共享值是持久存储，当前账号密码必须最小化停留时间并在所有终止路径删除。
- HttpOnly Cookie 清理依赖用户同时开启全局和脚本级权限；清理验证失败必须阻断队列。
- 控制台站点当前 TLS 与 nginx 配置异常；脚本只允许 HTTPS，真实浏览器验收前必须先修复证书和虚拟主机。
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
- 确认 `https://www.havefun.eu.cc/` 证书有效且页面正常加载；HTTP 页面不得出现控制台。
- 使用虚构/专用测试账号先跑单号，确认控制台显示、自动填表、CF 暂停和 Token 轮询状态。
- 验证成功后检查 refresh token 导出格式，并确认控制台、日志和共享值中不存在密码。
- 检查目标域 Cookie 二次枚举为空；故意关闭权限时队列必须在清理阶段停止。
- 使用两个测试账号串行运行，确认第二号不会继承第一号登录态。
- 手工关闭登录标签、点击停止、制造 Token 超时，确认轮询取消和敏感共享值删除。

## 回滚

- 禁用或删除 Violentmonkey 脚本即可回滚。
- 若脚本已开始批次，先点击停止并清空敏感数据，再禁用脚本。
- 本任务不修改数据库、Sub2API 配置或现网容器，无数据迁移回滚项。
