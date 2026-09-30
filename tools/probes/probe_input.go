package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	pGetCursor     = user32.NewProc("GetCursorPos")
	pSetCursor     = user32.NewProc("SetCursorPos")
	pSendInput     = user32.NewProc("SendInput")
	pOpenInputDesk = user32.NewProc("OpenInputDesktop")
	pSetThreadDesk = user32.NewProc("SetThreadDesktop")
	pGetThreadDesk = user32.NewProc("GetThreadDesktop")
	pGetObjInfo    = user32.NewProc("GetUserObjectInformationW")
	pGetCurrentTid = kernel32.NewProc("GetCurrentThreadId")
)

const (
	INPUT_MOUSE      = 0
	INPUT_KEYBOARD   = 1
	MOUSEEVENTF_MOVE = 0x0001
	KEYEVENTF_KEYUP  = 0x0002
	VK_SHIFT         = 0x10
)

type POINT struct{ X, Y int32 }

// winuser.h: LONG dx, LONG dy, DWORD mouseData, DWORD dwFlags, DWORD time, ULONG_PTR dwExtraInfo
type MOUSEINPUT struct {
	Dx, Dy      int32
	MouseData   uint32
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

// winuser.h: WORD wVk, WORD wScan, DWORD dwFlags, DWORD time, ULONG_PTR dwExtraInfo
type KEYBDINPUT struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

// INPUT is always 40B on x64: DWORD type + 4B pad + union (largest member MOUSEINPUT = 32B)
type InputMouse struct {
	Type uint32
	_    uint32
	Mi   MOUSEINPUT
}

type InputKey struct {
	Type uint32
	_    uint32
	Ki   KEYBDINPUT
	_    [8]byte // pad union to 32B so INPUT stays 40B
}

func deskName(h uintptr) string {
	var buf [256]uint16
	var need uint32
	pGetObjInfo.Call(h, 2, uintptr(unsafe.Pointer(&buf[0])), 512, uintptr(unsafe.Pointer(&need)))
	return syscall.UTF16ToString(buf[:])
}

// Attach the calling thread to the input desktop only if it is not already there
// (needed when launched from an isolated desktop / session 0; no-op in a normal terminal).
func ensureInputDesktop() {
	tid, _, _ := pGetCurrentTid.Call()
	cur, _, _ := pGetThreadDesk.Call(tid)
	in, _, err := pOpenInputDesk.Call(0, 0, 0x000F01FF)
	if in == 0 {
		fmt.Println("OpenInputDesktop failed:", err)
		return
	}
	if deskName(cur) == deskName(in) {
		fmt.Printf("desktop: %s (== input desktop, no attach needed)\n", deskName(in))
		return
	}
	if ok, _, err := pSetThreadDesk.Call(in); ok == 0 {
		fmt.Println("SetThreadDesktop failed:", err)
		return
	}
	fmt.Printf("desktop: %s -> attached to input desktop %s\n", deskName(cur), deskName(in))
}

func getPos() (POINT, error) {
	var pt POINT
	r1, _, err := pGetCursor.Call(uintptr(unsafe.Pointer(&pt)))
	if r1 == 0 {
		return pt, err
	}
	return pt, nil
}

func send(in unsafe.Pointer, cb uintptr) (uintptr, error) {
	r1, _, err := pSendInput.Call(1, uintptr(in), cb)
	return r1, err
}

func move(dx, dy int32) (uintptr, error) {
	in := InputMouse{Type: INPUT_MOUSE, Mi: MOUSEINPUT{Dx: dx, Dy: dy, DwFlags: MOUSEEVENTF_MOVE}}
	return send(unsafe.Pointer(&in), unsafe.Sizeof(in))
}

func shift(up bool) (uintptr, error) {
	var f uint32
	if up {
		f = KEYEVENTF_KEYUP
	}
	in := InputKey{Type: INPUT_KEYBOARD, Ki: KEYBDINPUT{WVk: VK_SHIFT, DwFlags: f}}
	return send(unsafe.Pointer(&in), unsafe.Sizeof(in))
}

func main() {
	fmt.Printf("sizeof INPUT(mouse)=%d INPUT(key)=%d\n", unsafe.Sizeof(InputMouse{}), unsafe.Sizeof(InputKey{}))
	ensureInputDesktop()

	before, e0 := getPos()
	fmt.Printf("1) GetCursorPos ok=%v pos=(%d,%d) err=%v\n", e0 == nil, before.X, before.Y, e0)

	n1, e1 := move(30, 0)
	mid, _ := getPos()
	fmt.Printf("2) SendInput MOUSEEVENTF_MOVE +30 -> inserted=%d err=%v | pos=(%d,%d) observed dx=%d\n",
		n1, e1, mid.X, mid.Y, mid.X-before.X)

	n2, e2 := move(-30, 0)
	after, e3 := getPos()
	fmt.Printf("3) SendInput MOUSEEVENTF_MOVE -30 -> inserted=%d err=%v\n", n2, e2)
	fmt.Printf("4) GetCursorPos ok=%v pos=(%d,%d) err=%v | delta after symmetric move=(%d,%d)\n",
		e3 == nil, after.X, after.Y, e3, after.X-before.X, after.Y-before.Y)

	if after != before {
		if r, _, err := pSetCursor.Call(uintptr(before.X), uintptr(before.Y)); r != 0 {
			after, _ = getPos()
			fmt.Printf("   SetCursorPos exact restore -> pos=(%d,%d) err=%v\n", after.X, after.Y, err)
		}
	}
	fmt.Printf("   RESTORED=%v\n", after == before)

	k1, k1e := shift(false)
	k2, k2e := shift(true)
	fmt.Printf("5) SendInput VK_SHIFT keydown/keyup -> inserted=%d/%d err=%v/%v\n", k1, k2, k1e, k2e)
}
