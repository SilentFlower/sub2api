'use strict'

const test = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const vm = require('node:vm')
const core = require('./grok-bulk-login.user.js')

const scriptSource = fs.readFileSync(require.resolve('./grok-bulk-login.user.js'), 'utf8')

/**
 * 创建可按延迟值推进的假定时器。
 * @return {object} 假定时器控制对象。
 */
function createFakeTimers() {
  const timers = new Map()
  let nextId = 1
  return {
    setTimeout(callback, delayMs) {
      const id = nextId++
      timers.set(id, { callback, delayMs })
      return id
    },
    clearTimeout(id) {
      timers.delete(id)
    },
    setInterval() {
      return nextId++
    },
    clearInterval() {},
    runDelay(delayMs) {
      const entry = Array.from(timers.entries()).find(([, timer]) => timer.delayMs === delayMs)
      if (!entry) return false
      timers.delete(entry[0])
      entry[1].callback()
      return true
    },
    hasDelay(delayMs) {
      return Array.from(timers.values()).some(timer => timer.delayMs === delayMs)
    }
  }
}

/**
 * 创建可观察清理调用的 Storage mock。
 * @param {boolean} failClear 是否让 clear 抛出异常。
 * @return {object} Storage mock。
 */
function createStorage(failClear = false) {
  const entries = new Map()
  return {
    get length() {
      return entries.size
    },
    clearCalls: 0,
    clear() {
      this.clearCalls++
      if (failClear) throw new Error('fake storage cleanup failure')
      entries.clear()
    },
    getItem(key) {
      return entries.has(String(key)) ? entries.get(String(key)) : null
    },
    setItem(key, value) {
      entries.set(String(key), String(value))
    },
    removeItem(key) {
      entries.delete(String(key))
    }
  }
}

/**
 * 创建并启动一个最小 xAI 页面环境。
 * @param {object} [options] 页面和共享值配置。
 * @return {object} 驱动测试控制对象。
 */
function createDriverHarness(options = {}) {
  const timers = createFakeTimers()
  const values = new Map(Object.entries(options.values || {}))
  const listeners = new Map()
  let nextListenerId = 1
  let randomId = 1
  let mutationCallback = null
  let now = Number(options.now || 1000)

  class FakeDate extends Date {
    constructor(...args) {
      super(...(args.length ? args : [now]))
    }

    static now() {
      return now
    }
  }

  class FakeElement {
    constructor(properties = {}) {
      Object.assign(this, properties)
      this.disabled = Boolean(this.disabled)
      this.readOnly = Boolean(this.readOnly)
      this.isConnected = this.isConnected !== false
      this.events = []
      this.focusCalls = 0
    }

    getAttribute(name) {
      const attributes = this.attributes || {}
      return Object.prototype.hasOwnProperty.call(attributes, name) ? attributes[name] : null
    }

    getBoundingClientRect() {
      return { width: 160, height: 34 }
    }

    focus() {
      this.focusCalls++
    }

    dispatchEvent(event) {
      this.events.push(event)
      return true
    }
  }

  class FakeInput extends FakeElement {}
  class FakeTextArea extends FakeElement {}
  class FakeButton extends FakeElement {
    constructor(properties = {}) {
      super(properties)
      this.clickCalls = 0
    }

    click() {
      this.clickCalls++
    }
  }

  class FakeEvent {
    constructor(type, init = {}) {
      this.type = type
      Object.assign(this, init)
    }
  }

  const input = options.input ? new FakeInput({
    type: options.input.type || 'text',
    name: options.input.name || '',
    id: options.input.id || '',
    autocomplete: options.input.autocomplete || '',
    placeholder: options.input.placeholder || '',
    labels: [],
    value: '',
    attributes: options.input.attributes || {},
    parentElement: options.input.nearbyText
      ? { textContent: options.input.nearbyText, parentElement: null }
      : null
  }) : null
  const button = options.button ? new FakeButton({
    textContent: options.button.textContent || 'Continue',
    attributes: options.button.attributes || {},
    disabled: Boolean(options.button.disabled)
  }) : null
  const iframes = (options.iframeSrcs || []).map(src => ({ src }))
  const body = { innerText: options.bodyText || '' }
  const documentElement = {}
  const document = {
    title: options.title || 'Sign in to xAI',
    body,
    documentElement,
    querySelectorAll(selector) {
      if (selector === 'input') return input ? [input] : []
      if (selector === 'iframe') return iframes
      if (selector.includes('button')) return button ? [button] : []
      return []
    },
    querySelector() {
      return null
    },
    addEventListener() {}
  }
  const location = {
    protocol: 'https:',
    hostname: options.hostname || 'auth.x.ai',
    host: options.hostname || 'auth.x.ai',
    pathname: options.pathname || '/sign-in',
    hash: options.hash || '#grok-bulk-login=tab-1',
    href: options.href || `https://${options.hostname || 'auth.x.ai'}${options.pathname || '/sign-in'}${options.hash || '#grok-bulk-login=tab-1'}`
  }
  const window = {
    name: options.windowName || '',
    addEventListener() {}
  }
  const localStorage = createStorage(Boolean(options.failStorageCleanup))
  const sessionStorage = createStorage()

  function notify(key, oldValue, newValue) {
    for (const listener of listeners.values()) {
      if (listener.key === key) listener.callback(key, oldValue, newValue, true)
    }
  }

  function setValue(key, value) {
    const oldValue = values.get(key)
    values.set(key, value)
    notify(key, oldValue, value)
  }

  function deleteValue(key) {
    const oldValue = values.get(key)
    values.delete(key)
    notify(key, oldValue, undefined)
  }

  const sandbox = {
    URL,
    URLSearchParams,
    AbortController,
    Date: FakeDate,
    console,
    crypto: { randomUUID: () => `fake-${randomId++}` },
    window,
    document,
    location,
    navigator: {},
    localStorage,
    sessionStorage,
    Element: FakeElement,
    HTMLInputElement: FakeInput,
    HTMLTextAreaElement: FakeTextArea,
    Event: FakeEvent,
    KeyboardEvent: FakeEvent,
    MutationObserver: class {
      constructor(callback) {
        mutationCallback = callback
      }

      observe() {}
      disconnect() {}
    },
    CSS: { escape: value => value },
    getComputedStyle: () => ({ display: 'block', visibility: 'visible', opacity: '1' }),
    setTimeout: timers.setTimeout,
    clearTimeout: timers.clearTimeout,
    setInterval: timers.setInterval,
    clearInterval: timers.clearInterval,
    GM_getValue: (key, fallback) => values.has(key) ? values.get(key) : fallback,
    GM_setValue: setValue,
    GM_deleteValue: deleteValue,
    GM_addValueChangeListener(key, callback) {
      const id = nextListenerId++
      listeners.set(id, { key, callback })
      return id
    },
    GM_removeValueChangeListener(id) {
      listeners.delete(id)
    }
  }

  vm.runInNewContext(scriptSource, sandbox, { filename: 'grok-bulk-login.user.js' })

  return {
    timers,
    values,
    input,
    button,
    body,
    location,
    window,
    localStorage,
    sessionStorage,
    advanceTime(ms) {
      now += Number(ms || 0)
    },
    setValue,
    deleteValue,
    triggerMutation() {
      if (typeof mutationCallback === 'function') mutationCallback([])
    },
    async flush() {
      await new Promise(resolve => setImmediate(resolve))
    }
  }
}

function createActiveTask(overrides = {}) {
  return {
    run_id: 'run-1',
    account_id: 'account-1',
    tab_marker: 'tab-1',
    email: 'user@example.com',
    password: 'fake-password',
    user_code: 'FAKE-CODE',
    created_at: Date.now(),
    expires_at: Date.now() + 60000,
    ...overrides
  }
}

function createPasswordSubmittedTask(overrides = {}) {
  const task = createActiveTask(overrides)
  delete task.password
  task.password_consumed_at = overrides.password_consumed_at || Date.now()
  return task
}

function createLoginOnlyTask(overrides = {}) {
  const task = createActiveTask(overrides)
  delete task.user_code
  delete task.verification_url
  delete task.verification_launch_url
  delete task.device_ready_at
  return task
}

function createPasswordSubmittedLoginTask(overrides = {}) {
  const task = createLoginOnlyTask(overrides)
  delete task.password
  task.password_consumed_at = overrides.password_consumed_at || Date.now()
  return task
}

function createAuthenticatedTask(overrides = {}) {
  const task = createPasswordSubmittedTask(overrides)
  task.authenticated_at = overrides.authenticated_at || Date.now()
  return task
}

test('登录驱动在模拟密码页填入并只提交一次', () => {
  const harness = createDriverHarness({
    values: { [core.CONFIG.sharedKeys.task]: createActiveTask() },
    input: { type: 'password', name: 'password', autocomplete: 'current-password' },
    button: { textContent: 'Continue' }
  })

  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  assert.equal(harness.input.value, 'fake-password')
  assert.equal(harness.timers.runDelay(150), true)
  assert.equal(harness.button.clickCalls, 1)
  assert.equal(Object.prototype.hasOwnProperty.call(harness.values.get(core.CONFIG.sharedKeys.task), 'password'), false)
})

test('共享任务取消后延迟提交不会点击登录按钮', () => {
  const task = createActiveTask()
  const harness = createDriverHarness({
    values: { [core.CONFIG.sharedKeys.task]: task },
    input: { type: 'password', name: 'password', autocomplete: 'current-password' },
    button: { textContent: 'Continue' }
  })

  harness.timers.runDelay(core.CONFIG.scanDebounceMs)
  assert.equal(harness.timers.hasDelay(150), true)
  harness.setValue(core.CONFIG.sharedKeys.task, core.cancelSharedTask(task, 123))

  assert.equal(harness.timers.hasDelay(150), false)
  assert.equal(harness.button.clickCalls, 0)
})

test('页面 URL 变化后延迟提交不会执行旧登录动作', () => {
  const harness = createDriverHarness({
    values: { [core.CONFIG.sharedKeys.task]: createActiveTask() },
    input: { type: 'password', name: 'password', autocomplete: 'current-password' },
    button: { textContent: 'Continue' }
  })

  harness.timers.runDelay(core.CONFIG.scanDebounceMs)
  assert.equal(harness.timers.hasDelay(150), true)
  harness.location.href = 'https://auth.x.ai/sign-in/next#grok-bulk-login=tab-1'
  harness.location.pathname = '/sign-in/next'
  assert.equal(harness.timers.runDelay(150), true)

  assert.equal(harness.button.clickCalls, 0)
  assert.equal(harness.input.events.some(event => event.type === 'keydown' || event.type === 'keyup'), false)
  const task = harness.values.get(core.CONFIG.sharedKeys.task)
  assert.equal(Object.prototype.hasOwnProperty.call(task, 'password'), true)
  assert.equal(Object.prototype.hasOwnProperty.call(task, 'password_consumed_at'), false)
})

test('页面切换为 Cloudflare challenge 后延迟提交不会派发 Enter', () => {
  const harness = createDriverHarness({
    values: { [core.CONFIG.sharedKeys.task]: createActiveTask() },
    input: { type: 'password', name: 'password', autocomplete: 'current-password' }
  })

  harness.timers.runDelay(core.CONFIG.scanDebounceMs)
  assert.equal(harness.timers.hasDelay(150), true)
  harness.body.innerText = 'Checking your browser. Verify you are human.'
  assert.equal(harness.timers.runDelay(150), true)

  assert.equal(harness.input.events.some(event => event.type === 'keydown' || event.type === 'keyup'), false)
  const task = harness.values.get(core.CONFIG.sharedKeys.task)
  assert.equal(Object.prototype.hasOwnProperty.call(task, 'password'), true)
  assert.equal(Object.prototype.hasOwnProperty.call(task, 'password_consumed_at'), false)
})

test('同 URL challenge 完成后延迟提交恢复且只执行一次', () => {
  const harness = createDriverHarness({
    values: { [core.CONFIG.sharedKeys.task]: createActiveTask() },
    input: { type: 'password', name: 'password', autocomplete: 'current-password' },
    button: { textContent: 'Continue' }
  })

  harness.timers.runDelay(core.CONFIG.scanDebounceMs)
  harness.body.innerText = 'Checking your browser. Verify you are human.'
  assert.equal(harness.timers.runDelay(150), true)
  assert.equal(harness.button.clickCalls, 0)

  harness.body.innerText = ''
  harness.triggerMutation()
  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  assert.equal(harness.timers.runDelay(150), true)
  assert.equal(harness.button.clickCalls, 1)

  const task = harness.values.get(core.CONFIG.sharedKeys.task)
  assert.equal(Object.prototype.hasOwnProperty.call(task, 'password'), false)
  assert.equal(Object.prototype.hasOwnProperty.call(task, 'password_consumed_at'), true)
  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  assert.equal(harness.button.clickCalls, 1)
})

test('Cloudflare 页面只上报等待人工验证且不自动点击', () => {
  const harness = createDriverHarness({
    values: { [core.CONFIG.sharedKeys.task]: createActiveTask() },
    bodyText: 'Checking your browser. Verify you are human.',
    input: { type: 'email', name: 'email', autocomplete: 'email' },
    button: { textContent: 'Continue' }
  })

  harness.timers.runDelay(core.CONFIG.scanDebounceMs)
  const event = harness.values.get(core.CONFIG.sharedKeys.event)

  assert.equal(event.type, 'waiting_human')
  assert.equal(event.detail, 'CLOUDFLARE_OR_CAPTCHA')
  assert.equal(harness.button.clickCalls, 0)
})

test('Cloudflare 消失但登录页未恢复时继续等待避免过早 unknown', () => {
  const harness = createDriverHarness({
    values: { [core.CONFIG.sharedKeys.task]: createActiveTask() },
    bodyText: 'Checking your browser. Verify you are human.'
  })

  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.event).detail, 'CLOUDFLARE_OR_CAPTCHA')

  harness.body.innerText = ''
  harness.advanceTime(core.CONFIG.pageUnknownTimeoutMs + 1)
  assert.equal(harness.timers.runDelay(core.CONFIG.challengeRecheckMs), true)
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.event).type, 'waiting_human')
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.event).detail, 'CLOUDFLARE_RESULT_PENDING')

  harness.advanceTime(core.CONFIG.postChallengeUnknownGraceMs + 1)
  assert.equal(harness.timers.runDelay(core.CONFIG.challengeRecheckMs), true)
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.event).type, 'page_unknown')
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.event).detail, 'PAGE_UNKNOWN')
})

test('切换账号后不会沿用上一账号的 Cloudflare 后置等待窗口', () => {
  const harness = createDriverHarness({
    values: { [core.CONFIG.sharedKeys.task]: createActiveTask() },
    bodyText: 'Checking your browser. Verify you are human.'
  })

  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  harness.body.innerText = ''
  harness.setValue(core.CONFIG.sharedKeys.task, createActiveTask({ account_id: 'account-2' }))
  harness.advanceTime(core.CONFIG.pageUnknownTimeoutMs + 1)
  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)

  assert.equal(harness.values.get(core.CONFIG.sharedKeys.event).account_id, 'account-2')
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.event).type, 'page_unknown')
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.event).detail, 'PAGE_UNKNOWN')
})

test('Cloudflare 成功后等待稳定时间再提交登录', () => {
  const harness = createDriverHarness({
    values: { [core.CONFIG.sharedKeys.task]: createActiveTask() },
    bodyText: '成功! CLOUDFLARE 隐私 · 条款',
    iframeSrcs: ['https://challenges.cloudflare.com/turnstile/v0/fake'],
    input: { type: 'password', name: 'password', autocomplete: 'current-password' },
    button: { textContent: '登录' }
  })

  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  assert.equal(harness.input.value, '')
  assert.equal(harness.button.clickCalls, 0)
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.event).detail, 'CLOUDFLARE_PASSED_SETTLING')

  harness.advanceTime(core.CONFIG.challengePassedGraceMs)
  assert.equal(harness.timers.runDelay(core.CONFIG.challengePassedGraceMs), true)
  assert.equal(harness.input.value, 'fake-password')
  assert.equal(harness.timers.runDelay(150), true)
  assert.equal(harness.button.clickCalls, 1)
  assert.equal(Object.prototype.hasOwnProperty.call(harness.values.get(core.CONFIG.sharedKeys.task), 'password'), false)
})

test('Cloudflare 成功后登录按钮未就绪时不派发 Enter 且可恢复点击', () => {
  const harness = createDriverHarness({
    values: { [core.CONFIG.sharedKeys.task]: createActiveTask() },
    bodyText: '成功! CLOUDFLARE 隐私 · 条款',
    iframeSrcs: ['https://challenges.cloudflare.com/turnstile/v0/fake'],
    input: { type: 'password', name: 'password', autocomplete: 'current-password' },
    button: { textContent: '登录', disabled: true }
  })

  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  harness.advanceTime(core.CONFIG.challengePassedGraceMs)
  assert.equal(harness.timers.runDelay(core.CONFIG.challengePassedGraceMs), true)
  assert.equal(harness.input.value, 'fake-password')
  assert.equal(harness.timers.runDelay(150), true)
  assert.equal(harness.button.clickCalls, 0)
  assert.equal(harness.input.events.some(event => event.type === 'keydown' || event.type === 'keyup'), false)
  assert.equal(Object.prototype.hasOwnProperty.call(harness.values.get(core.CONFIG.sharedKeys.task), 'password'), true)

  harness.button.disabled = false
  assert.equal(harness.timers.runDelay(core.CONFIG.actionReadyRetryMs), true)
  assert.equal(harness.timers.runDelay(150), true)
  assert.equal(harness.button.clickCalls, 1)
  assert.equal(Object.prototype.hasOwnProperty.call(harness.values.get(core.CONFIG.sharedKeys.task), 'password'), false)
})

test('密码已消费但页面仍停在密码表单时重按登录避免未知页', () => {
  const harness = createDriverHarness({
    values: { [core.CONFIG.sharedKeys.task]: createPasswordSubmittedTask() },
    input: { type: 'password', name: 'password', autocomplete: 'current-password' },
    button: { textContent: '登录' }
  })
  harness.input.value = 'fake-password'

  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.event).type, 'password_filled')
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.event).detail, 'PASSWORD_RESUBMIT')
  assert.equal(harness.timers.runDelay(150), true)
  assert.equal(harness.button.clickCalls, 1)
  assert.equal(Object.prototype.hasOwnProperty.call(harness.values.get(core.CONFIG.sharedKeys.task), 'password'), false)
})

test('登录入口优先点击邮箱登录方式', () => {
  const harness = createDriverHarness({
    hostname: 'accounts.x.ai',
    pathname: '/sign-in',
    href: 'https://accounts.x.ai/sign-in#grok-bulk-login=tab-1',
    title: 'Sign In to Your SpaceXAI API Account',
    values: {
      [core.CONFIG.sharedKeys.task]: createActiveTask({
        verification_url: 'https://accounts.x.ai/oauth2/device'
      })
    },
    button: { textContent: 'Login with email' }
  })

  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)

  assert.equal(harness.button.clickCalls, 1)
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.event).type, 'email_method_selected')
})

test('未提交密码的登录入口无表单时不会跳转 Device Flow', () => {
  const harness = createDriverHarness({
    hostname: 'accounts.x.ai',
    pathname: '/sign-in',
    href: 'https://accounts.x.ai/sign-in#grok-bulk-login=tab-1',
    title: 'Sign In to Your SpaceXAI API Account',
    values: {
      [core.CONFIG.sharedKeys.task]: createActiveTask({
        verification_url: 'https://accounts.x.ai/oauth2/device'
      })
    }
  })

  harness.advanceTime(core.CONFIG.loginToVerificationDelayMs + 1)
  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)

  assert.equal(harness.location.href, 'https://accounts.x.ai/sign-in#grok-bulk-login=tab-1')
})

test('Device Flow 写入后登录入口无表单时导航到官方验证页', () => {
  const harness = createDriverHarness({
    hostname: 'accounts.x.ai',
    pathname: '/sign-in',
    href: 'https://accounts.x.ai/sign-in#grok-bulk-login=tab-1',
    title: 'xAI Accounts',
    values: {
      [core.CONFIG.sharedKeys.task]: createAuthenticatedTask({
        verification_url: 'https://accounts.x.ai/oauth2/device',
        verification_launch_url: 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE',
        device_ready_at: Date.now()
      })
    }
  })

  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)

  assert.equal(harness.location.href, 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE#grok-bulk-login=tab-1')
  assert.equal(harness.window.name, 'grok-bulk-login:tab-1')
})

test('密码提交后落到 xAI 账户页时忽略账户控件并跳转 Device Flow', () => {
  const harness = createDriverHarness({
    hostname: 'accounts.x.ai',
    pathname: '/account',
    href: 'https://accounts.x.ai/account',
    windowName: 'grok-bulk-login:tab-1',
    title: 'xAI Account',
    values: {
      [core.CONFIG.sharedKeys.task]: createPasswordSubmittedLoginTask()
    },
    button: { textContent: 'Email' }
  })

  harness.advanceTime(core.CONFIG.authenticatedLandingGraceMs + 1)
  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)

  assert.equal(harness.button.clickCalls, 0)
  assert.equal(harness.location.href, 'https://accounts.x.ai/account')
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.task).authenticated_at > 0, true)
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.event).type, 'password_submitted')

  harness.setValue(core.CONFIG.sharedKeys.task, {
    ...harness.values.get(core.CONFIG.sharedKeys.task),
    user_code: 'FAKE-CODE',
    verification_url: 'https://accounts.x.ai/oauth2/device',
    verification_launch_url: 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE',
    device_ready_at: Date.now()
  })
  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)

  assert.equal(harness.location.href, 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE#grok-bulk-login=tab-1')
  assert.equal(harness.window.name, 'grok-bulk-login:tab-1')
})

test('xAI 登录跳转丢失 hash 和 window.name 后仍可用 sessionStorage 归属标签', () => {
  const harness = createDriverHarness({
    hostname: 'accounts.x.ai',
    pathname: '/sign-in',
    href: 'https://accounts.x.ai/sign-in#grok-bulk-login=tab-1',
    hash: '#grok-bulk-login=tab-1',
    windowName: '',
    values: { [core.CONFIG.sharedKeys.task]: createLoginOnlyTask() },
    input: { type: 'password', name: 'password', autocomplete: 'current-password' },
    button: { textContent: '登录' }
  })

  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  assert.equal(harness.timers.runDelay(150), true)
  assert.equal(Object.prototype.hasOwnProperty.call(harness.values.get(core.CONFIG.sharedKeys.task), 'password'), false)
  assert.equal(harness.sessionStorage.getItem('grok-bulk-login:tab-marker'), 'tab-1')

  harness.location.href = 'https://accounts.x.ai/account'
  harness.location.pathname = '/account'
  harness.location.hash = ''
  harness.window.name = ''
  harness.button.textContent = 'Email'
  harness.triggerMutation()
  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)

  assert.equal(harness.location.href, 'https://accounts.x.ai/account')
  assert.equal(harness.button.clickCalls, 1)
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.task).authenticated_at > 0, true)
})

test('密码未消费但已落到账户页时先删除共享密码再跳转 Device Flow', () => {
  const harness = createDriverHarness({
    hostname: 'accounts.x.ai',
    pathname: '/account',
    href: 'https://accounts.x.ai/account',
    windowName: 'grok-bulk-login:tab-1',
    values: {
      [core.CONFIG.sharedKeys.task]: createLoginOnlyTask()
    },
    button: { textContent: 'Email' }
  })

  harness.advanceTime(core.CONFIG.loginToVerificationDelayMs + 1)
  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  assert.equal(Object.prototype.hasOwnProperty.call(harness.values.get(core.CONFIG.sharedKeys.task), 'password'), false)
  assert.equal(harness.location.href, 'https://accounts.x.ai/account')
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.event).type, 'password_submitted')

  harness.setValue(core.CONFIG.sharedKeys.task, {
    ...harness.values.get(core.CONFIG.sharedKeys.task),
    user_code: 'FAKE-CODE',
    verification_url: 'https://accounts.x.ai/oauth2/device',
    verification_launch_url: 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE',
    device_ready_at: Date.now()
  })
  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  assert.equal(harness.button.clickCalls, 0)
  assert.equal(harness.location.href, 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE#grok-bulk-login=tab-1')
})

test('完整模拟 xAI 自然跳转链路时不会提前接管账户页且状态事件继续推进', () => {
  const harness = createDriverHarness({
    hostname: 'accounts.x.ai',
    pathname: '/sign-in',
    href: 'https://accounts.x.ai/sign-in#grok-bulk-login=tab-1',
    windowName: 'grok-bulk-login:tab-1',
    values: {
      [core.CONFIG.sharedKeys.task]: createLoginOnlyTask()
    },
    input: { type: 'password', name: 'password', autocomplete: 'current-password' },
    button: { textContent: '登录' }
  })

  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  assert.equal(harness.input.value, 'fake-password')
  assert.equal(harness.timers.runDelay(150), true)
  assert.equal(harness.button.clickCalls, 1)
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.event).type, 'password_submitted')
  assert.equal(Object.prototype.hasOwnProperty.call(harness.values.get(core.CONFIG.sharedKeys.task), 'password'), false)

  harness.advanceTime(core.CONFIG.authenticatedLandingGraceMs / 2)
  harness.location.href = 'https://accounts.x.ai/account'
  harness.location.pathname = '/account'
  harness.button.textContent = 'Email'
  harness.triggerMutation()
  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  assert.equal(harness.location.href, 'https://accounts.x.ai/account')
  assert.equal(harness.button.clickCalls, 1)
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.event).type, 'password_submitted')

  harness.setValue(core.CONFIG.sharedKeys.task, {
    ...harness.values.get(core.CONFIG.sharedKeys.task),
    user_code: 'FAKE-CODE',
    verification_url: 'https://accounts.x.ai/oauth2/device',
    verification_launch_url: 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE',
    device_ready_at: Date.now()
  })
  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  assert.equal(harness.location.href, 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE#grok-bulk-login=tab-1')

  harness.location.href = 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE#grok-bulk-login=tab-1'
  harness.location.pathname = '/oauth2/device'
  harness.input.type = 'text'
  harness.input.name = ''
  harness.input.autocomplete = ''
  harness.input.parentElement = { textContent: '登录 Grok Build。输入设备代码。', parentElement: null }
  harness.input.value = ''
  harness.button.textContent = '继续'
  harness.triggerMutation()
  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  assert.equal(harness.input.value, 'FAKE-CODE')
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.event).type, 'user_code_filled')
  assert.equal(harness.timers.runDelay(150), true)
  assert.equal(harness.button.clickCalls, 2)
})

test('未提交密码前 Device Flow 页面有邮箱登录入口时优先点击入口', () => {
  const harness = createDriverHarness({
    hostname: 'accounts.x.ai',
    pathname: '/oauth2/device',
    href: 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE#grok-bulk-login=tab-1',
    title: 'Device Sign-in | SpaceXAI Accounts',
    windowName: 'grok-bulk-login:tab-1',
    values: {
      [core.CONFIG.sharedKeys.task]: createActiveTask({
        verification_url: 'https://accounts.x.ai/oauth2/device',
        verification_launch_url: 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE'
      })
    },
    button: { textContent: 'Login with email' }
  })

  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)

  assert.equal(harness.location.href, 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE#grok-bulk-login=tab-1')
  assert.equal(harness.button.clickCalls, 1)
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.event).type, 'email_method_selected')
})

test('未提交密码前 Device Flow 页面会先提交设备码并保留密码', () => {
  const harness = createDriverHarness({
    hostname: 'accounts.x.ai',
    pathname: '/oauth2/device',
    href: 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE#grok-bulk-login=tab-1',
    title: 'Device Sign-in | SpaceXAI Accounts',
    windowName: 'grok-bulk-login:tab-1',
    values: {
      [core.CONFIG.sharedKeys.task]: createActiveTask({
        verification_url: 'https://accounts.x.ai/oauth2/device',
        verification_launch_url: 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE'
      })
    },
    bodyText: '登录 Grok Build 输入终端中显示的代码。仅当您刚刚从设备发起登录时才输入此代码。',
    input: {
      type: 'text',
      nearbyText: '登录 Grok Build 输入终端中显示的代码。输入设备代码 仅当您刚刚从设备发起登录时才输入此代码。'
    },
    button: { textContent: '继续' }
  })

  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  assert.equal(harness.location.href, 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE#grok-bulk-login=tab-1')
  assert.equal(harness.input.value, 'FAKE-CODE')
  assert.equal(harness.timers.runDelay(150), true)
  assert.equal(harness.button.clickCalls, 1)
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.task).password, 'fake-password')
  assert.equal(harness.values.has(core.CONFIG.sharedKeys.event), false)
})

test('未提交密码前 Device Flow 已预填设备码时直接点击继续并保留密码', () => {
  const harness = createDriverHarness({
    hostname: 'accounts.x.ai',
    pathname: '/oauth2/device',
    href: 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE#grok-bulk-login=tab-1',
    title: 'Device Sign-in | SpaceXAI Accounts',
    windowName: 'grok-bulk-login:tab-1',
    values: {
      [core.CONFIG.sharedKeys.task]: createActiveTask({
        verification_url: 'https://accounts.x.ai/oauth2/device',
        verification_launch_url: 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE'
      })
    },
    bodyText: '登录 Grok Build。输入终端中显示的代码。',
    button: { textContent: '继续' }
  })

  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  assert.equal(harness.timers.runDelay(150), true)

  assert.equal(harness.button.clickCalls, 1)
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.task).password, 'fake-password')
  assert.equal(harness.values.has(core.CONFIG.sharedKeys.event), false)
})

test('登录态 Device Flow 已预填设备码时删除密码并点击继续', () => {
  const harness = createDriverHarness({
    hostname: 'accounts.x.ai',
    pathname: '/oauth2/device',
    href: 'https://accounts.x.ai/oauth2/device#grok-bulk-login=tab-1',
    title: 'Device Sign-in | SpaceXAI Accounts',
    windowName: 'grok-bulk-login:tab-1',
    values: {
      [core.CONFIG.sharedKeys.task]: createActiveTask({
        verification_url: 'https://accounts.x.ai/oauth2/device',
        verification_launch_url: 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE'
      })
    },
    bodyText: '登录 Grok Build 输入终端中显示的代码。FAKE - CODE 退出登录',
    button: { textContent: '继续' }
  })

  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  assert.equal(Object.prototype.hasOwnProperty.call(harness.values.get(core.CONFIG.sharedKeys.task), 'password'), false)
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.event).type, 'user_code_filled')
  assert.equal(harness.timers.runDelay(150), true)
  assert.equal(harness.button.clickCalls, 1)
})

test('可信 Grok Build 授权页会点击允许', () => {
  const harness = createDriverHarness({
    hostname: 'accounts.x.ai',
    pathname: '/oauth2/authorize',
    href: 'https://accounts.x.ai/oauth2/authorize?client_id=fake#grok-bulk-login=tab-1',
    title: 'Authorize Grok Build',
    windowName: 'grok-bulk-login:tab-1',
    values: {
      [core.CONFIG.sharedKeys.task]: createPasswordSubmittedTask({
        user_code: 'FAKE-CODE',
        verification_url: 'https://accounts.x.ai/oauth2/device',
        verification_launch_url: 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE'
      })
    },
    bodyText: '已以 user@example.com 身份登录 授权 Grok Build Use the xAI API Read your email address',
    button: { textContent: '允许' }
  })

  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  assert.equal(harness.button.clickCalls, 1)
  assert.equal(harness.values.get(core.CONFIG.sharedKeys.event).type, 'authorization_submitted')
})

test('未提交密码前 Device Flow 页面没有设备码或登录控件才回邮箱登录入口', () => {
  const harness = createDriverHarness({
    hostname: 'accounts.x.ai',
    pathname: '/oauth2/device',
    href: 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE#grok-bulk-login=tab-1',
    title: 'Device Sign-in | SpaceXAI Accounts',
    windowName: 'grok-bulk-login:tab-1',
    values: {
      [core.CONFIG.sharedKeys.task]: createActiveTask({
        verification_url: 'https://accounts.x.ai/oauth2/device',
        verification_launch_url: 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE'
      })
    },
    bodyText: '正在准备设备登录'
  })

  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  assert.equal(harness.location.href, 'https://accounts.x.ai/oauth2/device?user_code=FAKE-CODE#grok-bulk-login=tab-1')

  harness.advanceTime(core.CONFIG.loginToVerificationDelayMs + 1)
  harness.triggerMutation()
  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  assert.equal(harness.location.href, 'https://accounts.x.ai/sign-in#grok-bulk-login=tab-1')
})

test('密码提交后中文 Device Flow 页面通过附近文案填入设备码并提交', () => {
  const harness = createDriverHarness({
    hostname: 'accounts.x.ai',
    pathname: '/oauth2/device',
    href: 'https://accounts.x.ai/oauth2/device',
    title: 'Device Sign-in | SpaceXAI Accounts',
    windowName: 'grok-bulk-login:tab-1',
    values: {
      [core.CONFIG.sharedKeys.task]: createPasswordSubmittedTask({
        verification_url: 'https://accounts.x.ai/oauth2/device'
      })
    },
    bodyText: '登录 Grok Build 输入终端中显示的代码。仅当您刚刚从设备发起登录时才输入此代码。',
    input: {
      type: 'text',
      nearbyText: '登录 Grok Build 输入终端中显示的代码。输入设备代码 仅当您刚刚从设备发起登录时才输入此代码。'
    },
    button: { textContent: '继续' }
  })

  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  assert.equal(harness.input.value, 'FAKE-CODE')
  assert.equal(harness.timers.runDelay(150), true)
  assert.equal(harness.button.clickCalls, 1)
})

test('密码提交后填入设备码的延迟提交会重新查找刚启用的继续按钮', () => {
  const harness = createDriverHarness({
    hostname: 'accounts.x.ai',
    pathname: '/oauth2/device',
    href: 'https://accounts.x.ai/oauth2/device',
    windowName: 'grok-bulk-login:tab-1',
    values: {
      [core.CONFIG.sharedKeys.task]: createPasswordSubmittedTask({
        verification_url: 'https://accounts.x.ai/oauth2/device'
      })
    },
    input: {
      type: 'text',
      nearbyText: '输入设备代码'
    },
    button: { textContent: '继续', disabled: true }
  })

  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)
  assert.equal(harness.input.value, 'FAKE-CODE')
  harness.button.disabled = false
  assert.equal(harness.timers.runDelay(150), true)
  assert.equal(harness.button.clickCalls, 1)
  assert.equal(harness.input.events.some(event => event.type === 'keydown' || event.type === 'keyup'), false)
})

test('清理标签返回包含 cleanup_id 和 host 的成功 ACK', async () => {
  const request = {
    run_id: 'run-1',
    account_id: 'account-1',
    cleanup_id: 'cleanup-1',
    tab_marker: 'cleanup-tab-1',
    target_host: 'auth.x.ai',
    at: Date.now()
  }
  const harness = createDriverHarness({
    hash: '#grok-bulk-cleanup=cleanup-tab-1',
    values: { [core.CONFIG.sharedKeys.cleanup]: request }
  })

  await harness.flush()
  const ack = harness.values.get(core.CONFIG.sharedKeys.cleanupAck)

  assert.equal(ack.cleanup_id, 'cleanup-1')
  assert.equal(ack.host, 'auth.x.ai')
  assert.equal(ack.ok, true)
  assert.equal(harness.localStorage.clearCalls, 1)
  assert.equal(harness.sessionStorage.clearCalls, 1)
})

test('清理请求不会接受登录标签 marker', async () => {
  const request = {
    run_id: 'run-1',
    account_id: 'account-1',
    cleanup_id: 'cleanup-login-marker',
    tab_marker: 'cleanup-tab-login',
    target_host: 'auth.x.ai',
    at: Date.now()
  }
  const harness = createDriverHarness({
    hash: '#grok-bulk-login=cleanup-tab-login',
    values: { [core.CONFIG.sharedKeys.cleanup]: request }
  })

  await harness.flush()

  assert.equal(harness.values.has(core.CONFIG.sharedKeys.cleanupAck), false)
})

test('站点存储清理异常返回失败 ACK', async () => {
  const harness = createDriverHarness({
    hash: '#grok-bulk-cleanup=cleanup-tab-2',
    failStorageCleanup: true,
    values: {
      [core.CONFIG.sharedKeys.cleanup]: {
        run_id: 'run-2',
        account_id: 'account-2',
        cleanup_id: 'cleanup-2',
        tab_marker: 'cleanup-tab-2',
        target_host: 'auth.x.ai',
        at: Date.now()
      }
    }
  })

  await harness.flush()
  const ack = harness.values.get(core.CONFIG.sharedKeys.cleanupAck)

  assert.equal(ack.ok, false)
  assert.equal(ack.error_code, 'CLEANUP_STORAGE_FAILED')
})
