package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"
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

// buildPie renders the devices-view pie: one string per text row, exactly
// inner runes wide (pie cells + legend + padding, no border). vals are used
// bytes per mount, names their mountpoints. Small slices are grouped into
// an "other" slice colored Secondary. R is the pie radius, so the chart is
// (2R+1) cells wide and R+1 text rows tall — round on 2:1 cells. Returns
// nil when there's no room or nothing to show.
func (a *App) buildPie(vals []float64, names []string, inner, R int) []string {
	t := a.theme
	total := 0.0
	for _, v := range vals {
		total += v
	}
	if total <= 0 {
		return nil
	}

	// largest slices first; five on the chart, the rest as "other"
	idx := make([]int, len(vals))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(i, j int) bool { return vals[idx[i]] > vals[idx[j]] })
	type slice struct {
		name string
		frac float64
	}
	var sl []slice
	rest := 0.0
	for k, i := range idx {
		if k < 5 {
			sl = append(sl, slice{names[i], vals[i] / total})
		} else {
			rest += vals[i] / total
		}
	}
	if rest > 0 {
		sl = append(sl, slice{"other", rest})
	}

	base := pieBase(t)
	col := func(i int) string {
		if sl[i].name == "other" {
			return t.Secondary
		}
		return base[i%len(base)]
	}
	fracs := make([]float64, len(sl))
	for i, s := range sl {
		fracs[i] = s.frac
	}

	if R < 2 {
		return nil
	}
	S := 2*R + 1
	textRows := (S + 1) / 2
	maxName := inner - S - 2 - 10 // pie + gap + swatch/space/pct
	if maxName < 3 {
		return nil
	}
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
				pie.WriteString(fgSeq(col(top.si)) + "█")
			case top.in && bot.in: // two slices meet in one cell
				pie.WriteString(fgSeq(col(top.si)) + bgSeq(col(bot.si)) + "▀")
			case top.in:
				pie.WriteString(fgSeq(col(top.si)) + bgSeq(t.BG) + "▀")
			case bot.in:
				pie.WriteString(fgSeq(col(bot.si)) + bgSeq(t.BG) + "▄")
			default:
				pie.WriteString(" ")
			}
		}
		pie.WriteString(reset + bgSeq(t.BG))

		var leg strings.Builder
		if r < len(sl) {
			leg.WriteString(fgSeq(col(r)) + "██ ")
			name := trunc(sl[r].name, maxName)
			leg.WriteString(fgSeq(t.Secondary) + name +
				strings.Repeat(" ", maxName-utf8.RuneCountInString(name)))
			leg.WriteString(fgSeq(t.Text) +
				padR(fmt.Sprintf("%.1f%%", sl[r].frac*100), 7))
		} else {
			leg.WriteString(strings.Repeat(" ", 10+maxName))
		}
		leg.WriteString(reset + bgSeq(t.BG))

		lines = append(lines, pie.String()+" "+leg.String())
	}
	return lines
}
