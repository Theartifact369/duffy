package main

import (
	"regexp"
	"strings"
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

// TestPieMatrixSplit checks a 50/50 pie splits its pixels roughly in half,
// and a full pie fills everything inside the circle.
func TestPieMatrixSplit(t *testing.T) {
	m := pieMatrix(5, []float64{0.5, 0.5})
	c0, c1 := 0, 0
	for _, row := range m {
		for _, p := range row {
			if p.in && p.si == 0 {
				c0++
			} else if p.in && p.si == 1 {
				c1++
			}
		}
	}
	diff := c0 - c1
	if diff < 0 {
		diff = -diff
	}
	if c0+c1 == 0 || diff > 20 {
		t.Fatalf("50/50 pie unbalanced: %d vs %d pixels", c0, c1)
	}

	one := pieMatrix(5, []float64{1})
	if one[5][5] != (piePixel{true, 0}) {
		t.Fatal("center pixel not filled in full pie")
	}
}

// TestBuildPieWidth renders pie lines at width 80 and checks each is exactly
// inner runes wide (strip ANSI), with a legend entry per row.
func TestBuildPieWidth(t *testing.T) {
	a := &App{theme: DefaultTheme()}
	// seven mounts: the two smallest group into an "other" slice
	lines := a.buildPie(
		[]float64{500, 300, 100, 50, 40, 25, 10},
		[]string{"/", "/home", "/mnt", "/boot", "/var", "/run", "/srv"}, 78, 7)
	if len(lines) == 0 {
		t.Fatal("no pie lines")
	}
	ansi := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	for i, l := range lines {
		vis := ansi.ReplaceAllString(l, "")
		// rows are "│ " + line + "│", so the frame (inner+2) needs line = inner-1
		if got := utf8RuneLen(vis); got != 77 {
			t.Fatalf("line %d is %d runes, want 77", i, got)
		}
	}
	if !strings.Contains(ansi.ReplaceAllString(lines[0], ""), "██") {
		t.Fatal("legend swatch missing")
	}
	if !strings.Contains(ansi.ReplaceAllString(strings.Join(lines, ""), ""), "other") {
		t.Fatal("small slices did not group into 'other'")
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
