/**
 * 与 PC 端 phone-kvm/server.go 的二进制协议严格对齐。
 * 字节序一律小端（little-endian）。
 *
 * 手机 -> 电脑
 *   0x01 cMove    int16 dx, int16 dy
 *   0x02 cButton  uint8 键位(1左 2右 3中 4后退 5前进), uint8 1按下 0抬起
 *   0x03 cWheel   int16 垂直, int16 水平（单位 1/120 格）
 *   0x04 cKey     uint16 虚拟键码, uint8 1按下 0抬起
 *   0x05 cText    uint16 字节数 + UTF-8 正文
 *   0x06 cPing    uint32 序号，原样回 sPong
 *   0x08 cCenter  无载荷：光标回主屏中央
 *   0x09 cQuery   无载荷：查询光标 -> sCursor
 *
 * 电脑 -> 手机
 *   0x07 sPong    原样回 cPing 的 4 字节
 *   0x0A sCursor  int32 x, int32 y
 */

export const C = {
  MOVE: 0x01,
  BUTTON: 0x02,
  WHEEL: 0x03,
  KEY: 0x04,
  TEXT: 0x05,
  PING: 0x06,
  CENTER: 0x08,
  QUERY: 0x09
}

export const S = {
  PONG: 0x07,
  CURSOR: 0x0A
}

/** Windows 虚拟键码，与 PC 端 win_input.go 使用的 VK_* 一致 */
export const VK = {
  CTRL: 17, ALT: 18, SHIFT: 16, WIN: 91, SPACE: 32,
  ENTER: 13, ESC: 27, TAB: 9, BACK: 8, DEL: 46, INSERT: 45,
  LEFT: 37, UP: 38, RIGHT: 39, DOWN: 40,
  HOME: 36, END: 35, PGUP: 33, PGDN: 34,
  F1: 112, F2: 113, F3: 114, F4: 115, F5: 116, F6: 117,
  F7: 118, F8: 119, F9: 120, F10: 121, F11: 122, F12: 123
}

/** 鼠标键位编号（对应 cButton 的第二个字节） */
export const BTN = { LEFT: 1, RIGHT: 2, MIDDLE: 3, BACK: 4, FORWARD: 5 }

// ------------------------------------------------------------------ UTF-8

/** 自实现 UTF-8 编码，避免依赖部分 WebView 缺失的 TextEncoder */
export function utf8Encode(str) {
  const out = []
  for (let i = 0; i < str.length; i++) {
    let c = str.codePointAt(i)
    if (c > 0xffff) i++ // 代理对占两个码元，codePointAt 已给出完整码点
    if (c < 0x80) {
      out.push(c)
    } else if (c < 0x800) {
      out.push(0xc0 | (c >> 6), 0x80 | (c & 0x3f))
    } else if (c < 0x10000) {
      out.push(0xe0 | (c >> 12), 0x80 | ((c >> 6) & 0x3f), 0x80 | (c & 0x3f))
    } else {
      out.push(
        0xf0 | (c >> 18),
        0x80 | ((c >> 12) & 0x3f),
        0x80 | ((c >> 6) & 0x3f),
        0x80 | (c & 0x3f)
      )
    }
  }
  return new Uint8Array(out)
}

// ------------------------------------------------------------------ 编码

export function encMove(dx, dy) {
  const buf = new ArrayBuffer(5)
  const u = new Uint8Array(buf)
  const v = new DataView(buf)
  u[0] = C.MOVE
  v.setInt16(1, dx, true)
  v.setInt16(3, dy, true)
  return buf
}

export function encButton(btn, down) {
  const buf = new ArrayBuffer(3)
  const u = new Uint8Array(buf)
  u[0] = C.BUTTON
  u[1] = btn
  u[2] = down ? 1 : 0
  return buf
}

export function encWheel(dv, dh) {
  const buf = new ArrayBuffer(5)
  const u = new Uint8Array(buf)
  const v = new DataView(buf)
  u[0] = C.WHEEL
  v.setInt16(1, dv, true)
  v.setInt16(3, dh, true)
  return buf
}

export function encKey(vk, down) {
  const buf = new ArrayBuffer(4)
  const u = new Uint8Array(buf)
  const v = new DataView(buf)
  u[0] = C.KEY
  v.setUint16(1, vk, true)
  u[3] = down ? 1 : 0
  return buf
}

export function encText(str) {
  const body = utf8Encode(str)
  if (!body.length) return null
  const buf = new ArrayBuffer(3 + body.length)
  const u = new Uint8Array(buf)
  const v = new DataView(buf)
  u[0] = C.TEXT
  v.setUint16(1, body.length, true)
  u.set(body, 3)
  return buf
}

export function encPing(seq) {
  const buf = new ArrayBuffer(5)
  const u = new Uint8Array(buf)
  const v = new DataView(buf)
  u[0] = C.PING
  v.setUint32(1, seq >>> 0, true)
  return buf
}

export function encSimple(type) {
  const buf = new ArrayBuffer(1)
  new Uint8Array(buf)[0] = type
  return buf
}

// ------------------------------------------------------------ 地址解析

/**
 * 解析用户输入 / 二维码内容，得到连接目标。
 * 支持这些写法：
 *   http://192.168.1.5:8123/abcd1234/
 *   192.168.1.5:8123/abcd1234
 *   192.168.1.5:8123
 *   192.168.1.5/abcd1234
 *   192.168.1.5
 */
export function parseTarget(input) {
  let s = String(input == null ? '' : input).trim()
  if (!s) return null
  s = s.replace(/^[a-z]+:\/\//i, '') // 去掉 http:// https:// ws:// 等
  s = s.replace(/[/?#].*$/, (m) => m) // 保留路径部分，下面单独切
  let host = s
  let token = ''
  const slash = s.indexOf('/')
  if (slash >= 0) {
    host = s.slice(0, slash)
    token = s.slice(slash + 1).replace(/[^0-9A-Za-z_-].*$/, '')
  }
  let port = 8123
  const colon = host.lastIndexOf(':')
  if (colon >= 0) {
    const p = parseInt(host.slice(colon + 1), 10)
    if (p > 0 && p < 65536) {
      port = p
      host = host.slice(0, colon)
    }
  }
  host = host.replace(/[^0-9A-Za-z.\-]/g, '')
  if (!host) return null
  return {
    host,
    port,
    token,
    wsURL: `ws://${host}:${port}/${token ? token + '/' : ''}ws`
  }
}

/** 组合出一个便于展示的地址串 */
export function formatTarget(t) {
  if (!t) return ''
  return `${t.host}:${t.port}${t.token ? '/' + t.token : ''}`
}

// ------------------------------------------------------- 粘滞修饰键顺序

export const MOD_ORDER = [VK.CTRL, VK.ALT, VK.SHIFT, VK.WIN]

export const MOD_LABEL = {
  [VK.CTRL]: 'Ctrl',
  [VK.ALT]: 'Alt',
  [VK.SHIFT]: 'Shift',
  [VK.WIN]: 'Win'
}

/** 单个可打印字符对应的虚拟键码（用于配合修饰键发 Ctrl+C 这类组合） */
export function charVk(ch) {
  const c = String(ch).toLowerCase()
  if (c >= 'a' && c <= 'z') return c.charCodeAt(0) - 32
  if (c >= '0' && c <= '9') return c.charCodeAt(0)
  return 0
}
