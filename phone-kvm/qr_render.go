//go:build windows

package main

import (
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode = kernel32.NewProc("SetConsoleMode")
	procSetConsoleCP   = kernel32.NewProc("SetConsoleOutputCP")
)

const enableVirtualTerminalProcessing = 0x0004

// enableUTF8Console 尽量把控制台输出码页切成 UTF-8（65001）。
// Go 往控制台写的是 UTF-8 字节，码页还停在 936/437 时中文和 █▀▄ 都是乱码，
// 二维码也就扫不出来。输出被重定向时这个调用没有意义，失败也不影响流程。
func enableUTF8Console() {
	procSetConsoleCP.Call(65001)
}

// enableANSI 打开控制台的 VT 转义序列支持。Windows Terminal 天生支持，
// 传统 conhost 要显式打开；输出被重定向时返回 false，调用方改用纯 ASCII 渲染。
func enableANSI() bool {
	h, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil || h == 0 {
		return false
	}
	var mode uint32
	if r, _, _ := procGetConsoleMode.Call(uintptr(h), uintptr(unsafe.Pointer(&mode))); r == 0 {
		return false
	}
	if r, _, _ := procSetConsoleMode.Call(uintptr(h), uintptr(mode|enableVirtualTerminalProcessing)); r == 0 {
		return false
	}
	return true
}

// qrText 把矩阵渲染成可直接打印的字符串（自带静默区）。
// ansi=true：深色模块画成黑块、浅色画成白底，在深色终端里对比度也是对的，扫得最稳。
// ansi=false：退回 ## 与空格，一个模块两个字符，保证模块接近正方形。
func qrText(m *qrMatrix, ansi bool) string {
	const quiet = 4
	n := m.size + quiet*2
	at := func(x, y int) bool {
		if x < quiet || y < quiet || x >= quiet+m.size || y >= quiet+m.size {
			return false
		}
		return m.get(x-quiet, y-quiet)
	}
	var sb strings.Builder
	if !ansi {
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				if at(x, y) {
					sb.WriteString("##")
				} else {
					sb.WriteString("  ")
				}
			}
			sb.WriteString("\n")
		}
		return sb.String()
	}
	for y := 0; y < n; y += 2 {
		sb.WriteString("\x1b[30;47m")
		for x := 0; x < n; x++ {
			top := at(x, y)
			bottom := y+1 < n && at(x, y+1)
			switch {
			case top && bottom:
				sb.WriteString("█")
			case top:
				sb.WriteString("▀")
			case bottom:
				sb.WriteString("▄")
			default:
				sb.WriteString(" ")
			}
		}
		sb.WriteString("\x1b[0m\n")
	}
	return sb.String()
}
