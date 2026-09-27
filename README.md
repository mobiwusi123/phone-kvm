# PhoneKVM · 把手机当成电脑的虚拟键鼠

手机（Android + Chrome）打开一个网址，就能当 Windows 电脑的**无线触摸板 + 键盘 + 滚轮**。
电脑端只跑一个常驻的单文件 exe，手机端不装任何 App。

- **不装 App**：手机端就是普通网页，扫码即用，可以「添加到主屏幕」当图标点。
- **不需要管理员权限**：普通用户权限就能注入键鼠（走 `SendInput`）。
- **零第三方依赖**：WebSocket 服务端和 QR 二维码编码器都是自己写的，只用 Go 标准库；离线也能构建。
- **单文件交付**：`outputs/phonekvm.exe` 约 6.5 MB，手机端界面已经 `go:embed` 进去，拷过去就能跑。
- **仅局域网**：不联网、不做公网穿透。随机密钥藏在网址路径里，没密钥一律 403；只接受私有网段来源。

## 快速开始

1. 双击 `outputs/phonekvm.exe`。
2. 控制台会直接打印二维码，最多两张：
   - `1/2 *` —— 手机连**同一个 Wi-Fi** 时扫这张；
   - `2/2 +` —— 手机连**电脑自己开的热点**时扫这张。

   二维码下面还列着所有网卡的可用网址，扫不出来就手动挑一条手机能连到的。
3. 首次运行 Windows 防火墙会弹一次「是否允许访问网络」，点**允许**。
4. 手机浏览器打开就能用。建议用 Chrome 菜单的「添加到主屏幕」。

> 二维码是用半块字符（`█▀▄`）画的，需要终端窗口至少 45 列宽、字体包含方块字符。
> 程序启动时会把控制台输出码页切成 UTF-8（65001），否则中文和方块字符会乱码、码也扫不出来。

## 功能（v1）

严格只做**触摸板 + 键盘 + 滚轮**，其余全部留到 v2。

| 操作 | 行为 |
| --- | --- |
| 单指滑动 | 移动指针 |
| 单指点按 | 左键单击 |
| 双指点按 | 右键单击 |
| 双指滑动 | 滚轮 / 横向滚轮 |
| 双击后不抬手 | 拖拽（拖到位再抬手） |
| 右下角按钮 | 指针回主屏中央 |
| 键盘区 | 粘滞修饰键 Ctrl / Alt / Shift / Win + 常用功能键 + 音量键 |
| 文字输入 | 实时镜像：字母数字逐键发；中文等输入法选完词整段上屏 |

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

## 它是怎么工作的

```
手机 Chrome  ── HTTP / WebSocket（局域网） ──>  phonekvm.exe  ── SendInput ──>  Windows
```

- **指针**：用绝对坐标（`MOUSEEVENTF_ABSOLUTE | VIRTUALDESK`）配合自建增益曲线，绕开 Windows 的指针加速。PC 端维护自己的光标模型，空闲超过 250ms 才重新对齐真实光标，所以和真鼠标同时用不会打架。
- **文字**：用 `KEYEVENTF_UNICODE` 注入，不依赖键盘布局，中文输入法在手机上组完字才整段发过来。
- **分层**：传输层和注入层是分开的，将来想加 USB / 蓝牙通道只换传输层。
- **安全**：随机密钥在网址路径里；只接受私有网段来源；Radmin VPN（`26.0.0.0/8` 以及本机 VPN 网卡网段）默认拒绝。

## 目录结构

```
phone-kvm/           Go 模块（go.mod + 全部源码 + www/ + build.ps1）
  main.go            入口：参数、启动、打印可用网址与二维码、Ctrl+C 退出
  server.go          HTTP 路由 + 密钥校验 + 来源策略 + 协议解析
  ws.go              极简 WebSocket 服务端（自写，零第三方依赖）
  qr.go              自写 QR 编码器（字节模式、纠错等级 L、版本 1-10）
  qr_render.go       二维码终端渲染（半块字符 / ## 回退）+ 控制台码页与 VT 开关
  win_input.go       Win32 注入层：SendInput、绝对定位、增益曲线、Unicode 文字
  selftest.go        -selftest（注入自检）与 -selftest-net（全链路自检）
  www/               手机端界面（index.html / style.css / app.js），go:embed 进 exe
  build.ps1          构建到 outputs/phonekvm.exe（必须保持纯 ASCII）
  *_test.go          单测：选地址、二维码矩阵与渲染、坐标标定、光标漂移探针
docs/design.md       设计与实测验收结果（含二维码逐模块比对的方法）
outputs/phonekvm.exe 交付的单文件程序
tools/check_qr.py    用 Python segno 逐模块校验二维码矩阵
tools/fetch_segno.py 下载 segno（离线开发用；产物 tools/pylibs 不进仓库）
tools/probes/        早期可行性验证脚本（SendInput 免管理员可用、输入桌面附着等）
```

## 从源码构建

需要 Go 1.26 或更高，**不需要联网**（没有任何第三方依赖）：

```powershell
powershell -ExecutionPolicy Bypass -File phone-kvm\build.ps1
```

产物写到 `outputs\phonekvm.exe`。跑单测：

```powershell
cd phone-kvm
$env:GOCACHE="$PWD\.gocache"; $env:GOTMPDIR="$PWD\.gotmp"
go test -count=1 ./...
```

> `go test ./...` 里的 `TestCalibAbsolute` / `TestCalibRepeat` 会真的移动并读取光标，需要交互式桌面；
> `TestCursorDriftProbe`（要 `CURSOR_DRIFT_PROBE=1`）和 `TestQRDump`（要 `PHONEKVM_QR_DUMP=1`）默认跳过。
> CI 里只跑不碰光标的那部分。

> `phone-kvm\build.ps1` 必须保持纯 ASCII：Windows PowerShell 5.1 在文件没有 UTF-8 BOM 时按 ANSI 解析 `.ps1`，非 ASCII 字符会让脚本解析失败。

## 已知限制

- **控制不了以管理员身份运行的窗口**（Windows 的 UIPI 机制）。需要时以管理员身份运行本程序。
- **手机息屏会断连**，解锁后 1 秒内自动重连。HTTP 页面拿不到浏览器的 Wake Lock，做不到常亮，把手机「息屏时间」调长更实际。
- 走 Raw Input / DirectInput 的全屏游戏收不到注入。
- 绝对坐标归一化有**最多 1 像素**的取整误差（已用 `ceil` 补偿，可复现、不累积）。
- 只支持 Windows，只在局域网内使用。
- 二维码只覆盖「同一个 Wi-Fi」和「电脑热点」两条路；其它网卡（VPN、链路本地地址）只列网址、不画码。
- `-selftest-net` 读的是真实光标，那 3 秒里如果真鼠标在动就会报大偏差，重跑一次即可。

## 开发期：怎么验证二维码

动过 `qr.go` 就跑一遍。用 Python 的 segno 逐模块比对，不靠肉眼：

```powershell
cd phone-kvm
$env:GOCACHE="$PWD\.gocache"; $env:GOTMPDIR="$PWD\.gotmp"; $env:PHONEKVM_QR_DUMP=1
go test -run TestQRDump .                            # 生成 qr_dump.txt

$env:PYTHONPATH="$PWD\..\tools\pylibs"               # 缺了就 python ..\tools\fetch_segno.py
python ..\tools\check_qr.py qr_dump.txt              # 期望：FAILED: 0 / 88
```

细节（含参照实现与本实现的已知口径差异）见 `docs/design.md`。

## 许可证

暂未指定。
