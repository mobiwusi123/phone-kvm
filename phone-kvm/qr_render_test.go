//go:build windows

package main

import (
	"strings"
	"testing"
)

// TestQRRenderPlainRoundTrip parses the plain (##) rendering back into a matrix
// and compares it with the source matrix, quiet zone included.
func TestQRRenderPlainRoundTrip(t *testing.T) {
	src, _, err := qrMake("http://10.253.86.17:8123/abcdefgh/", -1)
	if err != nil {
		t.Fatal(err)
	}
	const quiet = 4
	lines := strings.Split(strings.TrimSuffix(qrText(src, false), "\n"), "\n")
	want := src.size + quiet*2
	if len(lines) != want {
		t.Fatalf("rows = %d, want %d", len(lines), want)
	}
	seenDark, seenLight := false, false
	for y, line := range lines {
		if len(line) != want*2 {
			t.Fatalf("row %d: width = %d chars, want %d", y, len(line), want*2)
		}
		for x := 0; x < want; x++ {
			cell := line[x*2 : x*2+2]
			var dark bool
			switch cell {
			case "##":
				dark = true
			case "  ":
				dark = false
			default:
				t.Fatalf("row %d col %d: unexpected cell %q", y, x, cell)
			}
			if dark {
				seenDark = true
			} else {
				seenLight = true
			}
			inside := x >= quiet && y >= quiet && x < want-quiet && y < want-quiet
			if inside {
				if got := src.get(x-quiet, y-quiet); got != dark {
					t.Fatalf("module (%d,%d): rendered %v, want %v", x-quiet, y-quiet, dark, got)
				}
			} else if dark {
				t.Fatalf("quiet zone (%d,%d) rendered dark", x, y)
			}
		}
	}
	if !seenDark || !seenLight {
		t.Fatal("rendering is monochrome")
	}
}

// TestQRRenderANSIRoundTrip does the same for the ANSI half-block rendering.
func TestQRRenderANSIRoundTrip(t *testing.T) {
	src, _, err := qrMake("http://10.253.86.17:8123/abcdefgh/", -1)
	if err != nil {
		t.Fatal(err)
	}
	const quiet = 4
	want := src.size + quiet*2
	lines := strings.Split(strings.TrimSuffix(qrText(src, true), "\n"), "\n")
	if len(lines) != (want+1)/2 {
		t.Fatalf("rows = %d, want %d", len(lines), (want+1)/2)
	}
	for row, line := range lines {
		if !strings.HasPrefix(line, "\x1b[30;47m") || !strings.HasSuffix(line, "\x1b[0m") {
			t.Fatalf("row %d: missing color escape: %q", row, line)
		}
		body := strings.TrimSuffix(strings.TrimPrefix(line, "\x1b[30;47m"), "\x1b[0m")
		runes := []rune(body)
		if len(runes) != want {
			t.Fatalf("row %d: width = %d runes, want %d", row, len(runes), want)
		}
		for x, r := range runes {
			top, bottom := false, false
			switch r {
			case '█':
				top, bottom = true, true
			case '▀':
				top = true
			case '▄':
				bottom = true
			case ' ':
			default:
				t.Fatalf("row %d col %d: unexpected rune %q", row, x, r)
			}
			y := row * 2
			if y < want {
				checkModule(t, src, quiet, want, x, y, top)
			}
			if y+1 < want {
				checkModule(t, src, quiet, want, x, y+1, bottom)
			} else if bottom {
				t.Fatalf("row %d col %d: stray bottom half past the end", row, x)
			}
		}
	}
}

func checkModule(t *testing.T, src *qrMatrix, quiet, want, x, y int, dark bool) {
	t.Helper()
	inside := x >= quiet && y >= quiet && x < want-quiet && y < want-quiet
	if inside {
		if got := src.get(x-quiet, y-quiet); got != dark {
			t.Fatalf("module (%d,%d): rendered %v, want %v", x-quiet, y-quiet, dark, got)
		}
		return
	}
	if dark {
		t.Fatalf("quiet zone (%d,%d) rendered dark", x, y)
	}
}
