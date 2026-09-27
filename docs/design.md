# 手机当电脑虚拟键鼠 · 设计与验收

自用工具：手机（Android + Chrome）当 Windows 11 电脑的无线触摸板 + 键盘 + 滚轮。电脑端跑一个常驻 exe，手机浏览器打开一个网址就能用 —— 不装 App、不需要管理员权限、不需要任何 SDK。

## 怎么用

1. 双击 `outputs\phonekvm.exe`（改了源码后用 `phone-kvm\build.ps1` 重新构建）。
2. 控制台会直接打印二维码，最多两张：**手机连同一个 Wi-Fi** 时扫第 1 张（`*` 那条），**手机连电脑热点**时扫第 2 张（`+` 那条），形如 `http://10.253.86.17:8123/<8位密钥>/`。二维码下面还列着所有网卡的网址，扫不了码就手动挑手机能连到的那条。密钥每次启动随机生成，可用 `-token` 固定。
3. 首次运行 Windows 防火墙会弹一次「是否允许访问网络」，点**允许**。手机上建议用 Chrome 菜单的「添加到主屏幕」，之后一点就进。
4. 二维码用半块字符（`█▀▄`）画在黑色/白色底上，需要终端窗口够宽（≥ 45 列）、字体含方块字符。程序启动时会把控制台输出码页切成 UTF-8（65001），否则中文和方块字符会变乱码、码也扫不出来。

命令行参数：

| 参数 | 说明 |
| --- | --- |
| `-port` | 监听端口，默认 8123 |
| `-token` | 固定访问密钥（默认每次随机） |
| `-sens` | 指针灵敏度倍数，默认 1.0 |
| `-allow-vpn` | 允许来自 VPN 网段的连接（默认拒绝） |
| `-selftest` | 只做键鼠注入自检然后退出 |
| `-selftest-net <网址>` | 对运行中的服务端做端到端自检 |

## 已锁定的设计决策

- **路线**：电脑常驻服务端 + 局域网传输 + PC 侧 `SendInput` 注入；传输层与注入层解耦，将来加 USB/蓝牙只换传输层。
- **平台**：只 Windows（本机 Windows 11 25H2 / build 26200），手机只 Android，仅局域网，不做公网穿透。
- **技术栈**：PC 端 Go 1.26 单 exe，零第三方依赖（自写的最小 WebSocket + 标准库），不联网也能构建；手机端界面内嵌在 exe 里。
- **指针控制**：用绝对坐标（`MOUSEEVENTF_ABSOLUTE | VIRTUALDESK`）配合自建增益曲线，绕开 Windows 的指针加速。PC 端维护自己的光标模型，空闲超过 250ms 才重新对齐真实光标，这样你和真鼠标同时用时两边不会打架。
- **v1 功能**：触摸板（单指移动 / 单指点=左键 / 双指点=右键 / 双指滑=滚轮与横向滚轮 / 双击后不抬手=拖拽 / 右下角按钮回主屏中央）+ 键盘（粘滞修饰键 Ctrl Alt Shift Win + 常用功能键 + 音量键）+ 文字输入。其余一律进 v2。
- **文字输入**：实时镜像。英文数字符号逐键实时发；中文等手机输入法选完词后整段上屏，组字期间绝不把拼音字母发出去。PC 端用 `KEYEVENTF_UNICODE` 注入，不依赖键盘布局。
- **安全**：随机密钥藏在网址路径里，无密钥一律 403；只接受私有网段来源；Radmin VPN 网段（26.0.0.0/8 以及本机 VPN 网卡网段）默认拒绝。
- **体验底线**：目标延迟 < 30ms；常驻但 Ctrl+C 一键退出；开机自启默认不做。
- **二维码**：自写编码器（字节模式、纠错等级 L、版本 1–10），继续零第三方依赖；矩阵正确性用 Python segno 逐模块比对，不靠肉眼。

## 实测验收结果

用内置自检验证（`phonekvm.exe -selftest` 与 `-selftest-net`）：

| 验收项 | 结果 |
| --- | --- |
| `SendInput` 注入可用、不需要管理员权限 | 通过（普通权限、Medium 完整性） |
| 光标往返一致性（+20/-20 回到原位） | 通过，偏差 ≤ 1 像素且不累积 |
| 增益曲线生效（单次 +30 实际走 78 像素） | 通过 |
| 错误密钥被拒绝 | 通过（HTTP 403） |
| 正确密钥握手并收到 hello | 通过 |
| ping/pong 往返 | 通过 |
| 静态页面 / app.js / style.css | 通过（200，Content-Type 正确） |
| 无密钥访问根路径 | 通过（HTTP 403） |
| 二维码矩阵（对照 segno） | 通过：版本 1–10（含 Wi-Fi 与热点两条真实网址）× 8 种掩码共 88 例，逐模块零差异 |
| 二维码渲染（半块字符与 `##` 两种） | 通过：把渲染结果反解回矩阵，与源码矩阵、静默区逐格一致 |
| 控制台启动输出 | 通过：Wi-Fi 与电脑热点各画一张码（分别对应 `*` / `+`），其余网卡地址照旧列出 |
| 选地址逻辑（`pickMainAddr` / `pickHotspotAddr`） | 通过：只连 Wi-Fi、只开热点、只有 VPN、只有链路本地地址等 8 种组合的单测 |

两点说明：自检里的 HTTP 探测**显式关掉了系统/环境代理**（本机设了 `HTTP_PROXY` 时，连局域网地址会被代理拒掉，实测踩过一次）；`-selftest-net` 读的是真实光标，所以那 3 秒里如果真鼠标在动，偏差会突然很大（见下方限制）。

## 已知限制

- **控制不了以管理员身份运行的窗口**（Windows 的 UIPI 机制）。需要时右键「以管理员身份运行」本程序。
- **手机息屏会断连**，解锁后 1 秒内自动重连。HTTP 页面拿不到浏览器的 Wake Lock，做不到常亮；界面上的开关按 HTTP 限制基本不会成功，真正的办法是把手机「息屏时间」调长。
- 走 Raw Input / DirectInput 的全屏游戏收不到注入。
- 绝对坐标归一化有**最多 1 像素**的取整误差（Windows 的逆变换是向下取整，代码用 `ceil` 补偿后实测误差 ≤ 1，完全可复现、不累积）。
- 只支持 Windows；只在局域网。
- **二维码只覆盖两条路**：同一个 Wi-Fi（普通私有网卡，优先非 VPN）和电脑自己开的热点（`本地连接* N`），各一张。其它网卡（VPN、链路本地地址）只列网址不画码，需要时手动输入。
- **自检偶尔报大偏差**：`-selftest-net` 要读真实光标，这 3 秒里只要真鼠标（或远控/宿主）在动，偏差就会突然很大。本机实测：完全不做注入、只观察光标，3 秒内光标自发移动 20~41 次。重跑一次即可，不影响实际使用。

## 代码地图

```
phone-kvm/ 就是一个 Go 模块（go.mod + 全部源码 + www/ + build.ps1）：
  main.go          入口：参数、启动、打印可用网址与二维码、Ctrl+C 退出
  server.go        HTTP 路由 + 密钥校验 + 来源策略 + 协议解析
  ws.go            极简 WebSocket 服务端（握手/分片/ping-pong），零第三方依赖
  qr.go            自写 QR 编码器（字节模式、等级 L、版本 1–10），零第三方依赖
  qr_render.go     二维码终端渲染（半块字符 / `##` 回退）+ 控制台码页与 VT 开关
  win_input.go     Win32 注入层：SendInput 封装、绝对定位、增益曲线、Unicode 文字注入
  selftest.go      两个自检：-selftest（注入）、-selftest-net（全链路）
  calib_test.go    坐标映射标定（go test -run TestCalibAbsolute -v .）
  calib2_test.go   重复性测试，用来区分确定性误差和外部干扰
  server_test.go   report() 里选地址、认热点网卡、中英混排对齐的单测
  qr_test.go       导出待比对矩阵（仅在 PHONEKVM_QR_DUMP=1 时写文件）
  qr_golden_test.go 固定几个矩阵哈希，防止改动悄悄改变输出
  qr_render_test.go 把渲染结果反解回矩阵核对
  cursor_drift_test.go 只观察不注入，用来看光标是不是在被别的东西动
                       （CURSOR_DRIFT_PROBE=1 go test -run TestCursorDriftProbe -v .）
  www/             手机端界面（index.html / style.css / app.js），go:embed 进 exe
  build.ps1        构建到 outputs\phonekvm.exe（必须保持纯 ASCII）
```

```
docs/design.md       本文档（设计与验收结果）
outputs/phonekvm.exe 交付的单文件程序
tools/check_qr.py    用 Python segno 逐模块校验二维码矩阵
tools/fetch_segno.py 联网下载 segno（离线开发用，产物 tools/pylibs 不进仓库）
tools/probes/        早期可行性验证脚本（SendInput 免管理员可用、输入桌面附着等）
```

## 二维码怎么验证（动过 qr.go 就跑一遍）

```powershell
cd phone-kvm
$env:GOCACHE="$PWD\.gocache"; $env:GOTMPDIR="$PWD\.gotmp"; $env:PHONEKVM_QR_DUMP=1
go test -run TestQRDump .                    # 生成 phone-kvm\qr_dump.txt
$env:PYTHONPATH="$PWD\..\tools\pylibs"       # 缺了就 python ..\tools\fetch_segno.py
python ..\tools\check_qr.py qr_dump.txt      # 期望：FAILED: 0 / 88
```

`tools\pylibs` 里是 segno 1.6.6（纯 Python，离线可用，只是开发期的参照物，不进 exe，也不进仓库；缺了就 `python tools\fetch_segno.py` 重新下）。参照实现有一处偏差要注意：`segno.encoder.write_padding_bits` 写的是 `8 - (length % 8)`，字节已经对齐时会多补一个 `0x00` 字节；规范（ISO/IEC 18004 7.4.10）是「不在码字边界才补」，Nayuki 的实现也是这个行为，所以 `check_qr.py` 先把 segno 校正到规范行为再比对。这个差异不影响扫码（解码器读完数据就停）。

另外，自动选掩码时的评分我们和 segno 口径不同：规则 3（`1:1:3:1:1` 图案）我们按「每一侧」各算一次（Nayuki/多数实现的做法），segno 按「每一处图案」只算一次。8 种掩码都是合法的，扫码结果一样，只是图案不同。

手机到电脑的消息（二进制，一帧里可以拼多条）：

| 类型 | 载荷 |
| --- | --- |
| 0x01 移动 | int16 dx, int16 dy |
| 0x02 鼠标键 | uint8 键位（1左 2右 3中 4后退 5前进）, uint8 按下 |
| 0x03 滚轮 | int16 垂直, int16 水平（单位 1/120 格） |
| 0x04 键盘 | uint16 虚拟键码, uint8 按下 |
| 0x05 文字 | uint16 字节数 + UTF-8 |
| 0x06 ping | uint32 序号，回 0x07 |
| 0x08 回主屏中央 | 无 |

## 调手感

- 手机页面「设置」里可调：指针灵敏度、滚轮速度、反向滚轮（存在手机本地）。
- 曲线本身在 `win_input.go` 顶部的 `gainMax / gainExp / speedRef / maxStep`：慢速 1:1 保证精确，快速放大到 `gainMax` 倍。想更跟手就调大 `gainMax` 或调小 `speedRef`。

## 下一轮（等你真机用过再定优先级）

- **屏幕常亮**：生成自签 CA 证书换成 HTTPS，解锁 Wake Lock。
- **托盘图标 + 开机自启开关**。
- v2 候选：剪贴板双向同步、语音输入、屏幕镜像、蓝牙免装（手机直接伪装成蓝牙键鼠）、USB 低延迟通道。

## 真机验证情况

PC 侧全链路（注入、协议、鉴权、静态页面、延迟）都用内置自检（`-selftest` / `-selftest-net`）验证过；手机端也已在真机上确认可用：单指滑动跟手、双指滚动方向正确、中文选词能整段上屏。
还没专门验证的：多台手机同时连接的表现、长时间息屏后的重连表现，以及 Android Chrome 以外浏览器的兼容性。
