package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestEverySizeAligned(t *testing.T) {
	mounts := make([]Mount, 8)
	for i := range mounts {
		mounts[i] = Mount{
			Mountpoint: "/dev/mapper/root", Device: "/dev/mapper/root",
			Total: 2 << 40, Used: 1 << 40, Avail: 1 << 40,
			InoTotal: 100, InoUsed: 50, Mounted: true,
		}
	}
	ansi := regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)
	for w := 70; w <= 360; w += 10 {
		for h := 15; h <= 100; h += 5 {
			a := NewApp(DefaultTheme(), mounts, false, "", "")
			a.w, a.h = w, h
			var b strings.Builder
			a.drawDevices(&b)
			out := ansi.ReplaceAllString(b.String(), "")
			// simulate wrap; track the width of each visual row
			var widths []int
			col, cur := 1, 0
			for range out {
				cur++
				col++
				if col > w {
					widths = append(widths, cur)
					col, cur = 1, 0
				}
			}
			if cur > 0 {
				widths = append(widths, cur)
			}
			for i, rw := range widths[:len(widths)-1] { // skip hint
				if rw != w {
					t.Errorf("w=%d h=%d row %d width=%d", w, h, i, rw)
					goto next
				}
			}
		next:
		}
	}
}
