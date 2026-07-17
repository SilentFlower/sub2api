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

test('isChallengeSnapshot 识别 Cloudflare 且不误判普通登录页', () => {
  assert.equal(core.isChallengeSnapshot({ iframeSrcs: ['https://challenges.cloudflare.com/turnstile'] }), true)
  assert.equal(core.isChallengeSnapshot({ title: 'Sign in to xAI', text: 'Email Password' }), false)
  assert.equal(core.isChallengeSnapshot({ url: 'https://accounts.x.ai/sign-in', text: 'Enter the verification code from your authenticator app' }), true)
  assert.equal(core.isChallengeSnapshot({ url: 'https://auth.x.ai/oauth2/device/complete', text: 'Enter verification code' }), false)
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
  assert.equal(core.isTrustedVerificationUrl('https://accounts.x.ai/oauth2/device/complete'), true)
  assert.equal(core.isTrustedVerificationUrl('http://auth.x.ai/oauth2/device/complete'), false)
  assert.equal(core.isTrustedVerificationUrl('https://x.ai.example.com/oauth2/device/complete'), false)
})

test('标签归属标记只写入 fragment 并可无损读回', () => {
  const marked = core.appendDriverMarker('https://auth.x.ai/oauth2/device/complete?user_code=FAKE#step=1', 'tab-fake-1')
  const parsed = new URL(marked)

  assert.equal(parsed.searchParams.get('user_code'), 'FAKE')
  assert.equal(core.extractDriverMarker(parsed.hash), 'tab-fake-1')
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

test('控制台只允许 HTTPS 且 Shadow DOM 不向页面开放', () => {
  assert.equal(scriptSource.includes('// @match        http://www.havefun.eu.cc/*'), false)
  assert.equal(scriptSource.includes('// @match        https://www.havefun.eu.cc/*'), true)
  assert.equal(scriptSource.includes("if (location.protocol !== 'https:') return"), true)
  assert.equal(scriptSource.includes("attachShadow({ mode: 'closed' })"), true)
})

test('sanitizeDetail 去除换行并限制长度', () => {
  const sanitized = core.sanitizeDetail(`line1\nline2\t${'x'.repeat(300)}`)
  assert.equal(sanitized.includes('\n'), false)
  assert.equal(sanitized.length, 160)
})
