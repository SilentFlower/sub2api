# Grok 批量登录助手

这是一个面向 Violentmonkey（暴力猴）的独立用户脚本。它在 `www.havefun.eu.cc` 显示批量控制台，在 xAI/Grok 登录页面隐藏运行自动填表逻辑，通过 xAI 官方 Device Flow 生成 refresh token。

## 安装前提

1. 使用最新版 Violentmonkey。
2. 浏览器能够通过 `https://www.havefun.eu.cc/` 正常打开控制台页面；脚本不会在 HTTP 页面运行。
3. 当前该域名存在 TLS 证书和 nginx 虚拟主机问题，必须先修复证书与虚拟主机，使 HTTPS 页面正常加载，再安装和运行本脚本。
4. 运行会清除当前 Chrome Profile 的 xAI/Grok Cookie，并退出已有 Grok/xAI 登录。

## 安装

1. 打开 Violentmonkey 管理页面，创建新脚本。
2. 将 `grok-bulk-login.user.js` 的完整内容粘贴并保存。
3. 在 Violentmonkey 全局设置中开启 GM Cookie 的 HttpOnly Cookie 访问能力。
4. 打开该脚本的设置页，开启脚本级“允许访问 HTTP-only Cookie”。
5. 刷新 `https://www.havefun.eu.cc/`，页面右侧应出现“Grok 批量授权”控制台。

脚本会在开始批次前写入并删除一个短期 HttpOnly 探针 Cookie。探针失败时不会处理任何账号。

## 使用

在账号输入框按一行一个账号粘贴：

```text
first@example.com|ExamplePassword1
second@example.com|ExamplePassword2
```

然后：

1. 勾选权限与 Session 清理确认。
2. 点击“开始”。
3. 脚本会串行打开 xAI 登录标签并自动填写邮箱、密码和授权页面。
4. 出现 Cloudflare、验证码、2FA 或其它安全验证时，脚本会暂停自动点击，请在登录标签中手工完成。
5. 成功后控制台会收集 refresh token，并清除本账号的 xAI/Grok Session。
6. 全部完成后点击“复制 RT”，粘贴到 Sub2API 的 Grok Refresh Token 批量导入入口。

密码只按第一个 `|` 分隔，因此密码本身可以包含后续 `|`。
开始后输入框会立即清空，账号密码只保留在当前控制台标签的运行内存中。脚本只会驱动自己用随机标记打开的登录标签，不会操作此前已经打开的其它 xAI/Grok 标签。

## 安全边界

- 完整批次只保存在控制台页面内存中。
- 跨页面自动填表时，当前账号会短暂写入 Violentmonkey 共享值；密码提交后立即删除。
- 脚本不会把账号密码发送到 xAI 官方认证端点之外的任何服务。
- 控制台、错误列表、日志和导出结果不会显示明文密码。
- refresh token 只保留在控制台结果区，点击“清空敏感数据”后删除。
- Cloudflare、验证码、2FA 和异常登录验证不会被自动绕过。

## Session 清理

每个账号完成、失败、跳过或停止时，脚本会：

1. 关闭脚本打开的登录标签并删除当前账号共享任务。
2. 依次在后台打开带随机标记的 `x.ai`、`auth.x.ai`、`accounts.x.ai` 和 `grok.com` 清理标签。
3. 每个清理标签删除当前 origin 可访问的 localStorage、sessionStorage、IndexedDB、Cache Storage 和 Service Worker，并返回带域名的 ACK；后台标签可能短暂出现在标签栏。
4. 使用 `GM_cookie` 删除上述目标域 Cookie。
5. 再次枚举 Cookie；任一目标域存储未确认、Cookie 仍有残留或权限报错时停止队列，不处理下一个账号。

若 HttpOnly 权限未开启，脚本会在开始前停止。

## 常见问题

### 页面没有出现控制台

- 确认地址栏 host 是 `www.havefun.eu.cc`。
- 确认地址栏使用 HTTPS，脚本不会在 HTTP 页面启动。
- 检查域名证书和 nginx 配置，确保 HTTPS 页面实际加载成功；不要通过忽略证书错误继续运行。

### 自动填写没有继续

- 页面可能处于 Cloudflare 或未知安全验证，请手工完成。
- xAI 页面结构可能变化。脚本会选择暂停，不会对低置信度按钮进行猜测性点击。
- 返回控制台查看当前状态，必要时使用“跳过当前”或停止后重试失败项。

### 清理 Session 失败

- 检查 Violentmonkey 全局与脚本级 HttpOnly Cookie 权限是否同时开启。
- 检查是否有其它扩展或页面持续重新写入 xAI Cookie。
- 清理失败时不要强行继续，否则下一个账号可能继承上一个账号的登录态。

## 本地验证

```bash
node --check tools/grok-login-userscript/grok-bulk-login.user.js
node --test tools/grok-login-userscript/*.test.cjs
```

自动化测试全部使用虚构邮箱、密码和 Token。真实 xAI、Cloudflare 和 Cookie 权限流程必须在安装 Violentmonkey 的浏览器中验收。
