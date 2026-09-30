package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	user32        = syscall.NewLazyDLL("user32.dll")
	procGetCursor = user32.NewProc("GetCursorPos")
	procSendInput = user32.NewProc("SendInput")
	procOpenInput = user32.NewProc("OpenInputDesktop")
	procSetThread = user32.NewProc("SetThreadDesktop")
	procGetThread = syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentThreadId")
	procGetDesk   = user32.NewProc("GetThreadDesktop")
)

func attach() {
	cur, _, _ := procGetDesk.Call(uintptr(mustInt(procGetThread.Call())))
	_ = cur
	h, _, err := procOpenInput.Call(0, 0, 0x000F01FF)
	if h == 0 {
		panic("OpenInputDesktop: " + err.Error())
	}
	ok, _, err := procSetThread.Call(h)
	if ok == 0 {
		panic("SetThreadDesktop: " + err.Error())
	}
	fmt.Println("[wrapper] attached to input desktop (Default)")
}

func mustInt(a uintptr, b uintptr, c error) uint64 { return uint64(a) }

const (
	INPUT_MOUSE      = 0
	INPUT_KEYBOARD   = 1
	MOUSEEVENTF_MOVE = 0x0001
	KEYEVENTF_KEYUP  = 0x0002
	VK_SHIFT         = 0x10
)

type POINT struct{ X, Y int32 }

type MOUSEINPUT struct {
	Dx, Dy      int32
	MouseData   uint32
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type KEYBDINPUT struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

// INPUT(x64)=40B: DWORD type + 4B padding + union(???? MOUSEINPUT=32B)
type INPUT struct {
	Type  uint32
	_     uint32
	Dx    int32
	Dy    int32
	MData uint32
	Flags uint32
	Time  uint32
	_     [4]byte
	Extra uintptr
	_     [8]byte
}

func getPos() (POINT, error) {
	var pt POINT
	r1, _, err := procGetCursor.Call(uintptr(unsafe.Pointer(&pt)))
	if r1 == 0 {
		return pt, err
	}
	return pt, nil
}

func sendInput(in *INPUT) (uintptr, error) {
	r1, _, err := procSendInput.Call(1, uintptr(unsafe.Pointer(in)), unsafe.Sizeof(*in))
	if err != nil {
		return r1, err
	}
	return r1, nil
}

func main() {
	attach()
	fmt.Printf("sizeof(INPUT)=%d\n", unsafe.Sizeof(INPUT{}))
	before, e0 := getPos()
	fmt.Printf("1) GetCursorPos ok=%v pos=(%d,%d) err=%v\n", e0 == nil, before.X, before.Y, e0)
	n1, e1 := sendInput(&INPUT{Type: INPUT_MOUSE, Dx: 30, Dy: 0, Flags: MOUSEEVENTF_MOVE})
	fmt.Printf("2) SendInput +30 -> inserted=%d err=%v\n", n1, e1)
	mid, _ := getPos()
	fmt.Printf("   pos=(%d,%d) (expect %d)\n", mid.X, mid.Y, before.X+30)
	n2, e2 := sendInput(&INPUT{Type: INPUT_MOUSE, Dx: -30, Dy: 0, Flags: MOUSEEVENTF_MOVE})
	fmt.Printf("3) SendInput -30 -> inserted=%d err=%v\n", n2, e2)
	after, e3 := getPos()
	fmt.Printf("4) GetCursorPos pos=(%d,%d) err=%v RESTORED=%v delta=(%d,%d)\n",
		after.X, after.Y, e3, after == before, after.X-before.X, after.Y-before.Y)
	k1, k1e := sendInput(&INPUT{Type: INPUT_KEYBOARD, Flags: 0, Dx: 0, MData: 0, Extra: 0, Time: 0,
		Dy: int32(VK_SHIFT)}) // wVk ? 16 ? = 0x10 (Shift), ? KEYEVENTF_KEYUP => keydown
	k2, k2e := sendInput(&INPUT{Type: INPUT_KEYBOARD, Flags: KEYEVENTF_KEYUP,
		Dy: int32(VK_SHIFT)}) // keyup
	fmt.Printf("5) VK_SHIFT down/up -> inserted=%d/%d err=%v/%v\n", k1, k2, k1e, k2e)
}
