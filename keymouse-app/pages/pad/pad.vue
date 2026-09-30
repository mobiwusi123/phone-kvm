<template>
  <view class="page">
    <!-- ============ 顶栏 ============ -->
    <view class="top">
      <view class="stat">
        <view class="dot" :class="connected ? 'dotOn' : 'dotOff'"></view>
        <text class="statText">{{ statusText }}</text>
      </view>
      <text class="rtt" v-if="connected">{{ rtt }} ms</text>
      <view class="spacer"></view>
      <view class="tbtn" hover-class="tbtnHi" @click="toggleKb">
        <text class="tbtnText">键盘</text>
      </view>
      <view class="tbtn" hover-class="tbtnHi" @click="settingsOn = !settingsOn">
        <text class="tbtnText">设置</text>
      </view>
      <view class="tbtn" hover-class="tbtnHi" @click="quit">
        <text class="tbtnText">退出</text>
      </view>
    </view>

    <!-- ============ 触摸板 ============ -->
    <view
      class="pad"
      @touchstart="padStart"
      @touchmove.stop.prevent="padMove"
      @touchend.prevent="padEnd"
      @touchcancel.prevent="padEnd"
    >
      <text class="padHint" :style="{ opacity: hintOpacity }">
        单指滑动移动光标 · 轻点左键 · 双指轻点右键
      </text>
      <text class="padHint2" :style="{ opacity: hintOpacity }">
        双指滑动滚动 · 双击后按住可拖拽
      </text>
    </view>

    <!-- ============ 鼠标键 ============ -->
    <view class="mbar">
      <view
        class="mbtn"
        :class="{ mbtnOn: pressed === 1 }"
        hover-class="none"
        @touchstart="mDown(1)"
        @touchend="mUp(1)"
        @touchcancel="mUp(1)"
      >
        <text class="mbtnText">左键</text>
      </view>
      <view
        class="mbtn"
        :class="{ mbtnOn: pressed === 3 }"
        hover-class="none"
        @touchstart="mDown(3)"
        @touchend="mUp(3)"
        @touchcancel="mUp(3)"
      >
        <text class="mbtnText">中键</text>
      </view>
      <view
        class="mbtn"
        :class="{ mbtnOn: pressed === 2 }"
        hover-class="none"
        @touchstart="mDown(2)"
        @touchend="mUp(2)"
        @touchcancel="mUp(2)"
      >
        <text class="mbtnText">右键</text>
      </view>
      <view class="mbtn mbtnGhost" hover-class="tbtnHi" @click="doCenter">
        <text class="mbtnText">回中</text>
      </view>
    </view>

    <!-- ============ 键盘面板 ============ -->
    <view class="kb" v-if="kbOn">
      <view class="kbRow">
        <view
          class="key keyMod"
          v-for="m in modKeys"
          :key="'m' + m.vk"
          :class="{ keyOn: sticky.indexOf(m.vk) >= 0 }"
          hover-class="none"
          @click="tapMod(m.vk)"
        >
          <text class="keyText">{{ m.label }}</text>
        </view>
      </view>

      <view class="kbRow">
        <view
          class="key"
          v-for="k in fnKeys"
          :key="'f' + k.vk"
          hover-class="keyHi"
          @click="tapKey(k.vk)"
        >
          <text class="keyText">{{ k.label }}</text>
        </view>
      </view>

      <scroll-view class="kbScroll" scroll-x="true" :show-scrollbar="false">
        <view class="kbRowInner">
          <view
            class="key"
            v-for="k in fKeys"
            :key="'x' + k.vk"
            hover-class="keyHi"
            @click="tapKey(k.vk)"
          >
            <text class="keyText">{{ k.label }}</text>
          </view>
        </view>
      </scroll-view>

      <view class="imeRow">
        <input
          class="imeInput"
          v-model="imeText"
          type="text"
          confirm-type="send"
          :adjust-position="true"
          placeholder="在这里打字，内容会发到电脑"
          placeholder-class="ph"
          @confirm="sendIme"
        />
        <view class="imeBtn" hover-class="tbtnHi" @click="sendIme">
          <text class="imeBtnText">发送</text>
        </view>
      </view>
    </view>

    <!-- ============ 设置面板 ============ -->
    <view class="mask" v-if="settingsOn" @click="settingsOn = false"></view>
    <view class="sheet" v-if="settingsOn">
      <text class="sheetTitle">设置</text>

      <view class="setRow">
        <text class="setLabel">指针灵敏度</text>
        <text class="setVal">{{ sens.toFixed(2) }}</text>
      </view>
      <slider
        class="slider"
        :value="sens"
        :min="0.4"
        :max="4"
        :step="0.1"
        activeColor="#56ccf2"
        backgroundColor="#2a3140"
        block-size="22"
        @changing="onSens"
        @change="onSens"
      />

      <view class="setRow">
        <text class="setLabel">滚动速度</text>
        <text class="setVal">{{ scrollGain.toFixed(1) }}</text>
      </view>
      <slider
        class="slider"
        :value="scrollGain"
        :min="0.4"
        :max="3"
        :step="0.1"
        activeColor="#56ccf2"
        backgroundColor="#2a3140"
        block-size="22"
        @changing="onScroll"
        @change="onScroll"
      />

      <view class="swRow">
        <text class="setLabel">自然滚动</text>
        <switch
          :checked="natural"
          color="#56ccf2"
          @change="onNatural"
        />
      </view>

      <view class="swRow">
        <text class="setLabel">保持屏幕常亮</text>
        <switch
          :checked="keepOn"
          color="#56ccf2"
          @change="onKeep"
        />
      </view>

      <view class="setRow">
        <text class="setLabel">当前地址</text>
        <text class="setVal small">{{ targetText }}</text>
      </view>

      <view class="sheetBtn" hover-class="tbtnHi" @click="settingsOn = false">
        <text class="sheetBtnText">关 闭</text>
      </view>
    </view>
  </view>
</template>

<script>
import {
  encMove,
  encButton,
  encWheel,
  encKey,
  encText,
  encSimple,
  C,
  VK,
  BTN,
  MOD_ORDER,
  charVk
} from '../../utils/kvm.js'
import { createLink } from '../../utils/link.js'

function clamp16(v) {
  if (v > 32767) return 32767
  if (v < -32768) return -32768
  return v
}

export default {
  data() {
    return {
      connected: false,
      statusText: '未连接',
      rtt: 0,
      hintOpacity: 0.55,
      kbOn: false,
      settingsOn: false,
      sticky: [],
      pressed: 0,
      imeText: '',
      sens: 1.5,
      scrollGain: 1.4,
      natural: true,
      keepOn: true,
      targetText: '',
      modKeys: [
        { vk: VK.CTRL, label: 'Ctrl' },
        { vk: VK.ALT, label: 'Alt' },
        { vk: VK.SHIFT, label: 'Shift' },
        { vk: VK.WIN, label: 'Win' }
      ],
      fnKeys: [
        { vk: VK.ESC, label: 'Esc' },
        { vk: VK.TAB, label: 'Tab' },
        { vk: VK.ENTER, label: 'Enter' },
        { vk: VK.BACK, label: '退格' },
        { vk: VK.DEL, label: 'Del' },
        { vk: VK.SPACE, label: '空格' }
      ],
      fKeys: [
        { vk: VK.LEFT, label: '←' },
        { vk: VK.UP, label: '↑' },
        { vk: VK.DOWN, label: '↓' },
        { vk: VK.RIGHT, label: '→' },
        { vk: VK.HOME, label: 'Home' },
        { vk: VK.END, label: 'End' },
        { vk: VK.PGUP, label: 'PgUp' },
        { vk: VK.PGDN, label: 'PgDn' },
        { vk: VK.INSERT, label: 'Ins' },
        { vk: VK.F1, label: 'F1' },
        { vk: VK.F2, label: 'F2' },
        { vk: VK.F3, label: 'F3' },
        { vk: VK.F4, label: 'F4' },
        { vk: VK.F5, label: 'F5' },
        { vk: VK.F6, label: 'F6' },
        { vk: VK.F7, label: 'F7' },
        { vk: VK.F8, label: 'F8' },
        { vk: VK.F9, label: 'F9' },
        { vk: VK.F10, label: 'F10' },
        { vk: VK.F11, label: 'F11' },
        { vk: VK.F12, label: 'F12' }
      ]
    }
  },

  onLoad() {
    const t = uni.getStorageSync('kvm.current')
    if (!t || !t.wsURL) {
      uni.redirectTo({ url: '/pages/connect/connect' })
      return
    }
    this.targetText = t.host + ':' + t.port + (t.token ? '/' + t.token : '')

    const s = uni.getStorageSync('kvm.sens')
    if (s) this.sens = parseFloat(s) || 1.5
    const g = uni.getStorageSync('kvm.scroll')
    if (g) this.scrollGain = parseFloat(g) || 1.4
    const n = uni.getStorageSync('kvm.natural')
    if (n !== '' && n !== undefined && n !== null) this.natural = n === true || n === '1'
    const k = uni.getStorageSync('kvm.keep')
    if (k !== '' && k !== undefined && k !== null) this.keepOn = k === true || k === '1'
    if (this.keepOn) uni.setKeepScreenOn({ keepScreenOn: true, fail: () => {} })

    // ---- 非响应式运行时状态（避免每帧触发视图更新）
    this.lastX = 0
    this.lastY = 0
    this.scrollX = 0
    this.scrollY = 0
    this.accX = 0
    this.accY = 0
    this.accW = 0
    this.accH = 0
    this.mode = null
    this.dragging = false
    this.downAt = 0
    this.movedDist = 0
    this.twoFingers = false
    this.lastTapEnd = 0
    this.isComposing = false

    this.link = createLink()
    this.link.connect(t.wsURL, {
      onOpen: () => {
        uni.vibrateShort({ fail: () => {} })
      },
      onStatus: (on) => {
        this.connected = on
        this.statusText = on ? '已连接' : '连接中…'
      },
      onClose: () => {
        this.statusText = '已断开，重连中…'
      },
      onRtt: (ms) => {
        this.rtt = ms
      },
      onBye: () => {
        this.connected = false
        this.statusText = '电脑端已退出'
      }
    })

    // 每 16ms 把累积的位移/滚轮整发一次
    this.flushTimer = setInterval(() => this.flush(), 16)
  },

  onUnload() {
    if (this.flushTimer) clearInterval(this.flushTimer)
    if (this.link) this.link.close()
  },

  onHide() {
    if (this.link) this.link.pause()
  },

  onShow() {
    if (this.link) this.link.resume()
    if (this.keepOn) uni.setKeepScreenOn({ keepScreenOn: true, fail: () => {} })
  },

  onBackPress() {
    if (this.settingsOn) {
      this.settingsOn = false
      return true
    }
    this.quit()
    return true
  },

  methods: {
    // ------------------------------------------------------- 发送
    send(buf) {
      if (!this.link) return false
      return this.link.send(buf)
    },

    flush() {
      if (!this.link || !this.link.open) return
      if (this.accX !== 0 || this.accY !== 0) {
        const s = this.sens
        const mx = this.accX * s
        const my = this.accY * s
        const ix = Math.trunc(mx)
        const iy = Math.trunc(my)
        this.accX = (mx - ix) / s
        this.accY = (my - iy) / s
        if (ix !== 0 || iy !== 0) this.send(encMove(clamp16(ix), clamp16(iy)))
      }
      if (this.accW !== 0 || this.accH !== 0) {
        const iw = Math.trunc(this.accW)
        const ih = Math.trunc(this.accH)
        this.accW -= iw
        this.accH -= ih
        if (iw !== 0 || ih !== 0) {
          this.send(encWheel(clamp16(this.natural ? iw : -iw), clamp16(this.natural ? ih : -ih)))
        }
      }
    },

    // --------------------------------------------------- 触摸板
    padStart(e) {
      if (this.hintOpacity > 0) this.hintOpacity = 0
      const now = Date.now()
      const touches = e.touches || []
      if (touches.length === 1) {
        const t = touches[0]
        if (!this.dragging && now - this.lastTapEnd < 320) {
          // 双击后不抬手 = 按住左键拖拽
          this.dragging = true
          this.send(encButton(BTN.LEFT, true))
        }
        this.mode = 'move'
        this.lastX = t.clientX
        this.lastY = t.clientY
        this.downAt = now
        this.movedDist = 0
        this.twoFingers = false
      } else if (touches.length >= 2) {
        this.twoFingers = true
        if (this.dragging) {
          this.dragging = false
          this.send(encButton(BTN.LEFT, false))
        }
        this.mode = 'scroll'
        this.scrollX = (touches[0].clientX + touches[1].clientX) / 2
        this.scrollY = (touches[0].clientY + touches[1].clientY) / 2
      }
    },

    padMove(e) {
      const touches = e.touches || []
      if (this.mode === 'move' && touches.length === 1) {
        const t = touches[0]
        const dx = t.clientX - this.lastX
        const dy = t.clientY - this.lastY
        this.accX += dx
        this.accY += dy
        this.movedDist += Math.abs(dx) + Math.abs(dy)
        this.lastX = t.clientX
        this.lastY = t.clientY
      } else if (this.mode === 'scroll' && touches.length >= 2) {
        const cx = (touches[0].clientX + touches[1].clientX) / 2
        const cy = (touches[0].clientY + touches[1].clientY) / 2
        const dx = cx - this.scrollX
        const dy = cy - this.scrollY
        // 160 像素的滑动 ≈ 1 格滚轮（1 格 = 120 单位）
        this.accW += (dy / 160) * 120 * this.scrollGain
        this.accH += (dx / 160) * 120 * this.scrollGain
        this.movedDist += Math.abs(dx) + Math.abs(dy)
        this.scrollX = cx
        this.scrollY = cy
      }
    },

    padEnd(e) {
      const left = (e.touches && e.touches.length) || 0
      if (left > 0) return
      const dur = Date.now() - this.downAt
      if (this.dragging) {
        this.dragging = false
        this.send(encButton(BTN.LEFT, false))
      } else if (this.mode && this.movedDist < 14 && dur < 280) {
        if (this.twoFingers) {
          this.send(encButton(BTN.RIGHT, true))
          this.send(encButton(BTN.RIGHT, false))
          uni.vibrateShort({ fail: () => {} })
        } else {
          this.send(encButton(BTN.LEFT, true))
          this.send(encButton(BTN.LEFT, false))
          this.lastTapEnd = Date.now()
        }
      }
      this.mode = null
      this.twoFingers = false
    },

    // --------------------------------------------------- 鼠标键
    mDown(btn) {
      this.pressed = btn
      this.send(encButton(btn, true))
      uni.vibrateShort({ fail: () => {} })
    },

    mUp(btn) {
      if (this.pressed === btn) this.pressed = 0
      this.send(encButton(btn, false))
    },

    doCenter() {
      this.send(encSimple(C.CENTER))
      uni.vibrateShort({ fail: () => {} })
    },

    // --------------------------------------------------- 键盘
    tapMod(vk) {
      const i = this.sticky.indexOf(vk)
      if (i >= 0) this.sticky.splice(i, 1)
      else this.sticky.push(vk)
      uni.vibrateShort({ fail: () => {} })
    },

    tapKey(vk) {
      const active = MOD_ORDER.filter((m) => this.sticky.indexOf(m) >= 0)
      active.forEach((m) => this.send(encKey(m, true)))
      this.send(encKey(vk, true))
      this.send(encKey(vk, false))
      active.slice().reverse().forEach((m) => this.send(encKey(m, false)))
      if (active.length) this.sticky = []
      uni.vibrateShort({ fail: () => {} })
    },

    sendIme() {
      const v = String(this.imeText || '')
      if (!v) return
      // 带修饰键时按组合键发，否则整段文本粘贴式发送
      if (this.sticky.length) {
        if (v.length === 1) {
          const code = charVk(v)
          if (code) {
            this.tapKey(code)
            this.imeText = ''
            return
          }
        }
      }
      const buf = encText(v)
      if (buf) this.send(buf)
      this.imeText = ''
      uni.vibrateShort({ fail: () => {} })
    },

    toggleKb() {
      this.kbOn = !this.kbOn
      if (!this.kbOn) uni.hideKeyboard()
    },

    // --------------------------------------------------- 设置
    onSens(e) {
      this.sens = parseFloat(e.detail.value) || 1.5
      uni.setStorageSync('kvm.sens', this.sens)
    },
    onScroll(e) {
      this.scrollGain = parseFloat(e.detail.value) || 1.4
      uni.setStorageSync('kvm.scroll', this.scrollGain)
    },
    onNatural(e) {
      this.natural = !!e.detail.value
      uni.setStorageSync('kvm.natural', this.natural)
    },
    onKeep(e) {
      this.keepOn = !!e.detail.value
      uni.setStorageSync('kvm.keep', this.keepOn)
      uni.setKeepScreenOn({ keepScreenOn: this.keepOn, fail: () => {} })
    },

    quit() {
      if (this.link) this.link.close()
      setTimeout(() => {
        uni.redirectTo({ url: '/pages/connect/connect' })
      }, 60)
    }
  }
}
</script>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  height: 100vh;
  background: #12141a;
}

/* ---------------- 顶栏 ---------------- */
.top {
  display: flex;
  align-items: center;
  height: 92rpx;
  padding: 0 20rpx;
  background: #161a24;
  border-bottom: 1rpx solid #232a38;
  padding-top: env(safe-area-inset-top);
  box-sizing: content-box;
}

.stat {
  display: flex;
  align-items: center;
}

.dot {
  width: 16rpx;
  height: 16rpx;
  border-radius: 50%;
  margin-right: 12rpx;
}

.dotOn {
  background: #3ddc84;
  box-shadow: 0 0 12rpx rgba(61, 220, 132, 0.8);
}

.dotOff {
  background: #ff7a7a;
}

.statText {
  font-size: 24rpx;
  color: #c3cbd9;
}

.rtt {
  font-size: 22rpx;
  color: #6f7889;
  margin-left: 14rpx;
}

.spacer {
  flex: 1;
}

.tbtn {
  padding: 10rpx 18rpx;
  border-radius: 12rpx;
  background: #232a38;
  margin-left: 12rpx;
}

.tbtnHi {
  background: #2f3a4d;
}

.tbtnText {
  font-size: 24rpx;
  color: #c3cbd9;
}

/* ---------------- 触摸板 ---------------- */
.pad {
  flex: 1;
  margin: 20rpx;
  border-radius: 24rpx;
  background: #171c27;
  border: 2rpx dashed #2b3346;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
}

.padHint {
  font-size: 26rpx;
  color: #5d6675;
  transition: opacity 0.4s;
}

.padHint2 {
  font-size: 26rpx;
  color: #5d6675;
  margin-top: 12rpx;
  transition: opacity 0.4s;
}

/* ---------------- 鼠标键 ---------------- */
.mbar {
  display: flex;
  padding: 0 20rpx 16rpx;
}

.mbtn {
  flex: 1;
  height: 108rpx;
  margin: 0 8rpx;
  border-radius: 18rpx;
  background: #1e2531;
  border: 1rpx solid #2b3346;
  display: flex;
  align-items: center;
  justify-content: center;
}

.mbtnOn {
  background: linear-gradient(135deg, #2f80ed, #56ccf2);
  border-color: #56ccf2;
}

.mbtnGhost {
  background: #232a38;
}

.mbtnText {
  font-size: 28rpx;
  color: #dbe2ee;
}

/* ---------------- 键盘面板 ---------------- */
.kb {
  background: #161a24;
  border-top: 1rpx solid #232a38;
  padding: 14rpx 12rpx 8rpx;
  padding-bottom: calc(8rpx + env(safe-area-inset-bottom));
}

.kbRow {
  display: flex;
  margin-bottom: 10rpx;
}

.kbScroll {
  white-space: nowrap;
  margin-bottom: 10rpx;
}

.kbRowInner {
  display: inline-flex;
}

.key {
  min-width: 96rpx;
  height: 84rpx;
  padding: 0 16rpx;
  margin: 0 6rpx;
  border-radius: 14rpx;
  background: #222937;
  border: 1rpx solid #2d3547;
  display: flex;
  align-items: center;
  justify-content: center;
  flex: 1;
}

.keyHi {
  background: #2f3a4d;
}

.keyMod {
  background: #1f2733;
}

.keyOn {
  background: linear-gradient(135deg, #2f80ed, #56ccf2);
  border-color: #56ccf2;
}

.keyText {
  font-size: 25rpx;
  color: #dbe2ee;
}

.imeRow {
  display: flex;
  align-items: center;
}

.imeInput {
  flex: 1;
  height: 82rpx;
  background: #12161f;
  border-radius: 14rpx;
  padding: 0 20rpx;
  font-size: 28rpx;
  color: #e8eaf0;
  border: 1rpx solid #2a3140;
}

.ph {
  color: #5d6675;
}

.imeBtn {
  width: 140rpx;
  height: 82rpx;
  margin-left: 14rpx;
  border-radius: 14rpx;
  background: #232a38;
  display: flex;
  align-items: center;
  justify-content: center;
}

.imeBtnText {
  font-size: 28rpx;
  color: #c3cbd9;
}

/* ---------------- 设置面板 ---------------- */
.mask {
  position: fixed;
  left: 0;
  right: 0;
  top: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.55);
  z-index: 10;
}

.sheet {
  position: fixed;
  left: 0;
  right: 0;
  bottom: 0;
  background: #1b202c;
  border-radius: 28rpx 28rpx 0 0;
  padding: 36rpx 32rpx;
  padding-bottom: calc(36rpx + env(safe-area-inset-bottom));
  z-index: 11;
  border-top: 1rpx solid #2a3140;
}

.sheetTitle {
  font-size: 32rpx;
  color: #f2f5ff;
  font-weight: 600;
  display: block;
  margin-bottom: 20rpx;
}

.setRow {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 26rpx;
}

.setLabel {
  font-size: 28rpx;
  color: #c3cbd9;
}

.setVal {
  font-size: 28rpx;
  color: #56ccf2;
}

.setVal.small {
  font-size: 22rpx;
  color: #6f7889;
}

.slider {
  margin: 6rpx 0 0;
}

.swRow {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 26rpx;
}

.sheetBtn {
  margin-top: 36rpx;
  height: 92rpx;
  border-radius: 18rpx;
  background: #232a38;
  display: flex;
  align-items: center;
  justify-content: center;
}

.sheetBtnText {
  font-size: 30rpx;
  color: #dbe2ee;
}
</style>
