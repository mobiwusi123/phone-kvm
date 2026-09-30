/**
 * 端到端协议自检：模拟 APP 的握手与二进制收发，验证与 PC 端 phonekvm.exe 的兼容性。
 * 只发 ping 和 query（不会注入任何键鼠事件），收到 pong 和 cursor 即判定通过。
 * 用法：node ws_check.mjs ws://127.0.0.1:8199/testtest/ws
 */
import { writeFileSync } from 'node:fs'

const url = process.argv[2]
const LOG = 'D:\\_wstest.log'
const log = []
let done = false

function finish(code) {
  if (done) return
  done = true
  try {
    writeFileSync(LOG, log.join('\n'), 'utf8')
  } catch (e) { /* 忽略 */ }
  process.exit(code)
}

const t0 = Date.now()
const ws = new WebSocket(url)
ws.binaryType = 'arraybuffer'

ws.onopen = () => {
  log.push('OPEN ok (' + (Date.now() - t0) + 'ms)')
  // 发 cPing(0x06) + uint32 seq
  const b = new ArrayBuffer(5)
  const u = new Uint8Array(b)
  const v = new DataView(b)
  u[0] = 0x06
  v.setUint32(1, 7, true)
  ws.send(b)
  log.push('SENT ping seq=7')
}

ws.onmessage = (ev) => {
  if (typeof ev.data === 'string') {
    log.push('RECV text: ' + ev.data)
    return
  }
  const dv = new DataView(ev.data)
  const u = new Uint8Array(ev.data)
  log.push('RECV bin len=' + u.length + ' type=0x' + u[0].toString(16))
  if (u[0] === 0x07) {
    log.push('PONG ok, seq=' + dv.getUint32(1, true))
    // 再测 cQuery(0x09) -> sCursor(0x0A)
    const q = new ArrayBuffer(1)
    new Uint8Array(q)[0] = 0x09
    ws.send(q)
    log.push('SENT query')
  } else if (u[0] === 0x0a) {
    log.push('CURSOR x=' + dv.getUint32(1, true) + ' y=' + dv.getUint32(5, true))
    log.push('RESULT: ALL_OK')
    ws.close()
    setTimeout(() => finish(0), 100)
  }
}

ws.onerror = () => {
  log.push('ERROR: 连接失败或握手被拒')
  finish(2)
}

ws.onclose = () => {
  log.push('CLOSED')
  finish(log.indexOf('RESULT: ALL_OK') >= 0 ? 0 : 3)
}

setTimeout(() => {
  log.push('TIMEOUT: 8s 内没有完成握手/应答')
  finish(1)
}, 8000)
