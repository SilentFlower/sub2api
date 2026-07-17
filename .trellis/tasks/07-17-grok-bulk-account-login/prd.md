# Grok 批量登录 Violentmonkey 脚本

## 目标

提供一个独立的 Violentmonkey（暴力猴）用户脚本。管理员在 `www.havefun.eu.cc` 页面粘贴多行 `邮箱|密码` 后，脚本逐号打开真实 xAI 登录页、自动填写账号密码、等待用户手工处理 Cloudflare 验证，并通过 xAI 官方 Device Flow 收集 refresh token。最终结果按“一行一个 refresh token”导出，供现有 Sub2API Grok Refresh Token 批量导入功能使用。

## 已确认事实

- 输入中的密码是 Grok/xAI 登录密码，不是 Hotmail/Outlook 邮箱密码。
- 用户使用 Violentmonkey，不使用 Tampermonkey 或独立 Chrome 扩展。
- 批量控制台只在 `www.havefun.eu.cc` 显示；同一脚本会在必要的 xAI/Grok 域名执行不可见的自动填表和清理逻辑。
- 账号预计没有 2FA；Cloudflare、验证码、异常登录验证仍必须停下并由用户手工处理。
- 每个账号结束后必须清除 xAI/Grok 登录 session，再处理下一个账号。
- 用户接受在 Violentmonkey 全局设置和脚本设置中开启 HttpOnly Cookie 访问权限。
- xAI OIDC Discovery 只支持 `authorization_code`、`refresh_token` 和 Device Code grant，不支持邮箱密码直接交换 Token。
- 普通 HTTP 客户端访问 xAI 授权页会被 Cloudflare 拦截，因此服务端直登和无头 HTTP 登录不作为实现路径。
- 当前 Sub2API 已支持 Grok refresh token 多行批量验证和逐个创建账号，脚本无需接入 Sub2API 管理 API。
- 当前现网镜像为 `build-83a472f`。数据库中已有 15 个 Grok OAuth 账号，其中 13 个 `active`、2 个 `error`；另有 2 个 `active` 账号仍不可调度且临时错误截止时间已过。
- `www.havefun.eu.cc` 当前 TLS 证书主机名不匹配，但用户日常通过 HTTP 打开该页面，并明确要求控制台支持 HTTP。
- 用户接受 HTTP 页面不能抵御网络中间人或页面内容被篡改的风险；脚本必须在开始批次前显示醒目提示并要求显式确认，不能把该风险描述为已消除。

## 需求

### R1 批量输入

- 接受多行 `邮箱|密码` 文本，一行一个账号。
- 忽略空行并去除每行两端空白。
- 只按第一个 `|` 分隔邮箱和密码，密码中的后续 `|` 必须原样保留。
- 邮箱为空、密码为空或缺少分隔符时，显示行号和非敏感错误，不进入执行队列。
- 同一批次按规范化邮箱去重，重复行必须显示明确状态。

### R2 控制台

- 只在 `http://www.havefun.eu.cc/*` 和 `https://www.havefun.eu.cc/*` 渲染控制台；即使用户手工扩大用户脚本匹配范围，运行时也必须拒绝其它 host 和协议。
- HTTP 控制台必须醒目提示账号密码和 refresh token 可能被网络中间人或被篡改页面窃取，并把风险确认纳入开始和重试门禁。
- 控制台包含账号输入、开始、暂停、继续、跳过、停止、重试失败项、当前状态、逐号结果、复制 refresh token 和清空敏感数据。
- 密码字段不得出现在状态表、错误详情、浏览器控制台或导出内容中。
- 同一时刻只允许一个账号进入登录、人工接管或 Token 轮询状态。

### R3 xAI Device Flow

- 控制台通过 `GM_xmlhttpRequest` 调用官方 `https://auth.x.ai/oauth2/device/code`，使用项目现有公开 client ID 和 Grok CLI scope。
- 脚本优先使用返回的 `verification_uri`，缺失时回退 `verification_uri_complete`，并按服务端返回的 `interval` 轮询 `https://auth.x.ai/oauth2/token`。
- 正确处理 `authorization_pending`、`slow_down`、`access_denied`、`expired_token`、网络错误和超时。
- 成功响应只保留 refresh token 作为批量导出结果；access token、ID token 和 device code 在当前账号完成后删除。
- 不保存或输出裸授权码，不调用 Sub2API 管理 API。

### R4 xAI 页面自动化

- 脚本在 `auth.x.ai`、`accounts.x.ai`、`x.ai` 和 `grok.com` 的必要登录页面运行，但不渲染控制台。
- 根据语义属性优先识别邮箱输入框、密码输入框和继续/登录/授权按钮，例如 `type`、`name`、`autocomplete`、`aria-label` 和可见文本；不得只依赖单一易变 CSS class。
- 自动填写当前账号邮箱和密码，并派发页面框架可识别的 `input`、`change` 和必要的键盘事件。
- 不自动点击 Cloudflare challenge、验证码或未知安全确认。检测到这些页面时，状态切换为“等待人工验证”。
- 用户处理完成且页面恢复为可识别登录/授权页面后，脚本自动继续。
- 页面结构无法识别时暂停并允许用户跳过或重试，不进行猜测性点击。

### R5 队列与状态

- 账号状态至少包括：`pending`、`requesting_device`、`opening_login`、`filling_email`、`filling_password`、`waiting_human`、`polling_token`、`success`、`failed`、`cleaning`、`skipped`。
- 单号失败不得终止整个批次；清理成功后继续下一号。
- 用户暂停时不得启动新账号，但当前 Cloudflare/登录页面保持可人工处理。
- 用户停止时终止轮询、关闭脚本打开的登录标签、清理当前账号临时数据，并保留此前成功的 refresh token 供复制。
- 所有超时、重试和延迟都必须有上限，禁止无限轮询或无限自动点击。

### R6 Session 清理

- 使用 Violentmonkey `GM_cookie` 定向列出并删除 `x.ai`、`auth.x.ai`、`accounts.x.ai` 和 `grok.com` 的 Cookie，包括启用权限后的 HttpOnly Cookie。
- 在已打开的目标域页面清除 localStorage、sessionStorage、IndexedDB、Cache Storage，并注销可访问的 Service Worker。
- 清理后再次检查目标域 Cookie；仍有 Cookie、API 报错或权限未开启时，状态保持 `cleaning`/`failed` 并停止队列，不能继续下一号。
- 清理范围只能覆盖 xAI/Grok 目标域，不得清除其它网站数据。
- 在当前 Chrome Profile 执行清理会退出该 Profile 中原有的 Grok/xAI 登录，控制台启动前必须明确提示。

### R7 凭据安全

- 完整批次账号密码只保存在控制台页面内存中。
- 为跨域自动填表，最多只把当前账号短暂写入 Violentmonkey 共享值；密码提交后立即删除，并在成功、失败、取消、停止和超时时兜底删除。
- 不使用远程脚本依赖，不把凭据发送到 xAI 官方认证端点之外的任何服务。
- 不在日志、通知、DOM 属性、URL、查询参数、location hash 或下载文件中写入明文密码。
- refresh token 仅在结果区保留到用户复制或点击清空；不得自动同步到云端或 Sub2API。
- HTTP 控制台属于用户明确接受的降级模式；closed Shadow DOM 和 Violentmonkey 隔离不能替代 TLS，不得宣称能防止网络层注入或全局键盘事件监听。

### R8 兼容与安装

- 以当前 Violentmonkey 官方 API 为准，声明所需 `@match`、`@grant` 和 `@connect`。
- 安装说明必须包含开启全局及脚本级 HttpOnly Cookie 权限的步骤。
- HTTP/HTTPS 控制台都必须持有同一个 Violentmonkey 共享租约，提供跨协议互斥；HTTPS 安全上下文还必须叠加 Web Locks，提供同源原子互斥。任一锁未获得时不得处理账号。
- 脚本发现运行器不是 Violentmonkey、权限缺失、站点域名不可访问或独占锁不可用时，必须阻止启动并显示明确提示。
- 不修改 Sub2API 后端、前端、数据库或现网配置。

## 验收标准

- [ ] 合法行、空行、非法行、重复邮箱和密码含额外 `|` 的输入都能得到正确解析结果。
- [ ] 控制台只在 `www.havefun.eu.cc` 的 HTTP/HTTPS 页面显示；HTTP 模式有显式风险确认，xAI/Grok 页面不会出现脚本面板或明文凭据。
- [ ] HTTP/HTTPS 使用同一个共享租约锁，HTTPS 额外使用 Web Locks；任意协议组合的第二个控制台不能覆盖当前批次，过期租约可以恢复。
- [ ] 非活动控制台关闭或点击清空时不能删除其它控制台的共享任务；页面卸载只清理自身 `run_id` 的共享值。
- [ ] 每个账号独立完成 Device Flow，正确轮询并收集 refresh token；单号失败不影响后续账号。
- [ ] 邮箱和密码可在模拟登录页面中自动填入，未知页面及 Cloudflare 场景只暂停、不自动绕过。
- [ ] 每个账号结束后删除目标域 Cookie 和站点存储；清理失败时队列停止，不发生账号 session 串用。
- [ ] 停止、跳过、超时和标签被手工关闭时，当前账号临时密码与轮询任务均被清理。
- [ ] 导出内容是一行一个 refresh token，不包含邮箱密码、access token、ID token 或错误正文。
- [ ] 脚本通过语法检查和纯逻辑自动化测试；真实 xAI/Cloudflare 流程给出可执行的手工验收步骤。
- [ ] 任何代码、测试夹具、日志、文档和任务文件均不包含用户提供的真实密码或完整 Token。

## 范围外

- 批量注册新的 Grok/xAI 账号。
- 绕过 Cloudflare、验证码、2FA、异常登录验证或平台风控。
- 保存账号密码供下次批量运行。
- 自动调用 Sub2API 管理 API、数据库或现网服务器创建账号。
- 抓取 Grok 网页内容作为推理 API。
