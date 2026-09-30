<template>
  <view class="wrap">
    <view class="hd">
      <view class="logo">
        <text class="logoIcon">🖱</text>
      </view>
      <text class="title">键鼠APP</text>
      <text class="sub">把手机变成电脑的无线键鼠</text>
    </view>

    <view class="card">
      <view class="row">
        <text class="lb">电脑地址</text>
        <input
          class="ipt"
          v-model="addr"
          type="text"
          placeholder="192.168.1.5:8123"
          placeholder-class="ph"
          :adjust-position="true"
        />
      </view>
      <view class="row">
        <text class="lb">访问密钥</text>
        <input
          class="ipt"
          v-model="token"
          type="text"
          placeholder="扫码会自动填入"
          placeholder-class="ph"
        />
      </view>

      <text class="err" v-if="err">{{ err }}</text>

      <button class="btn primary" @click="doConnect">
        <text class="btnText">连 接</text>
      </button>
      <button class="btn ghost" @click="doScan">
        <text class="btnText ghostText">扫 码 连 接</text>
      </button>
    </view>

    <view class="card" v-if="recent.length > 0">
      <text class="cardTitle">最近连接</text>
      <view
        class="item"
        v-for="(r, i) in recent"
        :key="r.key"
        @click="useRecent(r)"
      >
        <view class="itemMain">
          <text class="itemHost">{{ r.host }}:{{ r.port }}</text>
          <text class="itemMeta">{{ r.token ? '密钥 ' + r.token : '无密钥' }} · {{ r.time }}</text>
        </view>
        <view class="itemDel" @click.stop="delRecent(i)">
          <text class="itemDelText">✕</text>
        </view>
      </view>
    </view>

    <view class="tip">
      <text class="tipTitle">使用三步</text>
      <text class="tipLine">1. 电脑上双击运行 phonekvm.exe</text>
      <text class="tipLine">2. 手机与电脑连同一个 Wi-Fi</text>
      <text class="tipLine">3. 扫电脑窗口里的二维码</text>
      <text class="tipLine dim">首次运行 Windows 会弹防火墙提示，请点「允许」</text>
    </view>
  </view>
</template>

<script>
import { parseTarget } from '../../utils/kvm.js'

const RECENT_KEY = 'kvm.recent'
const LAST_KEY = 'kvm.last'

export default {
  data() {
    return {
      addr: '',
      token: '',
      recent: [],
      err: ''
    }
  },
  onLoad() {
    const last = uni.getStorageSync(LAST_KEY)
    if (last && last.host) {
      this.addr = last.host + ':' + last.port
      this.token = last.token || ''
    }
    this.loadRecent()
  },
  methods: {
    loadRecent() {
      const list = uni.getStorageSync(RECENT_KEY)
      this.recent = Array.isArray(list) ? list : []
    },
    saveRecent(target) {
      const list = this.recent.filter(
        (r) => !(r.host === target.host && r.port === target.port && r.token === target.token)
      )
      const now = new Date()
      const time =
        (now.getMonth() + 1) + '月' + now.getDate() + '日 ' +
        String(now.getHours()).padStart(2, '0') + ':' + String(now.getMinutes()).padStart(2, '0')
      list.unshift({
        host: target.host,
        port: target.port,
        token: target.token,
        time,
        key: target.host + ':' + target.port + '/' + target.token
      })
      this.recent = list.slice(0, 5)
      uni.setStorageSync(RECENT_KEY, this.recent)
    },
    delRecent(i) {
      this.recent.splice(i, 1)
      uni.setStorageSync(RECENT_KEY, this.recent)
    },
    useRecent(r) {
      this.addr = r.host + ':' + r.port
      this.token = r.token || ''
      this.doConnect()
    },
    doScan() {
      this.err = ''
      uni.scanCode({
        scanType: ['qrCode'],
        success: (res) => {
          const raw = res.result || ''
          const t = parseTarget(raw)
          if (!t) {
            this.err = '二维码内容无法识别：' + raw
            return
          }
          this.addr = t.host + ':' + t.port
          this.token = t.token
          this.err = ''
          uni.vibrateShort({ fail: () => {} })
          setTimeout(() => this.doConnect(), 120)
        },
        fail: (e) => {
          // 用户主动取消不提示
          const msg = (e && e.errMsg) || ''
          if (msg.indexOf('cancel') < 0) {
            this.err = '扫码失败，请检查相机权限'
          }
        }
      })
    },
    doConnect() {
      this.err = ''
      const raw = String(this.addr || '').trim()
      if (!raw) {
        this.err = '请先填写电脑地址'
        return
      }
      // 允许把密钥直接跟在地址后面一起输入，例如 192.168.1.5:8123/abcd1234
      let input = raw
      const tk = String(this.token || '').trim()
      if (tk && raw.indexOf('/') < 0) input = raw + '/' + tk

      let target = parseTarget(input)
      if (!target) {
        this.err = '地址格式不对，应该像 192.168.1.5:8123'
        return
      }
      if (tk && !target.token) target = parseTarget(raw + '/' + tk) || target
      if (!tk && !target.token) {
        // 没密钥仍允许连（PC 端未设密钥时），但给出提醒
        this.err = ''
      }

      uni.setStorageSync(LAST_KEY, target)
      uni.setStorageSync('kvm.current', target)
      this.saveRecent(target)
      uni.redirectTo({ url: '/pages/pad/pad' })
    }
  }
}
</script>

<style scoped>
.wrap {
  min-height: 100%;
  padding: 60rpx 40rpx 40rpx;
  background: linear-gradient(180deg, #12141a 0%, #161a24 60%, #12141a 100%);
}

.hd {
  align-items: center;
  display: flex;
  flex-direction: column;
  padding: 40rpx 0 48rpx;
}

.logo {
  width: 140rpx;
  height: 140rpx;
  border-radius: 36rpx;
  background: linear-gradient(135deg, #2f80ed, #56ccf2);
  align-items: center;
  justify-content: center;
  display: flex;
  box-shadow: 0 12rpx 40rpx rgba(47, 128, 237, 0.35);
}

.logoIcon {
  font-size: 68rpx;
  line-height: 68rpx;
}

.title {
  margin-top: 28rpx;
  font-size: 44rpx;
  font-weight: 700;
  color: #f2f5ff;
  letter-spacing: 2rpx;
}

.sub {
  margin-top: 12rpx;
  font-size: 26rpx;
  color: #8b95a8;
}

.card {
  background: #1b202c;
  border-radius: 24rpx;
  padding: 32rpx 28rpx;
  margin-bottom: 28rpx;
  border: 1rpx solid #262c3a;
}

.row {
  margin-bottom: 24rpx;
}

.lb {
  font-size: 24rpx;
  color: #8b95a8;
  margin-bottom: 12rpx;
  display: block;
}

.ipt {
  height: 88rpx;
  background: #12161f;
  border-radius: 16rpx;
  padding: 0 24rpx;
  font-size: 30rpx;
  color: #e8eaf0;
  border: 1rpx solid #2a3140;
}

.ph {
  color: #5d6675;
}

.err {
  display: block;
  font-size: 24rpx;
  color: #ff7a7a;
  margin-bottom: 16rpx;
}

.btn {
  height: 92rpx;
  border-radius: 18rpx;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-top: 16rpx;
  border: none;
}

.btn.primary {
  background: linear-gradient(135deg, #2f80ed, #56ccf2);
  box-shadow: 0 10rpx 26rpx rgba(47, 128, 237, 0.3);
}

.btn.ghost {
  background: #232a38;
  border: 1rpx solid #323a4b;
}

.btnText {
  font-size: 32rpx;
  font-weight: 600;
  color: #ffffff;
  letter-spacing: 4rpx;
}

.ghostText {
  color: #c3cbd9;
}

.cardTitle {
  font-size: 26rpx;
  color: #8b95a8;
  margin-bottom: 8rpx;
  display: block;
}

.item {
  display: flex;
  align-items: center;
  padding: 22rpx 0;
  border-bottom: 1rpx solid #242a37;
}

.itemMain {
  flex: 1;
}

.itemHost {
  font-size: 30rpx;
  color: #e8eaf0;
  display: block;
}

.itemMeta {
  font-size: 22rpx;
  color: #6f7889;
  margin-top: 6rpx;
  display: block;
}

.itemDel {
  width: 60rpx;
  height: 60rpx;
  align-items: center;
  justify-content: center;
  display: flex;
}

.itemDelText {
  font-size: 28rpx;
  color: #5d6675;
}

.tip {
  padding: 8rpx 12rpx;
}

.tipTitle {
  font-size: 24rpx;
  color: #6f7889;
  display: block;
  margin-bottom: 12rpx;
}

.tipLine {
  font-size: 24rpx;
  color: #8b95a8;
  line-height: 44rpx;
  display: block;
}

.tipLine.dim {
  color: #5d6675;
  margin-top: 10rpx;
}
</style>
