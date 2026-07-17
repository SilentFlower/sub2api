// ==UserScript==
// @name         Grok 批量登录助手
// @namespace    https://www.havefun.eu.cc/
// @version      0.2.10
// @description  在指定控制台页面串行登录 Grok/xAI 账号，通过官方 Device Flow 导出 refresh token。
// @author       silentflower
// @homepageURL  https://www.havefun.eu.cc/
// @match        http://www.havefun.eu.cc/*
// @match        https://www.havefun.eu.cc/*
// @include      http://www.havefun.eu.cc:8080/*
// @match        https://x.ai/*
// @match        https://*.x.ai/*
// @match        https://grok.com/*
// @match        https://*.grok.com/*
// @grant        GM_info
// @grant        GM_getValue
// @grant        GM_setValue
// @grant        GM_deleteValue
// @grant        GM_addValueChangeListener
// @grant        GM_removeValueChangeListener
// @grant        GM_openInTab
// @grant        GM_xmlhttpRequest
// @grant        GM_cookie
// @grant        GM_setClipboard
// @connect      auth.x.ai
// @run-at       document-start
// @inject-into  content
// @noframes
// ==/UserScript==

(function () {
  'use strict'

  const CONFIG = Object.freeze({
    controllerHosts: new Set(['www.havefun.eu.cc']),
    authHosts: new Set(['auth.x.ai', 'accounts.x.ai', 'x.ai', 'grok.com']),
    clientId: 'b1a00492-073a-47ea-816f-4c329264a828',
    scope: 'openid profile email offline_access grok-cli:access api:access conversations:read conversations:write',
    deviceCodeUrl: 'https://auth.x.ai/oauth2/device/code',
    tokenUrl: 'https://auth.x.ai/oauth2/token',
    loginStartUrl: 'https://accounts.x.ai/sign-in',
    storageUrls: [
      'https://x.ai/',
      // auth.x.ai 根路径在真实 Chrome 中可能直接显示 404，清理页改用同源授权端点承载。
      'https://auth.x.ai/oauth2/authorize',
      'https://accounts.x.ai/',
      'https://grok.com/'
    ],
    cookieDomains: ['.x.ai', '.grok.com'],
    sharedKeys: Object.freeze({
      task: 'grok-bulk-login:current-task',
      event: 'grok-bulk-login:driver-event',
      cleanup: 'grok-bulk-login:cleanup-request',
      cleanupAck: 'grok-bulk-login:cleanup-ack'
    }),
    taskTtlMs: 30 * 60 * 1000,
    cleanupAckTimeoutMs: 10000,
    pageUnknownTimeoutMs: 12000,
    scanDebounceMs: 350,
    scanIntervalMs: 1400,
    maxActionAttemptsPerStage: 1,
    accountCooldownMs: 1800,
    requestTimeoutMs: 30000,
    maxDeviceFlowMs: 30 * 60 * 1000,
    minPollMs: 1000,
    maxPollMs: 30000,
    loginToVerificationDelayMs: 1500,
    challengePassedGraceMs: 5000,
    postChallengeUnknownGraceMs: 60000,
    challengeRecheckMs: 1000,
    actionReadyRetryMs: 800,
    controllerLockName: 'grok-bulk-login:controller-v1',
    controllerLeaseClaimDelayMs: 180,
    controllerLeaseHeartbeatMs: 10000,
    controllerLeaseTtlMs: 60000
  })

  const DRIVER_MARKER_KEY = 'grok-bulk-login'
  const DRIVER_WINDOW_PREFIX = `${DRIVER_MARKER_KEY}:`
  const CLEANUP_MARKER_KEY = 'grok-bulk-cleanup'
  const CLEANUP_WINDOW_PREFIX = `${CLEANUP_MARKER_KEY}:`

  const STATUS_LABELS = Object.freeze({
    pending: '等待处理',
    requesting_device: '创建授权任务',
    opening_login: '打开登录页',
    filling_email: '填写邮箱',
    filling_password: '填写密码',
    waiting_human: '等待人工验证',
    polling_token: '等待授权结果',
    success: '成功',
    failed: '失败',
    cleaning: '清理 Session',
    skipped: '已跳过'
  })

  const ERROR_MESSAGES = Object.freeze({
    ACCESS_DENIED: '授权被拒绝',
    ACCOUNT_SKIPPED: '用户跳过当前账号',
    CLEANUP_ACK_TIMEOUT: '登录页没有确认站点存储清理',
    CLEANUP_STORAGE_FAILED: '目标域站点存储清理失败',
    CLEANUP_TAB_OPEN_FAILED: '无法打开目标域清理标签',
    CLEANUP_COOKIE_REMAINS: '目标域仍存在 Cookie，已停止队列',
    CLEANUP_FAILED: 'Session 清理失败，已停止队列',
    DEVICE_CODE_EXPIRED: '授权任务已过期',
    DEVICE_CODE_INVALID: 'xAI 返回的 Device Flow 数据不完整',
    DEVICE_CODE_REQUEST_FAILED: '创建 xAI 授权任务失败',
    DEVICE_FLOW_TIMEOUT: '等待授权超时',
    HTTPONLY_PERMISSION_REQUIRED: '未开启 Violentmonkey HttpOnly Cookie 权限',
    INVALID_TOKEN_RESPONSE: 'xAI Token 响应格式无效',
    LOGIN_FAILED: 'xAI 登录失败',
    LOGIN_TAB_CLOSED: '登录标签被关闭',
    NETWORK_ERROR: '网络请求失败',
    PAGE_UNKNOWN: '无法识别当前 xAI 页面',
    SCRIPT_STOPPED: '批次已停止',
    TOKEN_MISSING_REFRESH: 'Token 响应缺少 refresh token',
    TOKEN_REQUEST_FAILED: '轮询 xAI Token 失败',
    CONTROLLER_LOCK_FAILED: '无法建立控制台独占锁，未处理任何账号',
    VM_API_MISSING: 'Violentmonkey API 不完整'
  })

  const TRANSITIONS = Object.freeze({
    pending: new Set(['requesting_device', 'skipped']),
    requesting_device: new Set(['opening_login', 'failed', 'skipped', 'cleaning']),
    opening_login: new Set(['filling_email', 'filling_password', 'waiting_human', 'polling_token', 'success', 'failed', 'skipped', 'cleaning']),
    filling_email: new Set(['filling_password', 'waiting_human', 'polling_token', 'success', 'failed', 'skipped', 'cleaning']),
    filling_password: new Set(['waiting_human', 'polling_token', 'success', 'failed', 'skipped', 'cleaning']),
    waiting_human: new Set(['filling_email', 'filling_password', 'polling_token', 'success', 'failed', 'skipped', 'cleaning']),
    polling_token: new Set(['waiting_human', 'success', 'failed', 'skipped', 'cleaning']),
    success: new Set(['cleaning']),
    failed: new Set(['cleaning', 'pending', 'requesting_device']),
    skipped: new Set(['cleaning', 'pending', 'requesting_device']),
    cleaning: new Set(['success', 'failed', 'skipped', 'pending'])
  })

  /**
   * 规范化邮箱，用于批次内去重。
   * @param {string} value 原始邮箱。
   * @return {string} 去除两端空白并转为小写的邮箱。
   */
  function normalizeEmail(value) {
    return String(value || '').trim().toLowerCase()
  }

  /**
   * 判断当前位置是否为允许展示控制台的 HTTP/HTTPS 地址。
   * @param {string} protocol 页面协议。
   * @param {string} hostname 页面主机名。
   * @return {boolean} 是否为允许的控制台地址。
   */
  function isControllerLocation(protocol, hostname) {
    return (protocol === 'http:' || protocol === 'https:') && CONFIG.controllerHosts.has(String(hostname || ''))
  }

  /**
   * 解析多行账号输入，只按每行第一个竖线分隔。
   * @param {string} input 多行账号文本。
   * @return {{accounts: Array<object>, errors: Array<object>}} 合法账号和行级错误。
   */
  function parseAccounts(input) {
    const accounts = []
    const errors = []
    const seen = new Map()

    String(input || '').split(/\r?\n/).forEach((rawLine, index) => {
      const lineNumber = index + 1
      const line = rawLine.trim()
      if (!line) return

      const separatorIndex = line.indexOf('|')
      if (separatorIndex < 0) {
        errors.push({ line: lineNumber, code: 'MISSING_SEPARATOR', message: '缺少 | 分隔符' })
        return
      }

      const email = line.slice(0, separatorIndex).trim()
      const password = line.slice(separatorIndex + 1).trim()
      if (!email) {
        errors.push({ line: lineNumber, code: 'EMAIL_REQUIRED', message: '邮箱为空' })
        return
      }
      if (!password) {
        errors.push({ line: lineNumber, code: 'PASSWORD_REQUIRED', message: '密码为空' })
        return
      }

      const normalizedEmail = normalizeEmail(email)
      if (seen.has(normalizedEmail)) {
        errors.push({
          line: lineNumber,
          code: 'DUPLICATE_EMAIL',
          message: `与第 ${seen.get(normalizedEmail)} 行邮箱重复`
        })
        return
      }

      seen.set(normalizedEmail, lineNumber)
      accounts.push({
        id: `account-${lineNumber}-${accounts.length + 1}`,
        line: lineNumber,
        email,
        normalizedEmail,
        password,
        status: 'pending',
        errorCode: '',
        refreshToken: ''
      })
    })

    return { accounts, errors }
  }

  /**
   * 对邮箱进行脱敏展示。
   * @param {string} email 邮箱。
   * @return {string} 脱敏邮箱。
   */
  function maskEmail(email) {
    const value = String(email || '')
    const at = value.indexOf('@')
    if (at <= 0) return '***'
    const local = value.slice(0, at)
    const domain = value.slice(at + 1)
    const visible = local.length <= 2 ? local.slice(0, 1) : local.slice(0, 2)
    return `${visible}***@${domain}`
  }

  /**
   * 判断账号状态迁移是否合法。
   * @param {string} from 当前状态。
   * @param {string} to 目标状态。
   * @return {boolean} 是否允许迁移。
   */
  function canTransition(from, to) {
    if (from === to) return true
    return Boolean(TRANSITIONS[from] && TRANSITIONS[from].has(to))
  }

  /**
   * 分类 Device Flow Token 响应。
   * @param {number} status HTTP 状态码。
   * @param {object|null} payload JSON 响应。
   * @return {{kind: string, code: string}} 分类结果。
   */
  function classifyTokenResponse(status, payload) {
    const body = payload && typeof payload === 'object' ? payload : {}
    if (status >= 200 && status < 300 && body.access_token) {
      return { kind: 'success', code: 'TOKEN_READY' }
    }
    switch (String(body.error || '')) {
      case 'authorization_pending':
        return { kind: 'pending', code: 'AUTHORIZATION_PENDING' }
      case 'slow_down':
        return { kind: 'slow_down', code: 'SLOW_DOWN' }
      case 'access_denied':
        return { kind: 'terminal', code: 'ACCESS_DENIED' }
      case 'expired_token':
        return { kind: 'terminal', code: 'DEVICE_CODE_EXPIRED' }
      default:
        return { kind: 'error', code: 'TOKEN_REQUEST_FAILED' }
    }
  }

  /**
   * 根据轮询结果计算下一次等待时间。
   * @param {number} currentMs 当前毫秒间隔。
   * @param {string} kind 响应分类。
   * @return {number} 下一次毫秒间隔。
   */
  function nextPollDelay(currentMs, kind) {
    const safeCurrent = Math.min(Math.max(Number(currentMs) || CONFIG.minPollMs, CONFIG.minPollMs), CONFIG.maxPollMs)
    if (kind === 'slow_down') return Math.min(safeCurrent + 5000, CONFIG.maxPollMs)
    return safeCurrent
  }

  /**
   * 为输入框描述计算语义匹配分数。
   * @param {object} descriptor 输入框描述。
   * @param {'email'|'password'|'user_code'} kind 目标类型。
   * @return {number} 匹配分数。
   */
  function scoreInputDescriptor(descriptor, kind) {
    const text = [
      descriptor.type,
      descriptor.name,
      descriptor.id,
      descriptor.autocomplete,
      descriptor.placeholder,
      descriptor.ariaLabel,
      descriptor.label,
      descriptor.nearbyText
    ].map(value => String(value || '').toLowerCase()).join(' ')
    let score = 0

    if (kind === 'email') {
      if (descriptor.type === 'email') score += 12
      if (/\b(email|e-mail|邮箱|邮件)\b/.test(text)) score += 8
      if (/\b(username|user-name|账号|账户)\b/.test(text)) score += 5
      if (/\b(username|email)\b/.test(String(descriptor.autocomplete || '').toLowerCase())) score += 8
      if (descriptor.type === 'password') score -= 20
    } else if (kind === 'password') {
      if (descriptor.type === 'password') score += 14
      if (/\b(password|passwd|密码)\b/.test(text)) score += 8
      if (/current-password/.test(String(descriptor.autocomplete || '').toLowerCase())) score += 8
    } else if (kind === 'user_code') {
      if (/user.?code|device.?code|设备代码|授权代码/.test(text)) score += 12
      if (/verification.?code|one-time-code|验证码/.test(text)) score -= 8
      if (descriptor.type === 'password') score -= 20
    }

    if (descriptor.disabled || descriptor.readOnly || descriptor.hidden) score -= 100
    return score
  }

  /**
   * 从候选描述中选择唯一高置信度输入框。
   * @param {Array<object>} descriptors 候选描述。
   * @param {'email'|'password'|'user_code'} kind 目标类型。
   * @param {number} [minimumScore] 最低分数。
   * @return {object|null} 唯一最佳候选；并列或低分时返回 null。
   */
  function chooseBestDescriptor(descriptors, kind, minimumScore = 8) {
    const ranked = descriptors
      .map(descriptor => ({ descriptor, score: scoreInputDescriptor(descriptor, kind) }))
      .filter(item => item.score >= minimumScore)
      .sort((a, b) => b.score - a.score)
    if (!ranked.length) return null
    if (ranked.length > 1 && ranked[0].score === ranked[1].score) return null
    return ranked[0].descriptor
  }

  /**
   * 判断页面快照是否显示 Cloudflare / Turnstile 已通过。
   * @param {{url?: string, title?: string, text?: string, iframeSrcs?: Array<string>}} snapshot 页面快照。
   * @return {boolean} 是否已通过 Cloudflare / Turnstile。
   */
  function isChallengePassedSnapshot(snapshot) {
    const combined = [snapshot.url, snapshot.title, snapshot.text, ...(snapshot.iframeSrcs || [])]
      .map(value => String(value || '').toLowerCase())
      .join(' ')
    const hasChallengeContext = [
      'cloudflare',
      'turnstile',
      'challenges.cloudflare.com',
      'cf-turnstile',
      'cf-chl-'
    ].some(keyword => combined.includes(keyword))
    if (!hasChallengeContext) return false
    return [
      'success',
      'successful',
      'verified',
      'verification passed',
      'challenge passed',
      '成功',
      '验证成功',
      '已验证',
      '已通过'
    ].some(keyword => combined.includes(keyword))
  }

  /**
   * 判断页面快照是否属于 Cloudflare 或验证码人工验证。
   * @param {{url?: string, title?: string, text?: string, iframeSrcs?: Array<string>}} snapshot 页面快照。
   * @return {boolean} 是否需要人工处理。
   */
  function isChallengeSnapshot(snapshot) {
    if (isChallengePassedSnapshot(snapshot)) return false
    const combined = [snapshot.url, snapshot.title, snapshot.text, ...(snapshot.iframeSrcs || [])]
      .map(value => String(value || '').toLowerCase())
      .join(' ')
    const challengeKeywords = [
      'challenges.cloudflare.com',
      'cf-chl-',
      'cf-turnstile',
      'hcaptcha',
      'recaptcha',
      'captcha',
      'verify you are human',
      'checking your browser',
      'attention required',
      '验证您是真人',
      '正在检查您的浏览器',
      '人机验证'
    ]
    if (challengeKeywords.some(keyword => combined.includes(keyword))) return true

    let isDeviceFlowPage = false
    try {
      isDeviceFlowPage = isDeviceVerificationPath(new URL(String(snapshot.url || '')).pathname)
    } catch {
      // URL 不完整时按普通登录页处理，宁可暂停也不猜测安全验证流程。
    }
    if (isDeviceFlowPage) return false

    return [
      'two-factor authentication',
      'two factor authentication',
      'multi-factor authentication',
      'one-time code',
      'verification code',
      'authenticator app',
      'security verification',
      'enter the code',
      '2fa',
      'mfa',
      '两步验证',
      '双重验证',
      '验证码',
      '动态口令',
      '身份验证器',
      '安全验证'
    ].some(keyword => combined.includes(keyword))
  }

  /**
   * 将成功结果格式化为一行一个 refresh token。
   * @param {Array<object>} accounts 账号结果。
   * @return {string} 可复制的 Token 文本。
   */
  function formatRefreshTokens(accounts) {
    return accounts
      .filter(account => account && account.refreshToken)
      .map(account => String(account.refreshToken).trim())
      .filter(Boolean)
      .join('\n')
  }

  /**
   * 生成删除 Cookie 所需的 URL 和分区信息。
   * @param {object} cookie Violentmonkey Cookie 对象。
   * @param {string} fallbackUrl 枚举 Cookie 时使用的 URL。
   * @return {object} `GM_cookie.delete` 参数。
   */
  function buildCookieDeleteDetails(cookie, fallbackUrl) {
    const fallback = new URL(fallbackUrl)
    const host = String(cookie.domain || fallback.hostname).replace(/^\./, '')
    const scheme = fallback.protocol === 'https:' ? 'https:' : (cookie.secure === false ? 'http:' : 'https:')
    const path = String(cookie.path || '/')
    const details = {
      url: `${scheme}//${host}${path.startsWith('/') ? path : `/${path}`}`,
      name: cookie.name
    }
    if (cookie.firstPartyDomain) details.firstPartyDomain = cookie.firstPartyDomain
    if (cookie.partitionKey) details.partitionKey = cookie.partitionKey
    return details
  }

  /**
   * 清理跨页事件中的非敏感描述。
   * @param {unknown} detail 原始描述。
   * @return {string} 截断且去除换行后的描述。
   */
  function sanitizeDetail(detail) {
    return String(detail || '').replace(/[\r\n\t]+/g, ' ').trim().slice(0, 160)
  }

  /**
   * 判断 Device Flow 返回的验证地址是否属于受信任的 xAI HTTPS 域。
   * @param {unknown} value 待检查地址。
   * @return {boolean} 是否可安全打开。
   */
  function isTrustedVerificationUrl(value) {
    try {
      const url = new URL(String(value || ''))
      return url.protocol === 'https:' && (url.hostname === 'x.ai' || url.hostname.endsWith('.x.ai'))
    } catch {
      return false
    }
  }

  /**
   * 从 Device Flow 响应中选择官方验证页，优先使用标准 verification_uri。
   * @param {object} payload Device Flow 响应体。
   * @return {string} 可信验证页；缺失时为空字符串。
   */
  function selectTrustedVerificationUrl(payload) {
    if (!payload || typeof payload !== 'object') return ''
    const candidates = [payload.verification_uri, payload.verification_uri_complete]
    for (const candidate of candidates) {
      if (isTrustedVerificationUrl(candidate)) return String(candidate)
    }
    return ''
  }

  /**
   * 判断路径是否为 xAI Device Flow 验证页或其子路径。
   * @param {string} pathname 当前路径。
   * @return {boolean} 是否为设备授权路径。
   */
  function isDeviceVerificationPath(pathname) {
    return /^\/oauth2\/device(?:\/|$)/.test(String(pathname || ''))
  }

  /**
   * 判断密码是否已经提交并从共享任务删除。
   * @param {object|null} task 当前共享任务。
   * @return {boolean} 密码是否已提交。
   */
  function hasPasswordBeenSubmitted(task) {
    return Boolean(task
      && !Object.prototype.hasOwnProperty.call(task, 'password')
      && Object.prototype.hasOwnProperty.call(task, 'password_consumed_at'))
  }

  /**
   * 判断未完成邮箱密码登录时是否应从 Device 页返回邮箱登录入口。
   * @param {object|null} task 当前共享任务。
   * @param {string} currentHref 当前页面地址。
   * @return {boolean} 是否应回到邮箱登录入口。
   */
  function shouldReturnToLoginBeforePassword(task, currentHref) {
    if (!task || hasPasswordBeenSubmitted(task) || !Object.prototype.hasOwnProperty.call(task, 'password')) return false
    try {
      const current = new URL(String(currentHref || ''))
      return current.protocol === 'https:'
        && (current.hostname === 'x.ai' || current.hostname.endsWith('.x.ai'))
        && isDeviceVerificationPath(current.pathname)
    } catch {
      return false
    }
  }

  /**
   * 判断登录入口在无可识别控件时是否应进入官方 Device Flow 验证页。
   * @param {object|null} task 当前共享任务。
   * @param {string} currentHref 当前页面地址。
   * @param {number} now 当前时间。
   * @param {number} firstSeenAt 当前页面首次扫描时间。
   * @param {boolean} hasRecognizedControls 页面是否已有可处理的登录/授权控件。
   * @return {boolean} 是否应跳转。
   */
  function shouldNavigateToVerification(task, currentHref, now, firstSeenAt, hasRecognizedControls) {
    if (!task || hasRecognizedControls || !isTrustedVerificationUrl(task.verification_url)) return false
    if (!hasPasswordBeenSubmitted(task)) return false
    if (Number(now) - Number(firstSeenAt) < CONFIG.loginToVerificationDelayMs) return false

    try {
      const current = new URL(String(currentHref || ''))
      const target = new URL(String(task.verification_url))
      const currentHostAllowed = current.hostname === 'x.ai'
        || current.hostname.endsWith('.x.ai')
        || current.hostname === 'grok.com'
        || current.hostname.endsWith('.grok.com')
      if (current.protocol !== 'https:' || !currentHostAllowed) return false
      if (current.origin === target.origin && isDeviceVerificationPath(current.pathname)) return false
      return true
    } catch {
      return false
    }
  }

  /**
   * 判断当前位置是否是 xAI 登录成功后的账户页。
   * @param {string} currentHref 当前页面地址。
   * @return {boolean} 是否为已登录账户页。
   */
  function isAuthenticatedAccountLanding(currentHref) {
    try {
      const current = new URL(String(currentHref || ''))
      return current.protocol === 'https:'
        && (current.hostname === 'x.ai' || current.hostname.endsWith('.x.ai'))
        && /^\/account(?:\/|$)/.test(current.pathname)
    } catch {
      return false
    }
  }

  /**
   * 判断登录成功落到账户页时是否应强制跳转 Device Flow 验证页。
   * @param {object|null} task 当前共享任务。
   * @param {string} currentHref 当前页面地址。
   * @param {number} now 当前时间。
   * @param {number} firstSeenAt 当前页面首次扫描时间。
   * @return {boolean} 是否应跳转。
   */
  function shouldNavigateFromAuthenticatedLanding(task, currentHref, now, firstSeenAt) {
    if (!isAuthenticatedAccountLanding(currentHref)) return false
    return shouldNavigateToVerification(task, currentHref, now, firstSeenAt, false)
  }

  /**
   * 为脚本打开的标签追加只保存在 URL fragment 中的随机归属标记。
   * @param {string} value 原始地址。
   * @param {string} marker 标签随机标记。
   * @return {string} 带归属标记的地址。
   */
  function appendHashMarker(value, markerKey, marker) {
    const url = new URL(value)
    const markerPart = `${markerKey}=${encodeURIComponent(marker)}`
    const currentHash = url.hash.slice(1)
    url.hash = currentHash ? `${currentHash}&${markerPart}` : markerPart
    return url.toString()
  }

  /**
   * 为脚本打开的登录标签追加只保存在 URL fragment 中的随机归属标记。
   * @param {string} value 原始地址。
   * @param {string} marker 标签随机标记。
   * @return {string} 带登录归属标记的地址。
   */
  function appendDriverMarker(value, marker) {
    return appendHashMarker(value, DRIVER_MARKER_KEY, marker)
  }

  /**
   * 为脚本打开的清理标签追加独立清理标记，避免被误认为登录/授权标签。
   * @param {string} value 原始地址。
   * @param {string} marker 清理标签随机标记。
   * @return {string} 带清理归属标记的地址。
   */
  function appendCleanupMarker(value, marker) {
    return appendHashMarker(value, CLEANUP_MARKER_KEY, marker)
  }

  /**
   * 从 URL fragment 中读取指定归属标记。
   * @param {string} hash URL fragment。
   * @param {string} markerKey 标记键名。
   * @return {string} 找到的归属标记；不存在时为空字符串。
   */
  function extractHashMarker(hash, markerKey) {
    const value = String(hash || '').replace(/^#/, '')
    const escapedKey = String(markerKey).replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    const match = value.match(new RegExp(`(?:^|&)${escapedKey}=([^&]+)(?:&|$)`))
    if (!match) return ''
    try {
      return decodeURIComponent(match[1])
    } catch {
      return ''
    }
  }

  /**
   * 从 URL fragment 中读取脚本标签归属标记。
   * @param {string} hash URL fragment。
   * @return {string} 找到的归属标记；不存在时为空字符串。
   */
  function extractDriverMarker(hash) {
    return extractHashMarker(hash, DRIVER_MARKER_KEY)
  }

  /**
   * 从 URL fragment 中读取清理标签归属标记。
   * @param {string} hash URL fragment。
   * @return {string} 找到的清理标记；不存在时为空字符串。
   */
  function extractCleanupMarker(hash) {
    return extractHashMarker(hash, CLEANUP_MARKER_KEY)
  }

  /**
   * 判断共享任务是否已经超过有效期。
   * @param {object|null} task 共享任务。
   * @param {number} [now] 当前时间戳。
   * @return {boolean} 是否已过期。
   */
  function isExpiredSharedTask(task, now = Date.now()) {
    return Boolean(task && Number(task.expires_at || 0) > 0 && Number(task.expires_at) <= now)
  }

  /**
   * 生成 Cookie 去重标识，避免遗漏 Firefox First-Party Isolation 分区。
   * @param {object} cookie Violentmonkey Cookie 对象。
   * @return {string} Cookie 唯一标识。
   */
  function cookieIdentity(cookie) {
    const partition = cookie.partitionKey ? JSON.stringify(cookie.partitionKey) : ''
    return [cookie.domain, cookie.path, cookie.name, cookie.firstPartyDomain || '', partition].join('|')
  }

  /**
   * 创建可注入、可测试的 Violentmonkey Cookie Promise 适配器。
   * @param {object} cookieApi `GM_cookie` API。
   * @return {{list: function(object): Promise<Array<object>>, set: function(object): Promise<void>, delete: function(object): Promise<void>}} Cookie 适配器。
   */
  function createCookieAdapter(cookieApi) {
    return {
      list(details) {
        return new Promise((resolve, reject) => {
          try {
            cookieApi.list(details, (cookies, error) => {
              if (error) reject(new Error(String(error)))
              else resolve(Array.isArray(cookies) ? cookies : [])
            })
          } catch (error) {
            reject(error)
          }
        })
      },
      set(details) {
        return new Promise((resolve, reject) => {
          try {
            cookieApi.set(details, error => {
              if (error) reject(new Error(String(error)))
              else resolve()
            })
          } catch (error) {
            reject(error)
          }
        })
      },
      delete(details) {
        return new Promise((resolve, reject) => {
          try {
            cookieApi.delete(details, error => {
              if (error) reject(new Error(String(error)))
              else resolve()
            })
          } catch (error) {
            reject(error)
          }
        })
      }
    }
  }

  /**
   * 按 Cookie domain 全量收集目标 Cookie，覆盖路径限定和子域 Cookie。
   * @param {{list: function(object): Promise<Array<object>>}} cookieAdapter Cookie 查询适配器。
   * @param {Array<string>} domains 允许清理的 Cookie domain。
   * @return {Promise<Array<{cookie: object, fallbackUrl: string}>>} 去重后的 Cookie。
   */
  async function collectTargetCookies(cookieAdapter, domains) {
    const collected = []
    const seen = new Set()
    for (const domain of domains) {
      const cookies = await cookieAdapter.list({ domain })
      const fallbackUrl = `https://${String(domain).replace(/^\./, '')}/`
      for (const cookie of cookies) {
        const key = cookieIdentity(cookie)
        if (seen.has(key)) continue
        seen.add(key)
        collected.push({ cookie, fallbackUrl })
      }
    }
    return collected
  }

  /**
   * 删除目标 Cookie 并再次按 domain 枚举，供运行时门禁和 mock 测试复用。
   * @param {{list: function(object): Promise<Array<object>>, delete: function(object): Promise<void>}} cookieAdapter Cookie 适配器。
   * @param {Array<string>} domains 允许清理的 Cookie domain。
   * @return {Promise<{deletedCount: number, remaining: Array<object>}>} 删除数量和二次枚举残留。
   */
  async function clearAndVerifyTargetCookies(cookieAdapter, domains) {
    const cookies = await collectTargetCookies(cookieAdapter, domains)
    for (const item of cookies) {
      await cookieAdapter.delete(buildCookieDeleteDetails(item.cookie, item.fallbackUrl))
    }
    const remaining = await collectTargetCookies(cookieAdapter, domains)
    return { deletedCount: cookies.length, remaining }
  }

  /**
   * 创建按动作键限制最大尝试次数的门禁。
   * @param {number} maxAttempts 每个动作键允许的最大次数。
   * @return {{tryAcquire: function(string): boolean, attempts: function(string): number}} 动作门禁。
   */
  function createActionGate(maxAttempts = 1) {
    const limit = Math.max(1, Math.floor(Number(maxAttempts) || 1))
    const attemptsByKey = new Map()
    return {
      tryAcquire(key) {
        const normalizedKey = String(key || '')
        const attempts = attemptsByKey.get(normalizedKey) || 0
        if (attempts >= limit) return false
        attemptsByKey.set(normalizedKey, attempts + 1)
        return true
      },
      attempts(key) {
        return attemptsByKey.get(String(key || '')) || 0
      }
    }
  }

  /**
   * 创建只能保留一个待执行动作的可取消延迟控制器。
   * @param {{setTimeout?: function(function, number): unknown, clearTimeout?: function(unknown): void}} [timerApi] 可注入的定时器 API。
   * @return {{schedule: function(number, function(): boolean, function(): void): void, cancel: function(): void, pending: function(): boolean}} 延迟动作控制器。
   */
  function createDeferredActionController(timerApi = {}) {
    const scheduleTimer = timerApi.setTimeout || ((callback, delayMs) => setTimeout(callback, delayMs))
    const cancelTimer = timerApi.clearTimeout || (timer => clearTimeout(timer))
    let timer = null

    function cancel() {
      if (timer === null) return
      cancelTimer(timer)
      timer = null
    }

    return {
      schedule(delayMs, guard, action) {
        cancel()
        timer = scheduleTimer(() => {
          timer = null
          if (typeof guard === 'function' && !guard()) return
          action()
        }, Math.max(0, Number(delayMs) || 0))
      },
      cancel,
      pending() {
        return timer !== null
      }
    }
  }

  /**
   * 使用 Web Locks API 尝试独占执行控制台批次，不等待其它控制台释放。
   * @param {{request: function(string, object, function): Promise<unknown>}} lockManager Web Locks 管理器。
   * @param {string} lockName 独占锁名称。
   * @param {function(): Promise<void>} callback 获得锁后执行的批次函数。
   * @return {Promise<boolean>} 是否获得锁并执行了批次。
   */
  async function runWithExclusiveLock(lockManager, lockName, callback) {
    const result = await lockManager.request(lockName, { mode: 'exclusive', ifAvailable: true }, async lock => {
      if (!lock) return false
      await callback()
      return true
    })
    return result === true
  }

  /**
   * 使用 Violentmonkey 共享值租约尝试独占执行批次。
   * @param {{get: function(string): object|null, set: function(string, object): void, delete: function(string): void}} leaseStore 共享值适配器。
   * @param {string} lockName 租约键名。
   * @param {string} ownerId 当前控制台随机 owner。
   * @param {function(): Promise<void>} callback 获得租约后执行的批次函数。
   * @param {{now?: function(): number, wait?: function(number): Promise<void>, setInterval?: function(function, number): unknown, clearInterval?: function(unknown): void, claimDelayMs?: number, heartbeatMs?: number, ttlMs?: number, onLost?: function(): void}} [options] 可注入的租约参数。
   * @return {Promise<boolean>} 是否获得并持有租约直到批次结束。
   */
  async function runWithSharedLeaseLock(leaseStore, lockName, ownerId, callback, options = {}) {
    const now = options.now || (() => Date.now())
    const waitFor = options.wait || (delayMs => new Promise(resolve => setTimeout(resolve, delayMs)))
    const scheduleHeartbeat = options.setInterval || ((handler, delayMs) => setInterval(handler, delayMs))
    const cancelHeartbeat = options.clearInterval || (timer => clearInterval(timer))
    const configuredTtlMs = Number(options.ttlMs)
    const configuredHeartbeatMs = Number(options.heartbeatMs)
    const configuredClaimDelayMs = Number(options.claimDelayMs)
    const ttlMs = Math.max(1000, Number.isFinite(configuredTtlMs) ? configuredTtlMs : CONFIG.controllerLeaseTtlMs)
    const heartbeatMs = Math.max(250, Math.min(Number.isFinite(configuredHeartbeatMs) ? configuredHeartbeatMs : CONFIG.controllerLeaseHeartbeatMs, ttlMs - 1))
    const claimDelayMs = Math.max(0, Number.isFinite(configuredClaimDelayMs) ? configuredClaimDelayMs : CONFIG.controllerLeaseClaimDelayMs)
    const normalizedOwner = String(ownerId || '')
    if (!normalizedOwner) return false

    const current = leaseStore.get(lockName)
    if (current && current.owner !== normalizedOwner && Number(current.expires_at) > now()) return false

    const acquiredAt = now()
    leaseStore.set(lockName, {
      owner: normalizedOwner,
      acquired_at: acquiredAt,
      expires_at: acquiredAt + ttlMs
    })

    // GM 共享值没有 compare-and-swap；等待竞争窗口后只允许最终 owner 进入批次。
    await waitFor(claimDelayMs)
    const confirmed = leaseStore.get(lockName)
    if (!confirmed || confirmed.owner !== normalizedOwner || Number(confirmed.expires_at) <= now()) return false

    let leaseLost = false
    let heartbeat = null

    function markLeaseLost() {
      if (leaseLost) return
      leaseLost = true
      if (heartbeat !== null) cancelHeartbeat(heartbeat)
      if (typeof options.onLost === 'function') options.onLost()
    }

    heartbeat = scheduleHeartbeat(() => {
      try {
        const active = leaseStore.get(lockName)
        if (!active || active.owner !== normalizedOwner) {
          markLeaseLost()
          return
        }
        leaseStore.set(lockName, {
          ...active,
          expires_at: now() + ttlMs
        })
      } catch {
        markLeaseLost()
      }
    }, heartbeatMs)

    try {
      await callback()
      return !leaseLost
    } finally {
      if (heartbeat !== null) cancelHeartbeat(heartbeat)
      try {
        const latest = leaseStore.get(lockName)
        if (latest && latest.owner === normalizedOwner) leaseStore.delete(lockName)
      } catch {
        // 租约带过期时间；卸载或存储异常时由后续控制台回收过期记录。
      }
    }
  }

  /**
   * 使用共享租约提供跨协议互斥，Web Locks 可用时再叠加同源原子锁。
   * @param {{request?: function(string, object, function): Promise<unknown>}|null} lockManager Web Locks 管理器。
   * @param {{get: function(string): object|null, set: function(string, object): void, delete: function(string): void}} leaseStore 共享值适配器。
   * @param {string} lockName 独占锁名称。
   * @param {string} ownerId 当前控制台随机 owner。
   * @param {function(): Promise<void>} callback 获得锁后执行的批次函数。
   * @param {object} [options] 共享租约锁参数。
   * @return {Promise<boolean>} 是否获得锁并执行了批次。
   */
  async function runWithControllerLock(lockManager, leaseStore, lockName, ownerId, callback, options = {}) {
    if (lockManager && typeof lockManager.request === 'function') {
      let leaseAcquired = false
      const webLockAcquired = await runWithExclusiveLock(lockManager, lockName, async () => {
        leaseAcquired = await runWithSharedLeaseLock(leaseStore, lockName, ownerId, callback, options)
      })
      return webLockAcquired && leaseAcquired
    }
    return runWithSharedLeaseLock(leaseStore, lockName, ownerId, callback, options)
  }

  /**
   * 只删除属于指定批次的共享值，避免其它控制台卸载时清空活动任务。
   * @param {{get: function(string): object|null, delete: function(string): void}} sharedStore 共享值适配器。
   * @param {object} sharedKeys 需要检查的共享键。
   * @param {string} runId 当前控制台批次标识。
   * @return {number} 删除的共享值数量。
   */
  function clearRunScopedSharedValues(sharedStore, sharedKeys, runId) {
    const normalizedRunId = String(runId || '')
    if (!normalizedRunId) return 0
    let deleted = 0
    Object.values(sharedKeys).forEach(key => {
      try {
        const value = sharedStore.get(key)
        if (!value || value.run_id !== normalizedRunId) return
        sharedStore.delete(key)
        deleted++
      } catch {
        // 页面卸载阶段逐项尽力清理，单个共享键异常不影响其它键。
      }
    })
    return deleted
  }

  /**
   * 判断共享消息是否属于指定批次和账号。
   * @param {object|null} value 共享消息。
   * @param {string} runId 批次标识。
   * @param {string} accountId 账号标识。
   * @return {boolean} 是否匹配。
   */
  function taskMatches(value, runId, accountId) {
    return Boolean(value && value.run_id === runId && value.account_id === accountId)
  }

  /**
   * 判断共享任务是否仍属于当前标签且允许继续自动动作。
   * @param {object|null} value 当前共享任务。
   * @param {{runId: string, accountId: string, tabMarker: string}} expected 期望任务标识。
   * @return {boolean} 是否仍可执行自动动作。
   */
  function activeTaskMatches(value, expected) {
    return taskMatches(value, expected.runId, expected.accountId)
      && value.tab_marker === expected.tabMarker
      && !Object.prototype.hasOwnProperty.call(value, 'cancelled_at')
      && !isExpiredSharedTask(value)
  }

  /**
   * 判断站点存储清理 ACK 是否属于当前清理请求。
   * @param {object|null} ack 清理 ACK。
   * @param {{runId: string, accountId: string, cleanupId: string, host: string}} expected 期望标识。
   * @return {boolean} 是否为当前请求的 ACK。
   */
  function cleanupAckMatches(ack, expected) {
    return taskMatches(ack, expected.runId, expected.accountId)
      && ack.cleanup_id === expected.cleanupId
      && ack.host === expected.host
  }

  /**
   * 从共享任务副本中删除密码并记录消费时间。
   * @param {object} task 当前共享任务。
   * @param {number} [consumedAt] 密码消费时间。
   * @return {object} 不含密码的新任务对象。
   */
  function stripTaskPassword(task, consumedAt = Date.now()) {
    const sanitized = { ...task }
    delete sanitized.password
    sanitized.password_consumed_at = consumedAt
    return sanitized
  }

  /**
   * 将共享任务投影为不含密码的已取消状态。
   * @param {object} task 当前共享任务。
   * @param {number} [cancelledAt] 取消时间。
   * @return {object} 不含密码且带取消时间的新任务对象。
   */
  function cancelSharedTask(task, cancelledAt = Date.now()) {
    const cancelled = { ...task }
    delete cancelled.password
    cancelled.cancelled_at = cancelledAt
    return cancelled
  }

  /**
   * 统一计算单号异常后的状态、错误码和内存密码处理方式。
   * @param {{skipRequested?: boolean, driverError?: string, errorCode?: string}} input 异常上下文。
   * @return {{status: 'failed'|'skipped', errorCode: string, clearPassword: boolean}} 异常结果。
   */
  function resolveAccountFailure(input) {
    if (input && input.skipRequested) {
      return { status: 'skipped', errorCode: 'ACCOUNT_SKIPPED', clearPassword: true }
    }
    return {
      status: 'failed',
      errorCode: String(input && (input.driverError || input.errorCode) || 'TOKEN_REQUEST_FAILED'),
      clearPassword: false
    }
  }

  const CORE = Object.freeze({
    CONFIG,
    STATUS_LABELS,
    ERROR_MESSAGES,
    normalizeEmail,
    isControllerLocation,
    parseAccounts,
    maskEmail,
    canTransition,
    classifyTokenResponse,
    nextPollDelay,
    scoreInputDescriptor,
    chooseBestDescriptor,
    isChallengePassedSnapshot,
    isChallengeSnapshot,
    formatRefreshTokens,
    buildCookieDeleteDetails,
    sanitizeDetail,
    isTrustedVerificationUrl,
    selectTrustedVerificationUrl,
    isDeviceVerificationPath,
    hasPasswordBeenSubmitted,
    shouldReturnToLoginBeforePassword,
    shouldNavigateToVerification,
    isAuthenticatedAccountLanding,
    shouldNavigateFromAuthenticatedLanding,
    appendDriverMarker,
    appendCleanupMarker,
    extractDriverMarker,
    extractCleanupMarker,
    isExpiredSharedTask,
    cookieIdentity,
    createCookieAdapter,
    collectTargetCookies,
    clearAndVerifyTargetCookies,
    createActionGate,
    createDeferredActionController,
    runWithExclusiveLock,
    runWithSharedLeaseLock,
    runWithControllerLock,
    clearRunScopedSharedValues,
    taskMatches,
    activeTaskMatches,
    cleanupAckMatches,
    stripTaskPassword,
    cancelSharedTask,
    resolveAccountFailure
  })

  if (typeof module !== 'undefined' && module.exports) module.exports = CORE
  if (typeof window === 'undefined' || typeof document === 'undefined' || typeof location === 'undefined') return

  /**
   * 创建带稳定错误码的异常。
   * @param {string} code 错误码。
   * @param {string} [message] 错误描述。
   * @return {Error} 带 code 字段的异常。
   */
  function createCodedError(code, message) {
    const error = new Error(message || ERROR_MESSAGES[code] || code)
    error.code = code
    return error
  }

  /**
   * 生成本次运行或事件的随机标识。
   * @param {string} prefix 标识前缀。
   * @return {string} 随机标识。
   */
  function createId(prefix) {
    const random = globalThis.crypto && typeof globalThis.crypto.randomUUID === 'function'
      ? globalThis.crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(16).slice(2)}`
    return `${prefix}-${random}`
  }

  /**
   * 等待指定时间，并支持 AbortSignal 取消。
   * @param {number} ms 等待毫秒数。
   * @param {AbortSignal} [signal] 取消信号。
   * @return {Promise<void>} 等待完成 Promise。
   */
  function wait(ms, signal) {
    return new Promise((resolve, reject) => {
      if (signal && signal.aborted) {
        reject(createCodedError('SCRIPT_STOPPED'))
        return
      }
      const timer = setTimeout(finish, Math.max(0, ms))

      function finish() {
        if (signal) signal.removeEventListener('abort', abort)
        resolve()
      }

      function abort() {
        clearTimeout(timer)
        reject(createCodedError('SCRIPT_STOPPED'))
      }

      if (signal) signal.addEventListener('abort', abort, { once: true })
    })
  }

  /**
   * 安全解析 JSON 响应。
   * @param {object} response GM 请求响应。
   * @return {object|null} JSON 对象或 null。
   */
  function parseResponseJson(response) {
    if (response && response.response && typeof response.response === 'object') return response.response
    try {
      return JSON.parse(String(response && response.responseText || ''))
    } catch {
      return null
    }
  }

  /**
   * 发起可取消的 Violentmonkey HTTP 请求。
   * @param {object} details 请求参数。
   * @param {AbortSignal} [signal] 取消信号。
   * @return {Promise<object>} GM 请求响应。
   */
  function requestWithGM(details, signal) {
    return new Promise((resolve, reject) => {
      let settled = false
      let control

      const finish = callback => value => {
        if (settled) return
        settled = true
        if (signal) signal.removeEventListener('abort', abort)
        callback(value)
      }
      const succeed = finish(resolve)
      const fail = finish(reject)

      function abort() {
        try {
          if (control && typeof control.abort === 'function') control.abort()
        } catch {
          // 取消请求的异常不应覆盖原始停止动作。
        }
        fail(createCodedError('SCRIPT_STOPPED'))
      }

      if (signal && signal.aborted) {
        abort()
        return
      }

      try {
        control = GM_xmlhttpRequest({
          ...details,
          timeout: details.timeout || CONFIG.requestTimeoutMs,
          responseType: 'json',
          onload: succeed,
          onerror: () => fail(createCodedError('NETWORK_ERROR')),
          ontimeout: () => fail(createCodedError('NETWORK_ERROR', '网络请求超时')),
          onabort: () => fail(createCodedError('SCRIPT_STOPPED'))
        })
      } catch (error) {
        fail(error)
        return
      }

      if (signal) signal.addEventListener('abort', abort, { once: true })
    })
  }

  /**
   * 发送 x-www-form-urlencoded 请求并返回状态与 JSON。
   * @param {string} url 请求地址。
   * @param {object} fields 表单字段。
   * @param {AbortSignal} [signal] 取消信号。
   * @return {Promise<{status: number, payload: object|null}>} 响应结果。
   */
  async function requestForm(url, fields, signal) {
    const body = new URLSearchParams(fields).toString()
    const response = await requestWithGM({
      method: 'POST',
      url,
      anonymous: true,
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/x-www-form-urlencoded'
      },
      data: body
    }, signal)
    return { status: Number(response.status || 0), payload: parseResponseJson(response) }
  }

  /**
   * Promise 化 Violentmonkey Cookie list。
   * @param {object} details Cookie 查询参数。
   * @return {Promise<Array<object>>} Cookie 数组。
   */
  function cookieList(details) {
    return createCookieAdapter(GM_cookie).list(details)
  }

  /**
   * Promise 化 Violentmonkey Cookie set。
   * @param {object} details Cookie 写入参数。
   * @return {Promise<void>} 写入完成 Promise。
   */
  function cookieSet(details) {
    return createCookieAdapter(GM_cookie).set(details)
  }

  /**
   * Promise 化 Violentmonkey Cookie delete。
   * @param {object} details Cookie 删除参数。
   * @return {Promise<void>} 删除完成 Promise。
   */
  function cookieDelete(details) {
    return createCookieAdapter(GM_cookie).delete(details)
  }

  /**
   * 检查 Violentmonkey 与关键授权 API 是否存在。
   * @return {void} 缺失时抛出异常。
   */
  function assertViolentmonkeyApis() {
    const handler = String(typeof GM_info !== 'undefined' && GM_info.scriptHandler || '')
    const available = typeof GM_getValue === 'function'
      && typeof GM_setValue === 'function'
      && typeof GM_deleteValue === 'function'
      && typeof GM_addValueChangeListener === 'function'
      && typeof GM_removeValueChangeListener === 'function'
      && typeof GM_openInTab === 'function'
      && typeof GM_xmlhttpRequest === 'function'
      && typeof GM_setClipboard === 'function'
      && typeof GM_cookie === 'object'
      && typeof GM_cookie.list === 'function'
      && typeof GM_cookie.set === 'function'
      && typeof GM_cookie.delete === 'function'
    if (!/violentmonkey/i.test(handler) || !available) throw createCodedError('VM_API_MISSING')
  }

  /**
   * 通过写入临时 HttpOnly Cookie 验证双层权限已开启。
   * @return {Promise<void>} 验证成功 Promise。
   */
  async function verifyHttpOnlyPermission() {
    const probeName = `__grok_vm_probe_${Date.now()}`
    const probeUrl = `${location.protocol}//${location.host}/`
    try {
      await cookieSet({
        url: probeUrl,
        name: probeName,
        value: '1',
        httpOnly: true,
        secure: location.protocol === 'https:',
        path: '/',
        expirationDate: Math.floor(Date.now() / 1000) + 60
      })
      const cookies = await cookieList({ url: probeUrl, name: probeName })
      if (!cookies.some(cookie => cookie.name === probeName && cookie.httpOnly)) {
        throw createCodedError('HTTPONLY_PERMISSION_REQUIRED')
      }
    } catch (error) {
      throw createCodedError('HTTPONLY_PERMISSION_REQUIRED', error && error.message)
    } finally {
      try {
        await cookieDelete({ url: probeUrl, name: probeName })
      } catch {
        // 探针清理失败不覆盖权限检查的主错误。
      }
    }
  }

  /**
   * 删除并二次校验所有 xAI/Grok Cookie。
   * @return {Promise<void>} 清理成功 Promise。
   */
  async function clearTargetCookies() {
    const result = await clearAndVerifyTargetCookies(createCookieAdapter(GM_cookie), CONFIG.cookieDomains)
    if (result.remaining.length) throw createCodedError('CLEANUP_COOKIE_REMAINS')
  }

  /**
   * 删除脚本使用的全部共享键。
   * @return {void} 无返回值。
   */
  function clearSharedValues() {
    Object.values(CONFIG.sharedKeys).forEach(key => {
      try {
        GM_deleteValue(key)
      } catch {
        // 清理动作逐项执行，单个键异常不阻断其它键删除。
      }
    })
  }

  /**
   * 创建官方 xAI Device Flow。
   * @param {AbortSignal} signal 取消信号。
   * @return {Promise<object>} Device Flow 数据。
   */
  async function createDeviceFlow(signal) {
    const response = await requestForm(CONFIG.deviceCodeUrl, {
      client_id: CONFIG.clientId,
      scope: CONFIG.scope
    }, signal)
    if (response.status < 200 || response.status >= 300) {
      throw createCodedError('DEVICE_CODE_REQUEST_FAILED')
    }
    const payload = response.payload || {}
    const verificationUrl = selectTrustedVerificationUrl(payload)
    if (!payload.device_code || !payload.user_code || !verificationUrl) {
      throw createCodedError('DEVICE_CODE_INVALID')
    }
    return {
      deviceCode: String(payload.device_code),
      userCode: String(payload.user_code),
      verificationUrl,
      intervalMs: Math.max(CONFIG.minPollMs, Number(payload.interval || 5) * 1000),
      expiresInMs: Math.min(CONFIG.maxDeviceFlowMs, Math.max(60000, Number(payload.expires_in || 1800) * 1000))
    }
  }

  /**
   * 轮询官方 xAI Token 端点。
   * @param {object} device Device Flow 数据。
   * @param {AbortSignal} signal 取消信号。
   * @return {Promise<string>} refresh token；其它 Token 字段不离开轮询函数。
   */
  async function pollDeviceToken(device, signal) {
    const deadline = Date.now() + device.expiresInMs
    let delayMs = nextPollDelay(device.intervalMs, 'pending')

    while (Date.now() < deadline) {
      await wait(delayMs, signal)
      const response = await requestForm(CONFIG.tokenUrl, {
        grant_type: 'urn:ietf:params:oauth:grant-type:device_code',
        client_id: CONFIG.clientId,
        device_code: device.deviceCode
      }, signal)
      const classification = classifyTokenResponse(response.status, response.payload)
      if (classification.kind === 'success') return String(response.payload && response.payload.refresh_token || '')
      if (classification.kind === 'pending' || classification.kind === 'slow_down') {
        delayMs = nextPollDelay(delayMs, classification.kind)
        continue
      }
      throw createCodedError(classification.code)
    }

    throw createCodedError('DEVICE_FLOW_TIMEOUT')
  }

  /**
   * 读取元素关联的 label 文本。
   * @param {HTMLInputElement} element 输入元素。
   * @return {string} label 文本。
   */
  function getLabelText(element) {
    const labels = element.labels ? Array.from(element.labels) : []
    if (labels.length) return labels.map(label => label.textContent || '').join(' ')
    const id = element.id
    if (!id) return ''
    const escapedId = typeof CSS !== 'undefined' && CSS.escape ? CSS.escape(id) : id.replace(/"/g, '\\"')
    const label = document.querySelector(`label[for="${escapedId}"]`)
    return label ? label.textContent || '' : ''
  }

  /**
   * 读取输入框周围的短文本。xAI Device 页会把“输入设备代码”渲染在自定义容器中，
   * 不一定写入 input.placeholder 或 label，因此需要有限读取近邻文本。
   * @param {HTMLInputElement} element 输入元素。
   * @return {string} 附近文本。
   */
  function getNearbyInputText(element) {
    const texts = []
    const describedBy = String(element.getAttribute('aria-describedby') || '').trim()
    if (describedBy && typeof document.getElementById === 'function') {
      for (const id of describedBy.split(/\s+/)) {
        const described = document.getElementById(id)
        if (described && described.textContent) texts.push(described.textContent)
      }
    }
    let current = element.parentElement
    for (let depth = 0; current && depth < 3; depth++) {
      if (current === document.body || current === document.documentElement) break
      if (current.textContent) texts.push(current.textContent)
      current = current.parentElement
    }
    return texts.join(' ').replace(/\s+/g, ' ').slice(0, 600)
  }

  /**
   * 判断元素是否可见且可交互。
   * @param {Element} element DOM 元素。
   * @return {boolean} 是否可见。
   */
  function isVisible(element) {
    if (!element || !(element instanceof Element)) return false
    const style = getComputedStyle(element)
    const rect = element.getBoundingClientRect()
    return style.display !== 'none'
      && style.visibility !== 'hidden'
      && Number(style.opacity || 1) > 0
      && rect.width > 0
      && rect.height > 0
  }

  /**
   * 将页面输入框转换为可测试的描述对象。
   * @return {Array<object>} 输入框描述。
   */
  function collectInputDescriptors() {
    return Array.from(document.querySelectorAll('input')).map(element => ({
      element,
      type: String(element.type || '').toLowerCase(),
      name: element.name || '',
      id: element.id || '',
      autocomplete: element.autocomplete || '',
      placeholder: element.placeholder || '',
      ariaLabel: element.getAttribute('aria-label') || '',
      label: getLabelText(element),
      nearbyText: getNearbyInputText(element),
      disabled: element.disabled,
      readOnly: element.readOnly,
      hidden: !isVisible(element)
    }))
  }

  /**
   * 使用原生 setter 更新输入框，并派发框架可识别事件。
   * @param {HTMLInputElement} element 输入框。
   * @param {string} value 写入值。
   * @return {void} 无返回值。
   */
  function setInputValue(element, value) {
    const prototype = element instanceof HTMLTextAreaElement
      ? HTMLTextAreaElement.prototype
      : HTMLInputElement.prototype
    const descriptor = Object.getOwnPropertyDescriptor(prototype, 'value')
    if (descriptor && descriptor.set) descriptor.set.call(element, value)
    else element.value = value
    element.dispatchEvent(new Event('input', { bubbles: true, composed: true }))
    element.dispatchEvent(new Event('change', { bubbles: true, composed: true }))
  }

  /**
   * 获取按钮可见文本。
   * @param {Element} element 按钮元素。
   * @return {string} 规范化文本。
   */
  function getButtonText(element) {
    return String(element.textContent || element.getAttribute('value') || element.getAttribute('aria-label') || '')
      .replace(/\s+/g, ' ')
      .trim()
      .toLowerCase()
  }

  /**
   * 判断按钮类元素是否处于不可点击状态。
   * @param {Element|null} element 候选按钮。
   * @return {boolean} 是否不可点击。
   */
  function isDisabledActionElement(element) {
    return Boolean(element && (element.disabled || element.getAttribute('aria-disabled') === 'true'))
  }

  /**
   * 查找高置信度动作按钮。
   * @param {'login'|'consent'|'email_method'} kind 动作类型。
   * @param {{includeDisabled?: boolean}} [options] 是否包含 disabled 按钮用于等待可用。
   * @return {HTMLElement|null} 按钮元素。
   */
  function findActionButton(kind, options = {}) {
    const candidates = Array.from(document.querySelectorAll('button, input[type="submit"], [role="button"]'))
      .filter(isVisible)
      .filter(element => options.includeDisabled || !isDisabledActionElement(element))
    const patterns = {
      login: /^(continue|next|sign in|log in|login|submit|继续|下一步|登录|登入)$/i,
      consent: /^(allow|approve|authorize|grant access|同意|允许|授权|批准)$/i,
      email_method: /^(email|login with email|continue with email|sign in with email|log in with email|使用邮箱|使用邮箱登录|使用电子邮件登录|邮箱登录|电子邮件登录)$/i
    }
    const pattern = patterns[kind]
    const exact = candidates.filter(element => pattern.test(getButtonText(element)))
    if (exact.length === 1) return exact[0]
    return null
  }

  /**
   * 读取当前页面的安全验证快照。
   * @return {object} 页面快照。
   */
  function getChallengeSnapshot() {
    return {
      url: location.href,
      title: document.title,
      text: String(document.body && document.body.innerText || '').slice(0, 4000),
      iframeSrcs: Array.from(document.querySelectorAll('iframe')).map(frame => frame.src || '')
    }
  }

  /**
   * 检测明确的登录失败文案。
   * @return {boolean} 是否检测到登录失败。
   */
  function hasLoginFailureMessage() {
    const text = String(document.body && document.body.innerText || '').toLowerCase()
    return [
      'incorrect password',
      'invalid password',
      'wrong password',
      'invalid email or password',
      'credentials are invalid',
      '密码不正确',
      '密码错误',
      '账号或密码错误'
    ].some(message => text.includes(message))
  }

  /**
   * 向控制台上下文发送不含凭据的驱动事件。
   * @param {object} task 当前任务。
   * @param {string} type 事件类型。
   * @param {string} [detail] 非敏感描述。
   * @return {void} 无返回值。
   */
  function emitDriverEvent(task, type, detail) {
    if (!task || !task.run_id || !task.account_id) return
    GM_setValue(CONFIG.sharedKeys.event, {
      event_id: createId('event'),
      run_id: task.run_id,
      account_id: task.account_id,
      type,
      detail: sanitizeDetail(detail),
      at: Date.now()
    })
  }

  /**
   * 从共享任务中删除密码，避免密码在提交后继续持久化。
   * @param {object} task 当前任务。
   * @return {object|null} 不含密码的新任务；任务不匹配时返回 null。
   */
  function removeSharedPassword(task) {
    const current = GM_getValue(CONFIG.sharedKeys.task, null)
    if (!taskMatches(current, task.run_id, task.account_id) || !Object.prototype.hasOwnProperty.call(current, 'password')) return null
    const sanitized = stripTaskPassword(current)
    GM_setValue(CONFIG.sharedKeys.task, sanitized)
    return sanitized
  }

  /**
   * 清理当前 origin 可访问的浏览器站点数据。
   * @return {Promise<void>} 清理完成 Promise。
   */
  async function clearCurrentOriginStorage() {
    localStorage.clear()
    sessionStorage.clear()
    if (localStorage.length || sessionStorage.length) throw createCodedError('CLEANUP_STORAGE_FAILED')
    if (globalThis.caches && typeof globalThis.caches.keys === 'function') {
      const keys = await globalThis.caches.keys()
      await Promise.all(keys.map(key => globalThis.caches.delete(key)))
      if ((await globalThis.caches.keys()).length) throw createCodedError('CLEANUP_STORAGE_FAILED')
    }
    if (navigator.serviceWorker && typeof navigator.serviceWorker.getRegistrations === 'function') {
      const registrations = await navigator.serviceWorker.getRegistrations()
      await Promise.all(registrations.map(registration => registration.unregister()))
      if ((await navigator.serviceWorker.getRegistrations()).length) throw createCodedError('CLEANUP_STORAGE_FAILED')
    }
    if (globalThis.indexedDB && typeof globalThis.indexedDB.databases === 'function') {
      const databases = await globalThis.indexedDB.databases()
      await Promise.all(databases
        .filter(database => database && database.name)
        .map(database => deleteIndexedDatabase(database.name)))
      const remaining = await globalThis.indexedDB.databases()
      if (remaining.some(database => database && database.name)) throw createCodedError('CLEANUP_STORAGE_FAILED')
    }
  }

  /**
   * 删除指定 IndexedDB，并在阻塞或失败时拒绝清理。
   * @param {string} name 数据库名称。
   * @return {Promise<void>} 删除完成 Promise。
   */
  function deleteIndexedDatabase(name) {
    return new Promise((resolve, reject) => {
      const request = globalThis.indexedDB.deleteDatabase(name)
      const timer = setTimeout(() => reject(new Error(`IndexedDB cleanup timeout: ${name}`)), 2000)
      request.onsuccess = () => {
        clearTimeout(timer)
        resolve()
      }
      request.onerror = () => {
        clearTimeout(timer)
        reject(request.error || new Error(`IndexedDB cleanup failed: ${name}`))
      }
      request.onblocked = () => {
        clearTimeout(timer)
        reject(new Error(`IndexedDB cleanup blocked: ${name}`))
      }
    })
  }

  /**
   * 读取当前标签的随机归属标记，并在首次加载时写入 window.name 供后续导航复用。
   * @return {string} 当前标签归属标记。
   */
  function getCurrentDriverMarker() {
    const hashMarker = extractDriverMarker(location.hash)
    if (hashMarker) {
      try {
        window.name = `${DRIVER_WINDOW_PREFIX}${hashMarker}`
      } catch {
        // 部分安全页面可能限制 window.name；URL fragment 仍可完成当前页归属校验。
      }
      return hashMarker
    }
    const windowName = String(window.name || '')
    return windowName.startsWith(DRIVER_WINDOW_PREFIX) ? windowName.slice(DRIVER_WINDOW_PREFIX.length) : ''
  }

  /**
   * 读取当前清理标签标记，并写入独立 window.name，避免与登录标签混淆。
   * @return {string} 当前清理标签归属标记。
   */
  function getCurrentCleanupMarker() {
    const hashMarker = extractCleanupMarker(location.hash)
    if (hashMarker) {
      try {
        window.name = `${CLEANUP_WINDOW_PREFIX}${hashMarker}`
      } catch {
        // 清理标签只需要当前页归属；window.name 失败时仍可用 URL fragment 校验。
      }
      return hashMarker
    }
    const windowName = String(window.name || '')
    return windowName.startsWith(CLEANUP_WINDOW_PREFIX) ? windowName.slice(CLEANUP_WINDOW_PREFIX.length) : ''
  }

  /**
   * 启动 xAI/Grok 页面上的隐藏登录驱动。
   * @return {Promise<void>} 驱动结束 Promise。
   */
  async function runLoginDriver() {
    let task = GM_getValue(CONFIG.sharedKeys.task, null)
    let observer
    let interval
    let scanTimer
    let firstSeenAt = Date.now()
    let lastReported = ''
    let activeCleanupId = ''
    let challengePassedAt = 0
    let lastChallengeSeenAt = 0
    const actionGate = createActionGate(CONFIG.maxActionAttemptsPerStage)
    const deferredSubmit = createDeferredActionController()

    /**
     * 删除已过期的共享任务，避免控制台异常关闭后密码继续持久化。
     * @return {void} 无返回值。
     */
    function discardExpiredTask() {
      if (!isExpiredSharedTask(task)) return
      deferredSubmit.cancel()
      const current = GM_getValue(CONFIG.sharedKeys.task, null)
      if (taskMatches(current, task.run_id, task.account_id)) GM_deleteValue(CONFIG.sharedKeys.task)
      task = null
    }

    /**
     * 判断当前标签是否是控制台为该登录任务创建的唯一目标标签。
     * @return {boolean} 是否允许在当前标签执行自动填表。
     */
    function ownsCurrentTask() {
      return Boolean(task
        && !Object.prototype.hasOwnProperty.call(task, 'cancelled_at')
        && task.tab_marker
        && getCurrentDriverMarker() === task.tab_marker)
    }

    /**
     * 在专用后台标签中清理当前 origin，并向控制台返回成功或失败 ACK。
     * @param {object|null} request 清理请求。
     * @return {Promise<void>} 清理处理完成 Promise。
     */
    async function handleCleanupRequest(request) {
      if (!request || !request.cleanup_id || activeCleanupId === request.cleanup_id) return
      if (request.target_host !== location.hostname || request.tab_marker !== getCurrentCleanupMarker()) return
      activeCleanupId = request.cleanup_id
      let ok = false
      let errorCode = ''
      try {
        await clearCurrentOriginStorage()
        ok = true
      } catch {
        errorCode = 'CLEANUP_STORAGE_FAILED'
      }
      GM_setValue(CONFIG.sharedKeys.cleanupAck, {
        run_id: request.run_id,
        account_id: request.account_id,
        cleanup_id: request.cleanup_id,
        host: location.hostname,
        ok,
        error_code: errorCode,
        at: Date.now()
      })
    }

    discardExpiredTask()

    const taskListener = GM_addValueChangeListener(CONFIG.sharedKeys.task, (_key, _oldValue, newValue) => {
      const previousTask = task
      deferredSubmit.cancel()
      task = newValue || null
      discardExpiredTask()
      if (!previousTask
        || !task
        || !taskMatches(task, previousTask.run_id, previousTask.account_id)
        || task.tab_marker !== previousTask.tab_marker) {
        // challenge 记忆只属于当前登录标签；切到下一号时不能沿用上一号的后置等待窗口。
        challengePassedAt = 0
        lastChallengeSeenAt = 0
      }
      firstSeenAt = Date.now()
      scheduleScan()
    })
    const cleanupListener = GM_addValueChangeListener(CONFIG.sharedKeys.cleanup, (_key, _oldValue, request) => {
      void handleCleanupRequest(request)
    })

    function scheduleScan() {
      clearTimeout(scanTimer)
      scanTimer = setTimeout(scan, CONFIG.scanDebounceMs)
    }

    function scheduleScanAfter(delayMs) {
      clearTimeout(scanTimer)
      scanTimer = setTimeout(scan, Math.max(CONFIG.scanDebounceMs, Number(delayMs) || 0))
    }

    function reportOnce(type, detail) {
      const key = `${type}:${detail || ''}`
      if (key === lastReported) return
      lastReported = key
      emitDriverEvent(task, type, detail)
    }

    function clickOnce(element, key) {
      if (!actionGate.tryAcquire(key)) return false
      element.focus()
      element.click()
      return true
    }

    function submitFilledInput(element, button, key, afterSubmit, buttonKind) {
      if (!task || !task.run_id || !task.account_id || !task.tab_marker) return false
      if (actionGate.attempts(key) >= CONFIG.maxActionAttemptsPerStage) return false
      const expectedTask = {
        runId: task.run_id,
        accountId: task.account_id,
        tabMarker: task.tab_marker
      }
      const expectedUrl = location.href
      deferredSubmit.schedule(150, () => {
        const current = GM_getValue(CONFIG.sharedKeys.task, null)
        const challengeSnapshot = getChallengeSnapshot()
        if (isChallengePassedSnapshot(challengeSnapshot)) {
          if (!challengePassedAt) challengePassedAt = Date.now()
          if (Date.now() - challengePassedAt < CONFIG.challengePassedGraceMs) {
            scheduleScanAfter(CONFIG.challengePassedGraceMs - (Date.now() - challengePassedAt))
            return false
          }
        }
        if (!activeTaskMatches(current, expectedTask)
          || getCurrentDriverMarker() !== expectedTask.tabMarker
          || location.href !== expectedUrl
          || isChallengeSnapshot(challengeSnapshot)) return false
        const latestButton = buttonKind ? findActionButton(buttonKind, { includeDisabled: true }) : button
        if (buttonKind === 'login' && isDisabledActionElement(latestButton)) {
          scheduleScanAfter(CONFIG.actionReadyRetryMs)
          return false
        }
        // 守卫拒绝时不消耗动作次数，人工 challenge 结束后同一 URL 才能自动继续。
        return true
      }, () => {
        let submitted = false
        let targetButton = button
        let blockedByDisabledButton = false
        if (targetButton && isDisabledActionElement(targetButton)) {
          blockedByDisabledButton = true
          targetButton = null
        }
        // React 页面可能在 input/change 事件后才启用按钮，因此执行时再按语义重查一次。
        if ((!targetButton || !targetButton.isConnected || !isVisible(targetButton)) && buttonKind) {
          targetButton = findActionButton(buttonKind, { includeDisabled: true })
          if (isDisabledActionElement(targetButton)) {
            blockedByDisabledButton = true
            targetButton = null
          }
        }
        if (targetButton && targetButton.isConnected && isVisible(targetButton)) {
          if (!actionGate.tryAcquire(key)) return
          targetButton.focus()
          targetButton.click()
          submitted = true
        } else if (element && element.isConnected && !blockedByDisabledButton) {
          if (!actionGate.tryAcquire(key)) return
          element.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', code: 'Enter', bubbles: true }))
          element.dispatchEvent(new KeyboardEvent('keyup', { key: 'Enter', code: 'Enter', bubbles: true }))
          submitted = true
        } else if (blockedByDisabledButton) {
          scheduleScanAfter(CONFIG.actionReadyRetryMs)
        }
        if (submitted && typeof afterSubmit === 'function') afterSubmit()
      })
      return true
    }

    function scan() {
      discardExpiredTask()
      if (!task || !task.run_id || !task.account_id || !ownsCurrentTask()) return
      const challengeSnapshot = getChallengeSnapshot()
      if (isChallengePassedSnapshot(challengeSnapshot)) {
        lastChallengeSeenAt = Date.now()
        if (!challengePassedAt) challengePassedAt = Date.now()
        const remainingMs = CONFIG.challengePassedGraceMs - (Date.now() - challengePassedAt)
        if (remainingMs > 0) {
          reportOnce('waiting_human', 'CLOUDFLARE_PASSED_SETTLING')
          scheduleScanAfter(remainingMs)
          return
        }
      } else {
        challengePassedAt = 0
      }
      if (isChallengeSnapshot(challengeSnapshot)) {
        lastChallengeSeenAt = Date.now()
        reportOnce('waiting_human', 'CLOUDFLARE_OR_CAPTCHA')
        scheduleScanAfter(CONFIG.challengeRecheckMs)
        return
      }
      if (hasLoginFailureMessage()) {
        reportOnce('login_failed', 'LOGIN_FAILED')
        return
      }
      if (shouldReturnToLoginBeforePassword(task, location.href)) {
        location.href = appendDriverMarker(CONFIG.loginStartUrl, task.tab_marker)
        return
      }
      if (isAuthenticatedAccountLanding(location.href) && task.password) {
        task = removeSharedPassword(task) || stripTaskPassword(task)
        // 刚删除共享密码时停止本轮账户页识别，避免把账户设置里的 Email 按钮当成登录入口点击。
        return
      }
      if (shouldNavigateFromAuthenticatedLanding(task, location.href, Date.now(), firstSeenAt)) {
        location.href = appendDriverMarker(task.verification_url, task.tab_marker)
        return
      }

      const descriptors = collectInputDescriptors()
      const password = chooseBestDescriptor(descriptors, 'password')
      if (password && task.password) {
        setInputValue(password.element, task.password)
        reportOnce('password_filled', 'PASSWORD_FILLED')
        const button = findActionButton('login')
        submitFilledInput(password.element, button, `password:${location.href}`, () => {
          task = removeSharedPassword(task) || stripTaskPassword(task)
        }, 'login')
        return
      }
      if (password && hasPasswordBeenSubmitted(task) && String(password.element.value || '')) {
        const button = findActionButton('login', { includeDisabled: true })
        if (button) {
          reportOnce('password_filled', 'PASSWORD_RESUBMIT')
          submitFilledInput(password.element, button, `password-resubmit:${location.href}`, undefined, 'login')
          return
        }
      }

      const email = chooseBestDescriptor(descriptors, 'email')
      if (email) {
        setInputValue(email.element, task.email)
        reportOnce('email_filled', 'EMAIL_FILLED')
        const button = findActionButton('login')
        submitFilledInput(email.element, button, `email:${location.href}`, undefined, 'login')
        return
      }

      const userCode = chooseBestDescriptor(descriptors, 'user_code')
      if (userCode && task.user_code) {
        setInputValue(userCode.element, task.user_code)
        reportOnce('user_code_filled', 'USER_CODE_FILLED')
        const button = findActionButton('login')
        submitFilledInput(userCode.element, button, `user-code:${location.href}`, undefined, 'login')
        return
      }

      const consentButton = findActionButton('consent')
      if (consentButton && isDeviceVerificationPath(location.pathname)) {
        if (clickOnce(consentButton, `consent:${location.href}`)) {
          reportOnce('authorization_submitted', 'CONSENT_SUBMITTED')
        }
        return
      }

      const emailMethod = findActionButton('email_method')
      if (emailMethod) {
        if (clickOnce(emailMethod, `email-method:${location.href}`)) {
          reportOnce('email_method_selected', 'EMAIL_METHOD_SELECTED')
        }
        return
      }

      const hasRecognizedControls = Boolean(password || email || userCode || consentButton || emailMethod)
      if (shouldNavigateToVerification(task, location.href, Date.now(), firstSeenAt, hasRecognizedControls)) {
        location.href = appendDriverMarker(task.verification_url, task.tab_marker)
        return
      }

      if (Date.now() - firstSeenAt >= CONFIG.pageUnknownTimeoutMs) {
        const challengePendingMs = Date.now() - lastChallengeSeenAt
        if (lastChallengeSeenAt && challengePendingMs < CONFIG.postChallengeUnknownGraceMs) {
          reportOnce('waiting_human', 'CLOUDFLARE_RESULT_PENDING')
          scheduleScanAfter(Math.min(CONFIG.challengeRecheckMs, CONFIG.postChallengeUnknownGraceMs - challengePendingMs))
          return
        }
        reportOnce('page_unknown', 'PAGE_UNKNOWN')
      }
    }

    if (document.documentElement) {
      observer = new MutationObserver(scheduleScan)
      observer.observe(document.documentElement, { childList: true, subtree: true, attributes: true })
    } else {
      document.addEventListener('DOMContentLoaded', () => {
        observer = new MutationObserver(scheduleScan)
        observer.observe(document.documentElement, { childList: true, subtree: true, attributes: true })
        scheduleScan()
      }, { once: true })
    }
    interval = setInterval(scheduleScan, CONFIG.scanIntervalMs)
    void handleCleanupRequest(GM_getValue(CONFIG.sharedKeys.cleanup, null))
    window.addEventListener('beforeunload', () => {
      clearInterval(interval)
      clearTimeout(scanTimer)
      deferredSubmit.cancel()
      if (observer) observer.disconnect()
      GM_removeValueChangeListener(taskListener)
      GM_removeValueChangeListener(cleanupListener)
    }, { once: true })
    scheduleScan()
  }

  /**
   * 构建控制台 Shadow DOM。
   * @return {object} 控制台元素引用。
   */
  function createControllerUI() {
    const isHttpController = location.protocol === 'http:'
    const httpRiskWarning = isHttpController
      ? '<div class="warning http-risk">当前使用 HTTP：账号密码和 refresh token 可能被网络中间人、代理或被篡改页面窃取。只应在你信任的网络和服务器上运行。</div>'
      : ''
    const httpRiskAck = isHttpController
      ? '我知晓当前 HTTP 页面可能泄露账号密码和 refresh token；'
      : ''
    const host = document.createElement('div')
    host.id = 'grok-bulk-login-root'
    document.documentElement.appendChild(host)
    const shadow = host.attachShadow({ mode: 'closed' })
    shadow.innerHTML = `
      <style>
        :host { all: initial; }
        * { box-sizing: border-box; letter-spacing: 0; }
        .shell {
          position: fixed; z-index: 2147483647; top: 12px; right: 12px;
          width: min(780px, calc(100vw - 24px)); height: calc(100vh - 24px);
          display: grid; grid-template-rows: auto auto auto minmax(0, 1fr) auto;
          color: #182026; background: #f7f8fa; border: 1px solid #b8c0c8;
          border-radius: 6px; box-shadow: 0 16px 48px rgba(24, 32, 38, .24);
          font: 13px/1.45 system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
          overflow: hidden;
        }
        .shell.is-collapsed { display: none; }
        header { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 14px; background: #182026; color: #fff; }
        h1 { margin: 0; font-size: 16px; font-weight: 700; }
        .subtitle { color: #c8d0d8; font-size: 12px; }
        .header-actions { display: flex; align-items: center; gap: 8px; flex: 0 0 auto; }
        .section { padding: 11px 14px; border-bottom: 1px solid #d9dee3; }
        textarea { width: 100%; resize: vertical; min-height: 94px; max-height: 210px; border: 1px solid #aab4be; border-radius: 4px; padding: 9px; color: #182026; background: #fff; font: 12px/1.45 ui-monospace, SFMono-Regular, Consolas, monospace; }
        textarea:focus, button:focus-visible, input:focus-visible { outline: 2px solid #137cbd; outline-offset: 1px; }
        .warning { margin-top: 8px; padding: 8px 10px; border-left: 3px solid #d9822b; background: #fff7e8; color: #7a4b10; }
        .warning.http-risk { border-left-color: #b23a2b; background: #fff0ed; color: #7c2418; font-weight: 700; }
        .ack { display: flex; align-items: flex-start; gap: 8px; margin-top: 9px; color: #394b59; }
        .toolbar { display: flex; flex-wrap: wrap; gap: 7px; padding: 10px 14px; border-bottom: 1px solid #d9dee3; background: #eef1f4; }
        button { min-height: 34px; border: 1px solid #8a99a8; border-radius: 4px; padding: 0 11px; color: #25313c; background: #fff; font: inherit; font-weight: 600; cursor: pointer; }
        button.primary { border-color: #0e6ba8; color: #fff; background: #137cbd; }
        button.danger { border-color: #b23a2b; color: #9c2b1e; background: #fff; }
        button:disabled { cursor: not-allowed; opacity: .48; }
        button.header-action { min-height: 28px; border-color: rgba(255,255,255,.34); color: #fff; background: rgba(255,255,255,.12); }
        button.fab {
          position: fixed; z-index: 2147483647; right: 18px; bottom: 18px;
          display: grid; place-items: center; gap: 1px; min-width: 76px; min-height: 76px;
          border: 1px solid #0e6ba8; border-radius: 999px; padding: 10px;
          color: #fff; background: #137cbd; box-shadow: 0 14px 34px rgba(19, 124, 189, .34);
          font: 12px/1.2 system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
        }
        button.fab[hidden] { display: none; }
        .fab-title { font-size: 15px; font-weight: 800; }
        .fab-status { max-width: 60px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; opacity: .9; }
        .statusbar { display: grid; grid-template-columns: 1fr auto; gap: 12px; padding: 9px 14px; border-bottom: 1px solid #d9dee3; background: #fff; }
        .status { font-weight: 700; color: #25313c; }
        .progress { color: #5c7080; font-variant-numeric: tabular-nums; }
        .content { min-height: 0; overflow: auto; background: #fff; }
        table { width: 100%; border-collapse: collapse; table-layout: fixed; }
        th, td { padding: 8px 10px; border-bottom: 1px solid #e4e7eb; text-align: left; vertical-align: top; overflow-wrap: anywhere; }
        th { position: sticky; top: 0; z-index: 1; background: #eef1f4; color: #394b59; font-size: 12px; }
        th:nth-child(1), td:nth-child(1) { width: 54px; }
        th:nth-child(3), td:nth-child(3) { width: 118px; }
        th:nth-child(4), td:nth-child(4) { width: 190px; }
        .state-success { color: #0f7b3f; font-weight: 700; }
        .state-failed { color: #b23a2b; font-weight: 700; }
        .state-waiting_human { color: #a66300; font-weight: 700; }
        .footer { padding: 10px 14px 12px; background: #f7f8fa; }
        .results { min-height: 64px; max-height: 110px; resize: vertical; }
        .errors { margin-top: 7px; color: #b23a2b; white-space: pre-wrap; }
        .empty { padding: 26px 14px; color: #738694; text-align: center; }
        @media (max-width: 640px) {
          .shell { top: 0; right: 0; width: 100vw; height: 100vh; border: 0; border-radius: 0; }
          button.fab { right: 14px; bottom: 14px; min-width: 68px; min-height: 68px; }
          .subtitle { display: none; }
          th:nth-child(1), td:nth-child(1) { width: 42px; }
          th:nth-child(3), td:nth-child(3) { width: 96px; }
          th:nth-child(4), td:nth-child(4) { width: 130px; }
        }
      </style>
      <main id="shell" class="shell is-collapsed">
        <header>
          <div><h1>Grok 批量授权</h1><div class="subtitle">Violentmonkey · 官方 Device Flow · 串行 Session 清理</div></div>
          <div class="header-actions"><span id="runner">检测中</span><button id="toggle" class="header-action" type="button">收起</button></div>
        </header>
        <section class="section">
          <textarea id="accounts" spellcheck="false" autocomplete="off" placeholder="一行一个账号：邮箱|密码"></textarea>
          ${httpRiskWarning}
          <div class="warning">运行会清除当前 Chrome Profile 的 xAI/Grok 登录态。Cloudflare 或其它安全验证只允许人工处理。</div>
          <label class="ack"><input id="ack" type="checkbox"> <span>${httpRiskAck}我已开启 Violentmonkey 全局及脚本级 HttpOnly Cookie 权限，并确认允许清除当前 Profile 的 xAI/Grok Session。</span></label>
          <div id="parse-errors" class="errors"></div>
        </section>
        <div class="toolbar">
          <button id="start" class="primary">开始</button>
          <button id="pause" disabled>暂停</button>
          <button id="skip" disabled>跳过当前</button>
          <button id="stop" class="danger" disabled>停止</button>
          <button id="retry" disabled>重试失败项</button>
          <button id="copy" disabled>复制 RT</button>
          <button id="clear">清空敏感数据</button>
        </div>
        <div class="statusbar"><span id="status" class="status">等待输入</span><span id="progress" class="progress">0 / 0</span></div>
        <div class="content"><div id="empty" class="empty">尚未创建批次</div><table id="table" hidden><thead><tr><th>#</th><th>账号</th><th>状态</th><th>结果</th></tr></thead><tbody></tbody></table></div>
        <div class="footer"><textarea id="results" class="results" readonly placeholder="成功后将在这里生成一行一个 refresh token"></textarea></div>
      </main>
      <button id="fab" class="fab" type="button" title="展开 Grok 批量授权"><span class="fab-title">Grok</span><span id="fab-status" class="fab-status">待输入</span></button>
    `
    return {
      shadow,
      shell: shadow.getElementById('shell'),
      toggleButton: shadow.getElementById('toggle'),
      fabButton: shadow.getElementById('fab'),
      fabStatus: shadow.getElementById('fab-status'),
      accountsInput: shadow.getElementById('accounts'),
      ackInput: shadow.getElementById('ack'),
      parseErrors: shadow.getElementById('parse-errors'),
      startButton: shadow.getElementById('start'),
      pauseButton: shadow.getElementById('pause'),
      skipButton: shadow.getElementById('skip'),
      stopButton: shadow.getElementById('stop'),
      retryButton: shadow.getElementById('retry'),
      copyButton: shadow.getElementById('copy'),
      clearButton: shadow.getElementById('clear'),
      status: shadow.getElementById('status'),
      progress: shadow.getElementById('progress'),
      runner: shadow.getElementById('runner'),
      empty: shadow.getElementById('empty'),
      table: shadow.getElementById('table'),
      tableBody: shadow.querySelector('tbody'),
      results: shadow.getElementById('results')
    }
  }

  /**
   * 启动控制台状态机。
   * @return {Promise<void>} 控制台结束 Promise。
   */
  async function runController() {
    const ui = createControllerUI()
    const runtime = {
      runId: '',
      accounts: [],
      running: false,
      paused: false,
      stopRequested: false,
      skipRequested: false,
      cleanupBlocked: false,
      currentAccount: null,
      currentTab: null,
      currentAbort: null,
      currentDriverError: '',
      closingTab: false,
      currentCleanupTab: null,
      apiAvailable: true,
      lockPending: false,
      collapsed: true
    }
    const sharedValueStore = Object.freeze({
      get: key => GM_getValue(key, null),
      set: (key, value) => GM_setValue(key, value),
      delete: key => GM_deleteValue(key)
    })

    try {
      assertViolentmonkeyApis()
      const protocolLabel = location.protocol === 'http:' ? ' · HTTP 风险模式' : ''
      ui.runner.textContent = `Violentmonkey ${GM_info.version || ''}${protocolLabel}`.trim()
    } catch (error) {
      runtime.apiAvailable = false
      ui.runner.textContent = '运行器不兼容'
      ui.status.textContent = ERROR_MESSAGES[error.code] || error.message
      ui.startButton.disabled = true
    }
    if (runtime.apiAvailable && isExpiredSharedTask(GM_getValue(CONFIG.sharedKeys.task, null))) {
      clearSharedValues()
    }

    const eventListener = runtime.apiAvailable ? GM_addValueChangeListener(CONFIG.sharedKeys.event, (_key, _oldValue, event) => {
      if (!runtime.currentAccount || !event || event.run_id !== runtime.runId || event.account_id !== runtime.currentAccount.id) return
      switch (event.type) {
        case 'email_filled':
          updateAccount(runtime.currentAccount, 'filling_email')
          break
        case 'password_filled':
          updateAccount(runtime.currentAccount, 'filling_password')
          break
        case 'waiting_human':
        case 'page_unknown':
          updateAccount(runtime.currentAccount, 'waiting_human', event.detail || '')
          break
        case 'authorization_submitted':
          updateAccount(runtime.currentAccount, 'polling_token')
          break
        case 'login_failed':
          runtime.currentDriverError = event.detail || 'LOGIN_FAILED'
          if (runtime.currentAbort) runtime.currentAbort.abort()
          break
        default:
          break
      }
      render()
    }) : null

    function updateAccount(account, status, errorCode = '') {
      if (!account) return
      if (canTransition(account.status, status)) account.status = status
      if (errorCode) account.errorCode = errorCode
    }

    function statusText(account) {
      if (!account) return ''
      return STATUS_LABELS[account.status] || account.status
    }

    function resultText(account) {
      if (account.refreshToken && account.status === 'success') return 'refresh token 已获取'
      if (account.refreshToken && account.errorCode) return `Token 已获取；${ERROR_MESSAGES[account.errorCode] || account.errorCode}`
      if (account.errorCode) return ERROR_MESSAGES[account.errorCode] || account.errorCode
      return ''
    }

    function render() {
      const accounts = runtime.accounts
      ui.empty.hidden = accounts.length > 0
      ui.table.hidden = accounts.length === 0
      ui.tableBody.replaceChildren(...accounts.map((account, index) => {
        const row = document.createElement('tr')
        const values = [
          String(index + 1),
          maskEmail(account.email),
          statusText(account),
          resultText(account)
        ]
        values.forEach((value, cellIndex) => {
          const cell = document.createElement('td')
          cell.textContent = value
          if (cellIndex === 2) cell.className = `state-${account.status}`
          row.appendChild(cell)
        })
        return row
      }))
      const done = accounts.filter(account => ['success', 'failed', 'skipped'].includes(account.status)).length
      ui.progress.textContent = `${done} / ${accounts.length}`
      ui.fabStatus.textContent = runtime.running && runtime.currentAccount
        ? `${done}/${accounts.length} ${statusText(runtime.currentAccount)}`
        : (accounts.length ? `${done}/${accounts.length}` : '待输入')
      ui.results.value = formatRefreshTokens(accounts)
      ui.copyButton.disabled = !ui.results.value
      ui.retryButton.disabled = runtime.running
        || runtime.lockPending
        || !accounts.some(account => account.status === 'failed' && !account.refreshToken && account.password)
      ui.pauseButton.disabled = !runtime.running
      ui.pauseButton.textContent = runtime.paused ? '继续' : '暂停'
      ui.skipButton.disabled = !runtime.running || !runtime.currentAccount || runtime.currentAccount.status === 'cleaning'
      ui.stopButton.disabled = !runtime.running
      ui.startButton.disabled = runtime.running || runtime.lockPending || !runtime.apiAvailable
      ui.clearButton.disabled = runtime.running || runtime.lockPending
    }

    function setGlobalStatus(text) {
      ui.status.textContent = text
    }

    /**
     * 切换控制台展开状态。默认只保留悬浮球，避免长期遮挡业务页面。
     * @param {boolean} collapsed 是否收起为悬浮球。
     * @return {void} 无返回值。
     */
    function setControllerCollapsed(collapsed) {
      runtime.collapsed = Boolean(collapsed)
      ui.shell.classList.toggle('is-collapsed', runtime.collapsed)
      ui.fabButton.hidden = !runtime.collapsed
      ui.toggleButton.textContent = runtime.collapsed ? '展开' : '收起'
    }

    /**
     * 将当前共享任务标记为已取消并立即移除共享密码。
     * @return {void} 无返回值。
     */
    function cancelCurrentSharedTask() {
      if (!runtime.currentAccount) return
      try {
        const current = GM_getValue(CONFIG.sharedKeys.task, null)
        if (!taskMatches(current, runtime.runId, runtime.currentAccount.id)) return
        GM_setValue(CONFIG.sharedKeys.task, cancelSharedTask(current))
      } catch {
        try {
          GM_deleteValue(CONFIG.sharedKeys.task)
        } catch {
          // cleanupAccount 会再次执行强制共享值清理。
        }
      }
    }

    async function waitUntilResumed() {
      while (runtime.running && runtime.paused && !runtime.stopRequested) await wait(250)
    }

    /**
     * 在控制台独占锁内执行敏感共享值操作。
     * @param {function(): Promise<void>|void} callback 获得锁后执行的动作。
     * @return {Promise<boolean>} 是否获得锁并执行了动作。
     */
    async function runControllerLocked(callback) {
      if (runtime.running || runtime.lockPending) return false
      runtime.lockPending = true
      render()
      try {
        const acquired = await runWithControllerLock(
          navigator.locks,
          sharedValueStore,
          CONFIG.controllerLockName,
          createId('controller'),
          callback,
          {
            claimDelayMs: CONFIG.controllerLeaseClaimDelayMs,
            heartbeatMs: CONFIG.controllerLeaseHeartbeatMs,
            ttlMs: CONFIG.controllerLeaseTtlMs,
            onLost() {
              runtime.stopRequested = true
              cancelCurrentSharedTask()
              if (runtime.currentAbort) runtime.currentAbort.abort()
              setGlobalStatus('控制台独占租约已丢失，正在停止并清理当前账号')
              render()
            }
          }
        )
        if (!acquired) {
          ui.parseErrors.textContent = '另一个 Grok 批量授权控制台正在运行，请先在原控制台停止或等待完成。'
          setGlobalStatus('检测到其它活动控制台')
        }
        return acquired
      } catch {
        ui.parseErrors.textContent = ERROR_MESSAGES.CONTROLLER_LOCK_FAILED
        setGlobalStatus(ERROR_MESSAGES.CONTROLLER_LOCK_FAILED)
        return false
      } finally {
        runtime.lockPending = false
        render()
      }
    }

    /**
     * 在控制台独占锁内启动批次，防止多个控制台覆盖共享任务或交叉清理 Session。
     * @param {function(): Promise<void>|void} prepare 获得锁后、启动队列前执行的准备动作。
     * @return {Promise<boolean>} 是否获得锁并启动了队列。
     */
    async function runProcessQueueLocked(prepare) {
      return runControllerLocked(async () => {
        await prepare()
        await processQueue()
      })
    }

    /**
     * 关闭控制台持有的登录标签，并避免 onclose 将主动关闭误判为用户关闭。
     * @return {void} 无返回值。
     */
    function closeLoginTab() {
      runtime.closingTab = true
      try {
        if (runtime.currentTab && !runtime.currentTab.closed) runtime.currentTab.close()
      } finally {
        runtime.currentTab = null
        runtime.closingTab = false
      }
    }

    /**
     * 在带随机标记的后台标签中清理一个目标 origin，并等待该标签返回 ACK。
     * @param {object} account 当前账号。
     * @param {string} targetUrl 目标 origin 地址。
     * @return {Promise<void>} 目标 origin 清理成功 Promise。
     */
    function requestOriginCleanup(account, targetUrl) {
      const targetHost = new URL(targetUrl).hostname
      const cleanupId = createId('cleanup')
      const tabMarker = createId('tab')
      return new Promise((resolve, reject) => {
        let finished = false
        let cleanupTab = null

        function finish(error) {
          if (finished) return
          finished = true
          clearTimeout(timer)
          let finalError = error || null
          try {
            GM_removeValueChangeListener(listener)
          } catch {
            finalError = finalError || createCodedError('CLEANUP_FAILED')
          }
          try {
            if (cleanupTab && !cleanupTab.closed) cleanupTab.close()
          } catch {
            // 清理结果已经确定，关闭后台标签失败不覆盖主结果。
          }
          if (runtime.currentCleanupTab === cleanupTab) runtime.currentCleanupTab = null
          try {
            GM_deleteValue(CONFIG.sharedKeys.cleanup)
            GM_deleteValue(CONFIG.sharedKeys.cleanupAck)
          } catch {
            finalError = finalError || createCodedError('CLEANUP_FAILED')
          }
          if (finalError) reject(finalError)
          else resolve()
        }

        const listener = GM_addValueChangeListener(CONFIG.sharedKeys.cleanupAck, (_key, _oldValue, ack) => {
          if (!cleanupAckMatches(ack, {
            runId: runtime.runId,
            accountId: account.id,
            cleanupId,
            host: targetHost
          })) return
          finish(ack.ok ? null : createCodedError(ack.error_code || 'CLEANUP_STORAGE_FAILED'))
        })
        const timer = setTimeout(() => finish(createCodedError('CLEANUP_ACK_TIMEOUT')), CONFIG.cleanupAckTimeoutMs)

        try {
          GM_deleteValue(CONFIG.sharedKeys.cleanupAck)
          GM_setValue(CONFIG.sharedKeys.cleanup, {
            run_id: runtime.runId,
            account_id: account.id,
            cleanup_id: cleanupId,
            tab_marker: tabMarker,
            target_host: targetHost,
            at: Date.now()
          })
          cleanupTab = GM_openInTab(appendCleanupMarker(targetUrl, tabMarker), { active: false, insert: true })
          if (!cleanupTab || typeof cleanupTab.close !== 'function') {
            finish(createCodedError('CLEANUP_TAB_OPEN_FAILED'))
            return
          }
          runtime.currentCleanupTab = cleanupTab
        } catch {
          finish(createCodedError('CLEANUP_TAB_OPEN_FAILED'))
        }
      })
    }

    /**
     * 依次清理所有 xAI/Grok 目标 origin，任一失败都会阻断后续流程。
     * @param {object} account 用于关联清理 ACK 的账号或启动占位对象。
     * @return {Promise<void>} 全部目标 origin 清理成功 Promise。
     */
    async function clearTargetOriginStorage(account) {
      for (const targetUrl of CONFIG.storageUrls) {
        await requestOriginCleanup(account, targetUrl)
      }
    }

    /**
     * 完成单个账号的强制 Session 清理门禁。
     * @param {object} account 当前账号。
     * @param {string} finalStatus 清理成功后恢复的终态。
     * @return {Promise<void>} 清理成功 Promise。
     */
    async function cleanupAccount(account, finalStatus) {
      updateAccount(account, 'cleaning')
      render()
      closeLoginTab()
      GM_deleteValue(CONFIG.sharedKeys.task)
      GM_deleteValue(CONFIG.sharedKeys.event)
      GM_deleteValue(CONFIG.sharedKeys.cleanup)
      GM_deleteValue(CONFIG.sharedKeys.cleanupAck)

      await clearTargetOriginStorage(account)
      await clearTargetCookies()
      updateAccount(account, finalStatus)
      render()
    }

    async function processAccount(account) {
      runtime.currentAccount = account
      runtime.currentDriverError = ''
      runtime.skipRequested = false
      runtime.currentAbort = new AbortController()
      const signal = runtime.currentAbort.signal
      let finalStatus = 'failed'

      try {
        updateAccount(account, 'requesting_device')
        setGlobalStatus(`正在处理 ${maskEmail(account.email)}`)
        render()
        const device = await createDeviceFlow(signal)
        const tabMarker = createId('tab')
        const task = {
          run_id: runtime.runId,
          account_id: account.id,
          tab_marker: tabMarker,
          email: account.email,
          password: account.password,
          user_code: device.userCode,
          verification_url: device.verificationUrl,
          created_at: Date.now(),
          expires_at: Date.now() + Math.min(CONFIG.taskTtlMs, device.expiresInMs)
        }
        GM_setValue(CONFIG.sharedKeys.task, task)
        updateAccount(account, 'opening_login')
        render()

        runtime.currentTab = GM_openInTab(appendDriverMarker(CONFIG.loginStartUrl, tabMarker), { active: true, insert: true })
        if (!runtime.currentTab || typeof runtime.currentTab.close !== 'function') {
          throw createCodedError('LOGIN_TAB_CLOSED')
        }
        runtime.currentTab.onclose = () => {
          if (runtime.closingTab || !runtime.currentAccount || runtime.currentAccount.id !== account.id) return
          runtime.currentDriverError = 'LOGIN_TAB_CLOSED'
          if (runtime.currentAbort) runtime.currentAbort.abort()
        }

        const refreshToken = await pollDeviceToken(device, signal)
        if (runtime.currentDriverError) throw createCodedError(runtime.currentDriverError)
        if (!refreshToken) throw createCodedError('TOKEN_MISSING_REFRESH')
        account.refreshToken = refreshToken
        account.password = ''
        account.errorCode = ''
        updateAccount(account, 'success')
        finalStatus = 'success'
      } catch (error) {
        const failure = resolveAccountFailure({
          skipRequested: runtime.skipRequested,
          driverError: runtime.currentDriverError,
          errorCode: error.code
        })
        if (failure.clearPassword) account.password = ''
        account.errorCode = failure.errorCode
        updateAccount(account, failure.status, failure.errorCode)
        finalStatus = failure.status
      } finally {
        runtime.currentAbort = null
        try {
          await cleanupAccount(account, finalStatus)
        } catch (cleanupError) {
          account.errorCode = cleanupError.code || 'CLEANUP_FAILED'
          account.status = 'failed'
          runtime.cleanupBlocked = true
          runtime.stopRequested = true
          setGlobalStatus(ERROR_MESSAGES[account.errorCode] || account.errorCode)
          render()
        }
        runtime.currentAccount = null
      }
    }

    async function processQueue() {
      runtime.running = true
      runtime.stopRequested = false
      runtime.cleanupBlocked = false
      render()
      try {
        await verifyHttpOnlyPermission()
        setGlobalStatus('正在清理旧的 xAI/Grok Session；后台清理标签可能显示 403/404，这不是登录页')
        await clearTargetOriginStorage({ id: `initial-${runtime.runId}` })
        await clearTargetCookies()
        while (!runtime.stopRequested && !runtime.cleanupBlocked) {
          await waitUntilResumed()
          if (runtime.stopRequested) break
          const next = runtime.accounts.find(account => account.status === 'pending')
          if (!next) break
          await processAccount(next)
          if (!runtime.stopRequested && !runtime.cleanupBlocked) await wait(CONFIG.accountCooldownMs)
        }
        if (runtime.cleanupBlocked) setGlobalStatus('Session 清理失败，队列已停止')
        else if (runtime.stopRequested) setGlobalStatus('批次已停止')
        else setGlobalStatus('批次处理完成')
      } catch (error) {
        const code = error.code || 'CLEANUP_FAILED'
        setGlobalStatus(ERROR_MESSAGES[code] || error.message || code)
      } finally {
        runtime.running = false
        runtime.paused = false
        runtime.currentAbort = null
        runtime.currentAccount = null
        render()
      }
    }

    ui.startButton.addEventListener('click', async () => {
      if (runtime.running || runtime.lockPending) return
      ui.parseErrors.textContent = ''
      if (!ui.ackInput.checked) {
        ui.parseErrors.textContent = '请先确认当前页面协议风险、HttpOnly Cookie 权限和当前 Profile Session 清理风险。'
        return
      }
      let rawInput = ui.accountsInput.value
      let parsed = parseAccounts(rawInput)
      ui.parseErrors.textContent = parsed.errors.map(error => `第 ${error.line} 行：${error.message}`).join('\n')
      if (!parsed.accounts.length) {
        if (!rawInput.trim() && !parsed.errors.length && runtime.accounts.some(account => account.status === 'pending')) {
          await runProcessQueueLocked(() => {
            clearSharedValues()
            runtime.runId = createId('run')
            render()
          })
          return
        }
        if (!parsed.errors.length) ui.parseErrors.textContent = '请输入至少一个账号。'
        return
      }
      await runProcessQueueLocked(() => {
        clearSharedValues()
        runtime.runId = createId('run')
        runtime.accounts = parsed.accounts
        parsed.accounts = []
        parsed = null
        ui.accountsInput.value = ''
        rawInput = ''
        render()
      })
    })

    ui.toggleButton.addEventListener('click', () => {
      setControllerCollapsed(true)
      render()
    })

    ui.fabButton.addEventListener('click', () => {
      setControllerCollapsed(false)
      render()
    })

    ui.pauseButton.addEventListener('click', () => {
      runtime.paused = !runtime.paused
      setGlobalStatus(runtime.paused ? '已暂停；当前登录页仍可人工处理' : '已继续')
      render()
    })

    ui.skipButton.addEventListener('click', () => {
      if (!runtime.currentAccount) return
      runtime.skipRequested = true
      cancelCurrentSharedTask()
      if (runtime.currentAbort) runtime.currentAbort.abort()
      setGlobalStatus('正在跳过当前账号并清理 Session')
    })

    ui.stopButton.addEventListener('click', () => {
      runtime.stopRequested = true
      cancelCurrentSharedTask()
      if (runtime.currentAbort) runtime.currentAbort.abort()
      setGlobalStatus('正在停止并清理当前账号')
    })

    ui.retryButton.addEventListener('click', async () => {
      if (runtime.running || runtime.lockPending) return
      if (!ui.ackInput.checked) {
        ui.parseErrors.textContent = '请先确认当前页面协议风险、HttpOnly Cookie 权限和当前 Profile Session 清理风险。'
        return
      }
      await runProcessQueueLocked(() => {
        runtime.accounts.forEach(account => {
          if (account.status === 'failed' && !account.refreshToken && account.password) {
            account.status = 'pending'
            account.errorCode = ''
            account.refreshToken = ''
          }
        })
        clearSharedValues()
        runtime.runId = createId('run')
        render()
      })
    })

    ui.copyButton.addEventListener('click', () => {
      if (!ui.results.value) return
      GM_setClipboard(ui.results.value, 'text')
      setGlobalStatus('refresh token 已复制')
    })

    ui.clearButton.addEventListener('click', async () => {
      if (runtime.running || runtime.lockPending) {
        ui.parseErrors.textContent = '请先停止当前批次，再清空敏感数据。'
        return
      }
      await runControllerLocked(() => {
        runtime.accounts.forEach(account => {
          account.password = ''
          account.refreshToken = ''
        })
        runtime.accounts = []
        runtime.runId = ''
        ui.accountsInput.value = ''
        ui.results.value = ''
        ui.parseErrors.textContent = ''
        clearSharedValues()
        setGlobalStatus('敏感数据已清空')
        render()
      })
    })

    window.addEventListener('beforeunload', () => {
      runtime.stopRequested = true
      if (runtime.currentAbort) runtime.currentAbort.abort()
      try {
        if (runtime.currentTab && !runtime.currentTab.closed) runtime.currentTab.close()
        if (runtime.currentCleanupTab && !runtime.currentCleanupTab.closed) runtime.currentCleanupTab.close()
      } catch {
        // 页面卸载阶段只做尽力关闭，后续依靠共享值过期保护。
      }
      clearRunScopedSharedValues(sharedValueStore, CONFIG.sharedKeys, runtime.runId)
      if (eventListener !== null) GM_removeValueChangeListener(eventListener)
    }, { once: true })
    setControllerCollapsed(true)
    render()
  }

  /**
   * 根据当前 host 启动控制台或隐藏登录驱动。
   * @return {Promise<void>} 启动完成 Promise。
   */
  async function bootstrap() {
    if (isControllerLocation(location.protocol, location.hostname)) {
      if (document.readyState === 'loading') {
        await new Promise(resolve => document.addEventListener('DOMContentLoaded', resolve, { once: true }))
      }
      await runController()
      return
    }
    if (location.protocol !== 'https:') return
    if (CONFIG.authHosts.has(location.hostname) || location.hostname.endsWith('.x.ai') || location.hostname.endsWith('.grok.com')) {
      await runLoginDriver()
    }
  }

  void bootstrap()
})()
