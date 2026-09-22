package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTheme(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "x.theme")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadThemeFromFile(t *testing.T) {
	p := writeTheme(t, `
# a btop theme
theme[main_bg]="#101010"
theme[hi_fg]="green"
theme[title]=#ffffff
`)
	th, found := LoadTheme(p)
	if !found {
		t.Fatal("expected found")
	}
	if th.BG != "#101010" {
		t.Errorf("BG = %q, want #101010", th.BG)
	}
	if th.Accent != "green" {
		t.Errorf("Accent = %q, want green", th.Accent)
	}
	if th.Title != "#ffffff" {
		t.Errorf("Title = %q, want #ffffff", th.Title)
	}
	if th.FG != builtin.FG {
		t.Errorf("FG should fall back to builtin, got %q", th.FG)
	}
}

func TestLoadThemeMissing(t *testing.T) {
	if _, found := LoadTheme("/nonexistent/x.theme"); found {
		t.Fatal("expected not found for a missing path")
	}
	if _, found := LoadTheme("no-such-theme-name-xyz"); found {
		t.Fatal("expected not found for a missing name")
	}
}

func TestLoadThemeSkipEmpty(t *testing.T) {
	// An explicitly empty value means "transparent" in btop; for us it means
	// fall back to the builtin value (the theme key stays unset).
	p := writeTheme(t, "theme[main_bg]=\"\"\n")
	th, found := LoadTheme(p)
	if !found {
		t.Fatal("expected found")
	}
	if th.BG != builtin.BG {
		t.Errorf("BG = %q, want builtin %q", th.BG, builtin.BG)
	}
}

func TestFormatSize(t *testing.T) {
	cases := []struct {
		n    uint64
		want string
	}{
		{512, "512B"},
		{1 << 20, "1.0M"},
		{121 << 30, "121.0G"},
		{2 << 40, "2.0T"},
	}
	for _, c := range cases {
		if got := formatSize(c.n); got != c.want {
			t.Errorf("formatSize(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestTruncAndPad(t *testing.T) {
	if got := trunc("abcdef", 4); got != "abc…" {
		t.Errorf("trunc = %q, want abc…", got)
	}
	if got := trunc("ab", 4); got != "ab" {
		t.Errorf("trunc short = %q", got)
	}
	if got := padR("7G", 4); got != "  7G" {
		t.Errorf("padR = %q", got)
	}
	if got := padR("123456789", 4); got != "123…" {
		t.Errorf("padR trunc = %q", got)
	}
}

func TestMeterCell(t *testing.T) {
	cases := []struct {
		f    float64
		want string
	}{
		{0, "⢀"}, {0.49, "⢀"}, {0.5, "⢸"}, {1.5, "⢸"},
	}
	for _, c := range cases {
		if got := meterCell(c.f); got != c.want {
			t.Errorf("meterCell(%v) = %q, want %q", c.f, got, c.want)
		}
	}
}

func TestBarBraille(t *testing.T) {
	th := DefaultTheme()
	a := NewApp(th, nil, false, "", "")
	var b strings.Builder
	a.bar(&b, 0.5, 4) // half of 4 cells -> 2 filled, 2 tray
	out := b.String()
	full := strings.Count(out, "⣿") + strings.Count(out, "⢸")
	if want := 2; full != want {
		t.Errorf("filled cells = %d, want %d (out: %q)", full, want, out)
	}
	if !strings.Contains(out, "⣀") {
		t.Errorf("expected tray glyphs ⣀ in %q", out)
	}
}

func TestConfigRoundtrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeConfigTheme("nord")
	if got := readConfigTheme(); got != "nord" {
		t.Errorf("readConfigTheme after write = %q, want nord", got)
	}
	writeConfigTheme("system")
	if got := readConfigTheme(); got != "system" {
		t.Errorf("readConfigTheme after system write = %q, want system", got)
	}
}

func TestListThemes(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	dir := filepath.Join(xdg, "duffy", "themes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"nord", "dracula", "tokyo-night"} {
		if err := os.WriteFile(filepath.Join(dir, name+".theme"), []byte("theme[main_fg]=\"#fff\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// a non-theme file must be ignored
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := listThemes()
	for _, want := range []string{"nord", "dracula", "tokyo-night"} {
		found := false
		for _, g := range got {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("listThemes missing %q (got %v)", want, got)
		}
	}
	for _, g := range got {
		if g == "notes" {
			t.Errorf("listThemes included non-theme file %q", g)
		}
	}
}