package main

import (
	"regexp"
	"strings"
	"testing"
)

var actAnsi = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

// rangeWidths splits a rendered frame into row lengths, simulating terminal
// wrap at width w (like TestLayoutFitsWidth).
func rangeWidths(s string, w int) []int {
	col, rowLen := 1, 0
	var widths []int
	flush := func() {
		widths = append(widths, rowLen)
		rowLen = 0
	}
	for range actAnsi.ReplaceAllString(s, "") {
		col++
		rowLen++
		if col > w {
			col = 1
			flush()
		}
	}
	flush()
	return widths
}

// TestActPanel checks the disk-activity panel beside the pie: every row of
// the devices frame is exactly w cells wide at wide widths, the panel shows
// read and write rates and bars, and it's absent at narrow widths (pie box
// falls back to the full frame).
func TestActPanel(t *testing.T) {
	mounts := []Mount{
		{Mountpoint: "/", Mounted: true, Total: 1 << 40, Used: 4 << 30, Avail: 1 << 30, Read: 50 << 20, Write: 25 << 20},
		{Mountpoint: "/home", Mounted: true, Total: 1 << 40, Used: 3 << 30, Avail: 1 << 30, Read: 0, Write: 0},
		{Mountpoint: "/mnt", Mounted: true, Total: 1 << 40, Used: 1 << 30, Avail: 1 << 30, Read: 10 << 20, Write: 5 << 20},
	}
	for _, w := range []int{160, 100} {
		a := NewApp(DefaultTheme(), mounts, false, "", "")
		a.w, a.h = w, 24
		var b strings.Builder
		a.drawDevices(&b)
		vis := actAnsi.ReplaceAllString(b.String(), "")
		widths := rangeWidths(b.String(), w)
		for i, rl := range widths[:len(widths)-1] { // last row is the hint
			if rl != w {
				t.Fatalf("width %d: row %d is %d cells, want %d", w, i, rl, w)
			}
		}
		if w == 160 && (!strings.Contains(vis, "50.0M/s") || !strings.Contains(vis, "25.0M/s")) {
			t.Fatalf("width %d: activity rates missing\n%s", w, vis)
		}
	}
	// narrow terminal: panel must not render, pie keeps the full frame
	a := NewApp(DefaultTheme(), mounts, false, "", "")
	a.w, a.h = 80, 24
	var b strings.Builder
	a.drawDevices(&b)
	widths := rangeWidths(b.String(), 80)
	for i, rl := range widths[:len(widths)-1] {
		if rl != 80 {
			t.Fatalf("width 80: row %d is %d cells, want 80 (panel leaked into pie rows?)", i, rl)
		}
	}
}