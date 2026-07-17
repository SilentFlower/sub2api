# Violentmonkey 与 xAI 认证证据

## xAI 官方认证

- OIDC Discovery：`https://auth.x.ai/.well-known/openid-configuration`。
- 2026-07-17 实测声明的 grant：`authorization_code`、`refresh_token`、`urn:ietf:params:oauth:grant-type:device_code`。
- 未声明 Resource Owner Password Credentials，因此不能把邮箱密码直接 POST 到 Token 端点。
- 授权页和 `accounts.x.ai` 对普通 HTTP 客户端返回 Cloudflare 403，支持使用真实浏览器并由用户处理 challenge 的结论。

## 项目现有能力

- `backend/internal/pkg/xai/oauth.go` 定义公开 xAI client ID、scope、授权端点、Token 端点和 CLI base URL。
- `backend/internal/pkg/xai/sso_device.go` 已实现 Device Flow 的 device code、verify/approve 和 Token 轮询规则，可作为协议证据。
- `frontend/src/components/account/CreateAccountModal.vue` 已实现 Grok refresh token 多行批量验证和逐个创建账号。
- `backend/internal/handler/admin/grok_oauth_handler.go` 已实现 SSO/refresh token 转换后的 Grok OAuth 账号创建，说明脚本只需交付 refresh token。

## Violentmonkey 官方能力

- 官方 API：`https://violentmonkey.github.io/api/gm/`。
- `GM_openInTab` 支持 `active`、`container`（Firefox）、`insert`、`pinned`，不提供 Tampermonkey 的 `incognito` 选项。
- `GM_setValue` / `GM_getValue` / `GM_addValueChangeListener` 可在匹配页面间共享当前任务与状态。
- `GM_xmlhttpRequest` 默认可携带 Cookie，并支持跨域认证请求；脚本需声明对应权限。
- 当前官方源码 `src/background/utils/cookies.js` 支持 `GM_cookie` 的 list/set/delete，并通过脚本匹配规则约束目标 URL。
- HttpOnly Cookie 访问要求脚本设置 `config.httpOnly` 且全局 `gmCookieHttpOnly` 开启；默认全局值为 false。
- 这意味着当前 Profile 内逐号清 session 可实现，但必须把权限设置作为强制运行前置条件，并在删除后再次枚举验证。

## 设计结论

- 采用官方 Device Flow，避免捕获 localhost PKCE 回调和保存裸授权码。
- 控制台只在 `www.havefun.eu.cc` 展示；xAI/Grok 页面只运行隐藏驱动。
- 自动填写账号密码，Cloudflare/验证码/2FA 不自动处理。
- 串行运行，每号完成后定向删除目标域 Cookie 和站点存储；清理失败立即停止。
- 输出一行一个 refresh token，复用现有 Sub2API 导入能力，不修改 Sub2API。
