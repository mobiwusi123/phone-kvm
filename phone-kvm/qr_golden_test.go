//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func matrixHash(m *qrMatrix) string {
	var sb strings.Builder
	for y := 0; y < m.size; y++ {
		for x := 0; x < m.size; x++ {
			if m.get(x, y) {
				sb.WriteByte('1')
			} else {
				sb.WriteByte('0')
			}
		}
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])[:16]
}

// TestQRGolden pins the finished matrix of a few payloads against hashes taken
// from the reference implementation (Python segno 1.6.6, error level L, byte
// mode, explicitly forced masks).
//
// The mask is forced on purpose so this test does not depend on the mask
// scoring in penalty(): any mask yields a valid symbol, and segno counts
// penalty rule N3 slightly differently than we do (see qr.go).
//
// This is a regression guard, not a correctness proof: it only says "the
// output did not change". The real proof that the placement matches the spec is
// work/check_qr.py, which compares every module of every version 1..10 against
// segno for all 8 masks. Regenerate these hashes from segno - never from our
// own output - if the payload list changes.
func TestQRGolden(t *testing.T) {
	cases := []struct {
		payload string
		ver     int
		mask    int
		hash    string
	}{
		{"HELLO WORLD", 1, 0, "0617bc5fca65c181"},
		{"http://10.253.86.17:8123/", 2, 5, "6308381b79c5aef6"},
		{"http://10.253.86.17:8123/abcdefgh/", 3, 7, "10abdd43e32dcd0b"},
		{"http://[fe80::1234:5678:9abc:def0]:8123/ab/", 5, 3, "cc395e8b2e10423a"},
		{strings.Repeat("f", 250), 10, 4, "6f72205ae193b0cb"},
	}
	for _, c := range cases {
		m, mask, err := qrMakeWithVersion(c.payload, c.ver, c.mask)
		if err != nil {
			t.Fatalf("ver=%d: %v", c.ver, err)
		}
		if mask != c.mask {
			t.Fatalf("ver=%d: mask = %d, want %d", c.ver, mask, c.mask)
		}
		if got := matrixHash(m); got != c.hash {
			t.Errorf("ver=%d %q: hash = %s, want %s", c.ver, c.payload, got, c.hash)
		}
	}
}
