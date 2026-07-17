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

xAI Device Flow 契约以 `backend/internal/pkg/xai/oauth.go` 和
`backend/internal/pkg/xai/sso_device.go` 中的公开 client ID、scope、device code
端点和 Token 端点为核对来源。

### 3. Contracts

- 控制台只能在明确允许的 HTTPS host 渲染，并使用 `closed` Shadow DOM 隔离账号输入和 Token 结果；认证站点只运行隐藏驱动。
- 同一浏览器 Profile 的批次必须使用 Web Locks 独占；第二个控制台不得覆盖共享任务或交叉清理 Session。
- 登录驱动只有在 `run_id`、`account_id`、`tab_marker` 全部匹配且任务未取消、未过期时才能自动动作。标签标识只能放在 URL fragment 或标签私有状态中，禁止携带密码。
- Device Flow 只请求受信任的 HTTPS xAI 端点，处理 `authorization_pending`、`slow_down`、拒绝、过期、网络错误、超时和取消。业务结果只保留 refresh token，不持久化完整 Token 响应。
- Cloudflare、验证码、2FA 和未知安全确认只切换为人工等待状态。人工处理结束且页面重新成为高置信度登录/授权阶段后，驱动可以继续。
- 填表后的延迟动作必须按以下顺序执行：记录任务归属和调度 URL -> 等待有限延迟 -> 重新校验共享任务、标签、URL 和非 challenge 状态 -> 占用动作门禁次数 -> 点击高置信度按钮或派发 Enter。
- 守卫拒绝不得消耗动作门禁次数。否则同一 URL 临时进入 challenge 后，即使用户完成验证，也会因为旧定时器占用次数而永久无法恢复。
- 密码提交后立即从共享任务删除并写入 `password_consumed_at`。停止或跳过写入 `cancelled_at` 并删除密码；两个字段可以共存，表示提交后任务又被取消。
- 每个账号结束后必须关闭脚本打开的登录标签，清除目标域 localStorage、sessionStorage、IndexedDB、Cache Storage、Service Worker 和 Cookie，并二次验证 Cookie。任一清理失败必须阻断队列。
- 测试、日志、文档和导出内容不得包含真实账号密码、access token、ID token、完整 refresh token 或上游错误正文。

### 4. Validation & Error Matrix

| 条件 | 必须行为 |
| --- | --- |
| 当前页面不是 HTTPS 控制台 host | 不渲染控制台 |
| 任务、账号或标签标识不匹配 | 不填表、不点击、不清理其它标签的数据 |
| 任务过期、停止或跳过 | 取消待执行动作并删除共享密码 |
| 延迟期间 URL 变化 | 旧动作不执行，且不标记密码已提交 |
| 延迟期间出现 challenge | 旧动作不执行，门禁次数保持可用 |
| 同 URL challenge 完成 | 重新扫描后允许提交一次，后续重复扫描不得再次提交 |
| `authorization_pending` | 保持当前轮询间隔 |
| `slow_down` | 在上限内增加轮询间隔 |
| Token 成功但缺少 refresh token | 当前账号失败，不导出 access token |
| Cookie、站点存储或清理 ACK 失败 | 保持清理失败状态并停止后续账号 |
| Violentmonkey 或 HttpOnly 权限不足 | 批次开始前失败，不处理任何账号 |

### 5. Good/Base/Bad Cases

- Good：密码页填入后短暂出现 Cloudflare 文案，旧定时器被守卫拒绝；用户完成验证后，同一 URL 重新扫描并只提交一次。
- Good：用户点击停止时，共享任务先投影为不含密码的取消状态，登录驱动随后取消定时器，控制台再中止 Token 请求并清理 Session。
- Good：清理 ACK 同时匹配批次、账号、`cleanup_id` 和 host，Cookie 删除后二次 domain 枚举为空才进入下一账号。
- Base：普通邮箱页或密码页在 URL、任务和标签稳定时自动提交一次。
- Base：页面结构低置信度时仅上报未知页面，用户可停止、跳过或人工处理。
- Bad：创建延迟定时器时立即占用动作次数，导致守卫拒绝后无法恢复。
- Bad：只删除当前 URL 可见 Cookie，遗漏其它 path、子域、HttpOnly 或分区 Cookie。
- Bad：把整批账号密码写入 localStorage、GM 共享值、页面 DOM 属性、URL 或日志。

### 6. Tests Required

- 纯逻辑测试至少覆盖：输入解析、邮箱去重、Token 响应分类、轮询退避、挑战页识别、动作门禁、任务归属、密码消费/取消投影、Cookie domain 枚举与二次校验。
- Node VM 浏览器状态机测试至少覆盖：
  - 密码填入并提交一次，提交后共享密码删除。
  - 共享任务取消后待执行动作被取消。
  - URL 变化或 challenge 出现后，按钮和 Enter 都不触发，密码不标记为已提交。
  - 同 URL challenge 消失后重新扫描、提交一次，后续扫描不重复提交。
  - Cloudflare 页面只上报人工等待。
  - 站点存储清理成功和失败 ACK。
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
