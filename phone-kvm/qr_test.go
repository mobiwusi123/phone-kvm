//go:build windows

package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestQRDump exports sample payloads (every version 1..10, all 8 masks each)
// to qr_dump.txt in the format
//
//	payload|version|errorlevel|mask
//	<size lines of 0/1>
//
// work/check_qr.py compares every module against the reference implementation
// (Python segno). That comparison is the only real evidence that this encoder
// is correct; do not skip it when touching qr.go.
//
// Writing the file is opt-in so a plain `go test` stays side-effect free.
func TestQRDump(t *testing.T) {
	if os.Getenv("PHONEKVM_QR_DUMP") == "" {
		t.Skip("set PHONEKVM_QR_DUMP=1 to export qr_dump.txt for work/check_qr.py")
	}
	cases := []struct {
		payload string
		ver     int
	}{
		{"HELLO WORLD", 1},
		{"http://10.253.86.17:8123/", 2},
		{"http://10.253.86.17:8123/abcdefgh/", 3},
		{"http://192.168.137.1:8123/abcdefgh/", 3}, // 电脑热点那条，report() 会为它单独画一张码
		{strings.Repeat("a", 60), 4},
		{"http://[fe80::1234:5678:9abc:def0]:8123/ab/", 5},
		{strings.Repeat("b", 120), 6},
		{strings.Repeat("c", 140), 7},
		{strings.Repeat("d", 170), 8},
		{strings.Repeat("e", 200), 9},
		{strings.Repeat("f", 250), 10},
	}
	var sb strings.Builder
	for _, c := range cases {
		for mask := 0; mask < 8; mask++ {
			m, usedMask, err := qrMakeWithVersion(c.payload, c.ver, mask)
			if err != nil {
				t.Fatalf("payload=%q ver=%d mask=%d: %v", c.payload, c.ver, mask, err)
			}
			fmt.Fprintf(&sb, "%s|%d|l|%d\n", c.payload, c.ver, usedMask)
			for y := 0; y < m.size; y++ {
				for x := 0; x < m.size; x++ {
					if m.get(x, y) {
						sb.WriteByte('1')
					} else {
						sb.WriteByte('0')
					}
				}
				sb.WriteByte('\n')
			}
		}
	}
	if err := os.WriteFile("qr_dump.txt", []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Log("exported qr_dump.txt")
}

// TestQRAutoMask checks that automatic mask selection and both render paths
// (ANSI and plain) run without errors.
func TestQRAutoMask(t *testing.T) {
	m, mask, err := qrMake("http://10.253.86.17:8123/abcdefgh/", -1)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("auto mask = %d, size = %d, penalty = %d", mask, m.size, m.penalty())
	if len(qrText(m, true)) == 0 || len(qrText(m, false)) == 0 {
		t.Fatal("empty render")
	}
}
