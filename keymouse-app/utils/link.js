/**
 * WebSocket 连接管理：封装 uni.connectSocket，负责自动重连、心跳测延迟。
 * 二进制帧直接发 ArrayBuffer，PC 端 dispatch() 就是这个格式。
 */
import { encPing, C, S } from './kvm.js'

export function createLink() {
  const state = {
    task: null,
    open: false,
    url: '',
    retry: 0,
    retryTimer: null,
    pingTimer: null,
    seq: 0,
    pingAt: 0,
    rtt: 0,
    killed: false
  }

  let handlers = {
    onOpen() {},
    onClose() {},
    onStatus() {},
    onRtt() {},
    onBye() {}
  }

  function clearTimers() {
    if (state.retryTimer) {
      clearTimeout(state.retryTimer)
      state.retryTimer = null
    }
    if (state.pingTimer) {
      clearInterval(state.pingTimer)
      state.pingTimer = null
    }
  }

  function setOpen(v) {
    state.open = v
    handlers.onStatus(v)
  }

  function scheduleReconnect() {
    if (state.killed || !state.url) return
    clearTimeout(state.retryTimer)
    state.retry = Math.min(state.retry + 1, 8)
    const wait = Math.min(400 * state.retry, 4000)
    state.retryTimer = setTimeout(() => connect(state.url, handlers), wait)
  }

  function handleMessage(res) {
    const data = res && res.data
    if (typeof data === 'string') {
      // PC 端退出时会推一条 {"t":"bye"}
      if (data.indexOf('"bye"') >= 0) handlers.onBye()
      return
    }
    // 二进制：目前只有 sPong 需要处理（测延迟）
    try {
      const u = new Uint8Array(data)
      if (u[0] === S.PONG) {
        state.rtt = Math.round(Date.now() - state.pingAt)
        handlers.onRtt(state.rtt)
      }
    } catch (e) {
      /* 忽略无法解析的帧 */
    }
  }

  function connect(url, h) {
    if (h) handlers = Object.assign(handlers, h)
    state.url = url
    state.killed = false
    clearTimeout(state.retryTimer)

    try {
      if (state.task) {
        try { state.task.close({}) } catch (e) { /* 忽略 */ }
      }
    } catch (e) { /* 忽略 */ }

    setOpen(false)

    let task
    try {
      task = uni.connectSocket({
        url,
        complete: () => {}
      })
    } catch (e) {
      scheduleReconnect()
      return
    }
    state.task = task

    task.onOpen(() => {
      state.retry = 0
      setOpen(true)
      handlers.onOpen()
      // 心跳：每 2 秒一次，用来算 RTT 并保活
      clearInterval(state.pingTimer)
      state.pingTimer = setInterval(() => {
        if (!state.open) return
        state.seq++
        state.pingAt = Date.now()
        send(encPing(state.seq))
      }, 2000)
    })

    task.onMessage(handleMessage)

    task.onClose(() => {
      setOpen(false)
      clearInterval(state.pingTimer)
      handlers.onClose()
      scheduleReconnect()
    })

    task.onError(() => {
      // 随后一般会触发 onClose，统一在那里重连
    })
  }

  function send(buf) {
    if (!buf || !state.open || !state.task) return false
    try {
      state.task.send({
        data: buf,
        fail: () => { /* 发送失败由 onClose 统一处理 */ }
      })
      return true
    } catch (e) {
      return false
    }
  }

  function close() {
    state.killed = true
    clearTimers()
    state.url = ''
    if (state.task) {
      try { state.task.close({}) } catch (e) { /* 忽略 */ }
      state.task = null
    }
    setOpen(false)
  }

  /** 只断开但保留重连意图（切后台时用） */
  function pause() {
    clearTimers()
    if (state.task) {
      try { state.task.close({}) } catch (e) { /* 忽略 */ }
      state.task = null
    }
    setOpen(false)
  }

  /** 从暂停中恢复 */
  function resume() {
    if (state.killed || !state.url) return
    state.retry = 0
    connect(state.url, handlers)
  }

  return {
    connect,
    send,
    close,
    pause,
    resume,
    get open() { return state.open },
    get rtt() { return state.rtt },
    get url() { return state.url }
  }
}
