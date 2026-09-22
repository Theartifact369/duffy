package main

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Theme holds the colors duffy draws with. Names follow the btop theme keys
// they come from, so a btop theme file maps straight onto them.
type Theme struct {
	BG, FG, Title, Border  string
	Accent, SelBG, SelFG   string
	Secondary, Meter, Text string
}

// builtin is the fallback palette: Aether, this machine's btop look.
var builtin = Theme{
	BG: "#0f0e01", FG: "#e6e6d3", Title: "#e6e6d3",
	Accent: "#9aa124", SelBG: "#27261a", SelFG: "#9aa124",
	Secondary: "#666760", Meter: "#27261a", Text: "#eaeada",
	Border: "#9aa124", // box borders share the accent color
}

func DefaultTheme() Theme { return builtin }

// LoadTheme resolves a theme by name the way btop does: name -> file in a
// themes dir. duffy checks its own themes dir, then falls back to btop's, so
// both apps can share palette files. A missing file yields the builtin
// palette and found=false.
func LoadTheme(name string) (Theme, bool) {
	t := builtin
	p := themePath(name)
	if p == "" {
		return t, false
	}
	k := map[string]string{}
	f, err := os.Open(p)
	if err != nil {
		return t, false
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.Index(line, "theme[")
		if i < 0 {
			continue
		}
		line = line[i:]
		end := strings.IndexByte(line, ']')
		if end < 0 {
			continue
		}
		key := line[6:end]
		rest := strings.TrimSpace(line[end+1:])
		if rest == "" || rest[0] != '=' {
			continue
		}
		v := strings.Trim(strings.TrimSpace(rest[1:]), `"'`)
		if v != "" {
			k[key] = v
		}
	}

	t.BG = pick(k, "main_bg", t.BG)
	t.FG = pick(k, "main_fg", t.FG)
	t.Title = pick(k, "title", t.Title)
	t.Accent = pick(k, "hi_fg", t.Accent)
	t.Border = t.Accent // box borders share the accent color
	t.SelBG = pick(k, "selected_bg", t.SelBG)
	t.SelFG = pick(k, "selected_fg", t.SelFG)
	t.Secondary = pick(k, "inactive_fg", t.Secondary)
	t.Meter = pick(k, "meter_bg", t.Meter)
	t.Text = pick(k, "graph_text", t.Text)
	return t, true
}

func pick(k map[string]string, key, def string) string {
	if v, ok := k[key]; ok {
		return v
	}
	return def
}

// themePath resolves a theme name to a file, unless name is already a path.
func themePath(name string) string {
	if strings.ContainsAny(name, "/") {
		if _, err := os.Stat(name); err == nil {
			return name
		}
		return ""
	}
	home, _ := os.UserHomeDir()
	cands := []string{
		filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "duffy", "themes", name+".theme"),
		filepath.Join(home, ".config", "duffy", "themes", name+".theme"),
		filepath.Join(home, ".config", "btop", "themes", name+".theme"),
	}
	for _, c := range cands {
		if c != "" {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
	}
	return ""
}

// stockThemePath returns the read-only Omarchy btop theme for a system
// theme name, or "" if that theme ships no btop.theme.
func stockThemePath(name string) string {
	p := filepath.Join("/usr/share/omarchy/themes", name, "btop.theme")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

// listThemes returns theme names for the menu: *.theme files in
// ~/.config/duffy/themes and ~/.config/btop/themes, plus the names of
// Omarchy stock themes that ship a btop.theme.
func listThemes() []string {
	home, _ := os.UserHomeDir()
	dirs := []string{
		filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "duffy", "themes"),
		filepath.Join(home, ".config", "duffy", "themes"),
		filepath.Join(home, ".config", "btop", "themes"),
	}
	seen := map[string]bool{}
	var out []string
	add := func(d string) {
		fs, err := os.ReadDir(d)
		if err != nil {
			return
		}
		for _, f := range fs {
			name, ok := strings.CutSuffix(f.Name(), ".theme")
			if ok && name != "" && !f.IsDir() && !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	for _, d := range dirs {
		add(d)
	}
	if fs, err := os.ReadDir("/usr/share/omarchy/themes"); err == nil {
		for _, d := range fs {
			if d.IsDir() && !seen[d.Name()] && stockThemePath(d.Name()) != "" {
				seen[d.Name()] = true
				out = append(out, d.Name())
			}
		}
	}
	sort.Strings(out)
	return out
}

func configPath() string {
	if cfg := os.Getenv("XDG_CONFIG_HOME"); cfg != "" {
		return filepath.Join(cfg, "duffy", "config")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "duffy", "config")
}

// readConfigTheme reads color_theme from ~/.config/duffy/config ("" if unset).
func readConfigTheme() string {
	b, err := os.ReadFile(configPath())
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "color_theme") {
			if i := strings.IndexByte(line, '='); i >= 0 {
				return strings.Trim(strings.TrimSpace(line[i+1:]), `"`)
			}
		}
	}
	return ""
}

// writeConfigTheme persists the active theme, like btop's color_theme.
func writeConfigTheme(spec string) {
	if err := os.MkdirAll(filepath.Dir(configPath()), 0o755); err != nil {
		return
	}
	if err := os.WriteFile(configPath(), []byte("color_theme = "+strconv.Quote(spec)+"\n"), 0o644); err != nil {
		return
	}
}