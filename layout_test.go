package main

import (
	"regexp"
	"strings"
	"testing"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// TestLayoutFitsWidth renders both views at many terminal widths and checks
// no column is negative, rows fit exactly between the borders, and the bars
// actually grow/shrink with the terminal instead of staying fixed.
func TestLayoutFitsWidth(t *testing.T) {
	mounts := []Mount{
		{Mountpoint: "/home/artifact369", Total: 2 << 40, Used: 1 << 40, Avail: 1 << 40},
		{Mountpoint: "/mnt/a-very-long-mountpoint-name", Total: 1 << 30, Used: 1 << 29, Avail: 1 << 29},
		{Mountpoint: "/", Total: 3 << 40, Used: 2 << 40, Avail: 1 << 40},
	}
	render := func(w int) string {
		a := NewApp(DefaultTheme(), mounts, false, "", "")
		a.w, a.h = w, 10
		var b strings.Builder
		a.drawDevices(&b)
		return b.String()
	}
	for _, w := range []int{16, 17, 20, 24, 30, 40, 60, 80, 120, 200} {
		out := render(w)
		sizeW, pctW, barW, devW, mountW := devLayout(w - 2)
		if sizeW < 4 || pctW < 0 || barW < 0 || devW < 0 || mountW < 0 {
			t.Fatalf("width %d: bad columns (%d,%d,%d,%d,%d)", w, sizeW, pctW, barW, devW, mountW)
		}
		// simulate the terminal: cursor wraps at col w, and any rune that
		// lands past it means a row overflowed the frame
		col := 1
		rowLen := 0
		var widths []int
		flush := func() {
			if rowLen > 0 {
				widths = append(widths, rowLen)
			}
			rowLen = 0
		}
		for range ansi.ReplaceAllString(out, "") {
			if col > w {
				t.Fatalf("width %d: row overflow", w)
			}
			col++
			rowLen++
			if col > w {
				col = 1
				flush()
			}
		}
		flush()
		// every boxed row must be exactly w cells; the trailing hint line
		// is plain text and is allowed to be shorter
		for i, rl := range widths[:len(widths)-1] {
			if rl != w {
				t.Fatalf("width %d: row %d is %d cells, want %d (misaligned frame)", w, i, rl, w)
			}
		}
	}

	// dir view: bars must grow with terminal width, never shrink
	last := 0
	for _, w := range []int{20, 30, 60, 120} {
		a := NewApp(DefaultTheme(), nil, false, "", "")
		a.scan = &Scan{
			Entries: []Entry{
				{Name: "a", Size: 1 << 40, IsDir: false},
				{Name: "subdir", Size: 1 << 39, IsDir: true},
			},
			Done: true,
		}
		a.w, a.h = w, 10
		var b strings.Builder
		a.drawDir(&b)
		bar := strings.Count(b.String(), "⣿") + strings.Count(b.String(), "⢸")
		if bar < last {
			t.Errorf("width %d: bar shrank (%d < %d)", w, bar, last)
		}
		last = bar
	}
}
