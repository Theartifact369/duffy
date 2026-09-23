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

// pieCell is one braille cell of the pie: its dot mask (glyph = 0x2800+mask)
// and the slice coloring it — the most common slice among the cell's 8 dots,
// so a boundary cell bleeds to one side's color (cheap, invisible at a
// glance, and 4× the vertical detail the half-block chart used to have).
type pieCell struct {
	mask int
	si   int
}

// pieBraille lays fracs (which must sum to 1) on a (4R+1)² dot grid — 2
// dots wide, 4 tall per terminal cell — so the chart keeps the half-block
// version's box footprint but has a much rounder circle. Slices start at
// the top going clockwise; the whole grid is a filled disc of radius 2R
// dots, and braille dots are physically square (2 wide × 4 tall per cell).
func pieBraille(R int, fracs []float64) [][]pieCell {
	D := 4*R + 1
	cum := make([]float64, len(fracs)+1)
	for i, f := range fracs {
		cum[i+1] = cum[i] + f
	}
	sliceAt := func(dx, dy float64) int {
		t := (math.Atan2(dx, -dy) + math.Pi) / (2 * math.Pi)
		si := 0
		for si < len(cum)-2 && t > cum[si+1] {
			si++
		}
		return si
	}
	// braille dot bits per column, top row first: left dots 1,2,3,7,
	// right dots 4,5,6,8
	bit := [2][4]int{
		{0x01, 0x02, 0x04, 0x40},
		{0x08, 0x10, 0x20, 0x80},
	}
	cols, rows := (D+1)/2, (D+3)/4
	out := make([][]pieCell, rows)
	for cy := 0; cy < rows; cy++ {
		row := make([]pieCell, cols)
		for cx := 0; cx < cols; cx++ {
			var count [9]int // ≥ the 6 slices; keeps the majority scan safe
			for c := 0; c < 2; c++ {
				for r := 0; r < 4; r++ {
					dx := float64(2*cx + c - 2*R)
					dy := float64(4*cy + r - 2*R)
					if dx*dx+dy*dy > float64(4*R*R)-0.4 {
						continue
					}
					si := sliceAt(dx, dy)
					row[cx].mask |= bit[c][r]
					count[si]++
				}
			}
			for i, n := range count {
				if n > 0 && n >= count[row[cx].si] {
					row[cx].si = i
				}
			}
			if row[cx].mask == 0 {
				row[cx].si = -1
			}
		}
		out[cy] = row
	}
	return out
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
	m := pieBraille(R, fracs) // 2R+1 cells wide, R+1 tall — old footprint

	lines := make([]string, 0, len(m))
	for _, row := range m {
		var pie strings.Builder
		for _, c := range row {
			if c.mask == 0 {
				pie.WriteString(" ")
				continue
			}
			pie.WriteString(fgSeq(sliceColor(t, c.si, sl[c.si].other)))
			pie.WriteString(string(rune(0x2800 + c.mask)))
		}
		pie.WriteString(reset + bgSeq(t.BG))
		lines = append(lines, pie.String())
	}
	return lines
}
