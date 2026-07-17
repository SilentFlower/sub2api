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
  return {
    length: 0,
    clearCalls: 0,
    clear() {
      this.clearCalls++
      if (failClear) throw new Error('fake storage cleanup failure')
      this.length = 0
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
    attributes: options.input.attributes || {}
  }) : null
  const button = options.button ? new FakeButton({
    textContent: options.button.textContent || 'Continue',
    attributes: options.button.attributes || {}
  }) : null
  const body = { innerText: options.bodyText || '' }
  const documentElement = {}
  const document = {
    title: options.title || 'Sign in to xAI',
    body,
    documentElement,
    querySelectorAll(selector) {
      if (selector === 'input') return input ? [input] : []
      if (selector === 'iframe') return []
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
    name: '',
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

test('登录入口无表单时导航到官方 Device Flow 验证页', () => {
  const harness = createDriverHarness({
    hostname: 'accounts.x.ai',
    pathname: '/',
    href: 'https://accounts.x.ai/#grok-bulk-login=tab-1',
    title: 'xAI Accounts',
    values: {
      [core.CONFIG.sharedKeys.task]: createActiveTask({
        verification_url: 'https://accounts.x.ai/oauth2/device'
      })
    }
  })

  harness.advanceTime(core.CONFIG.loginToVerificationDelayMs + 1)
  assert.equal(harness.timers.runDelay(core.CONFIG.scanDebounceMs), true)

  assert.equal(harness.location.href, 'https://accounts.x.ai/oauth2/device')
  assert.equal(harness.window.name, 'grok-bulk-login:tab-1')
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
