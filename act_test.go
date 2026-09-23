package main

import (
	"regexp"
	"strings"
	"testing"
)

var actAnsi = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

// frameRows splits a rendered frame into rows of w cells, simulating
// terminal wrap (like TestLayoutFitsWidth).
func frameRows(s string, w int) []string {
	vis := []rune(actAnsi.ReplaceAllString(s, ""))
	var rows []string
	for len(vis) > 0 {
		n := min(w, len(vis))
		rows = append(rows, string(vis[:n]))
		vis = vis[n:]
	}
	return rows
}

// TestActPanel checks the merged pie key + disk-activity panel beside the
// chart: frame rows are exactly w cells wide at every width (no overflow),
// and the activity rates show up.
func TestActPanel(t *testing.T) {
	mounts := []Mount{
		{Mountpoint: "/", Mounted: true, Total: 1 << 40, Used: 4 << 30, Avail: 1 << 30, Read: 50 << 20, Write: 25 << 20},
		{Mountpoint: "/home", Mounted: true, Total: 1 << 40, Used: 3 << 30, Avail: 1 << 30, Read: 0, Write: 0},
		{Mountpoint: "/mnt", Mounted: true, Total: 1 << 40, Used: 1 << 30, Avail: 1 << 30, Read: 10 << 20, Write: 5 << 20},
	}
	for _, w := range []int{160, 100, 80} {
		a := NewApp(DefaultTheme(), mounts, false, "", "")
		a.w, a.h = w, 24
		a.pushHist() // one sample so the graphs have a current tick to show
		var b strings.Builder
		a.drawDevices(&b)
		vis := actAnsi.ReplaceAllString(b.String(), "")
		rows := frameRows(b.String(), w)
		for i, r := range rows[:len(rows)-1] { // last row is the hint
			if len([]rune(r)) != w {
				t.Fatalf("width %d: row %d is %d cells, want %d", w, i, len([]rune(r)), w)
			}
		}
		if !strings.Contains(vis, "50.0M/s") || !strings.Contains(vis, "25.0M/s") {
			t.Fatalf("width %d: activity rates missing\n%s", w, vis)
		}
	}
}

// TestPiePanelDedup checks the panel is the pie's key AND its activity
// readout: each mount's name (and "other") appears exactly once across the
// panel, alongside its share and rates; every row is exactly rightW wide.
func TestPiePanelDedup(t *testing.T) {
	a := NewApp(DefaultTheme(), []Mount{
		{Mountpoint: "/", Mounted: true, Read: 50 << 20, Write: 25 << 20},
		{Mountpoint: "/home", Mounted: true, Read: 1 << 20, Write: 2 << 20},
		{Mountpoint: "/var/log", Mounted: true, Read: 3 << 20, Write: 1 << 20},
		{Mountpoint: "/boot", Mounted: true, Read: 0, Write: 0},
	}, false, "", "")
	a.pushHist() // one sample so rates show and the graphs aren't empty trays
	vals := []float64{500, 300, 100, 50, 40, 25, 10}
	names := []string{"/", "/home", "/mnt", "/boot", "/var", "/run", "/srv"}
	sl := pieSlices(vals, names)
	panel := a.piePanel(sl, 60, 12)
	joined := strings.Join(panel, "")
	for _, s := range sl {
		if got := strings.Count(joined, " "+s.name+" "); got != 1 {
			t.Fatalf("name %q appears %d times in panel, want exactly 1\n%s", s.name, got, joined)
		}
	}
	if !strings.Contains(joined, "50.0M/s") || !strings.Contains(joined, "25.0M/s") {
		t.Fatalf("rates missing:\n%s", joined)
	}
	for i, r := range panel {
		if got := len([]rune(actAnsi.ReplaceAllString(r, ""))); got != 60 {
			t.Fatalf("panel row %d is %d runes, want 60", i, got)
		}
	}
}

// TestRateHistory checks pushHist caps each mount's history at histCap and
// that histGraph keeps rows exactly w cells wide, trays idle cells, and
// fills braille once traffic shows.
func TestRateHistory(t *testing.T) {
	a := NewApp(DefaultTheme(), []Mount{
		{Mountpoint: "/", Mounted: true, Read: 40e6, Write: 20e6},
	}, false, "", "")
	for i := 0; i < 7; i++ { // 7 ticks, oldest first
		a.mounts[0].Read = float64(i) * 10e6
		a.pushHist()
	}
	h := a.hist["/"]
	if len(h.r) != 7 || len(h.w) != 7 {
		t.Fatalf("history depth %d/%d, want 7/7", len(h.r), len(h.w))
	}
	if h.r[len(h.r)-1] != 60e6 {
		t.Fatalf("newest read = %v, want 60e6", h.r[len(h.r)-1])
	}
	a.mounts[0].Read = 1
	for i := 0; i < histCap+10; i++ {
		a.pushHist()
	}
	if len(a.hist["/"].r) != histCap {
		t.Fatalf("history not capped: %d", len(a.hist["/"].r))
	}

	var b strings.Builder
	a.histGraph(&b, []float64{0, 0, 40e6, 80e6, 80e6}, 5, 80e6, "#fff")
	vis := actAnsi.ReplaceAllString(b.String(), "")
	if got := utf8RuneLen(vis); got != 5 {
		t.Fatalf("graph is %d cells, want 5", got)
	}
	if !strings.Contains(vis, "⣀") {
		t.Fatal("zero samples should stay on the tray baseline")
	}
	var b2 strings.Builder
	a.histGraph(&b2, []float64{80e6, 80e6, 80e6, 80e6, 80e6, 80e6, 80e6, 80e6, 80e6, 80e6}, 5, 80e6, "#fff")
	vis2 := actAnsi.ReplaceAllString(b2.String(), "")
	if strings.Count(vis2, "⣀") != 0 || vis2 == "" {
		t.Fatalf("full-scale samples should fill braille, got %q", vis2)
	}
}
