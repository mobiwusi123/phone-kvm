# PhoneKVM · 用手机当电脑的虚拟键鼠

手机连上同一个局域网，就能变成 Windows 电脑的**无线触摸板 + 键盘 + 滚轮**。
电脑端只跑一个单文件 exe，**PC 端零改动**，也**不需要管理员权限**。

## 下载

| 我要…… | 文件 | 大小 |
| --- | --- | --- |
| **在手机上装个 App**（推荐） | [**`phonekvm.apk`**](phonekvm.apk) | 19.8 MB |
| 只要 Windows 服务端 | [`phonekvm.exe`](phonekvm.exe) | 6.8 MB |
| 不装 App，用浏览器 | 打开 exe 打印出来的网址就行 | — |

APK 是 **v1.0.0 正式版，不含广告**。装之前要在手机设置里允许「安装未知来源应用」，
而且手机和电脑必须在同一个局域网（同一个 Wi-Fi，或者手机连电脑开的热点）。

## 快速开始

1. 电脑上双击 `phonekvm.exe`。
2. 控制台会打印二维码，最多两张：`1/2` 是「手机连同一个 Wi-Fi 时扫这张」，`2/2` 是「手机连电脑热点时扫这张」。
3. 首次运行 Windows 防火墙会弹窗，点**允许**。
4. 手机扫码就连上了。用浏览器的话，也可以手动输入二维码下方列出的网址。

二维码是用半块字符（`█▀▄`）画的，所以终端窗口至少要 45 列宽、字体得包含方块字符。
程序启动时会自动把控制台码页切成 UTF-8，否则中文和方块字符会乱码、码也扫不出来。

## 两种客户端怎么选

| | 网页版 | Android App |
| --- | --- | --- |
| 手机端形态 | 浏览器网页 / 添加到主屏幕 | 独立 APK |
| 扫码连接 | 靠浏览器扫，或手输网址 | 内置扫码，扫完自动连接 |
| 中文输入 | 依赖手机输入法逐键上屏 | 输入框整段发送 |
| 屏幕常亮 | 做不到 | 可保持屏幕常亮 |
| 连接状态 | 无 | 顶栏显示状态 + RTT 延迟 |

两者复用同一套协议，配同一个 `phonekvm.exe`，随便换。

## 功能

严格只做**触摸板 + 键盘 + 滚轮**：

| 操作 | 行为 |
| --- | --- |
| 单指滑动 | 移动指针 |
| 单指点按 | 左键单击 |
| 双指点按 | 右键单击 |
| 双指滑动 | 滚轮 / 横向滚轮 |
| 双击后不抬手 | 拖拽（拖到位再抬手） |
| 右下角按钮 | 指针回主屏中央 |
| 键盘区 | 粘滞修饰键 Ctrl / Alt / Shift / Win + 常用功能键 + 音量键 |
| 文字输入 | 字母数字逐键发；中文等输入法选完词整段上屏 |

手机页面「设置」里可以调指针灵敏度、滚轮速度、反向滚轮（存在手机本地）。

## 命令行参数

| 参数 | 说明 |
| --- | --- |
| `-port` | 监听端口，默认 `8123` |
| `-token` | 固定访问密钥（默认每次启动随机生成 8 位） |
| `-sens` | 指针灵敏度倍数，默认 `1.0` |
| `-allow-vpn` | 允许来自 VPN 网段的连接（默认拒绝） |
| `-selftest` | 只做键鼠注入自检然后退出 |
| `-selftest-net <网址>` | 对运行中的服务端做端到端自检 |

## 自己打包 Android App

`keymouse-app/` 是手机 App 的源码（对应的 Windows 服务端源码在 `phone-kvm/`），
标准 uni-app + Vue 3 工程，不含 `package.json`，**不需要本地 Android SDK / Gradle**：

1. HBuilderX 打开 `keymouse-app/` 目录，登录 DCloud 账号。
2. 双击 `manifest.json`，「基础配置」里确认 AppID（`__UNI__FB980B9`）。
3. 菜单 `发行 → 原生App-云打包` → 平台选 **Android**、证书选 **云端证书**、打包方式 **正式版**。

APK 产物落在 `keymouse-app/unpackage/release/apk/` 下。云打包界面里如果勾了「DCloud 快捷广告」，
打出来的包会带开屏广告，那是 uni-ad 的云端开关，与代码无关，可以免费关掉，详见
[`keymouse-app/README.md`](keymouse-app/README.md)。

协议自检（只发 ping / query，**不会注入任何键鼠事件**）：

```powershell
node keymouse-app/tools/ws_check.mjs ws://127.0.0.1:8123/<密钥>/ws
```

## 从源码构建服务端

需要 Go 1.26 或更高，**不需要联网**（没有任何第三方依赖）：

```powershell
powershell -ExecutionPolicy Bypass -File phone-kvm\build.ps1
```

产物写到仓库根目录的 `phonekvm.exe`。跑单测：

```powershell
cd phone-kvm
$env:GOCACHE="$PWD\.gocache"; $env:GOTMPDIR="$PWD\.gotmp"
go test -count=1 ./...
```

`TestCalibAbsolute` / `TestCalibRepeat` 会真的移动并读取光标，需要交互式桌面；
`TestCursorDriftProbe`（要 `CURSOR_DRIFT_PROBE=1`）和 `TestQRDump`（要 `PHONEKVM_QR_DUMP=1`）默认跳过。

`phone-kvm\build.ps1` 必须保持纯 ASCII：Windows PowerShell 5.1 在文件没有 UTF-8 BOM 时按 ANSI 解析 `.ps1`，
非 ASCII 字符会让脚本解析失败。

## 已知限制

- **控制不了以管理员身份运行的窗口**（Windows 的 UIPI 机制）。需要时以管理员身份运行本程序。
- **手机息屏会断连**，解锁后 1 秒内自动重连。网页版拿不到浏览器的 Wake Lock，做不到常亮，把手机「息屏时间」调长更实际；Android App 版可以保持屏幕常亮。
- 走 Raw Input / DirectInput 的全屏游戏收不到注入。
- 绝对坐标归一化有**最多 1 像素**的取整误差（已用 `ceil` 补偿，可复现、不累积）。
- 只支持 Windows，只在局域网内使用。
- 二维码只覆盖「同一个 Wi-Fi」和「电脑热点」两条路；其它网卡（VPN、链路本地地址）只列网址、不画码。
- `-selftest-net` 读的是真实光标，那 3 秒里如果真鼠标在动就会报大偏差，重跑一次即可。

## 许可证

暂未指定。
