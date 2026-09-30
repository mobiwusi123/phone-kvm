//go:build windows

package main

import (
	"math"
	"testing"
	"time"
)

// TestCalibAbsolute 用来实测"目标像素 -> 归一化值 -> 实际像素"这条链路的取整行为。
// 它会横扫屏幕上的光标，只在开发时手动跑：go test -run TestCalibAbsolute -v .
func TestCalibAbsolute(t *testing.T) {
	vx, vy, vw, vh := virtualScreen()
	t.Logf("虚拟桌面 x=%d y=%d w=%d h=%d", vx, vy, vw, vh)
	inj := newInjector(1.0)
	time.Sleep(300 * time.Millisecond)

	xs := []int32{0, 200, 600, 1000, 1094, 1400, 1800, 2200, 2559, 2560, 3000, 3400, 3900, 4095}
	worstX := int32(0)
	for _, target := range xs {
		ax := int32(math.Round(float64(target-vx) * 65535 / float64(vw-1)))
		inj.SetPos(target, 700)
		time.Sleep(50 * time.Millisecond)
		gx, gy, ok := inj.Cursor(time.Second)
		if !ok {
			t.Fatalf("读不到光标")
		}
		if d := abs32(gx - target); d > worstX {
			worstX = d
		}
		t.Logf("x 目标=%-5d ax=%-6d 实际=%-5d 误差=%d  |  y 实际=%d", target, ax, gx, gx-target, gy)
	}
	t.Logf("x 方向最大误差 = %d", worstX)

	ys := []int32{0, 100, 400, 700, 938}
	worstY := int32(0)
	for _, target := range ys {
		ay := int32(math.Round(float64(target-vy) * 65535 / float64(vh-1)))
		inj.SetPos(1000, target)
		time.Sleep(50 * time.Millisecond)
		gx, gy, _ := inj.Cursor(time.Second)
		if d := abs32(gy - target); d > worstY {
			worstY = d
		}
		t.Logf("y 目标=%-5d ay=%-6d 实际=%-5d 误差=%d  |  x 实际=%d", target, ay, gy, gy-target, gx)
	}
	t.Logf("y 方向最大误差 = %d", worstY)
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}
