//go:build windows

// Win32 键鼠注入层。
//
// 三条硬性约束（实测得出，改代码前先读）：
//  1. x64 下 INPUT 结构体必须是 40 字节（4 字节 type + 4 字节对齐 + 32 字节 union），
//     传 32 字节会直接得到 ERROR_INVALID_PARAMETER(87)。见 init() 的断言。
//  2. SendInput 的成功判据是"返回值 == 事件数"，不是 GetLastError（它可能是陈旧值）。
//  3. 桌面附着是按线程生效的，而 goroutine 会在 OS 线程间迁移，所以所有注入都串行
//     跑在 run() 那个 LockOSThread 的 goroutine 里。
package main

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")

	procSendInput        = user32.NewProc("SendInput")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procOpenInputDesktop = user32.NewProc("OpenInputDesktop")
	procSetThreadDesktop = user32.NewProc("SetThreadDesktop")
)

const (
	evMouse    = 0
	evKeyboard = 1

	mouseMove        = 0x0001
	mouseLeftDown    = 0x0002
	mouseLeftUp      = 0x0004
	mouseRightDown   = 0x0008
	mouseRightUp     = 0x0010
	mouseMiddleDown  = 0x0020
	mouseMiddleUp    = 0x0040
	mouseXDown       = 0x0080
	mouseXUp         = 0x0100
	mouseWheelFlag   = 0x0800
	mouseHWheelFlag  = 0x1000
	mouseAbsolute    = 0x8000
	mouseVirtualDesk = 0x4000

	keyEventExtended = 0x0001
	keyEventUp       = 0x0002
	keyEventUnicode  = 0x0004

	smCxScreen        = 0
	smCyScreen        = 1
	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCxVirtualScreen = 78
	smCyVirtualScreen = 79

	wheelDelta = 120

	desktopGenericAll = 0x000F01FF
)

// 指针增益曲线：慢速保持 1:1 保证精确落点，快速放大到 gainMax 倍以便横跨 2560 宽的主屏。
// Windows 自带的"提高指针精确度"会被我们绕过（我们发绝对坐标），所以这条曲线是唯一手感来源。
const (
	gainMax    = 2.6
	gainExp    = 1.3
	speedRef   = 18.0
	maxStep    = 400.0
	resyncIdle = 250 * time.Millisecond
)

type point struct{ X, Y int32 }

// winuser.h 的 MOUSEINPUT：LONG dx, LONG dy, DWORD mouseData, DWORD dwFlags, DWORD time, ULONG_PTR dwExtraInfo
type mouseInput struct {
	dx, dy      int32
	mouseData   uint32
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

// winuser.h 的 KEYBDINPUT：WORD wVk, WORD wScan, DWORD dwFlags, DWORD time, ULONG_PTR dwExtraInfo
type keybdInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

type mouseEvent struct {
	typ uint32
	_   uint32
	mi  mouseInput
}

type keyEvent struct {
	typ uint32
	_   uint32
	ki  keybdInput
	_   [8]byte
}

func init() {
	if unsafe.Sizeof(mouseEvent{}) != 40 || unsafe.Sizeof(keyEvent{}) != 40 {
		panic("INPUT 结构体必须是 40 字节（x64），否则 SendInput 返回 ERROR_INVALID_PARAMETER(87)")
	}
}

// 需要 KEYEVENTF_EXTENDEDKEY 的虚拟键，漏掉的话方向键会被当成小键盘、音量键无效。
var extendedKeys = map[uint16]bool{
	0x21: true, 0x22: true, 0x23: true, 0x24: true, // PgUp PgDn End Home
	0x25: true, 0x26: true, 0x27: true, 0x28: true, // 方向键
	0x2D: true, 0x2E: true, // Insert Delete
	0x5B: true, 0x5C: true, // Win 左/右
	0x6F: true, 0x90: true, // 小键盘除号 NumLock
	0xA3: true, 0xA5: true, // 右 Ctrl / 右 Alt
	0xAD: true, 0xAE: true, 0xAF: true, // 静音 音量- 音量+
	0xB0: true, 0xB1: true, 0xB2: true, 0xB3: true, // 下一曲 上一曲 停止 播放
}

func sendEvents(ptr unsafe.Pointer, n int) error {
	if n == 0 {
		return nil
	}
	r, _, err := procSendInput.Call(uintptr(n), uintptr(ptr), unsafe.Sizeof(mouseEvent{}))
	if int(r) != n {
		return fmt.Errorf("SendInput 只插入了 %d/%d 个事件: %v", r, n, err)
	}
	return nil
}

func sysMetric(i int) int32 {
	r, _, _ := procGetSystemMetrics.Call(uintptr(i))
	return int32(r)
}

// 虚拟桌面 = 所有显示器拼起来的总区域（含负坐标），绝对坐标必须按它归一化。
func virtualScreen() (x, y, w, h int32) {
	return sysMetric(smXVirtualScreen), sysMetric(smYVirtualScreen),
		sysMetric(smCxVirtualScreen), sysMetric(smCyVirtualScreen)
}

func getCursorPos() (point, bool) {
	var p point
	r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return p, r != 0
}

// ensureInputDesktop 在正常终端里什么都不做：进程本来就在输入桌面（WinSta0\Default），
// 能直接读到光标。只有在被塞进隔离桌面（沙箱）时才需要显式附着。
func ensureInputDesktop() string {
	if _, ok := getCursorPos(); ok {
		return ""
	}
	h, _, err := procOpenInputDesktop.Call(0, 0, desktopGenericAll)
	if h == 0 {
		return fmt.Sprintf("警告: 打不开输入桌面, 键鼠注入会失败 (%v)", err)
	}
	if r, _, err := procSetThreadDesktop.Call(h); r == 0 {
		return fmt.Sprintf("警告: 附着输入桌面失败, 键鼠注入会失败 (%v)", err)
	}
	return "已附着到输入桌面（隔离环境适配）"
}

// ---------------------------------------------------------------- 命令队列

type cursorPos struct {
	X, Y int32
	OK   bool
}

type cmdKind int

const (
	cmdMove cmdKind = iota
	cmdButton
	cmdWheel
	cmdKey
	cmdText
	cmdCenter
	cmdSetPos
	cmdQuery
)

type command struct {
	kind           cmdKind
	dx, dy         float64
	btn            byte
	down           bool
	wheelV, wheelH int32
	vk             uint16
	text           string
	absX, absY     int32
	resp           chan cursorPos
}

type injector struct {
	ch      chan command
	sens    float64
	dropped atomic.Uint64
}

func newInjector(sens float64) *injector {
	in := &injector{ch: make(chan command, 1024), sens: sens}
	go in.run()
	return in
}

func (in *injector) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if msg := ensureInputDesktop(); msg != "" {
		logf("%s", msg)
	}
	st := &injectState{sens: in.sens}
	for c := range in.ch {
		switch c.kind {
		case cmdMove:
			st.move(c.dx, c.dy)
		case cmdButton:
			st.button(c.btn, c.down)
		case cmdWheel:
			st.wheel(c.wheelV, c.wheelH)
		case cmdKey:
			st.key(c.vk, c.down)
		case cmdText:
			st.text(c.text)
		case cmdCenter:
			st.center()
		case cmdSetPos:
			st.sendAbs(c.absX, c.absY)
		case cmdQuery:
			p, ok := getCursorPos()
			c.resp <- cursorPos{X: p.X, Y: p.Y, OK: ok}
		}
	}
}

// send 用于不可丢弃的事件（按键、点击、文字）。
func (in *injector) send(c command) {
	select {
	case in.ch <- c:
	case <-time.After(3 * time.Second):
		logf("注入队列拥塞，丢弃一个事件")
	}
}

func (in *injector) Move(dx, dy float64) {
	if dx == 0 && dy == 0 {
		return
	}
	select {
	case in.ch <- command{kind: cmdMove, dx: dx, dy: dy}:
	default:
		in.dropped.Add(1) // 指针移动可以丢，绝不能让它阻塞住收包
	}
}

func (in *injector) Button(btn byte, down bool) {
	in.send(command{kind: cmdButton, btn: btn, down: down})
}
func (in *injector) Wheel(v, h int32)         { in.send(command{kind: cmdWheel, wheelV: v, wheelH: h}) }
func (in *injector) Key(vk uint16, down bool) { in.send(command{kind: cmdKey, vk: vk, down: down}) }
func (in *injector) Text(s string)            { in.send(command{kind: cmdText, text: s}) }
func (in *injector) Center()                  { in.send(command{kind: cmdCenter}) }
func (in *injector) SetPos(x, y int32)        { in.send(command{kind: cmdSetPos, absX: x, absY: y}) }

func (in *injector) Cursor(timeout time.Duration) (int32, int32, bool) {
	resp := make(chan cursorPos, 1)
	select {
	case in.ch <- command{kind: cmdQuery, resp: resp}:
	case <-time.After(timeout):
		return 0, 0, false
	}
	select {
	case p := <-resp:
		return p.X, p.Y, p.OK
	case <-time.After(timeout):
		return 0, 0, false
	}
}

// ---------------------------------------------------------------- 注入实现

type injectState struct {
	sens      float64
	havePos   bool
	x, y      int32
	lastMove  time.Time
	wheelAcc  int32
	hwheelAcc int32
}

// resync 处理"用户同时还在用真鼠标"的情况：只要一段时间没收到触摸板事件，
// 就把我们内部的光标模型重新对齐到真实光标，避免两边打架。
func (s *injectState) resync() {
	if s.havePos && time.Since(s.lastMove) < resyncIdle {
		return
	}
	if p, ok := getCursorPos(); ok {
		s.x, s.y, s.havePos = p.X, p.Y, true
	}
}

func (s *injectState) move(dx, dy float64) {
	s.resync()
	if !s.havePos {
		return
	}
	speed := math.Hypot(dx, dy)
	gain := 1 + (gainMax-1)*math.Pow(math.Min(speed/speedRef, 1), gainExp)
	sdx, sdy := dx*gain*s.sens, dy*gain*s.sens
	if step := math.Hypot(sdx, sdy); step > maxStep {
		k := maxStep / step
		sdx, sdy = sdx*k, sdy*k
	}
	vx, vy, vw, vh := virtualScreen()
	nx := clampFloat(float64(s.x)+sdx, float64(vx), float64(vx+vw-1))
	ny := clampFloat(float64(s.y)+sdy, float64(vy), float64(vy+vh-1))
	s.sendAbs(int32(math.Round(nx)), int32(math.Round(ny)))
	s.lastMove = time.Now()
}

// sendAbs 用绝对坐标发送。这是绕过 Windows 指针加速的关键：
// MOUSEEVENTF_MOVE（相对）会被系统加速，实测请求 +30 会走 +37~+59，手感不可控。
func (s *injectState) sendAbs(x, y int32) {
	vx, vy, vw, vh := virtualScreen()
	if vw < 2 {
		vw = 2
	}
	if vh < 2 {
		vh = 2
	}
	// 这里必须用 ceil 而不是 round：实测 Windows 的逆变换是向下取整，
	// round 会把归一化值压到映射区间的下边缘，导致约一半的目标点少 1 像素。
	// 取 ceil 后 [x, x+1) 内的值都落在同一格，像素级精确还原。
	nx := clampInt(int32(math.Ceil(float64(x-vx)*65535/float64(vw-1))), 0, 65535)
	ny := clampInt(int32(math.Ceil(float64(y-vy)*65535/float64(vh-1))), 0, 65535)
	ev := mouseEvent{typ: evMouse, mi: mouseInput{dx: nx, dy: ny,
		dwFlags: mouseMove | mouseAbsolute | mouseVirtualDesk}}
	if err := sendEvents(unsafe.Pointer(&ev), 1); err != nil {
		logf("移动光标失败: %v", err)
		return
	}
	s.x, s.y, s.havePos = x, y, true
}

func (s *injectState) button(btn byte, down bool) {
	var flags, data uint32
	switch btn {
	case 1:
		if down {
			flags = mouseLeftDown
		} else {
			flags = mouseLeftUp
		}
	case 2:
		if down {
			flags = mouseRightDown
		} else {
			flags = mouseRightUp
		}
	case 3:
		if down {
			flags = mouseMiddleDown
		} else {
			flags = mouseMiddleUp
		}
	case 4:
		data = 1
		if down {
			flags = mouseXDown
		} else {
			flags = mouseXUp
		}
	case 5:
		data = 2
		if down {
			flags = mouseXDown
		} else {
			flags = mouseXUp
		}
	default:
		return
	}
	ev := mouseEvent{typ: evMouse, mi: mouseInput{mouseData: data, dwFlags: flags}}
	if err := sendEvents(unsafe.Pointer(&ev), 1); err != nil {
		logf("鼠标按键失败: %v", err)
	}
}

// wheel 的入参单位是 1/120 格（WHEEL_DELTA），累积到整格才发，避免把小数丢给应用。
func (s *injectState) wheel(v, h int32) {
	var evs []mouseEvent
	neg := int32(-wheelDelta)
	s.wheelAcc += v
	s.hwheelAcc += h
	for s.wheelAcc >= wheelDelta {
		evs = append(evs, mouseEvent{typ: evMouse, mi: mouseInput{mouseData: wheelDelta, dwFlags: mouseWheelFlag}})
		s.wheelAcc -= wheelDelta
	}
	for s.wheelAcc <= -wheelDelta {
		evs = append(evs, mouseEvent{typ: evMouse, mi: mouseInput{mouseData: uint32(neg), dwFlags: mouseWheelFlag}})
		s.wheelAcc += wheelDelta
	}
	for s.hwheelAcc >= wheelDelta {
		evs = append(evs, mouseEvent{typ: evMouse, mi: mouseInput{mouseData: wheelDelta, dwFlags: mouseHWheelFlag}})
		s.hwheelAcc -= wheelDelta
	}
	for s.hwheelAcc <= -wheelDelta {
		evs = append(evs, mouseEvent{typ: evMouse, mi: mouseInput{mouseData: uint32(neg), dwFlags: mouseHWheelFlag}})
		s.hwheelAcc += wheelDelta
	}
	if len(evs) == 0 {
		return
	}
	if err := sendEvents(unsafe.Pointer(&evs[0]), len(evs)); err != nil {
		logf("滚轮失败: %v", err)
	}
}

func (s *injectState) key(vk uint16, down bool) {
	var flags uint32
	if extendedKeys[vk] {
		flags = keyEventExtended
	}
	if !down {
		flags |= keyEventUp
	}
	ev := keyEvent{typ: evKeyboard, ki: keybdInput{wVk: vk, dwFlags: flags}}
	if err := sendEvents(unsafe.Pointer(&ev), 1); err != nil {
		logf("按键失败: %v", err)
	}
}

// text 走 KEYEVENTF_UNICODE：绕开键盘布局直接塞字符，这是中文能上屏的原因。
func (s *injectState) text(str string) {
	if str == "" {
		return
	}
	str = strings.ReplaceAll(str, "\r\n", "\n")
	str = strings.ReplaceAll(str, "\n", "\r") // 多数应用只认 \r 作为回车
	units := utf16.Encode([]rune(str))
	evs := make([]keyEvent, 0, len(units)*2)
	for _, u := range units {
		evs = append(evs, keyEvent{typ: evKeyboard, ki: keybdInput{wScan: u, dwFlags: keyEventUnicode}})
		evs = append(evs, keyEvent{typ: evKeyboard, ki: keybdInput{wScan: u, dwFlags: keyEventUnicode | keyEventUp}})
	}
	for i := 0; i < len(evs); i += 128 {
		end := min(i+128, len(evs))
		if err := sendEvents(unsafe.Pointer(&evs[i]), end-i); err != nil {
			logf("文字注入失败: %v", err)
			return
		}
	}
}

func (s *injectState) center() {
	s.sendAbs(sysMetric(smCxScreen)/2, sysMetric(smCyScreen)/2)
}

func clampInt(v, lo, hi int32) int32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
