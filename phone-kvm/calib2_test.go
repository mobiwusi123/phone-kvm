//go:build windows

package main

import (
	"testing"
	"time"
)

// TestCalibRepeat 判断"落点少 1 像素"是映射规律还是外部干扰（比如真鼠标在抖）：
// 同一个目标连发 5 次，读数若各不相同，就是外部干扰。
func TestCalibRepeat(t *testing.T) {
	inj := newInjector(1.0)
	time.Sleep(300 * time.Millisecond)
	for _, target := range []int32{600, 1000, 1094, 1400, 1800, 2200} {
		readings := make([]int32, 0, 5)
		for rep := 0; rep < 5; rep++ {
			inj.SetPos(target, 700)
			time.Sleep(40 * time.Millisecond)
			gx, _, ok := inj.Cursor(time.Second)
			if !ok {
				t.Fatalf("读不到光标")
			}
			readings = append(readings, gx)
		}
		stable := true
		for _, r := range readings {
			if r != readings[0] {
				stable = false
			}
		}
		t.Logf("目标 x=%-5d 连续 5 次读数 = %v  稳定=%v 误差=%d", target, readings, stable, readings[0]-target)
	}
}
