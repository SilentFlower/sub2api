# Browser Auth Automation Guidelines

> 独立浏览器用户脚本处理第三方认证、人工 challenge、跨页面凭据和 Session 清理时的可执行契约。

## Scenario: Violentmonkey 浏览器认证自动化

### 1. Scope / Trigger

- Trigger：修改 `tools/grok-login-userscript/`，或新增需要在真实浏览器中跨页面填写凭据、等待人工验证并收集 OAuth Token 的用户脚本时，必须按本节检查。
- 适用范围：Violentmonkey 元数据、控制台与登录驱动通信、Device Flow、延迟 DOM 动作、凭据生命周期、Cookie 和站点存储清理。
- 不适用：服务端密码登录、无头浏览器绕过 Cloudflare、批量注册账号或自动处理验证码/2FA。
- 独立用户脚本属于明确领域 owner，不得为了复用页面外壳把凭据状态接入 Sub2API 前端 store、后端 API 或数据库。

### 2. Signatures

账号输入与导出格式：

```text
input:  email|password
output: refresh_token\nrefresh_token\n...
```

当前登录任务必须携带完整归属标识：

```json
{
  "run_id": "批次标识",
  "account_id": "账号标识",
  "tab_marker": "脚本标签标识",
  "email": "当前邮箱",
  "password": "仅提交前存在",
  "user_code": "登录完成并创建 Device Flow 后写入的用户代码",
  "verification_url": "登录完成并创建 Device Flow 后写入的可信 xAI Device Flow 验证页",
  "verification_launch_url": "登录完成并创建 Device Flow 后写入的可信 xAI Device Flow 浏览器启动页",
  "created_at": 0,
  "expires_at": 0,
  "password_consumed_at": "密码提交后可选",
  "authenticated_at": "登录驱动确认真实 xAI 登录态后可选",
  "device_ready_at": "控制台创建并写回 Device Flow 后可选",
  "cancelled_at": "停止或跳过后可选"
}
```

站点存储清理请求和 ACK 必须使用单次清理标识：

```json
{
  "request": {
    "run_id": "批次标识",
    "account_id": "账号标识",
    "cleanup_id": "清理标识",
    "tab_marker": "清理标签标识",
    "target_host": "目标 host"
  },
  "ack": {
    "run_id": "批次标识",
    "account_id": "账号标识",
    "cleanup_id": "原样回传",
    "host": "实际 host",
    "ok": true,
    "error_code": "失败时的稳定错误码"
  }
}
```

延迟动作的调用关系：

```javascript
submitFilledInput(element, button, stageAndUrlKey, afterSubmit)
```

控制台独占锁调用关系：

```javascript
runWithControllerLock(lockManager, leaseStore, lockName, ownerId, callback, options)
```

跨协议共享租约记录：

```json
{
  "owner": "当前控制台随机标识",
  "acquired_at": 0,
  "expires_at": 0
}
```

xAI Device Flow 契约以 `backend/internal/pkg/xai/oauth.go` 和
`backend/internal/pkg/xai/sso_device.go` 中的公开 client ID、scope、device code
端点和 Token 端点为核对来源。

### 3. Contracts

- 控制台只能在显式允许的 host 和 HTTP/HTTPS 协议渲染，默认收起为悬浮球并可展开完整面板，使用 `closed` Shadow DOM 隔离账号输入和 Token 结果；认证站点只允许 HTTPS 并只运行隐藏驱动。
- Violentmonkey 元数据必须覆盖用户实际打开的控制台 URL。非默认端口不能只依赖 `// @match http://host/*` 静态检查；已知实际入口如 `http://www.havefun.eu.cc:8080/*` 必须显式声明 `@include` 或等价规则，并用测试断言精确端口规则存在。
- HTTP 控制台必须醒目提示网络中间人、代理、页面篡改和全局输入监听风险，并把风险确认纳入开始和重试门禁。Shadow DOM 和用户脚本隔离不能被描述为 TLS 替代品。
- HTTP/HTTPS 控制台必须持有同一个 Violentmonkey 共享租约，提供跨协议互斥；HTTPS 还必须先获得 Web Lock，再在其回调内获得共享租约。只使用 Web Lock 会因锁按 origin 隔离而无法阻止 HTTP 与 HTTPS 标签同时运行。
- GM 共享值没有 compare-and-swap。租约必须使用随机 owner、有限竞争确认延迟、心跳续租和过期回收；结束时只能删除 owner 仍匹配的租约。锁 API 或共享存储异常时必须显示稳定错误，并保持批次未启动。
- 清空敏感数据必须先获得同一控制台锁。页面卸载只能删除与自身 `run_id` 匹配的任务、事件和清理消息，空闲或旧控制台不得清除其它活动批次。
- 登录驱动只有在 `run_id`、`account_id`、`tab_marker` 全部匹配且任务未取消、未过期时才能自动动作。标签标识只能放在 URL fragment 或标签私有状态中，禁止携带密码。登录标签首次从 `#grok-bulk-login=...` 读到随机标记时，必须把该标记写入同标签 `sessionStorage`；xAI 跳转到 `/account` 后可能清除 URL hash 或 `window.name`，驱动必须能从该私有状态恢复归属，但仍只能在共享任务的 `tab_marker` 同时匹配时执行。
- 登录/授权标签和站点存储清理标签必须使用不同的 URL fragment / `window.name` marker key。登录标签使用 `grok-bulk-login`，清理标签使用 `grok-bulk-cleanup`；清理 ACK 只接受 cleanup marker，不能接受 login marker，即使 `tab_marker` 值相同。
- 站点存储清理标签必须使用目标 origin 下可稳定加载脚本文档的承载 URL。已知 `auth.x.ai` 根路径会在真实 Chrome 中显示 404/403 并造成误判，必须使用同源非根承载页，例如 `https://auth.x.ai/oauth2/authorize`。
- Device Flow 只请求受信任的 HTTPS xAI 端点，处理 `authorization_pending`、`slow_down`、拒绝、过期、网络错误、超时和取消。业务结果只保留 refresh token，不持久化完整 Token 响应。
- 控制台主流程必须先打开 `https://accounts.x.ai/sign-in` 并完成邮箱密码登录；仅当登录驱动确认真实 xAI 登录态并写入 `authenticated_at` 后，控制台才允许调用 Device Flow。`password_consumed_at` 只表示密码已提交，不等于登录完成；已取消、过期或缺少 `authenticated_at` 的任务不得创建或合并 Device Flow。
- Device Flow 响应的验证页必须从可信 xAI HTTPS 字段中选择：标准验证页 `verification_url` 优先使用 `verification_uri`、缺失时回退 `verification_uri_complete`；浏览器启动页 `verification_launch_url` 优先使用带 `user_code` 的 `verification_uri_complete`、缺失时回退 `verification_uri`。控制台创建 Device Flow 后把 `user_code`、`verification_url`、`verification_launch_url` 和 `device_ready_at` 合并回当前共享任务；登录标签看到 `device_ready_at` 后应立即跳转可信验证页。设备码页必须提交当前任务的 `user_code` 或点击已预填当前码的“继续”；授权页必须确认是当前任务可信的 Grok Build OAuth 授权页后才允许点击“允许”/“Allow”/“继续”/“Continue”，且授权页识别优先级必须高于设备码页识别，避免 `/oauth2/device/approve` 被当成设备码页重复提交。自动点击 consent 时必须优先真实表单按钮，禁止点击会直接 GET `/oauth2/device/approve` 的链接式伪按钮。控制台收到 Token 成功后，如果最近一次设备码提交或最终授权提交距当前不足稳定窗口，必须等待剩余窗口后再关闭登录标签并进入 Session 清理；若未捕获提交事件，也应在 Token 成功后保留授权页一个固定短窗口。停止、跳过和取消信号仍必须立即中断该等待。如果登录成功后先落到 `https://accounts.x.ai/account` 账户页，驱动只把它视为已登录中间态并写入 `authenticated_at`，不得点击账户页里的 Email 等设置控件；后续由控制台创建 Device Flow 后再跳转官方验证页。
- 登录驱动中基于页面停留时间的判断必须在 URL 变化时重置计时。xAI 自己从密码页跳到账户页、再跳到 Device Flow 页时，脚本不得沿用上一页的 `firstSeenAt` 提前触发未知页或兜底跳转。
- 控制台状态必须跟随关键自动化阶段推进：密码实际提交并确认登录态后不得继续显示“填写密码”，应进入授权页等待状态；Device Code 输入页填入 `user_code` 后应进入 Token 轮询/等待授权结果状态，即使页面没有单独的 consent 按钮。
- 只有共享任务已经写入当前 `user_code` 时，登录驱动才允许把 Device Sign-in 识别为设备码页。设备码识别必须通过 input 自身属性、label、placeholder、输入框附近短文本、URL `user_code` 参数或页面已渲染的当前设备码确认中文/英文设备码表单，填入当前任务的 `user_code` 或直接点击“继续”；不得把通用 OTP/验证码页误识别为设备码页。
- xAI 设备授权路径判断必须兼容 `/oauth2/device` 基础路径和其子路径，禁止只匹配 `/oauth2/device/`。官方验证页路径变化时，低置信度页面仍应停机或等待人工，不得猜测点击。
- Cloudflare、验证码、2FA 和未知安全确认只切换为人工等待状态。Cloudflare / Turnstile 显示“成功 / Success / Verified”后仍可能需要短暂写入验证结果，驱动必须等待稳定窗口再恢复提交；如果曾经检测到 challenge，但 challenge DOM 消失后登录/授权控件尚未恢复，必须在有限后置窗口内继续等待并复扫，不得立刻按普通未知页停机；人工处理结束且页面重新成为高置信度登录/授权阶段后，驱动可以继续。
- 填表后的延迟动作必须按以下顺序执行：记录任务归属和调度 URL -> 等待有限延迟 -> 重新校验共享任务、标签、URL 和非 challenge 状态 -> 占用动作门禁次数 -> 点击高置信度按钮或派发 Enter。
- 守卫拒绝不得消耗动作门禁次数。否则同一 URL 临时进入 challenge 后，即使用户完成验证，也会因为旧定时器占用次数而永久无法恢复。
- 密码提交后立即从共享任务删除并写入 `password_consumed_at`。停止或跳过写入 `cancelled_at` 并删除密码；两个字段可以共存，表示提交后任务又被取消。
- 密码已消费但当前页面仍停留在同一密码表单时，只要 DOM 密码框仍有值且存在高置信度登录按钮，驱动可以重按登录按钮；不得重新写回或持久化密码，也不得在密码框为空时猜测重填。
- 每个账号结束后必须关闭脚本打开的登录标签，清除目标域 localStorage、sessionStorage、IndexedDB、Cache Storage、Service Worker 和 Cookie，并二次验证 Cookie。任一清理失败必须阻断队列。
- 测试、日志、文档和导出内容不得包含真实账号密码、access token、ID token、完整 refresh token 或上游错误正文。

### 4. Validation & Error Matrix

| 条件 | 必须行为 |
| --- | --- |
| 当前页面不是明确允许的控制台 host，或协议不是 HTTP/HTTPS | 不渲染控制台 |
| 用户实际控制台 URL 使用非默认端口，但脚本元数据没有显式匹配该端口 | 不宣称脚本已支持该入口；补元数据规则和回归测试 |
| HTTP 控制台未确认明文传输风险 | 不启动或重试批次 |
| HTTP/HTTPS 任一协议已持有未过期共享租约 | 第二个控制台不处理账号，也不清空共享值 |
| HTTPS 未获得 Web Lock，或任一协议未获得共享租约 | 不进入批次准备、旧 Session 清理或账号处理 |
| 租约 owner 在批次期间变化 | 停止当前批次、取消请求并进入当前账号清理 |
| Web Lock、GM 共享存储或租约调度抛错 | 显示稳定锁失败错误，不处理任何账号 |
| 旧控制台卸载且共享值属于其它 `run_id` | 保留其它控制台的共享值 |
| 任务、账号或标签标识不匹配 | 不填表、不点击、不清理其它标签的数据 |
| 登录跳转到 `/account` 后 URL hash / `window.name` 丢失，但同标签 `sessionStorage` 中有当前 `tab_marker` | 继续识别为当前登录标签，写入 `authenticated_at` 并等待控制台创建 Device Flow |
| 清理请求出现在 `grok-bulk-login` 登录 marker 标签中 | 不返回清理 ACK，不清理该页存储 |
| 任务过期、停止或跳过 | 取消待执行动作并删除共享密码 |
| 延迟期间 URL 变化 | 旧动作不执行，且不标记密码已提交 |
| 延迟期间出现 challenge | 旧动作不执行，门禁次数保持可用 |
| Cloudflare / Turnstile 显示成功但验证结果仍在写入 | 等待稳定窗口，不提交、不消耗动作门禁 |
| Cloudflare / Turnstile 消失但登录控件尚未恢复 | 在有限后置窗口内保持等待人工验证并复扫，不触发普通 unknown 超时 |
| 同 URL challenge 完成 | 重新扫描后允许提交一次，后续重复扫描不得再次提交 |
| 密码已消费但页面仍停在已填写密码表单 | 使用当前 DOM 已有密码值重按高置信度登录按钮，不重新保存密码 |
| `authorization_pending` | 保持当前轮询间隔 |
| `slow_down` | 在上限内增加轮询间隔 |
| Device Flow 只返回 `verification_uri` 或返回当前 `/oauth2/device` 基础路径 | 使用可信验证页继续流程 |
| 密码已提交但尚未写入 `authenticated_at` | 继续等待登录态，不创建 Device Flow |
| 已取消或过期任务已经写入 `authenticated_at` | 不创建 Device Flow，不合并授权任务 |
| 密码提交后进入 `accounts.x.ai/account` 账户页 | 视为登录成功中间态，删除共享密码并写入 `authenticated_at`；等待控制台创建 Device Flow 后再跳转官方验证页，不点击账户设置控件 |
| 控制台写入 `device_ready_at` 后登录标签仍停在 `/account` 或登录后页面 | 立即跳转当前任务可信 Device Flow 验证页 |
| Device Sign-in 页 | 仅当共享任务已有当前 `user_code` 时，才通过输入框、URL 参数或页面已预填当前码提交/点击“继续” |
| 密码提交后 Device Sign-in 中文页只在输入框附近显示“输入设备代码” | 通过近邻文本识别设备码输入框，填入 `user_code` 并提交 |
| `/oauth2/device/approve` 页面显示 Grok Build 授权且按钮文案是“继续”/`Continue` | 按最终授权页处理，点击高置信表单按钮并上报 `authorization_submitted`，不得走设备码提交分支 |
| `/oauth2/device/approve` 候选按钮是带 `href` 的链接式伪按钮 | 不自动点击，避免 GET approve 导致 `Invalid action`；继续等待真实表单按钮或进入低置信停机 |
| Token 成功时距最近一次设备码提交或 Grok Build 授权提交不足稳定窗口 | 等待剩余时间，保持授权页可见，然后再成功、关闭登录标签和清理 Session |
| Token 成功但缺少 refresh token | 当前账号失败，不导出 access token |
| 初始 Session 清理标签显示 403/404 或 Cloudflare 页面 | 只作为清理标签状态处理，不得向用户描述为登录页失败；`auth.x.ai` 不得使用根路径承载 |
| Cookie、站点存储或清理 ACK 失败 | 保持清理失败状态并停止后续账号 |
| Violentmonkey 或 HttpOnly 权限不足 | 批次开始前失败，不处理任何账号 |

### 5. Good/Base/Bad Cases

- Good：密码页填入后短暂出现 Cloudflare 文案，旧定时器被守卫拒绝；用户完成验证后，同一 URL 重新扫描并只提交一次。
- Good：Cloudflare 小组件显示“成功!”且页面仍保留 `challenges.cloudflare.com` iframe 时，驱动先等待约 5 秒稳定窗口；窗口结束且“登录”按钮可用后再点击，并且旧延迟动作不提前消耗提交次数。
- Good：Cloudflare DOM 消失但 xAI 登录页面还在恢复或短暂无控件时，驱动在约 60 秒后置窗口内继续等待并复扫，避免按 12 秒普通未知页超时误报。
- Good：首次登录点击后页面未跳转且共享密码已经删除时，密码框里的浏览器 DOM 值仍存在；驱动重按高置信度“登录”按钮，避免 12 秒后误报未知页。
- Good：用户点击停止时，共享任务先投影为不含密码的取消状态，登录驱动随后取消定时器，控制台再中止 Token 请求并清理 Session。
- Good：清理 ACK 同时匹配批次、账号、`cleanup_id` 和 host，Cookie 删除后二次 domain 枚举为空才进入下一账号。
- Good：HTTPS 控制台持有 Web Lock 和共享租约时，HTTP 控制台读取同一租约并拒绝启动；反向顺序同样成立。
- Good：旧控制台关闭时只删除自身 `run_id` 的共享值；另一个活动批次的任务、事件和清理 ACK 保持不变。
- Good：实际入口是 `http://www.havefun.eu.cc:8080/admin/accounts` 时，元数据包含精确 `:8080` 规则，运行时仍用 host/protocol 校验限制控制台。
- Good：控制台先打开 `accounts.x.ai/sign-in`；驱动自动填写邮箱密码，确认进入 `/account` 或其它登录态页面后写入 `authenticated_at`；控制台随后创建 Device Flow 并把 `user_code` / 验证页写回共享任务。
- Good：密码提交后 xAI 跳到 `https://accounts.x.ai/account`，页面有 “Email” 账户设置按钮，且跳转已清掉 URL hash / `window.name`；驱动通过同标签 `sessionStorage` 恢复 `tab_marker`，忽略账户设置按钮，只写入登录态确认；控制台写入 `device_ready_at` 后登录标签立即跳转可信 Device Flow 验证页。
- Good：Device Flow 返回 `verification_uri: "https://accounts.x.ai/oauth2/device"` 与 `verification_uri_complete` 时，控制台在登录完成后保存基础验证页和带 `user_code` 的启动页；登录标签随后在设备码页填入当前码或点击已预填当前码的“继续”。
- Good：授权页显示 “授权 Grok Build” / “Authorize Grok Build” 且路径是可信 xAI OAuth 路径时，驱动点击“允许”/“Allow”/“继续”/“Continue”；若页面落在 `/oauth2/device/approve`，也必须先按授权页处理而不是设备码页处理；Token 很快成功时，控制台补足约 5 秒稳定窗口后再关闭标签和清理 Session；未知 OAuth 页面、非 xAI HTTPS 页面或只会 GET approve 的链接式伪按钮不点击。
- Good：初始 Session 清理打开 `https://auth.x.ai/oauth2/authorize#grok-bulk-cleanup=...`，避免 `auth.x.ai` 根路径在真实 Chrome 中显示找不到网页。
- Base：普通邮箱页或密码页在 URL、任务和标签稳定时自动提交一次。
- Base：页面结构低置信度时仅上报未知页面，用户可停止、跳过或人工处理。
- Bad：测试只检查源码包含 `http://www.havefun.eu.cc/*`，却没有覆盖用户实际使用的 `:8080` URL。
- Bad：只接受 `verification_uri_complete` 或只匹配 `/oauth2/device/complete`，导致当前官方 `/oauth2/device` 基础路径被误判为未知/验证页面。
- Bad：密码提交后只看到 `password_consumed_at` 就创建 Device Flow；应等登录驱动确认 `/account` 或登录态并写入 `authenticated_at`。
- Bad：清理标签也使用 `#grok-bulk-login=...`，或继续使用 `https://auth.x.ai/#grok-bulk-cleanup=...` 根路径，导致手工验收时被误认为授权页或错误页。
- Bad：把所有 `/oauth2/device/*` 都优先当成设备码页，导致 `/oauth2/device/approve` 上的“继续”被错误提交并返回 `Invalid action`。
- Bad：创建延迟定时器时立即占用动作次数，导致守卫拒绝后无法恢复。
- Bad：只删除当前 URL 可见 Cookie，遗漏其它 path、子域、HttpOnly 或分区 Cookie。
- Bad：把整批账号密码写入 localStorage、GM 共享值、页面 DOM 属性、URL 或日志。
- Bad：HTTPS 只使用 Web Lock、HTTP 只使用共享租约；两种锁互不相通，会让两个协议的标签同时处理账号。

### 6. Tests Required

- 纯逻辑测试至少覆盖：输入解析、邮箱去重、Token 响应分类、轮询退避、挑战页识别、动作门禁、任务归属、密码消费/取消投影、Cookie domain 枚举与二次校验。
- 控制台与锁测试至少覆盖：
  - 只允许精确控制台 host 的 HTTP/HTTPS 地址，实际使用的非默认端口元数据规则存在，xAI 验证地址仍只接受 HTTPS。
  - 未过期租约拒绝、过期租约接管、心跳续租、竞争确认失败和 owner 限定释放。
  - HTTPS 活动时 HTTP 获取锁失败，HTTP 活动时 HTTPS 获取锁失败。
  - Web Lock 或共享存储抛错时批次回调不执行，控制台使用稳定锁失败提示。
  - 页面卸载只删除当前 `run_id` 的共享值，保留其它批次。
- Node VM 浏览器状态机测试至少覆盖：
  - 登录入口出现 `Login with email` / 邮箱登录按钮时优先点击该入口。
  - 控制台先打开 `accounts.x.ai/sign-in`，并且只有共享任务带 `password_consumed_at` 与 `authenticated_at` 时才创建并合并 Device Flow；已取消或过期任务不能触发创建。
  - 登录页读到 `#grok-bulk-login=...` 后写入同标签 `sessionStorage`；模拟跳转到 `/account` 且 URL hash / `window.name` 为空时，仍能用该私有标记确认当前标签并写入 `authenticated_at`。
  - 密码提交后落到 `accounts.x.ai/account` 时，即使页面存在 “Email” 等账户设置按钮，也不会点击账户页控件；只写入登录态确认，待 `device_ready_at` 写入后才跳转官方设备验证页。
  - 完整模拟密码页 -> `/account` 登录态确认 -> 控制台写入 Device Flow -> `/oauth2/device`，断言 URL 变化重置页面计时、账户页不提前点击设置控件、设备码填入后状态事件继续推进。
  - 共享任务已有当前 `user_code` 后进入 Device Sign-in 页时，若有设备码输入框、URL 参数或页面已预填当前码，则填码或直接点击“继续”；中文 Device Sign-in 页仍可通过输入框附近文本识别设备码输入框，并提交当前任务 `user_code`。
  - `/oauth2/device/approve` 上的 Grok Build 授权页如果按钮文案是“继续”/`Continue`，必须上报 `authorization_submitted`，不得上报 `user_code_filled`；带 `href=/oauth2/device/approve` 的链接式伪按钮不得被点击。
  - 最终设备授权提交后的稳定窗口计算：Token 成功早于固定窗口时补足剩余等待，已超过窗口时不额外等待；无提交事件时也在 Token 成功后保留一个短窗口。
  - 密码填入并提交一次，提交后共享密码删除。
  - 共享任务取消后待执行动作被取消。
  - URL 变化或 challenge 出现后，按钮和 Enter 都不触发，密码不标记为已提交。
  - Cloudflare / Turnstile 显示成功后等待稳定窗口，再恢复密码提交。
  - Cloudflare / Turnstile 消失但页面暂未恢复登录控件时，在后置窗口内继续等待，窗口耗尽后才进入未知页。
  - 同 URL challenge 消失后重新扫描、提交一次，后续扫描不重复提交。
  - Cloudflare 页面只上报人工等待。
  - 站点存储清理成功和失败 ACK。
  - 清理请求必须使用独立 cleanup marker；登录 marker 即使值匹配也不得返回清理 ACK。
- 所有夹具使用 `example.com` 邮箱、虚构密码和明显的假 Token。
- 最低验证命令：

```bash
node --check tools/grok-login-userscript/grok-bulk-login.user.js
node --test tools/grok-login-userscript/*.test.cjs
git diff --check
```

- 真实 xAI 页面、Cloudflare 和 HttpOnly Cookie 权限必须在安装 Violentmonkey 的用户浏览器中手工验收，不能用 mock 结果宣称真实流程已验证。

### 7. Wrong vs Correct

#### Wrong

```javascript
if (!actionGate.tryAcquire(key)) return false
deferredSubmit.schedule(150, guard, action)
```

问题：定时器创建时已经消耗动作次数；如果 `guard` 因同 URL challenge 返回 false，后续恢复扫描也无法再次调度有效提交。

#### Correct

```javascript
if (actionGate.attempts(key) >= maxAttempts) return false
deferredSubmit.schedule(150, () => {
  if (!taskMatchesCurrentRun() || location.href !== expectedUrl || isChallenge()) return false
  return actionGate.tryAcquire(key)
}, action)
```

先通过所有执行时守卫，再占用一次动作机会。守卫拒绝表示动作从未发生，因此不得消耗“最多执行一次”的额度。

#### Wrong

```javascript
if (navigator.locks) {
  return runWithExclusiveLock(navigator.locks, lockName, callback)
}
return runWithSharedLeaseLock(leaseStore, lockName, ownerId, callback)
```

问题：Web Locks 按 origin 隔离，`http://` 与 `https://` 控制台不会共享同一把锁；两个分支可能同时进入批次。

#### Correct

```javascript
if (navigator.locks) {
  let leaseAcquired = false
  const webLockAcquired = await runWithExclusiveLock(navigator.locks, lockName, async () => {
    leaseAcquired = await runWithSharedLeaseLock(leaseStore, lockName, ownerId, callback)
  })
  return webLockAcquired && leaseAcquired
}
return runWithSharedLeaseLock(leaseStore, lockName, ownerId, callback)
```

共享租约承担跨协议互斥，HTTPS Web Lock 只增加同源原子互斥；两者不是互相替代关系。

#### Wrong

```javascript
// @match        http://www.havefun.eu.cc/*

if (/\/oauth2\/device\//.test(location.pathname)) {
  clickConsent()
}
```

问题：Violentmonkey 对非默认端口的匹配不能靠通用 host 规则臆测；同时当前 xAI 官方验证页可能是 `/oauth2/device` 基础路径。

#### Correct

```javascript
// @match        http://www.havefun.eu.cc/*
// @include      http://www.havefun.eu.cc:8080/*

if (isDeviceVerificationPath(location.pathname)) {
  clickConsent()
}
```

用元数据覆盖真实入口，用共享 helper 同时匹配 `/oauth2/device` 和其子路径；测试必须覆盖这两个契约。
