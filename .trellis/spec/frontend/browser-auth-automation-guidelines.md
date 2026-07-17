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
  "user_code": "Device Flow 用户代码",
  "verification_url": "可信 xAI Device Flow 验证页",
  "created_at": 0,
  "expires_at": 0,
  "password_consumed_at": "密码提交后可选",
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

- 控制台只能在显式允许的 host 和 HTTP/HTTPS 协议渲染，并使用 `closed` Shadow DOM 隔离账号输入和 Token 结果；认证站点只允许 HTTPS 并只运行隐藏驱动。
- Violentmonkey 元数据必须覆盖用户实际打开的控制台 URL。非默认端口不能只依赖 `// @match http://host/*` 静态检查；已知实际入口如 `http://www.havefun.eu.cc:8080/*` 必须显式声明 `@include` 或等价规则，并用测试断言精确端口规则存在。
- HTTP 控制台必须醒目提示网络中间人、代理、页面篡改和全局输入监听风险，并把风险确认纳入开始和重试门禁。Shadow DOM 和用户脚本隔离不能被描述为 TLS 替代品。
- HTTP/HTTPS 控制台必须持有同一个 Violentmonkey 共享租约，提供跨协议互斥；HTTPS 还必须先获得 Web Lock，再在其回调内获得共享租约。只使用 Web Lock 会因锁按 origin 隔离而无法阻止 HTTP 与 HTTPS 标签同时运行。
- GM 共享值没有 compare-and-swap。租约必须使用随机 owner、有限竞争确认延迟、心跳续租和过期回收；结束时只能删除 owner 仍匹配的租约。锁 API 或共享存储异常时必须显示稳定错误，并保持批次未启动。
- 清空敏感数据必须先获得同一控制台锁。页面卸载只能删除与自身 `run_id` 匹配的任务、事件和清理消息，空闲或旧控制台不得清除其它活动批次。
- 登录驱动只有在 `run_id`、`account_id`、`tab_marker` 全部匹配且任务未取消、未过期时才能自动动作。标签标识只能放在 URL fragment 或标签私有状态中，禁止携带密码。
- 登录/授权标签和站点存储清理标签必须使用不同的 URL fragment / `window.name` marker key。登录标签使用 `grok-bulk-login`，清理标签使用 `grok-bulk-cleanup`；清理 ACK 只接受 cleanup marker，不能接受 login marker，即使 `tab_marker` 值相同。
- 站点存储清理标签必须使用目标 origin 下可稳定加载脚本文档的承载 URL。已知 `auth.x.ai` 根路径会在真实 Chrome 中显示 404/403 并造成误判，必须使用同源非根承载页，例如 `https://auth.x.ai/oauth2/authorize`。
- Device Flow 只请求受信任的 HTTPS xAI 端点，处理 `authorization_pending`、`slow_down`、拒绝、过期、网络错误、超时和取消。业务结果只保留 refresh token，不持久化完整 Token 响应。
- Device Flow 响应的验证页必须从可信 xAI HTTPS 字段中选择：优先使用标准 `verification_uri`，缺失时回退 `verification_uri_complete`。当前任务必须保存 `verification_url`，登录标签首次进入登录入口后再按该 URL 跳转；不要把清理标签或根路径页面当作登录页证据。
- xAI 在未登录状态可能先展示 Device Sign-in 设备码输入页，再进入邮箱/密码登录页。登录驱动必须能通过 input 自身属性、label、placeholder 或输入框附近短文本识别中文/英文设备码输入框，填入当前任务的 `user_code` 并提交；不得把通用 OTP/验证码页误识别为设备码页。
- xAI 设备授权路径判断必须兼容 `/oauth2/device` 基础路径和其子路径，禁止只匹配 `/oauth2/device/`。官方验证页路径变化时，低置信度页面仍应停机或等待人工，不得猜测点击。
- Cloudflare、验证码、2FA 和未知安全确认只切换为人工等待状态。人工处理结束且页面重新成为高置信度登录/授权阶段后，驱动可以继续。
- 填表后的延迟动作必须按以下顺序执行：记录任务归属和调度 URL -> 等待有限延迟 -> 重新校验共享任务、标签、URL 和非 challenge 状态 -> 占用动作门禁次数 -> 点击高置信度按钮或派发 Enter。
- 守卫拒绝不得消耗动作门禁次数。否则同一 URL 临时进入 challenge 后，即使用户完成验证，也会因为旧定时器占用次数而永久无法恢复。
- 密码提交后立即从共享任务删除并写入 `password_consumed_at`。停止或跳过写入 `cancelled_at` 并删除密码；两个字段可以共存，表示提交后任务又被取消。
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
| 清理请求出现在 `grok-bulk-login` 登录 marker 标签中 | 不返回清理 ACK，不清理该页存储 |
| 任务过期、停止或跳过 | 取消待执行动作并删除共享密码 |
| 延迟期间 URL 变化 | 旧动作不执行，且不标记密码已提交 |
| 延迟期间出现 challenge | 旧动作不执行，门禁次数保持可用 |
| 同 URL challenge 完成 | 重新扫描后允许提交一次，后续重复扫描不得再次提交 |
| `authorization_pending` | 保持当前轮询间隔 |
| `slow_down` | 在上限内增加轮询间隔 |
| Device Flow 只返回 `verification_uri` 或返回当前 `/oauth2/device` 基础路径 | 使用可信验证页继续流程 |
| Device Sign-in 中文页只在输入框附近显示“输入设备代码” | 通过近邻文本识别设备码输入框，填入 `user_code` 并提交 |
| Token 成功但缺少 refresh token | 当前账号失败，不导出 access token |
| 初始 Session 清理标签显示 403/404 或 Cloudflare 页面 | 只作为清理标签状态处理，不得向用户描述为登录页失败；`auth.x.ai` 不得使用根路径承载 |
| Cookie、站点存储或清理 ACK 失败 | 保持清理失败状态并停止后续账号 |
| Violentmonkey 或 HttpOnly 权限不足 | 批次开始前失败，不处理任何账号 |

### 5. Good/Base/Bad Cases

- Good：密码页填入后短暂出现 Cloudflare 文案，旧定时器被守卫拒绝；用户完成验证后，同一 URL 重新扫描并只提交一次。
- Good：用户点击停止时，共享任务先投影为不含密码的取消状态，登录驱动随后取消定时器，控制台再中止 Token 请求并清理 Session。
- Good：清理 ACK 同时匹配批次、账号、`cleanup_id` 和 host，Cookie 删除后二次 domain 枚举为空才进入下一账号。
- Good：HTTPS 控制台持有 Web Lock 和共享租约时，HTTP 控制台读取同一租约并拒绝启动；反向顺序同样成立。
- Good：旧控制台关闭时只删除自身 `run_id` 的共享值；另一个活动批次的任务、事件和清理 ACK 保持不变。
- Good：实际入口是 `http://www.havefun.eu.cc:8080/admin/accounts` 时，元数据包含精确 `:8080` 规则，运行时仍用 host/protocol 校验限制控制台。
- Good：xAI Device Flow 返回 `verification_uri: "https://accounts.x.ai/oauth2/device"` 与 `verification_uri_complete` 时，任务保存基础验证页，登录标签先进入 `accounts.x.ai` 登录入口，登录完成后再跳转验证页。
- Good：`accounts.x.ai/oauth2/device` 中文页的 input 没有稳定 `name`/`placeholder`，但附近容器显示“输入设备代码”；驱动仍能填入 `user_code` 并点击“继续”，随后再处理邮箱/密码页。
- Good：初始 Session 清理打开 `https://auth.x.ai/oauth2/authorize#grok-bulk-cleanup=...`，避免 `auth.x.ai` 根路径在真实 Chrome 中显示找不到网页。
- Base：普通邮箱页或密码页在 URL、任务和标签稳定时自动提交一次。
- Base：页面结构低置信度时仅上报未知页面，用户可停止、跳过或人工处理。
- Bad：测试只检查源码包含 `http://www.havefun.eu.cc/*`，却没有覆盖用户实际使用的 `:8080` URL。
- Bad：只接受 `verification_uri_complete` 或只匹配 `/oauth2/device/complete`，导致当前官方 `/oauth2/device` 基础路径被误判为未知/验证页面。
- Bad：清理标签也使用 `#grok-bulk-login=...`，或继续使用 `https://auth.x.ai/#grok-bulk-cleanup=...` 根路径，导致手工验收时被误认为授权页或错误页。
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
  - 登录入口无表单但任务含可信 `verification_url` 时，会在有限延迟后跳转官方设备验证页，并保留标签归属。
  - 中文 Device Sign-in 页通过输入框附近文本识别设备码输入框，并提交当前任务 `user_code`。
  - 密码填入并提交一次，提交后共享密码删除。
  - 共享任务取消后待执行动作被取消。
  - URL 变化或 challenge 出现后，按钮和 Enter 都不触发，密码不标记为已提交。
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
