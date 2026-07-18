# Grok 批量登录助手

这是一个面向 Violentmonkey（暴力猴）的独立用户脚本。它在 `www.havefun.eu.cc` 显示批量控制台，在 xAI/Grok 登录页面隐藏运行自动填表逻辑，通过 xAI 官方 Device Flow 生成 refresh token。

## 安装前提

1. 使用最新版 Violentmonkey。
2. 浏览器能够通过 `http://www.havefun.eu.cc/`、`http://www.havefun.eu.cc:8080/` 或 `https://www.havefun.eu.cc/` 打开控制台页面。
3. HTTP 模式不能防止网络中间人、代理或被篡改页面窃取账号密码和 refresh token，只应在可信网络和服务器上使用；具备有效证书时仍建议改用 HTTPS。
4. 运行会清除当前 Chrome Profile 的 xAI/Grok Cookie，并退出已有 Grok/xAI 登录。

## 安装

1. 打开 Violentmonkey 管理页面，创建新脚本。
2. 将 `grok-bulk-login.user.js` 的完整内容粘贴并保存。
3. 在 Violentmonkey 全局设置中开启 GM Cookie 的 HttpOnly Cookie 访问能力。
4. 打开该脚本的设置页，开启脚本级“允许访问 HTTP-only Cookie”。
5. 刷新 `http://www.havefun.eu.cc/`、`http://www.havefun.eu.cc:8080/admin/accounts` 或 `https://www.havefun.eu.cc/`，页面右下角应出现 “Grok” 悬浮球；点击后展开“Grok 批量授权”控制台。

脚本会在开始批次前写入并删除一个短期 HttpOnly 探针 Cookie。探针失败时不会处理任何账号。

## 使用

在账号输入框按一行一个账号粘贴：

```text
first@example.com|ExamplePassword1
second@example.com|ExamplePassword2
```

然后：

1. 勾选页面协议风险、权限与 Session 清理确认；HTTP 页面会显示额外的红色风险提示。
2. 点击“开始”。
3. 脚本会先打开 `https://accounts.x.ai/sign-in`，自动填写邮箱和密码；如果页面要求 Cloudflare、验证码、2FA 或其它安全验证，脚本只等待人工处理。
4. 登录驱动确认已经进入 xAI 登录态后，控制台才调用官方 Device Flow 创建设备授权任务，并把当前 `user_code` 和可信验证页写回同一个登录标签。脚本会把登录标签的随机标记额外保存到本标签 `sessionStorage`，用于 xAI 跳转到 `/account` 后 URL hash 或 `window.name` 被清掉时继续识别同一个标签；这里不保存邮箱、密码或 token。
5. 登录标签随后跳转到官方 Device Flow 验证页：设备码页会填入或确认当前 `user_code` 并点击“继续”，授权页识别 “授权 Grok Build” / “Authorize Grok Build” 后点击“允许”/“Allow”。最终设备码或授权提交后，脚本会让授权页至少保留约 5 秒再进入成功清理，避免 xAI 页面刚完成授权就被立刻关闭。
6. Cloudflare 显示“成功”后脚本会等待约 5 秒让 Turnstile token 回写，登录按钮可用后再点击登录；如果 challenge 消失但登录页还没恢复，脚本会继续有限等待约 60 秒，避免过早显示未知页面。
7. 稳定窗口结束后控制台会保留 refresh token，并清除本账号的 xAI/Grok Session。
8. 全部完成后点击“复制 RT”，粘贴到 Sub2API 的 Grok Refresh Token 批量导入入口。

密码只按第一个 `|` 分隔，因此密码本身可以包含后续 `|`。
开始后输入框会立即清空，账号密码只保留在当前控制台标签的运行内存中。脚本只会驱动自己用随机标记打开的登录标签，不会操作此前已经打开的其它 xAI/Grok 标签。

## 安全边界

- 完整批次只保存在控制台页面内存中。
- 跨页面自动填表时，当前账号会短暂写入 Violentmonkey 共享值；密码提交后立即删除。
- 脚本不会把账号密码发送到 xAI 官方认证端点之外的任何服务。
- 控制台、错误列表、日志和导出结果不会显示明文密码。
- refresh token 只保留在控制台结果区，点击“清空敏感数据”后删除。
- Cloudflare、验证码、2FA 和异常登录验证不会被自动绕过。
- HTTP 控制台下，closed Shadow DOM 和 Violentmonkey 隔离不能替代 TLS，也不能阻止网络层注入或全局键盘事件监听。
- xAI 登录页、Device Flow 和 Token 请求始终使用受信任的 HTTPS 地址，不接受 HTTP xAI 验证地址。
- 控制台默认收起为右下角悬浮球，展开后可用标题栏“收起”按钮恢复，避免长期遮挡 `www.havefun.eu.cc` 页面。

## 控制台独占锁

- HTTP 和 HTTPS 页面共用带随机 owner、竞争确认、心跳和 60 秒过期时间的 Violentmonkey 共享租约锁，确保两个协议之间也能互斥。
- HTTPS 页面还会叠加 Chrome Web Locks，为同源标签提供原子互斥。
- 第二个控制台检测到未获得 Web Lock 或未过期共享租约时不会处理账号；控制台异常关闭后，最多等待租约过期即可重新开始。
- 共享租约是 HTTP 兼容降级，Violentmonkey 共享值本身不提供原子 compare-and-swap；不要在多个标签中同时点击开始。

## Session 清理

每个账号完成、失败、跳过或停止时，脚本会：

1. 关闭脚本打开的登录标签并删除当前账号共享任务。
2. 依次在后台打开带随机标记的 `x.ai`、`auth.x.ai`、`accounts.x.ai` 和 `grok.com` 清理标签；其中 `auth.x.ai` 使用 `https://auth.x.ai/oauth2/authorize` 作为同源承载页，不使用会在 Chrome 中显示 404 的根路径。
3. 每个清理标签删除当前 origin 可访问的 localStorage、sessionStorage、IndexedDB、Cache Storage 和 Service Worker，并返回带域名的 ACK；后台标签可能短暂出现在标签栏，也可能显示 403/404/Cloudflare 页面，这不是登录页失败。清理标签 URL 会带 `#grok-bulk-cleanup=...`，登录/授权标签才会带 `#grok-bulk-login=...`。
4. 使用 `GM_cookie` 删除上述目标域 Cookie。
5. 再次枚举 Cookie；任一目标域存储未确认、Cookie 仍有残留或权限报错时停止队列，不处理下一个账号。

若 HttpOnly 权限未开启，脚本会在开始前停止。

## 常见问题

### 页面没有出现控制台

- 确认地址栏 host 是 `www.havefun.eu.cc`。
- 确认地址栏协议是 HTTP 或 HTTPS；其它协议不会启动控制台。若地址是 `http://www.havefun.eu.cc:8080/admin/accounts`，脚本 `0.2.17` 已内置精确 include。
- 若仍看到 `https://auth.x.ai/#grok-bulk-cleanup=...`，说明浏览器里还是旧版脚本；`0.2.17` 应显示 `https://auth.x.ai/oauth2/authorize#grok-bulk-cleanup=...`。
- 若使用 HTTPS，证书必须匹配该域名；不要通过忽略证书错误继续运行，证书异常时可按风险提示改用 HTTP。

### 自动填写没有继续

- 页面可能处于 Cloudflare 或未知安全验证，请手工完成。
- 如果 Cloudflare 已显示“成功!”但没有立刻点击登录，等待约 5 秒；脚本 `0.2.17` 会等验证结果稳定且登录按钮可用后继续提交。
- 如果 Cloudflare 消失后页面短暂空白、加载中或登录控件还没恢复，脚本 `0.2.17` 会继续等待约 60 秒并复扫，不会在原 12 秒未知页超时时立即报“无法识别当前 xAI 页面”。
- 如果控制台显示“无法识别当前 xAI 页面”，但登录标签还停在已填写密码的“使用您的邮箱登录”页，说明旧版可能已删除共享密码但第一次点击没有跳转；`0.2.17` 会在密码框仍有值时重按“登录”。
- xAI 页面结构可能变化。脚本会选择暂停，不会对低置信度按钮进行猜测性点击。
- 若密码提交后页面进入 `https://accounts.x.ai/account`，脚本 `0.2.17` 会标记已登录；控制台随后创建 Device Flow，并让该登录标签跳转官方验证页，不点击账户页上的 Email 等设置按钮。若已经进入 `/account` 但控制台仍停在“填写密码”或没有创建 Device Flow，通常是浏览器里仍安装旧版，旧版可能因跳转丢失 URL hash / `window.name` 后不再认为这是当前批次标签。
- 若页面进入 `accounts.x.ai/oauth2/device` 且设备码已填好但只差点“继续”，脚本 `0.2.17` 会直接点击“继续”；如果后续出现 “授权 Grok Build” 页面，会点击“允许”。最终提交后如果 RT 很快返回，控制台会显示“授权已提交，等待 xAI 页面稳定 ... 秒”，等满约 5 秒后再清理 Session。
- 若控制台一直停在“进入授权页”，检查登录标签是否已显示设备码页或授权页；旧版脚本可能在登录前就打开 Device Flow，`0.2.17` 的主流程应先登录，再创建设备授权。
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
