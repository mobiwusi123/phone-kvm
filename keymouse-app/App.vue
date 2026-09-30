<script>
export default {
  onLaunch() {
    console.log('[键鼠APP] 启动')
  },
  onShow() {
    console.log('[键鼠APP] 前台')
  },
  onHide() {
    console.log('[键鼠APP] 后台')
  }
}
</script>

<style>
/* 全局基础样式：所有页面共用 */
page {
  height: 100%;
  background-color: #12141a;
  color: #e8eaf0;
  font-family: -apple-system, "PingFang SC", "Helvetica Neue", "Microsoft YaHei", sans-serif;
  -webkit-user-select: none;
  user-select: none;
  -webkit-tap-highlight-color: transparent;
}

view,
text,
button,
input {
  box-sizing: border-box;
}
</style>
