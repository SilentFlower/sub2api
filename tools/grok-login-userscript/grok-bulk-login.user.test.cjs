'use strict'

const test = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const core = require('./grok-bulk-login.user.js')

const scriptSource = fs.readFileSync(require.resolve('./grok-bulk-login.user.js'), 'utf8')

test('parseAccounts 只按第一个竖线分隔并保留密码内容', () => {
  const result = core.parseAccounts([
    'user1@example.com|Pass|With|Pipes',
    '',
    'user2@example.com|Secret2'
  ].join('\n'))

  assert.equal(result.errors.length, 0)
  assert.equal(result.accounts.length, 2)
  assert.equal(result.accounts[0].password, 'Pass|With|Pipes')
})

test('parseAccounts 标记非法行和重复邮箱', () => {
  const result = core.parseAccounts([
    'missing-separator',
    '|password',
    'user@example.com|one',
    ' USER@example.com |two'
  ].join('\n'))

  assert.deepEqual(result.errors.map(item => item.code), [
    'MISSING_SEPARATOR',
    'EMAIL_REQUIRED',
    'DUPLICATE_EMAIL'
  ])
  assert.equal(result.accounts.length, 1)
})

test('maskEmail 不暴露完整本地部分', () => {
  assert.equal(core.maskEmail('example@example.com'), 'ex***@example.com')
  assert.equal(core.maskEmail('a@example.com'), 'a***@example.com')
})

test('classifyTokenResponse 覆盖 Device Flow 状态', () => {
  assert.deepEqual(core.classifyTokenResponse(200, { access_token: 'fake-access' }), { kind: 'success', code: 'TOKEN_READY' })
  assert.deepEqual(core.classifyTokenResponse(400, { error: 'authorization_pending' }), { kind: 'pending', code: 'AUTHORIZATION_PENDING' })
  assert.deepEqual(core.classifyTokenResponse(400, { error: 'slow_down' }), { kind: 'slow_down', code: 'SLOW_DOWN' })
  assert.deepEqual(core.classifyTokenResponse(400, { error: 'access_denied' }), { kind: 'terminal', code: 'ACCESS_DENIED' })
  assert.deepEqual(core.classifyTokenResponse(400, { error: 'expired_token' }), { kind: 'terminal', code: 'DEVICE_CODE_EXPIRED' })
})

test('nextPollDelay 仅在 slow_down 时增加等待', () => {
  assert.equal(core.nextPollDelay(5000, 'pending'), 5000)
  assert.equal(core.nextPollDelay(5000, 'slow_down'), 10000)
  assert.equal(core.nextPollDelay(29000, 'slow_down'), 30000)
})

test('chooseBestDescriptor 选择语义明确的邮箱和密码输入框', () => {
  const descriptors = [
    { type: 'text', name: 'query', placeholder: 'Search' },
    { type: 'email', name: 'email', autocomplete: 'email', placeholder: 'Email' },
    { type: 'password', name: 'password', autocomplete: 'current-password' }
  ]

  assert.equal(core.chooseBestDescriptor(descriptors, 'email').name, 'email')
  assert.equal(core.chooseBestDescriptor(descriptors, 'password').name, 'password')
})

test('chooseBestDescriptor 在最高分并列时拒绝猜测', () => {
  const descriptors = [
    { type: 'email', name: 'email-a' },
    { type: 'email', name: 'email-b' }
  ]

  assert.equal(core.chooseBestDescriptor(descriptors, 'email'), null)
})

test('Device user code 不会误填到通用 OTP 验证码输入框', () => {
  const descriptors = [
    { type: 'text', name: 'user_code', placeholder: 'Device code' },
    { type: 'text', name: 'otp', autocomplete: 'one-time-code', placeholder: 'Verification code' }
  ]

  assert.equal(core.chooseBestDescriptor(descriptors, 'user_code').name, 'user_code')
  assert.equal(core.scoreInputDescriptor(descriptors[1], 'user_code') < 8, true)
})

test('Device user code 可通过中文附近文案识别', () => {
  const descriptors = [
    {
      type: 'text',
      name: '',
      placeholder: '',
      nearbyText: '登录 Grok Build 输入终端中显示的代码。输入设备代码 继续'
    }
  ]

  assert.equal(core.chooseBestDescriptor(descriptors, 'user_code'), descriptors[0])
})

test('isChallengeSnapshot 识别 Cloudflare 且不误判普通登录页', () => {
  assert.equal(core.isChallengeSnapshot({ iframeSrcs: ['https://challenges.cloudflare.com/turnstile'] }), true)
  assert.equal(core.isChallengePassedSnapshot({
    text: '成功! CLOUDFLARE 隐私 · 条款',
    iframeSrcs: ['https://challenges.cloudflare.com/turnstile/v0/fake']
  }), true)
  assert.equal(core.isChallengeSnapshot({
    text: '成功! CLOUDFLARE 隐私 · 条款',
    iframeSrcs: ['https://challenges.cloudflare.com/turnstile/v0/fake']
  }), false)
  assert.equal(core.isChallengePassedSnapshot({ text: 'success' }), false)
  assert.equal(core.isChallengeSnapshot({ title: 'Sign in to xAI', text: 'Email Password' }), false)
  assert.equal(core.isChallengeSnapshot({ url: 'https://accounts.x.ai/sign-in', text: 'Enter the verification code from your authenticator app' }), true)
  assert.equal(core.isChallengeSnapshot({ url: 'https://accounts.x.ai/oauth2/device', text: 'Enter verification code' }), false)
})

test('canTransition 拒绝跳过强制清理门禁', () => {
  assert.equal(core.canTransition('success', 'cleaning'), true)
  assert.equal(core.canTransition('success', 'pending'), false)
  assert.equal(core.canTransition('cleaning', 'success'), true)
  assert.equal(core.canTransition('failed', 'pending'), true)
})

test('formatRefreshTokens 只导出存在的 refresh token', () => {
  const output = core.formatRefreshTokens([
    { refreshToken: 'fake-refresh-1' },
    { refreshToken: '' },
    { refreshToken: 'fake-refresh-2' }
  ])

  assert.equal(output, 'fake-refresh-1\nfake-refresh-2')
})

test('buildCookieDeleteDetails 保留 Cookie path 和分区信息', () => {
  const details = core.buildCookieDeleteDetails({
    domain: '.x.ai',
    path: '/oauth2',
    name: 'fake-session',
    secure: true,
    partitionKey: { topLevelSite: 'https://x.ai' }
  }, 'https://auth.x.ai/')

  assert.deepEqual(details, {
    url: 'https://x.ai/oauth2',
    name: 'fake-session',
    partitionKey: { topLevelSite: 'https://x.ai' }
  })
})

test('buildCookieDeleteDetails 对 xAI 非 Secure Cookie 仍使用已授权的 HTTPS URL', () => {
  const details = core.buildCookieDeleteDetails({
    domain: '.x.ai',
    path: '/',
    name: 'fake-non-secure',
    secure: false
  }, 'https://accounts.x.ai/')

  assert.equal(details.url, 'https://x.ai/')
})

test('cookieIdentity 区分 First-Party 与 PartitionKey Cookie', () => {
  const base = { domain: '.x.ai', path: '/', name: 'fake-session' }

  assert.notEqual(
    core.cookieIdentity({ ...base, firstPartyDomain: 'x.ai' }),
    core.cookieIdentity({ ...base, firstPartyDomain: 'grok.com' })
  )
  assert.notEqual(
    core.cookieIdentity({ ...base, partitionKey: { topLevelSite: 'https://x.ai' } }),
    core.cookieIdentity({ ...base, partitionKey: { topLevelSite: 'https://grok.com' } })
  )
})

test('Device Flow 验证地址只接受 xAI HTTPS 域', () => {
  assert.equal(core.isTrustedVerificationUrl('https://auth.x.ai/oauth2/device/complete'), true)
  assert.equal(core.isTrustedVerificationUrl('https://accounts.x.ai/oauth2/device'), true)
  assert.equal(core.isTrustedVerificationUrl('http://auth.x.ai/oauth2/device/complete'), false)
  assert.equal(core.isTrustedVerificationUrl('https://x.ai.example.com/oauth2/device/complete'), false)
})

test('Device Flow 优先使用 verification_uri 并回退 verification_uri_complete', () => {
  assert.equal(core.selectTrustedVerificationUrl({
    verification_uri: 'https://accounts.x.ai/oauth2/device',
    verification_uri_complete: 'https://accounts.x.ai/oauth2/device?user_code=FAKE'
  }), 'https://accounts.x.ai/oauth2/device')
  assert.equal(core.selectTrustedVerificationUrl({
    verification_uri_complete: 'https://accounts.x.ai/oauth2/device?user_code=FAKE'
  }), 'https://accounts.x.ai/oauth2/device?user_code=FAKE')
  assert.equal(core.selectTrustedVerificationUrl({
    verification_uri: 'http://accounts.x.ai/oauth2/device',
    verification_uri_complete: 'https://x.ai.example.com/oauth2/device'
  }), '')
})

test('Device Flow 浏览器启动页优先使用 verification_uri_complete', () => {
  assert.equal(core.selectTrustedVerificationLaunchUrl({
    verification_uri: 'https://accounts.x.ai/oauth2/device',
    verification_uri_complete: 'https://accounts.x.ai/oauth2/device?user_code=FAKE'
  }), 'https://accounts.x.ai/oauth2/device?user_code=FAKE')
  assert.equal(core.selectTrustedVerificationLaunchUrl({
    verification_uri: 'https://accounts.x.ai/oauth2/device'
  }), 'https://accounts.x.ai/oauth2/device')
  assert.equal(core.selectTrustedVerificationLaunchUrl({
    verification_uri: 'https://evil.example.com/oauth2/device',
    verification_uri_complete: 'http://accounts.x.ai/oauth2/device?user_code=FAKE'
  }), '')
})

test('共享任务跳转地址优先使用带 user_code 的启动页', () => {
  assert.equal(core.taskVerificationUrl({
    verification_url: 'https://accounts.x.ai/oauth2/device',
    verification_launch_url: 'https://accounts.x.ai/oauth2/device?user_code=FAKE'
  }), 'https://accounts.x.ai/oauth2/device?user_code=FAKE')
  assert.equal(core.taskVerificationUrl({
    verification_url: 'https://accounts.x.ai/oauth2/device'
  }), 'https://accounts.x.ai/oauth2/device')
  assert.equal(core.taskVerificationUrl({
    verification_url: 'https://evil.example.com/oauth2/device',
    verification_launch_url: 'http://accounts.x.ai/oauth2/device?user_code=FAKE'
  }), '')
})

test('Device Flow 授权路径兼容基础路径和子路径', () => {
  assert.equal(core.isDeviceVerificationPath('/oauth2/device'), true)
  assert.equal(core.isDeviceVerificationPath('/oauth2/device/consent'), true)
  assert.equal(core.isDeviceVerificationPath('/oauth2/device/complete'), true)
  assert.equal(core.isDeviceVerificationPath('/oauth2/devices'), false)
})

test('登录入口无可识别控件时才跳转官方 Device Flow 验证页', () => {
  const task = { verification_url: 'https://accounts.x.ai/oauth2/device', password_consumed_at: 123 }

  assert.equal(core.shouldNavigateToVerification(task, 'https://accounts.x.ai/', 2000, 0, false), true)
  assert.equal(core.shouldNavigateToVerification({ ...task, device_ready_at: 1500 }, 'https://accounts.x.ai/', 1000, 1000, false), true)
  assert.equal(core.shouldNavigateToVerification(task, 'https://accounts.x.ai/', 2000, 0, true), false)
  assert.equal(core.shouldNavigateToVerification(task, 'https://accounts.x.ai/', 1000, 0, false), false)
  assert.equal(core.shouldNavigateToVerification(task, 'https://accounts.x.ai/oauth2/device', 2000, 0, false), false)
  assert.equal(core.shouldNavigateToVerification({ ...task, password: 'fake-password' }, 'https://accounts.x.ai/', 2000, 0, false), false)
  assert.equal(core.shouldNavigateToVerification({ verification_url: 'http://accounts.x.ai/oauth2/device' }, 'https://accounts.x.ai/', 2000, 0, false), false)
})

test('登录成功落到账户页时即使有账户设置控件也跳转 Device Flow', () => {
  const task = { verification_url: 'https://accounts.x.ai/oauth2/device', password_consumed_at: 123 }

  assert.equal(core.isAuthenticatedAccountLanding('https://accounts.x.ai/account'), true)
  assert.equal(core.isAuthenticatedAccountLanding('https://accounts.x.ai/account/profile'), true)
  assert.equal(core.isAuthenticatedAccountLanding('https://accounts.x.ai/sign-in'), false)
  assert.equal(core.shouldNavigateToVerification(task, 'https://accounts.x.ai/account', 2000, 0, true), false)
  assert.equal(core.shouldNavigateFromAuthenticatedLanding(task, 'https://accounts.x.ai/account', 2000, 0), false)
  assert.equal(core.shouldNavigateFromAuthenticatedLanding(task, 'https://accounts.x.ai/account', core.CONFIG.authenticatedLandingGraceMs + 1, 0), true)
  assert.equal(core.shouldNavigateFromAuthenticatedLanding({ ...task, device_ready_at: 123 }, 'https://accounts.x.ai/account', 1000, 1000), true)
  assert.equal(core.shouldNavigateFromAuthenticatedLanding(task, 'https://accounts.x.ai/account', 1000, 0), false)
})

test('授权中状态覆盖密码提交后到设备码提交的自然跳转阶段', () => {
  assert.equal(core.STATUS_LABELS.authorizing, '进入授权页')
  assert.equal(core.canTransition('pending', 'opening_login'), true)
  assert.equal(core.canTransition('filling_password', 'authorizing'), true)
  assert.equal(core.canTransition('authorizing', 'requesting_device'), true)
  assert.equal(core.canTransition('authorizing', 'polling_token'), true)
  assert.equal(core.canTransition('authorizing', 'filling_password'), true)
})

test('Device Flow 在登录完成后才合并到共享任务', () => {
  const loginTask = {
    run_id: 'run-1',
    account_id: 'account-1',
    tab_marker: 'tab-1',
    email: 'user@example.com',
    password: 'fake-password',
    created_at: 1000,
    expires_at: Date.now() + 60000
  }
  const submittedTask = core.stripTaskPassword(loginTask, 1234)
  const authenticatedTask = core.markTaskAuthenticated(submittedTask, 1500)
  const deviceTask = core.attachDeviceFlowToTask(submittedTask, {
    userCode: 'FAKE-CODE',
    verificationUrl: 'https://accounts.x.ai/oauth2/device',
    verificationLaunchUrl: 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE',
    expiresInMs: 600000
  }, 2000)

  assert.equal(core.hasAuthenticatedLogin(submittedTask), false)
  assert.equal(core.hasAuthenticatedLogin(authenticatedTask), true)
  assert.equal(core.shouldRequestDeviceFlowAfterLogin(loginTask), false)
  assert.equal(core.shouldRequestDeviceFlowAfterLogin(submittedTask), false)
  assert.equal(core.shouldRequestDeviceFlowAfterLogin(authenticatedTask), true)
  assert.equal(core.shouldRequestDeviceFlowAfterLogin({ ...authenticatedTask, cancelled_at: 1600 }), false)
  assert.equal(core.shouldRequestDeviceFlowAfterLogin({ ...authenticatedTask, expires_at: 1 }), false)
  assert.equal(core.shouldRequestDeviceFlowAfterLogin(deviceTask), false)
  const authenticatedDeviceTask = core.attachDeviceFlowToTask(authenticatedTask, {
    userCode: 'FAKE-CODE',
    verificationUrl: 'https://accounts.x.ai/oauth2/device',
    verificationLaunchUrl: 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE',
    expiresInMs: 600000
  }, 2000)
  assert.equal(Object.prototype.hasOwnProperty.call(authenticatedDeviceTask, 'password'), false)
  assert.equal(authenticatedDeviceTask.user_code, 'FAKE-CODE')
  assert.equal(authenticatedDeviceTask.verification_launch_url, 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE')
  assert.equal(authenticatedDeviceTask.authenticated_at, 1500)
  assert.equal(authenticatedDeviceTask.device_ready_at, 2000)
})

test('未提交密码前进入 Device 页时允许先提交当前设备码', () => {
  const task = { verification_url: 'https://accounts.x.ai/oauth2/device', password: 'fake-password' }
  const submittedTask = { verification_url: 'https://accounts.x.ai/oauth2/device', password_consumed_at: 123 }
  const descriptor = { element: {}, type: 'text' }

  assert.equal(core.hasPasswordBeenSubmitted(task), false)
  assert.equal(core.hasPasswordBeenSubmitted(submittedTask), true)
  assert.equal(core.shouldReturnToLoginBeforePassword(task, 'https://accounts.x.ai/oauth2/device', 1000, 1000, false), false)
  assert.equal(core.shouldReturnToLoginBeforePassword(task, 'https://accounts.x.ai/oauth2/device', core.CONFIG.loginToVerificationDelayMs + 1, 0, false), true)
  assert.equal(core.shouldReturnToLoginBeforePassword(task, 'https://accounts.x.ai/oauth2/device', core.CONFIG.loginToVerificationDelayMs + 1, 0, true), false)
  assert.equal(core.shouldReturnToLoginBeforePassword(task, 'https://accounts.x.ai/sign-in'), false)
  assert.equal(core.shouldReturnToLoginBeforePassword(submittedTask, 'https://accounts.x.ai/oauth2/device'), false)
  assert.equal(core.shouldReturnToLoginBeforePassword(task, 'http://accounts.x.ai/oauth2/device'), false)
  assert.equal(core.canSubmitDeviceUserCode({ ...task, user_code: 'FAKE-CODE' }, 'https://accounts.x.ai/oauth2/device', descriptor), true)
  assert.equal(core.canSubmitDeviceUserCode({ ...task, user_code: 'FAKE-CODE' }, 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE', null, true), true)
  assert.equal(core.canSubmitDeviceUserCode({ ...task, user_code: 'FAKE-CODE' }, 'https://accounts.x.ai/sign-in', descriptor), false)
  assert.equal(core.canSubmitDeviceUserCode({ ...task, user_code: 'FAKE-CODE' }, 'http://accounts.x.ai/oauth2/device', descriptor), false)
  assert.equal(core.canSubmitDeviceUserCode({ ...task, user_code: 'FAKE-CODE' }, 'https://accounts.x.ai/oauth2/device', null), false)
  assert.equal(core.pageContainsDeviceUserCode(
    { ...task, user_code: 'VZVA-E9VE' },
    'https://accounts.x.ai/oauth2/device?user_code=VZVA-E9VE',
    ''
  ), true)
  assert.equal(core.pageContainsDeviceUserCode(
    { ...task, user_code: 'VZVA-E9VE' },
    'https://accounts.x.ai/oauth2/device',
    '登录 Grok Build 输入终端中显示的代码 VZVA - E9VE'
  ), true)
  assert.equal(core.hasAuthenticatedSessionText('右上角 退出登录'), true)
})

test('可信 Grok Build 授权页允许点击 consent', () => {
  const task = {
    user_code: 'FAKE-CODE',
    password_consumed_at: 123
  }

  assert.equal(core.isTrustedDeviceConsentPage(
    task,
    'https://accounts.x.ai/oauth2/authorize?client_id=fake',
    '授权 Grok Build Read your email address Use the xAI API'
  ), true)
  assert.equal(core.isTrustedDeviceConsentPage(
    task,
    'https://accounts.x.ai/oauth2/authorize?client_id=fake',
    'Authorize Unknown App'
  ), false)
  assert.equal(core.isTrustedDeviceConsentPage(
    { ...task, password: 'fake-password' },
    'https://accounts.x.ai/oauth2/authorize?client_id=fake',
    '授权 Grok Build'
  ), false)
  assert.equal(core.isTrustedDeviceConsentPage(
    task,
    'https://evil.example.com/oauth2/authorize',
    '授权 Grok Build'
  ), false)
})

test('控制台地址精确允许 havefun HTTP 和 HTTPS', () => {
  assert.equal(core.isControllerLocation('http:', 'www.havefun.eu.cc'), true)
  assert.equal(core.isControllerLocation('https:', 'www.havefun.eu.cc'), true)
  assert.equal(core.isControllerLocation('ftp:', 'www.havefun.eu.cc'), false)
  assert.equal(core.isControllerLocation('http:', 'havefun.eu.cc'), false)
  assert.equal(core.isControllerLocation('http:', 'www.havefun.eu.cc.example.com'), false)
})

test('标签归属标记只写入 fragment 并可无损读回', () => {
  const marked = core.appendDriverMarker('https://auth.x.ai/oauth2/device/complete?user_code=FAKE#step=1', 'tab-fake-1')
  const parsed = new URL(marked)

  assert.equal(parsed.searchParams.get('user_code'), 'FAKE')
  assert.equal(core.extractDriverMarker(parsed.hash), 'tab-fake-1')
})

test('清理标签使用独立 fragment 标记，不会被登录标记读取', () => {
  const marked = core.appendCleanupMarker('https://auth.x.ai/oauth2/authorize#step=cleanup', 'cleanup-tab-1')
  const parsed = new URL(marked)

  assert.equal(core.extractCleanupMarker(parsed.hash), 'cleanup-tab-1')
  assert.equal(core.extractDriverMarker(parsed.hash), '')
  assert.equal(`${parsed.origin}${parsed.pathname}`, 'https://auth.x.ai/oauth2/authorize')
  assert.equal(parsed.hash.includes('grok-bulk-cleanup='), true)
  assert.equal(parsed.hash.includes('grok-bulk-login='), false)
})

test('清理承载页不使用 auth.x.ai 根路径', () => {
  assert.equal(core.CONFIG.storageUrls.includes('https://auth.x.ai/'), false)
  assert.equal(core.CONFIG.storageUrls.includes('https://auth.x.ai/oauth2/authorize'), true)
})

test('isExpiredSharedTask 只清理达到过期时间的任务', () => {
  assert.equal(core.isExpiredSharedTask({ expires_at: 1000 }, 999), false)
  assert.equal(core.isExpiredSharedTask({ expires_at: 1000 }, 1000), true)
  assert.equal(core.isExpiredSharedTask({ expires_at: 0 }, 1000), false)
})

test('createCookieAdapter 遵循 Violentmonkey Cookie 回调契约', async () => {
  const calls = []
  const adapter = core.createCookieAdapter({
    list(details, callback) {
      calls.push(['list', details])
      callback([{ name: 'fake-session' }], null)
    },
    set(details, callback) {
      calls.push(['set', details])
      callback(null)
    },
    delete(details, callback) {
      calls.push(['delete', details])
      callback(null)
    }
  })

  assert.deepEqual(await adapter.list({ url: 'https://x.ai/' }), [{ name: 'fake-session' }])
  await adapter.set({ url: 'https://x.ai/', name: 'fake-session', value: 'fake' })
  await adapter.delete({ url: 'https://x.ai/', name: 'fake-session' })
  assert.deepEqual(calls.map(call => call[0]), ['list', 'set', 'delete'])
})

test('createCookieAdapter 传播 Violentmonkey Cookie 错误', async () => {
  const adapter = core.createCookieAdapter({
    list(_details, callback) {
      callback([], 'permission denied')
    },
    set(_details, callback) {
      callback('permission denied')
    },
    delete(_details, callback) {
      callback('permission denied')
    }
  })

  await assert.rejects(adapter.list({}), /permission denied/)
  await assert.rejects(adapter.set({}), /permission denied/)
  await assert.rejects(adapter.delete({}), /permission denied/)
})

test('collectTargetCookies 使用 domain 枚举路径和子域 Cookie', async () => {
  const queries = []
  const adapter = {
    async list(details) {
      queries.push(details)
      if (details.domain === '.x.ai') {
        return [
          { domain: 'auth.x.ai', path: '/oauth2', name: 'auth-session' },
          { domain: 'accounts.x.ai', path: '/sign-in', name: 'account-session' }
        ]
      }
      return [{ domain: '.grok.com', path: '/', name: 'grok-session' }]
    }
  }

  const cookies = await core.collectTargetCookies(adapter, ['.x.ai', '.grok.com'])

  assert.deepEqual(queries, [{ domain: '.x.ai' }, { domain: '.grok.com' }])
  assert.equal(cookies.length, 3)
  assert.equal(cookies[0].fallbackUrl, 'https://x.ai/')
  assert.equal(cookies[0].cookie.path, '/oauth2')
  assert.equal(cookies[1].cookie.domain, 'accounts.x.ai')
})

test('clearAndVerifyTargetCookies 删除后执行二次 domain 枚举', async () => {
  let cookies = [
    { domain: 'auth.x.ai', path: '/oauth2', name: 'auth-session' },
    { domain: '.grok.com', path: '/', name: 'grok-session' }
  ]
  let listCalls = 0
  const adapter = {
    async list({ domain }) {
      listCalls++
      const suffix = domain.replace(/^\./, '')
      return cookies.filter(cookie => String(cookie.domain).replace(/^\./, '').endsWith(suffix))
    },
    async delete(details) {
      cookies = cookies.filter(cookie => cookie.name !== details.name)
    }
  }

  const result = await core.clearAndVerifyTargetCookies(adapter, ['.x.ai', '.grok.com'])

  assert.equal(result.deletedCount, 2)
  assert.deepEqual(result.remaining, [])
  assert.equal(listCalls, 4)
})

test('createActionGate 对同一阶段只允许有限次数', () => {
  const gate = core.createActionGate(1)

  assert.equal(gate.tryAcquire('email:/sign-in'), true)
  assert.equal(gate.tryAcquire('email:/sign-in'), false)
  assert.equal(gate.tryAcquire('password:/sign-in'), true)
  assert.equal(gate.attempts('email:/sign-in'), 1)
})

test('createDeferredActionController 支持取消和执行前守卫', () => {
  const timers = new Map()
  let nextTimer = 1
  const controller = core.createDeferredActionController({
    setTimeout(callback) {
      const id = nextTimer++
      timers.set(id, callback)
      return id
    },
    clearTimeout(id) {
      timers.delete(id)
    }
  })
  let actions = 0

  controller.schedule(150, () => true, () => { actions++ })
  assert.equal(controller.pending(), true)
  controller.cancel()
  assert.equal(timers.size, 0)
  assert.equal(controller.pending(), false)

  controller.schedule(150, () => false, () => { actions++ })
  timers.values().next().value()
  assert.equal(actions, 0)
  assert.equal(controller.pending(), false)
})

test('runWithExclusiveLock 阻止第二个控制台并发执行', async () => {
  let held = false
  const lockManager = {
    async request(name, options, callback) {
      if (held && options.ifAvailable) return callback(null)
      held = true
      try {
        return await callback({ name })
      } finally {
        held = false
      }
    }
  }
  let releaseFirst
  let notifyEntered
  const entered = new Promise(resolve => { notifyEntered = resolve })
  const blocker = new Promise(resolve => { releaseFirst = resolve })
  const first = core.runWithExclusiveLock(lockManager, 'controller', async () => {
    notifyEntered()
    await blocker
  })
  await entered

  const second = await core.runWithExclusiveLock(lockManager, 'controller', async () => {})
  releaseFirst()

  assert.equal(second, false)
  assert.equal(await first, true)
})

test('runWithSharedLeaseLock 拒绝未过期的其它控制台租约', async () => {
  const values = new Map([['controller', { owner: 'other', expires_at: 2000 }]])
  const store = {
    get: key => values.get(key) || null,
    set: (key, value) => values.set(key, value),
    delete: key => values.delete(key)
  }
  let callbackCalls = 0

  const acquired = await core.runWithSharedLeaseLock(store, 'controller', 'self', async () => {
    callbackCalls++
  }, {
    now: () => 1000,
    wait: async () => {},
    setInterval: () => 1,
    clearInterval() {}
  })

  assert.equal(acquired, false)
  assert.equal(callbackCalls, 0)
  assert.equal(values.get('controller').owner, 'other')
})

test('runWithSharedLeaseLock 接管过期租约并在结束后释放自己的 owner', async () => {
  const values = new Map([['controller', { owner: 'expired', expires_at: 999 }]])
  const store = {
    get: key => values.get(key) || null,
    set: (key, value) => values.set(key, value),
    delete: key => values.delete(key)
  }
  let callbackCalls = 0

  const acquired = await core.runWithSharedLeaseLock(store, 'controller', 'self', async () => {
    callbackCalls++
  }, {
    now: () => 1000,
    wait: async () => {},
    setInterval: () => 1,
    clearInterval() {},
    ttlMs: 5000
  })

  assert.equal(acquired, true)
  assert.equal(callbackCalls, 1)
  assert.equal(values.has('controller'), false)
})

test('runWithSharedLeaseLock 心跳按当前时间续租', async () => {
  const values = new Map()
  const store = {
    get: key => values.get(key) || null,
    set: (key, value) => values.set(key, value),
    delete: key => values.delete(key)
  }
  let nowMs = 1000
  let heartbeat = null
  let releaseCallback
  let notifyEntered
  const entered = new Promise(resolve => { notifyEntered = resolve })
  const blocker = new Promise(resolve => { releaseCallback = resolve })

  const running = core.runWithSharedLeaseLock(store, 'controller', 'self', async () => {
    notifyEntered()
    await blocker
  }, {
    now: () => nowMs,
    wait: async () => {},
    setInterval(handler) {
      heartbeat = handler
      return 1
    },
    clearInterval() {},
    ttlMs: 5000
  })

  await entered
  nowMs = 2500
  heartbeat()
  assert.equal(values.get('controller').expires_at, 7500)
  releaseCallback()

  assert.equal(await running, true)
  assert.equal(values.has('controller'), false)
})

test('runWithSharedLeaseLock 竞争确认和释放都限定当前 owner', async () => {
  const values = new Map()
  const store = {
    get: key => values.get(key) || null,
    set: (key, value) => values.set(key, value),
    delete: key => values.delete(key)
  }
  let callbackCalls = 0

  const acquired = await core.runWithSharedLeaseLock(store, 'controller', 'self', async () => {
    callbackCalls++
  }, {
    now: () => 1000,
    wait: async () => {
      values.set('controller', { owner: 'winner', expires_at: 5000 })
    },
    setInterval: () => 1,
    clearInterval() {}
  })

  assert.equal(acquired, false)
  assert.equal(callbackCalls, 0)
  assert.equal(values.get('controller').owner, 'winner')
})

test('runWithSharedLeaseLock 心跳发现 owner 丢失后停止续租且不误删新租约', async () => {
  const values = new Map()
  const store = {
    get: key => values.get(key) || null,
    set: (key, value) => values.set(key, value),
    delete: key => values.delete(key)
  }
  let heartbeat = null
  let releaseCallback
  let notifyEntered
  let lostCalls = 0
  const entered = new Promise(resolve => { notifyEntered = resolve })
  const blocker = new Promise(resolve => { releaseCallback = resolve })

  const running = core.runWithSharedLeaseLock(store, 'controller', 'self', async () => {
    notifyEntered()
    await blocker
  }, {
    now: () => 1000,
    wait: async () => {},
    setInterval(handler) {
      heartbeat = handler
      return 1
    },
    clearInterval() {},
    onLost() {
      lostCalls++
    }
  })

  await entered
  values.set('controller', { owner: 'replacement', expires_at: 5000 })
  heartbeat()
  releaseCallback()

  assert.equal(await running, false)
  assert.equal(lostCalls, 1)
  assert.equal(values.get('controller').owner, 'replacement')
})

test('runWithControllerLock 在 Web Locks 可用时同时持有并释放共享租约', async () => {
  const lockManager = {
    async request(_name, _options, callback) {
      return callback({ name: 'controller' })
    }
  }
  const values = new Map()
  const leaseStore = {
    get: key => values.get(key) || null,
    set: (key, value) => values.set(key, value),
    delete: key => values.delete(key)
  }
  let callbackCalls = 0

  const acquired = await core.runWithControllerLock(lockManager, leaseStore, 'controller', 'self', async () => {
    callbackCalls++
    assert.equal(values.get('controller').owner, 'self')
  }, {
    wait: async () => {},
    setInterval: () => 1,
    clearInterval() {}
  })

  assert.equal(acquired, true)
  assert.equal(callbackCalls, 1)
  assert.equal(values.has('controller'), false)
})

test('runWithControllerLock 阻止 HTTPS 与 HTTP 控制台同时执行', async () => {
  const values = new Map()
  const leaseStore = {
    get: key => values.get(key) || null,
    set: (key, value) => values.set(key, value),
    delete: key => values.delete(key)
  }
  let releaseHttps
  let notifyHttpsEntered
  const httpsEntered = new Promise(resolve => { notifyHttpsEntered = resolve })
  const httpsBlocker = new Promise(resolve => { releaseHttps = resolve })
  const lockManager = {
    async request(_name, _options, callback) {
      return callback({ name: 'controller' })
    }
  }
  const leaseOptions = {
    wait: async () => {},
    setInterval: () => 1,
    clearInterval() {}
  }

  const httpsRun = core.runWithControllerLock(lockManager, leaseStore, 'controller', 'https-owner', async () => {
    notifyHttpsEntered()
    await httpsBlocker
  }, leaseOptions)
  await httpsEntered

  const httpAcquired = await core.runWithControllerLock(null, leaseStore, 'controller', 'http-owner', async () => {}, leaseOptions)
  releaseHttps()

  assert.equal(httpAcquired, false)
  assert.equal(await httpsRun, true)
})

test('runWithControllerLock 阻止 HTTP 活动批次期间启动 HTTPS 控制台', async () => {
  const values = new Map()
  const leaseStore = {
    get: key => values.get(key) || null,
    set: (key, value) => values.set(key, value),
    delete: key => values.delete(key)
  }
  let releaseHttp
  let notifyHttpEntered
  const httpEntered = new Promise(resolve => { notifyHttpEntered = resolve })
  const httpBlocker = new Promise(resolve => { releaseHttp = resolve })
  const lockManager = {
    async request(_name, _options, callback) {
      return callback({ name: 'controller' })
    }
  }
  const leaseOptions = {
    wait: async () => {},
    setInterval: () => 1,
    clearInterval() {}
  }

  const httpRun = core.runWithControllerLock(null, leaseStore, 'controller', 'http-owner', async () => {
    notifyHttpEntered()
    await httpBlocker
  }, leaseOptions)
  await httpEntered

  const httpsAcquired = await core.runWithControllerLock(lockManager, leaseStore, 'controller', 'https-owner', async () => {}, leaseOptions)
  releaseHttp()

  assert.equal(httpsAcquired, false)
  assert.equal(await httpRun, true)
})

test('runWithControllerLock 传播锁 API 异常且不执行批次', async () => {
  const lockManager = {
    async request() {
      throw new Error('fake lock failure')
    }
  }
  const leaseStore = {
    get: () => null,
    set() {},
    delete() {}
  }
  let callbackCalls = 0

  await assert.rejects(core.runWithControllerLock(lockManager, leaseStore, 'controller', 'self', async () => {
    callbackCalls++
  }), /fake lock failure/)

  assert.equal(callbackCalls, 0)
})

test('clearRunScopedSharedValues 只删除当前 run_id 的共享值', () => {
  const values = new Map([
    ['task', { run_id: 'run-self', account_id: 'account-1' }],
    ['event', { run_id: 'run-other', account_id: 'account-2' }],
    ['cleanup', { run_id: 'run-self', cleanup_id: 'cleanup-1' }],
    ['empty', null]
  ])
  const store = {
    get: key => values.get(key) || null,
    delete: key => values.delete(key)
  }

  const deleted = core.clearRunScopedSharedValues(store, {
    task: 'task',
    event: 'event',
    cleanup: 'cleanup',
    empty: 'empty'
  }, 'run-self')

  assert.equal(deleted, 2)
  assert.equal(values.has('task'), false)
  assert.equal(values.has('cleanup'), false)
  assert.equal(values.get('event').run_id, 'run-other')
})

test('共享消息、清理 ACK 和密码投影保持批次隔离', () => {
  const task = { run_id: 'run-1', account_id: 'account-1', tab_marker: 'tab-1', expires_at: Date.now() + 60000, password: 'fake-password', email: 'user@example.com' }
  const sanitized = core.stripTaskPassword(task, 123)
  const cancelled = core.cancelSharedTask(task, 456)

  assert.equal(core.taskMatches(task, 'run-1', 'account-1'), true)
  assert.equal(core.taskMatches(task, 'run-2', 'account-1'), false)
  assert.equal(core.activeTaskMatches(task, { runId: 'run-1', accountId: 'account-1', tabMarker: 'tab-1' }), true)
  assert.equal(core.activeTaskMatches({ ...task, cancelled_at: 123 }, { runId: 'run-1', accountId: 'account-1', tabMarker: 'tab-1' }), false)
  assert.equal(core.activeTaskMatches({ ...task, cancelled_at: 0 }, { runId: 'run-1', accountId: 'account-1', tabMarker: 'tab-1' }), false)
  assert.equal(core.cleanupAckMatches({
    run_id: 'run-1',
    account_id: 'account-1',
    cleanup_id: 'cleanup-1',
    host: 'auth.x.ai'
  }, {
    runId: 'run-1',
    accountId: 'account-1',
    cleanupId: 'cleanup-1',
    host: 'auth.x.ai'
  }), true)
  assert.equal(Object.prototype.hasOwnProperty.call(sanitized, 'password'), false)
  assert.equal(sanitized.password_consumed_at, 123)
  assert.equal(Object.prototype.hasOwnProperty.call(cancelled, 'password'), false)
  assert.equal(cancelled.cancelled_at, 456)
  assert.equal(task.password, 'fake-password')
})

test('resolveAccountFailure 覆盖跳过、停止和标签关闭', () => {
  assert.deepEqual(core.resolveAccountFailure({ skipRequested: true }), {
    status: 'skipped',
    errorCode: 'ACCOUNT_SKIPPED',
    clearPassword: true
  })
  assert.deepEqual(core.resolveAccountFailure({ errorCode: 'SCRIPT_STOPPED' }), {
    status: 'failed',
    errorCode: 'SCRIPT_STOPPED',
    clearPassword: false
  })
  assert.deepEqual(core.resolveAccountFailure({ driverError: 'LOGIN_TAB_CLOSED', errorCode: 'SCRIPT_STOPPED' }), {
    status: 'failed',
    errorCode: 'LOGIN_TAB_CLOSED',
    clearPassword: false
  })
})

test('控制台允许 HTTP/HTTPS 且 Shadow DOM 不向页面开放', () => {
  assert.equal(scriptSource.includes('// @version      0.2.16'), true)
  assert.equal(scriptSource.includes('// @match        http://www.havefun.eu.cc/*'), true)
  assert.equal(scriptSource.includes('// @match        https://www.havefun.eu.cc/*'), true)
  assert.equal(scriptSource.includes('// @include      http://www.havefun.eu.cc:8080/*'), true)
  assert.equal(scriptSource.includes("loginStartUrl: 'https://accounts.x.ai/sign-in'"), true)
  assert.equal(scriptSource.includes('GM_openInTab(appendDriverMarker(CONFIG.loginStartUrl, tabMarker)'), true)
  assert.equal(scriptSource.includes('sessionStorage.setItem(DRIVER_SESSION_MARKER_KEY, hashMarker)'), true)
  assert.equal(scriptSource.includes('GM_openInTab(appendDriverMarker(task.verification_launch_url'), false)
  assert.equal(scriptSource.includes('if (isControllerLocation(location.protocol, location.hostname))'), true)
  assert.equal(scriptSource.includes("if (location.protocol !== 'https:') return"), true)
  assert.equal(scriptSource.includes("attachShadow({ mode: 'closed' })"), true)
  assert.equal(scriptSource.includes('class="shell is-collapsed"'), true)
  assert.equal(scriptSource.includes('id="fab" class="fab"'), true)
  assert.equal(scriptSource.includes('当前使用 HTTP：账号密码和 refresh token 可能被网络中间人'), true)
  assert.equal(scriptSource.includes('ui.parseErrors.textContent = ERROR_MESSAGES.CONTROLLER_LOCK_FAILED'), true)
})

test('sanitizeDetail 去除换行并限制长度', () => {
  const sanitized = core.sanitizeDetail(`line1\nline2\t${'x'.repeat(300)}`)
  assert.equal(sanitized.includes('\n'), false)
  assert.equal(sanitized.length, 160)
})
