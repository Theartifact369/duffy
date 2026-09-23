package main

import (
	"regexp"
	"testing"
)

func TestMix(t *testing.T) {
	if got, want := mix("#000000", "#ffffff", 0.5), "#808080"; got != want {
		t.Fatalf("mix = %s, want %s", got, want)
	}
	if got, want := mix("#9aa124", "#0f0e01", 0), "#9aa124"; got != want {
		t.Fatalf("mix at 0 = %s, want %s", got, want)
	}
}

// TestPieBrailleSplit checks a 50/50 pie splits its cells roughly in half,
// and a full pie fills every cell (the disc covers the whole dot grid).
func TestPieBrailleSplit(t *testing.T) {
	m := pieBraille(5, []float64{0.5, 0.5})
	c0, c1 := 0, 0
	for _, row := range m {
		for _, c := range row {
			if c.mask == 0 {
				continue
			}
			if c.si == 0 {
				c0++
			} else if c.si == 1 {
				c1++
			}
		}
	}
	diff := c0 - c1
	if diff < 0 {
		diff = -diff
	}
	if c0+c1 == 0 || diff > 12 {
		t.Fatalf("50/50 pie unbalanced: %d vs %d cells", c0, c1)
	}

	one := pieBraille(5, []float64{1})
	// the cell containing the center dot is fully filled (all 8 dots)
	center := one[2][5] // dot (10,10) -> cell row 2 (dots 8..11), col 5 (dots 10..11)
	if center.mask != 0xFF || center.si != 0 {
		t.Fatalf("center cell = %#v, want full mask in slice 0", center)
	}
}

// TestPieChartWidth renders pie lines and checks each is exactly S runes
// wide (strip ANSI); the key/legend lives in piePanel, so the chart itself
// carries no text.
func TestPieChartWidth(t *testing.T) {
	a := &App{theme: DefaultTheme()}
	// seven mounts: the two smallest group into an "other" slice
	lines := a.buildPie(
		[]float64{500, 300, 100, 50, 40, 25, 10},
		[]string{"/", "/home", "/mnt", "/boot", "/var", "/run", "/srv"}, 7)
	if len(lines) == 0 {
		t.Fatal("no pie lines")
	}
	ansi := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	for i, l := range lines {
		if got := utf8RuneLen(ansi.ReplaceAllString(l, "")); got != 15 {
			t.Fatalf("line %d is %d runes, want 15 (2R+1 chart only)", i, got)
		}
	}
	sl := pieSlices([]float64{500, 300, 100, 50, 40, 25, 10},
		[]string{"/", "/home", "/mnt", "/boot", "/var", "/run", "/srv"})
	if len(sl) != 6 || !sl[5].other || sl[5].name != "other" {
		t.Fatalf("expected top-5 + other, got %+v", sl)
	}
}

func TestTruncHead(t *testing.T) {
	if got, want := truncHead("/dev/nvme0n1p2", 8), "…me0n1p2"; got != want {
		t.Fatalf("truncHead = %q, want %q", got, want)
	}
	if got := truncHead("/dev/sda", 12); got != "/dev/sda" {
		t.Fatalf("truncHead short path = %q", got)
	}
}

func utf8RuneLen(s string) int { return len([]rune(s)) }
