# Grok 批量登录 Violentmonkey 脚本设计

## 架构边界

实现放在独立目录 `tools/grok-login-userscript/`，不接入 Sub2API 前后端构建。交付物是一个可直接安装的 `grok-bulk-login.user.js`、纯逻辑测试和中文安装说明。

单个用户脚本根据当前 host 分成两个运行上下文：

1. `www.havefun.eu.cc` 控制台上下文：解析批次、维护队列、启动 Device Flow、打开登录标签、轮询 Token、清理 Cookie、汇总 refresh token。
2. xAI/Grok 驱动上下文：隐藏运行，读取当前账号临时任务，识别登录页面、自动填写、检测人工验证、清理当前 origin 的站点存储并上报状态。

两个上下文通过带 `run_id`、`account_id` 和随机 `tab_marker` 的 Violentmonkey 共享值通信。所有事件都校验当前运行标识；登录驱动还必须验证当前标签 fragment 中的 `tab_marker`，并在首次读到 fragment 标记时把随机标记写入同标签 `sessionStorage` 作为降级恢复状态，避免 xAI 跳转到 `/account` 后清掉 URL hash / `window.name` 导致标签归属丢失。该私有状态只保存随机标记，不保存邮箱、密码或 token，仍必须与共享任务中的 `run_id`、`account_id`、`tab_marker` 同时匹配后才允许自动动作。

控制台声明 `http://www.havefun.eu.cc/*` 和 `https://www.havefun.eu.cc/*` 匹配规则，并在启动入口再次校验 host 与协议。xAI/Grok 驱动仍只允许 HTTPS。控制台使用 `closed` Shadow DOM 隔离页面脚本对账号输入和 refresh token 结果区的直接访问；HTTP 模式额外显示网络注入和凭据泄露风险，并要求用户显式确认后才能开始或重试。

HTTP/HTTPS 控制台都使用同一个 Violentmonkey 共享值租约提供跨协议互斥：随机 owner 写入锁记录，等待短暂竞争窗口后二次确认归属，批次运行期间定时续租，结束时只释放自己的租约。HTTPS 安全上下文先获得 Chrome Web Lock，再在其回调内获得共享租约，以同时覆盖同源原子互斥和 HTTP/HTTPS 跨协议互斥。未过期的其它 owner 会直接阻止启动；过期租约允许接管。同一 Profile 内第二个控制台不得覆盖共享任务或交叉清理 Session。清空敏感数据也必须先获得同一控制台锁；页面卸载只删除与自身 `run_id` 匹配的任务、事件和清理消息。锁 API 或共享存储异常时显示稳定错误并保持批次未启动。

## 领域模块

虽然最终交付为单文件，源码内部按以下区域保持边界：

- `config`：官方端点、client ID、scope、目标 host、超时和共享键。
- `parser`：`邮箱|密码` 解析、规范化、去重和行级错误。
- `vmAdapter`：封装 `GM_getValue`、`GM_setValue`、`GM_deleteValue`、`GM_addValueChangeListener`、`GM_openInTab`、`GM_xmlhttpRequest`、`GM_cookie` 和剪贴板。
- `deviceFlow`：创建 device code、轮询 Token、错误分类、慢轮询和取消。
- `loginDriver`：页面识别、表单填写、框架事件派发、人工验证检测和授权按钮处理。
- `cleanup`：Cookie 删除、当前 origin 站点存储清理、二次校验和清理失败阻断。
- `controller`：批次状态机、暂停/继续/跳过/停止/重试、标签生命周期和结果管理。
- `ui`：仅在控制台域名渲染，使用 `closed` Shadow DOM 隔离站点样式和敏感输入区域。

## 数据契约

当前账号临时任务：

```json
{
  "run_id": "随机运行标识",
  "account_id": "批次内标识",
  "tab_marker": "脚本打开标签的随机归属标记",
  "email": "账号邮箱",
  "password": "当前账号密码",
  "user_code": "登录完成并创建 Device Flow 后写入的用户代码",
  "verification_url": "登录完成并创建 Device Flow 后写入的 xAI 官方 Device Flow 验证页",
  "verification_launch_url": "登录完成并创建 Device Flow 后写入的 xAI 官方 Device Flow 浏览器启动页",
  "created_at": 0,
  "expires_at": 0,
  "password_consumed_at": "密码提交后出现，任务不再包含 password",
  "authenticated_at": "登录驱动确认真实 xAI 登录态后出现",
  "device_ready_at": "控制台创建并写回 Device Flow 后出现",
  "cancelled_at": "停止或跳过后出现，任务不再包含 password"
}
```

`password_consumed_at` 表示密码已提交并从共享任务删除，不等于真实登录完成；`authenticated_at` 才表示登录驱动已经在 `/account` 或授权页登录态文案中确认当前 xAI session 可用。控制台只有在任务同时具备 `password_consumed_at` 和 `authenticated_at`，且未取消、未过期、尚未写入 `user_code` 时，才允许调用 Device Flow。`cancelled_at` 表示任务被停止或跳过；若密码提交后用户再停止，`password_consumed_at`、`authenticated_at` 和 `cancelled_at` 可以共存，但不得继续创建设备授权。登录驱动只有在 `run_id`、`account_id`、`tab_marker` 均匹配，任务未取消且未过期时才允许执行自动动作。

跨页事件：

```json
{
  "run_id": "随机运行标识",
  "account_id": "批次内标识",
  "type": "email_filled|password_filled|password_submitted|user_code_filled|email_method_selected|waiting_human|page_unknown|authorization_submitted|login_failed",
  "detail": "不含凭据的短描述",
  "at": 0
}
```

站点存储清理使用独立请求与 ACK，不复用登录驱动事件：

```json
{
  "request": {
    "run_id": "随机运行标识",
    "account_id": "批次内标识",
    "cleanup_id": "单次清理标识",
    "tab_marker": "专用清理标签标识",
    "target_host": "本次清理的目标 host",
    "at": 0
  },
  "ack": {
    "run_id": "随机运行标识",
    "account_id": "批次内标识",
    "cleanup_id": "原样回传",
    "host": "实际执行清理的 host",
    "ok": true,
    "error_code": "失败时为 CLEANUP_STORAGE_FAILED",
    "at": 0
  }
}
```

清理标签只在请求的 `target_host`、`tab_marker` 与当前页面全部匹配时执行；控制台只接受 `run_id`、`account_id`、`cleanup_id` 和 host 全部匹配的 ACK。清理标签使用独立 `grok-bulk-cleanup` URL fragment 和 `window.name` 前缀，登录/授权标签继续使用 `grok-bulk-login`，两者不得混用。

账号结果只在控制台内存保存：

```json
{
  "id": "批次内标识",
  "line": "原始输入行号",
  "email": "仅在控制台内存中保存，渲染时脱敏",
  "normalizedEmail": "批次去重键",
  "password": "仅在控制台内存中保存，成功、跳过或清空时置空",
  "status": "账号状态",
  "refreshToken": "仅成功时存在",
  "errorCode": "稳定错误码"
}
```

## 状态流

```text
pending
  -> requesting_device
  -> opening_login
  -> filling_email / filling_password
  -> waiting_human（可选，用户完成后回到填表或授权）
  -> polling_token
  -> success | failed | skipped
  -> cleaning
  -> pending（下一账号）或 finished
```

`cleaning` 是强制门禁。只有 Cookie 二次检查通过、当前账号共享值已删除、登录标签已关闭后，控制台才能推进下一账号。

## 控制台 UI

- 控制台在 `www.havefun.eu.cc` 默认以右下角悬浮球显示，避免长期遮挡业务页面。
- 点击悬浮球展开完整控制台；展开后标题栏提供“收起”按钮，运行状态仍在悬浮球中保留简短进度。
- 悬浮球与完整控制台同属 closed Shadow DOM，不向页面 DOM 暴露明文账号密码或 refresh token。

## Device Flow

- 使用 `application/x-www-form-urlencoded` 请求 `/oauth2/device/code`。
- Device Flow 响应保存两类可信 HTTPS 验证页：`verification_url` 优先使用标准 `verification_uri`、缺失时回退 `verification_uri_complete`；`verification_launch_url` 优先使用带 `user_code` 的 `verification_uri_complete`、缺失时回退 `verification_uri`。
- 控制台主流程先打开 `https://accounts.x.ai/sign-in`，让登录驱动完成邮箱密码登录和人工 challenge 等待。只有登录驱动写入 `authenticated_at` 后，控制台才调用 Device Flow，并将 `user_code`、`verification_url`、`verification_launch_url` 和 `device_ready_at` 合并回当前共享任务。
- 邮箱密码登录成功后如果 xAI 先跳到 `https://accounts.x.ai/account` 账户页，驱动必须把它视为已登录中间态；即使跳转清除了 URL hash 或 `window.name`，也要用同标签 `sessionStorage` 恢复 `tab_marker` 后继续确认共享密码已删除并写入 `authenticated_at`。账户页里的 “Email” 等设置按钮不得按邮箱登录入口处理。控制台写入 `device_ready_at` 后，登录标签再立即跳转 `verification_launch_url` 或 `verification_url`。
- 页面停留时间必须按当前 URL 计算。xAI 自己从密码页跳到 `/account` 再跳到 `/oauth2/device` 时，驱动要在每次 URL 变化时重置页面计时，不能沿用上一页时间提前触发兜底跳转。
- 当任务已有 `user_code` 后进入 `/oauth2/device`，登录驱动必须识别设备码输入框、URL `user_code` 参数或页面已渲染的当前设备码，并提交当前 `user_code` 或直接点击“继续”。授权页显示 “授权 Grok Build” / “Authorize Grok Build” 且路径可信时，才点击“允许”/“Allow”。控制台在 Token 成功后必须按最近一次设备码提交或最终授权提交时间补足约 5 秒稳定窗口；如果没有捕获提交事件，则从 Token 成功时开始保留该短窗口，之后才关闭登录标签并清理 Session。
- 使用服务端返回的 `interval`，最小轮询间隔不低于 1 秒。
- `authorization_pending` 保持当前间隔；`slow_down` 增加 5 秒；`access_denied` 和 `expired_token` 结束当前账号。
- 控制台停止或切换账号时调用请求控制对象的 `abort()` 并使旧回调因 `run_id` 不匹配而失效。
- refresh token 到达后立即从响应对象投影所需字段，不持久化完整 Token 响应。

## 登录驱动

- 每次 DOM 变化后按有限频率重新分类页面，避免高频 MutationObserver 回调。
- 输入框选择优先级：明确 `autocomplete`/`type`/`name`/label，其次使用可见 placeholder 和输入框附近短文本；多个候选或低置信度时暂停。附近文本只用于提高语义置信度，不能绕过 challenge 或 OTP 判断。
- 写值时使用原生 setter，再派发 `input` 和 `change`；必要时派发 Enter，但不对未知按钮模拟点击。
- Cloudflare 检测参考 challenge iframe、`challenges.cloudflare.com`、页面标题和已知提示文本。验证未完成时只上报 `waiting_human` 并按有限间隔复扫；看到 Cloudflare / Turnstile “成功 / Success / Verified”后先等待约 5 秒稳定窗口，避免验证结果尚未写入时过早点击登录。曾经检测到 challenge 后，如果 challenge DOM 消失但登录/授权控件尚未恢复，驱动继续保留约 60 秒后置恢复窗口，期间复扫并维持人工等待状态，不按普通 12 秒未知页超时停机。
- 登录/授权按钮点击必须满足高置信度语义匹配。动作门禁按“动作阶段 + 当前 URL”建立键，每个键最多执行一次；DOM observer 和定时扫描只能重复识别，不能重复提交同一阶段。
- 填表后的短延迟提交由单实例可取消控制器管理。共享任务变化、过期、停止、跳过或页面卸载时必须取消定时器；回调执行前再次读取共享任务并校验完整归属、调度时 URL 未变化、当前页面不是未完成 challenge，且 Cloudflare 成功状态已过稳定窗口。登录按钮仍 disabled 时不得派发 Enter，也不得消耗动作次数，应继续等待按钮可用，禁止在取消、导航或人工验证页面切换后补交。动作次数只在实际点击或 Enter 前占用，守卫拒绝不得消耗次数，确保同 URL 的人工 challenge 完成后可以自动继续且最多提交一次。
- 密码提交并从共享任务删除后，如果页面仍停在同一密码表单且密码框 DOM 值仍存在，驱动允许重按高置信度“登录”按钮作为恢复动作；该动作不得重新写回密码，密码框为空时仍按未知页停机。

## Session 清理

1. 控制台关闭登录标签并删除当前账号共享任务和事件。
2. 控制台依次为 `x.ai`、`auth.x.ai`、`accounts.x.ai` 和 `grok.com` 打开带独立 `grok-bulk-cleanup` fragment 与 `tab_marker` 的后台清理标签；`auth.x.ai` 使用 `https://auth.x.ai/oauth2/authorize` 作为同源非根承载页，避免根路径在真实 Chrome 中显示找不到网页。
3. 每个清理标签只删除当前 origin 可访问的 localStorage、sessionStorage、IndexedDB、Cache Storage 和 Service Worker，并返回带 `cleanup_id` 与 host 的 ACK；任一域超时、标识不匹配或清理报错都立即阻断。
4. 控制台按 `.x.ai`、`.grok.com` domain 使用 `GM_cookie.list` 全量枚举路径限定和子域 Cookie，逐个保留 path/分区信息调用 `GM_cookie.delete`。
5. 删除后再次按 domain 全量枚举；任何残留 Cookie 或 API 错误都视为清理失败，不能进入下一账号。

Violentmonkey 的 HttpOnly Cookie 权限默认关闭，因此 UI 在开始前展示强制检查项，README 说明全局和脚本级开关位置。脚本只能验证 API 行为，不能修改 Violentmonkey 自身设置。

## 安全设计

- 控制台输入框不自动保存，不写 localStorage。
- 共享值一次只包含当前账号，并设置短过期时间。
- 密码输入完成后删除共享密码；页面后续若再次请求密码则进入失败/人工状态，不重新从持久化位置读取。
- 停止或跳过时，控制台先把共享任务投影为不含密码的 `cancelled_at` 状态，再中止 Token 请求并进入强制 Session 清理。
- `console` 只输出稳定事件码和脱敏邮箱，不输出请求正文或上游响应正文。
- UI 采用 `closed` Shadow DOM；复制 refresh token 需要显式点击，“清空敏感数据”会覆盖内存数组并删除全部脚本共享键。
- HTTP 模式明确提示 TLS 缺失风险；Shadow DOM 和用户脚本隔离只降低普通页面误读风险，不能抵御网络中间人、被篡改页面或全局输入监听。
- Violentmonkey 共享租约锁覆盖所有协议，HTTPS 再叠加 Web Locks；两者共同覆盖批次准备、初始清理和完整账号队列。租约释放必须校验 owner，不能删除其它控制台的锁。
- 所有外部请求只允许 `auth.x.ai`，不加载远程 `@require` 或第三方资源。

## 兼容与回滚

- 用户脚本为独立文件，删除或禁用脚本即可回滚，不影响 Sub2API。
- xAI 页面结构变化时，低置信度检测会停止自动化而不是误点。
- Device Flow 官方契约变化时，错误会停留在当前账号并保留此前成功结果。
- `www.havefun.eu.cc` TLS/虚拟主机问题不由本任务修改；HTTP 控制台可运行，但风险提示不能隐藏，后续证书修复后可直接改用 HTTPS。

## 验证策略

- Node 纯逻辑测试覆盖解析、状态迁移、Token 错误分类、选择器候选评分、脱敏、动作门禁、可取消延迟动作和异常收尾投影。
- Node VM 浏览器 mock 真实执行用户脚本 `bootstrap()` 与隐藏登录驱动，覆盖模拟密码页填入/提交、取消后禁止补交、Cloudflare 只等待人工处理、Cloudflare 成功后等待稳定窗口再提交、Cloudflare 消失但页面暂未恢复时不提前 unknown、登录后落到 `/account` 时只写入 `authenticated_at` 且不点击账户设置、跳转后 URL hash / `window.name` 丢失时通过同标签 `sessionStorage` 恢复 `tab_marker`、控制台写入 `device_ready_at` 后立即跳转 Device Flow、设备码页提交当前 `user_code`、Grok Build 授权页点击“允许”、Token 成功后补足最终授权稳定窗口、密码已消费但页面仍停在密码表单时重按登录、站点存储清理成功与失败 ACK。
- GM API mock 测试覆盖 Cookie 适配器、共享事件/清理 ACK 过滤、domain Cookie 枚举与二次检查、Web Locks 独占、共享租约竞争/过期/释放、按 `run_id` 卸载清理和 HTTP/HTTPS/closed Shadow DOM 静态约束。
- 真实 xAI 页面、Cloudflare 和 Violentmonkey HttpOnly 权限只能在用户浏览器中手工验收；测试使用虚构账号和假 Token，不使用用户真实凭据。
