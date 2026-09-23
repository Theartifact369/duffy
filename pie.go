package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// mix returns the hex color t of the way from a to b.
func mix(a, b string, t float64) string {
	ra, ga, ba, ok1 := hexRGB(a)
	rb, gb, bb, ok2 := hexRGB(b)
	if !ok1 || !ok2 {
		return a
	}
	lerp := func(x, y int) int { return int(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return fmt.Sprintf("#%02x%02x%02x", lerp(ra, rb), lerp(ga, gb), lerp(ba, bb))
}

// piePixel is one pixel of a pie matrix: inside a slice (si) or empty.
type piePixel struct {
	in bool
	si int
}

// pieMatrix lays fracs (which must sum to 1) over a (2R+1)² pixel grid: a
// circle R pixels in radius, slices starting at the top going clockwise.
// Every two pixel rows become one text row of half-block glyphs, so the
// chart ends up (2R+1) cells wide and ~R+1 cells tall — round on terminal
// cells, which are roughly 2:1 tall. (Dividing dy by 2 instead would crop
// the top/bottom arcs out of the square matrix: the barrel bug.)
func pieMatrix(R int, fracs []float64) [][]piePixel {
	S := 2*R + 1
	cum := make([]float64, len(fracs)+1)
	for i, f := range fracs {
		cum[i+1] = cum[i] + f
	}
	m := make([][]piePixel, S)
	for yp := 0; yp < S; yp++ {
		m[yp] = make([]piePixel, S)
		for xp := 0; xp < S; xp++ {
			dx := float64(xp - R)
			dy := float64(yp - R)
			if dx*dx+dy*dy > float64(R*R)-0.4 {
				continue
			}
			// angle from straight up, clockwise, scaled to 0..1
			t := (math.Atan2(dx, -dy) + math.Pi) / (2 * math.Pi)
			si := 0
			for si < len(cum)-2 && t > cum[si+1] {
				si++
			}
			m[yp][xp] = piePixel{true, si}
		}
	}
	return m
}

// pieBase is the accent-tinted slice palette, ordered like the slices.
func pieBase(t Theme) []string {
	return []string{
		t.Accent,
		mix(t.Accent, t.BG, 0.4),
		mix(t.Accent, t.BG, 0.65),
		mix(t.Accent, t.BG, 0.85),
		mix(t.Accent, t.Text, 0.5),
	}
}

// sliceColor returns the palette color of pie slice i (its swatch and its
// chart fill match); the "other" slice is Secondary.
func sliceColor(t Theme, i int, other bool) string {
	if other {
		return t.Secondary
	}
	return pieBase(t)[i%len(pieBase(t))]
}

// pieSlice is one pie chart slice: its label, share, and palette index.
type pieSlice struct {
	name  string
	frac  float64
	other bool
	i     int // palette index (irrelevant for "other")
}

// pieSlices groups vals into the chart's slices: the five largest by size,
// everything else folded into "other". Same ordering the pie draws.
func pieSlices(vals []float64, names []string) []pieSlice {
	total := 0.0
	for _, v := range vals {
		total += v
	}
	if total <= 0 {
		return nil
	}
	idx := make([]int, len(vals))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(i, j int) bool { return vals[idx[i]] > vals[idx[j]] })
	var sl []pieSlice
	rest := 0.0
	for k, i := range idx {
		if k < 5 {
			sl = append(sl, pieSlice{names[i], vals[i] / total, false, k})
		} else {
			rest += vals[i] / total
		}
	}
	if rest > 0 {
		sl = append(sl, pieSlice{"other", rest, true, len(sl)})
	}
	return sl
}

// buildPie renders the devices-view pie chart: one string per text row,
// exactly S = 2R+1 runes wide (no border). It carries no legend — the key
// lives in piePanel, which renders next to it so names aren't duplicated.
// vals are used bytes per mount, names their mountpoints. Returns nil when
// there's nothing to show.
func (a *App) buildPie(vals []float64, names []string, R int) []string {
	t := a.theme
	total := 0.0
	for _, v := range vals {
		total += v
	}
	if total <= 0 {
		return nil
	}
	sl := pieSlices(vals, names)
	fracs := make([]float64, len(sl))
	for i, s := range sl {
		fracs[i] = s.frac
	}

	if R < 2 {
		return nil
	}
	S := 2*R + 1
	textRows := (S + 1) / 2
	m := pieMatrix(R, fracs)

	lines := make([]string, 0, textRows)
	for r := 0; r < textRows; r++ {
		var pie strings.Builder
		for xp := 0; xp < S; xp++ {
			top := m[2*r][xp]
			var bot piePixel
			if 2*r+1 < len(m) {
				bot = m[2*r+1][xp]
			}
			switch {
			case top.in && bot.in && top.si == bot.si:
				pie.WriteString(fgSeq(sliceColor(t, top.si, sl[top.si].other)) + "█")
			case top.in && bot.in: // two slices meet in one cell
				pie.WriteString(fgSeq(sliceColor(t, top.si, sl[top.si].other)) + bgSeq(sliceColor(t, bot.si, sl[bot.si].other)) + "▀")
			case top.in:
				pie.WriteString(fgSeq(sliceColor(t, top.si, sl[top.si].other)) + bgSeq(t.BG) + "▀")
			case bot.in:
				pie.WriteString(fgSeq(sliceColor(t, bot.si, sl[bot.si].other)) + bgSeq(t.BG) + "▄")
			default:
				pie.WriteString(" ")
			}
		}
		pie.WriteString(reset + bgSeq(t.BG))
		lines = append(lines, pie.String())
	}
	return lines
}
