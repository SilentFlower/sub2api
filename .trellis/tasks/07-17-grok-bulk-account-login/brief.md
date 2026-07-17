# Brief — Grok 批量账号接入与自动登录

## Goal

- 交付一个独立 Violentmonkey 用户脚本：在 `www.havefun.eu.cc` 展示批量控制台，逐号自动填写 Grok/xAI 账号密码，Cloudflare 由用户手工处理，通过官方 Device Flow 收集并导出 refresh token。

## Scope

- 在 `tools/grok-login-userscript/` 提供可直接安装的 `grok-bulk-login.user.js`、无真实凭据的自动化测试和中文 README。
- 解析多行 `邮箱|密码`，校验、去重并串行处理账号。
- 控制台仅在 `www.havefun.eu.cc` 的 HTTP/HTTPS 页面以可展开悬浮球显示；xAI/Grok 页面只在 HTTPS 下运行隐藏的自动填表、人工验证检测、授权推进和站点存储清理逻辑。
- 使用 xAI 官方 Device Flow 创建 device code、打开验证页、有限轮询 Token，并输出一行一个 refresh token。
- 提供开始、暂停、继续、跳过、停止、失败重试、状态列表、复制结果和清空敏感数据。
- 每号结束后使用 Violentmonkey `GM_cookie` 清除目标域 Cookie（含授权后的 HttpOnly Cookie）并二次校验；清理失败时阻断后续账号。
- 不修改 Sub2API 后端、前端、数据库、容器或现网配置。

## Non-Goals

- 不批量注册 Grok/xAI 账号。
- 不绕过 Cloudflare、验证码、2FA、异常登录验证或平台风控。
- 不持久保存邮箱密码，也不把凭据发送到非 xAI 官方端点。
- 不直接调用 Sub2API 管理 API 创建账号，不保存管理员认证信息。
- 不抓取 Grok 网页作为推理 API。

## Key Context

- xAI OIDC 不支持密码 grant；实现采用官方 Device Flow，不保存裸授权码。
- 普通 HTTP 客户端会被 Cloudflare 拦截，因此必须使用用户真实浏览器，challenge 只允许人工处理。
- Violentmonkey `GM_openInTab` 不提供主动无痕选项；脚本在当前 Chrome Profile 运行，清理会退出该 Profile 原有的 Grok/xAI 登录。
- HttpOnly Cookie 清理要求用户同时开启 Violentmonkey 全局和脚本级权限；权限缺失或二次枚举仍有 Cookie 时必须停机。
- Violentmonkey 共享值是持久存储，最多短暂保存当前账号；密码提交及所有终止路径都必须删除。
- xAI 页面 DOM 无法在当前环境稳定获取，自动化必须依赖语义选择器和低置信度停机，禁止猜测性点击。
- 当前 Sub2API 已支持 refresh token 多行导入；脚本只需输出 refresh token。
- `www.havefun.eu.cc` 当前 TLS 证书和 nginx 虚拟主机不匹配；用户明确要求使用 HTTP 控制台并接受对应风险，本任务不修改服务器配置。
- HTTP/HTTPS 控制台共用带竞争确认、心跳和过期时间的 Violentmonkey 共享租约锁，HTTPS 额外叠加 Web Locks；xAI Device Flow 和登录页面仍只允许 HTTPS。

## Acceptance

- 输入解析覆盖空行、非法行、重复邮箱和密码含额外 `|`，且不泄露密码。
- 控制台只在指定站点的 HTTP/HTTPS 页面显示；HTTP 模式必须提示凭据泄露风险，xAI/Grok 页面不显示脚本 UI 或明文凭据。
- Device Flow 能正确处理 pending、slow down、拒绝、过期、网络错误、超时和取消，并导出 refresh token。
- 模拟登录页能自动填入邮箱密码；Cloudflare、验证码、2FA 和未知页面只暂停等待人工处理。
- 单号失败不影响批次；但 session 清理失败必须阻断队列，防止账号串用。
- 停止、跳过、超时和登录标签被关闭时，轮询、当前密码和共享状态都被清理。
- 通过 `node --check`、Node 纯逻辑/GM API mock 测试和 `git diff --check`；真实 xAI/Cloudflare 流程提供手工验收步骤。
- 代码、测试、日志和文档不包含用户真实密码或完整 Token。

## Next Step

- 继续验证 0.2.9 Cloudflare 成功后 5 秒稳定等待、challenge 消失但页面未恢复时 60 秒后置等待、按钮可用后再提交、密码已消费但页面仍停在密码表单时重按登录、邮箱登录优先、未登录 Device 页回退和悬浮球 UI；检查通过后再进入 Phase 3.3/3.4。
