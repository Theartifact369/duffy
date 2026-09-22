package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

const reset = "\x1b[0m"

// warnColor flags nearly-full mounts (bars and percentages).
const warnColor = "#e5484d"

func fgSeq(c string) string {
	if c == "" {
		return ""
	}
	if r, g, b, ok := hexRGB(c); ok {
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
	}
	if i, ok := namedColor[strings.ToLower(c)]; ok {
		return fmt.Sprintf("\x1b[38;5;%dm", i)
	}
	return ""
}

func bgSeq(c string) string {
	if c == "" {
		return ""
	}
	if r, g, b, ok := hexRGB(c); ok {
		return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
	}
	if i, ok := namedColor[strings.ToLower(c)]; ok {
		return fmt.Sprintf("\x1b[48;5;%dm", i)
	}
	return ""
}

func hexRGB(h string) (r, g, b int, ok bool) {
	h = strings.TrimPrefix(strings.ToLower(h), "#")
	if len(h) == 3 {
		h = h[:1] + h[:1] + h[1:2] + h[1:2] + h[2:] + h[2:]
	}
	if len(h) != 6 {
		return 0, 0, 0, false
	}
	var v uint64
	if _, err := fmt.Sscanf(h, "%x", &v); err != nil {
		return 0, 0, 0, false
	}
	return int(v >> 16), int(v >> 8 & 0xff), int(v & 0xff), true
}

// namedColor maps the color names btop themes use to 256-color indices.
// Indices 0-15 hit the terminal's palette, so a themed terminal (like
// Omarchy's) recolors them automatically.
var namedColor = map[string]int{
	"black": 0, "red": 1, "green": 2, "yellow": 3, "blue": 4,
	"magenta": 5, "cyan": 6, "white": 7,
	"gray": 8, "grey": 8, "darkgray": 8,
	"bright black": 8, "bright red": 9, "bright green": 10, "bright yellow": 11,
	"bright blue": 12, "bright magenta": 13, "bright cyan": 14, "bright white": 15,
	"aqua": 14, "olive": 3, "violet": 5, "teal": 6, "navy": 4, "orange": 208,
}

// ---- App ----

type App struct {
	w, h   int
	fd     int
	theme  Theme
	all    bool
	mounts []Mount
	sel    int
	dir    string
	dirSel int
	scan   *Scan

	// theme menu state
	themeSpec string // what's active: "system" ("" is same), a name, or a path
	menuOpen  bool
	menuSel   int
	entries   []themeEntry

	err string // last mount/unmount failure, shown on the hint line

	// ioPrev/ioAt are the last /proc/diskstats sample and when it was taken;
	// the 2 s refresh diffs against them for per-mount read/write rates.
	ioPrev map[string][2]uint64
	ioAt   time.Time

	// detailPath/detailScan cache the biggest-dirs scan of the selected
	// mount; restarting only when the selection changes.
	detailPath string
	detailScan *Scan
}

// themeEntry is one row in the theme menu: what the menu shows and what
// setTheme should load (a name, a path, or "system").
type themeEntry struct {
	label string
	spec  string
}

func NewApp(t Theme, mounts []Mount, all bool, startDir, themeSpec string) *App {
	a := &App{theme: t, mounts: mounts, all: all, fd: int(os.Stdin.Fd()), w: 80, h: 24, sel: -1, themeSpec: themeSpec}
	if startDir != "" {
		a.dir = startDir
		a.scan = StartScan(startDir)
		a.dirSel = 0
	} else if len(mounts) > 0 {
		a.sel = 0
	}
	return a
}

func (a *App) run() error {
	old, err := term.MakeRaw(a.fd)
	if err != nil {
		return err
	}
	restore := func() {
		term.Restore(a.fd, old)
		fmt.Fprint(os.Stdout, "\x1b[?25h\x1b[?1049l")
	}
	defer restore()

	// Leave the terminal usable if we're killed, not just on 'q'.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sig
		restore()
		os.Exit(0)
	}()

	fmt.Fprint(os.Stdout, "\x1b[?1049h\x1b[?25l") // alt screen, hide cursor
	a.resize()

	keys := make(chan string, 16)
	go a.readKeys(keys)
	tick := time.NewTicker(150 * time.Millisecond)
	defer tick.Stop()
	refresh := time.NewTicker(2 * time.Second) // auto-rescan mounts
	defer refresh.Stop()

	a.draw()
	for {
		select {
		case k := <-keys:
			if k == "quit" {
				return nil
			}
			a.handle(k)
			a.draw()
		case <-tick.C:
			if a.scan != nil && !a.scan.done() {
				a.draw() // progressive repaint while scanning
			}
		case <-refresh.C:
			if a.dir == "" { // devices view: pick up newly plugged drives
				a.mounts = Mounts(a.all)
				if a.sel > len(a.mounts)-1 {
					a.sel = len(a.mounts) - 1
				}
				cur := diskIO()
				attachIO(a.mounts, a.ioPrev, cur, time.Since(a.ioAt).Seconds())
				a.ioPrev = cur // snapshot for the next refresh
				a.ioAt = time.Now()
				a.draw()
			}
		}
	}
}

func (a *App) resize() {
	if w, h, err := term.GetSize(a.fd); err == nil && w > 0 && h > 0 {
		a.w, a.h = w, h
	}
}

// readKeys translates raw bytes into semantic keys. Runs on its own
// goroutine; key handling happens back on the main loop.
func (a *App) readKeys(ch chan<- string) {
	r := bufio.NewReader(os.Stdin)
	for {
		b, err := r.ReadByte()
		if err != nil {
			return
		}
		switch b {
		case 'q', 0x03:
			ch <- "quit"
			return
		case '\r', '\n':
			ch <- "enter"
		case 0x7f, '\b':
			ch <- "backspace"
		case 'j':
			ch <- "down"
		case 'k':
			ch <- "up"
		case 'h':
			ch <- "left"
		case 'l':
			ch <- "right"
		case 'g':
			ch <- "home"
		case 'G':
			ch <- "end"
		case 'r':
			ch <- "rescan"
		case 'm':
			ch <- "menu"
		case 'M':
			ch <- "mount"
		case 'u':
			ch <- "unmount"
		case 0x1b: // ESC or the start of a sequence
			if r.Buffered() == 0 {
				ch <- "esc"
				continue
			}
			first, err := r.ReadByte()
			if err != nil {
				return
			}
			if first == '[' || first == 'O' {
				seq := []byte{first}
				for {
					c, err := r.ReadByte()
					if err != nil {
						return
					}
					seq = append(seq, c)
					if c >= 0x40 && c <= 0x7e {
						break
					}
				}
				switch string(seq) {
				case "[A":
					ch <- "up"
				case "[B":
					ch <- "down"
				case "[C":
					ch <- "right"
				case "[D":
					ch <- "left"
				case "[H":
					ch <- "home"
				case "[F":
					ch <- "end"
				case "[5~":
					ch <- "pageup"
				case "[6~":
					ch <- "pagedown"
				}
			}
		}
	}
}

func (a *App) handle(k string) {
	if a.menuOpen {
		a.handleMenu(k)
		return
	}
	if k == "menu" {
		a.openMenu()
		return
	}
	if a.dir == "" {
		a.handleDevices(k)
	} else {
		a.handleDir(k)
	}
}

// openMenu builds the theme entry list and opens the overlay.
func (a *App) openMenu() {
	a.entries = nil
	a.entries = append(a.entries, themeEntry{"system theme", "system"})
	for _, name := range listThemes() {
		spec := themePath(name)
		if spec == "" {
			spec = stockThemePath(name)
		}
		a.entries = append(a.entries, themeEntry{name, spec})
	}
	a.menuOpen = true
	a.menuSel = 0
	for i, e := range a.entries {
		if themeKey(e.spec) == themeKey(a.themeSpec) {
			a.menuSel = i
			break
		}
	}
}

// setTheme applies a spec live and persists it to ~/.config/duffy/config.
func (a *App) setTheme(spec string) {
	if spec == "" || spec == "system" {
		a.theme, _ = LoadTheme("current")
		a.themeSpec = ""
		writeConfigTheme("system")
		return
	}
	if t, ok := LoadTheme(spec); ok {
		a.theme = t
		a.themeSpec = spec
		writeConfigTheme(spec)
	}
}

func (a *App) handleMenu(k string) {
	n := len(a.entries)
	step := max(1, a.h-6)
	switch k {
	case "up":
		if a.menuSel > 0 {
			a.menuSel--
		}
	case "down":
		if a.menuSel < n-1 {
			a.menuSel++
		}
	case "pageup":
		a.menuSel = max(0, a.menuSel-step)
	case "pagedown":
		a.menuSel = min(n-1, a.menuSel+step)
	case "home":
		a.menuSel = 0
	case "end":
		a.menuSel = n - 1
	case "enter", "right":
		if a.menuSel >= 0 && a.menuSel < n {
			a.setTheme(a.entries[a.menuSel].spec)
		}
	case "menu", "esc", "left", "backspace":
		a.menuOpen = false
	}
}

func (a *App) handleDevices(k string) {
	n := len(a.mounts)
	step := max(1, a.h-5)
	switch k {
	case "up":
		if a.sel > 0 {
			a.sel--
		}
	case "down":
		if a.sel < n-1 {
			a.sel++
		}
	case "pageup":
		a.sel = max(0, a.sel-step)
	case "pagedown":
		a.sel = min(n-1, a.sel+step)
	case "home":
		a.sel = 0
	case "end":
		a.sel = n - 1
	case "enter", "right":
		if a.sel >= 0 && a.sel < n {
			a.enterDir(a.mounts[a.sel].Mountpoint)
		}
	case "rescan":
		a.mounts = Mounts(a.all)
		if a.sel > len(a.mounts)-1 {
			a.sel = len(a.mounts) - 1
		}
	case "unmount":
		if a.sel >= 0 && a.sel < n {
			if m := a.mounts[a.sel]; m.Mounted && m.Mountpoint != "/" {
				a.mountAction("unmount", m.Device)
			}
		}
	case "mount":
		if a.sel >= 0 && a.sel < n {
			if m := a.mounts[a.sel]; !m.Mounted {
				a.mountAction("mount", m.Device)
			}
		}
	}
}

// mountAction mounts or unmounts a device via udisksctl, then refreshes the
// device list. Failures land in a.err, shown on the hint line.
func (a *App) mountAction(kind, dev string) {
	a.err = ""
	cmd := exec.Command("udisksctl", kind, "-b", dev)
	if out, err := cmd.CombinedOutput(); err != nil {
		a.err = strings.TrimSpace(string(out))
		if a.err == "" {
			a.err = err.Error()
		}
	}
	a.mounts = Mounts(a.all)
	if n := len(a.mounts); a.sel >= n {
		a.sel = n - 1
	}
}

func (a *App) handleDir(k string) {
	entries, _, _ := a.scan.Snapshot()
	rows := len(entries) + 1 // +1 for the virtual ".." row
	step := max(1, a.h-5)
	switch k {
	case "up":
		if a.dirSel > 0 {
			a.dirSel--
		}
	case "down":
		if a.dirSel < rows-1 {
			a.dirSel++
		}
	case "pageup":
		a.dirSel = max(0, a.dirSel-step)
	case "pagedown":
		a.dirSel = min(rows-1, a.dirSel+step)
	case "home":
		a.dirSel = 0
	case "end":
		a.dirSel = rows - 1
	case "enter", "right":
		if a.dirSel == 0 {
			a.goUp()
			return
		}
		if e := entries[a.dirSel-1]; e.IsDir {
			a.enterDir(filepath.Join(a.dir, e.Name))
		}
	case "left", "backspace", "esc":
		a.goUp()
	case "rescan":
		a.scan = StartScan(a.dir)
		a.dirSel = 0
	}
}

func (a *App) goUp() {
	if parent := filepath.Dir(a.dir); parent != a.dir {
		a.enterDir(parent)
		return
	}
	a.dir, a.scan = "", nil
}

func (a *App) enterDir(path string) {
	a.dir, a.scan, a.dirSel = path, StartScan(path), 0
}

// ---- rendering ----

func (a *App) draw() {
	a.resize()
	var b strings.Builder
	b.WriteString("\x1b[2J\x1b[H")
	if a.theme.BG != "" {
		b.WriteString(bgSeq(a.theme.BG))
	}
	if a.dir == "" {
		a.drawDevices(&b)
	} else {
		a.drawDir(&b)
	}
	if a.menuOpen {
		a.drawMenu(&b)
	}
	fmt.Fprint(os.Stdout, b.String())
}

// at moves the cursor to row/col (1-based), for drawing the menu overlay.
func (a *App) at(b *strings.Builder, row, col int) {
	b.WriteString(fmt.Sprintf("\x1b[%d;%dH", row, col))
}

// drawMenu draws the theme-picker overlay centered on the current view.
func (a *App) drawMenu(b *strings.Builder) {
	t := a.theme
	n := len(a.entries)
	w := min(44, max(20, a.w-6), a.w-2)
	h := clamp(3, min(n+2, a.h-6), 200)
	x := max(1, (a.w-w)/2)
	y := max(1, (a.h-h)/2)

	title := "menu"
	if a.themeSpec == "" {
		title = "menu · system theme"
	}
	title = trunc(title, max(1, w-6))
	dashes := max(0, w-5-utf8.RuneCountInString(title))

	a.at(b, y, x)
	b.WriteString(fgSeq(t.Border))
	b.WriteString("╭─ " + title + " ")
	b.WriteString(strings.Repeat("─", dashes) + "╮")

	vis := min(n, h-2)
	top := 0
	if a.menuSel > top+vis-1 {
		top = a.menuSel - vis + 1
	} else if a.menuSel < top {
		top = a.menuSel
	}
	for i := top; i < top+vis; i++ {
		e := a.entries[i]
		sel := i == a.menuSel
		fgC, bgC := t.FG, ""
		if sel {
			fgC, bgC = t.SelFG, t.SelBG
		}
		marker := "  "
		if themeKey(e.spec) == themeKey(a.themeSpec) {
			marker = "● "
		}
		label := trunc(marker+e.label, w-4)
		label += strings.Repeat(" ", w-4-utf8.RuneCountInString(label))
		a.at(b, y+1+i-top, x)
		b.WriteString(fgSeq(t.Border))
		b.WriteString("│ ")
		a.text(b, label, fgC, bgC)
		b.WriteString(fgSeq(t.Border))
		b.WriteString(" │")
	}

	a.at(b, y+h-1, x)
	b.WriteString(fgSeq(t.Border))
	b.WriteString("╰" + strings.Repeat("─", max(0, w-2)) + "╯")
	b.WriteString(reset)
	if t.BG != "" {
		b.WriteString(bgSeq(t.BG))
	}
}

// clamp bounds v to [lo, hi].
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// themeKey normalizes a theme spec for comparison: system and empty are the
// same thing (both mean "the current/omarchy theme").
func themeKey(spec string) string {
	if spec == "system" {
		return ""
	}
	return spec
}

// text writes s with fg and optional bg, then restores the base background.
func (a *App) text(b *strings.Builder, s, fgC, bgC string) {
	if fgC != "" {
		b.WriteString(fgSeq(fgC))
	}
	if bgC != "" {
		b.WriteString(bgSeq(bgC))
	}
	b.WriteString(s)
	b.WriteString(reset)
	if a.theme.BG != "" {
		b.WriteString(bgSeq(a.theme.BG))
	}
}

// bar draws a btop-style braille meter: dotted ⣿ cells for the used part
// (partial boundary cell in ⢀/⢸), ⣀ bottom-dot cells for the tray. hi is
// the color of the filled cells (Accent normally, warnColor when full).
func (a *App) bar(b *strings.Builder, frac float64, w int, hi string) {
	if w <= 0 {
		return
	}
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	b.WriteString(bgSeq(a.theme.Meter))
	level := frac * float64(w) // continuous filled cells
	for i := 0; i < w; i++ {
		cell := level - float64(i) // fill fraction of this cell, 0..1
		if cell >= 1 {
			b.WriteString(fgSeq(hi))
			b.WriteString("⣿")
		} else if cell > 0 {
			b.WriteString(fgSeq(hi))
			b.WriteString(meterCell(cell))
		} else {
			b.WriteString(fgSeq(a.theme.Secondary))
			b.WriteString("⣀")
		}
	}
	b.WriteString(reset)
	if a.theme.BG != "" {
		b.WriteString(bgSeq(a.theme.BG))
	}
}

// meterCell returns the braille glyph for a partially filled cell.
func meterCell(f float64) string {
	if f >= 0.5 {
		return "⢸"
	}
	return "⢀"
}

func (a *App) boxTop(b *strings.Builder, inner int, title string) {
	t := a.theme
	title = trunc(title, max(0, inner-8))
	dashes := max(0, a.w-5-utf8.RuneCountInString(title))
	b.WriteString(fgSeq(t.Border))
	b.WriteString("╭─ ")
	a.text(b, title, t.Title, "")
	b.WriteString(fgSeq(t.Border))
	b.WriteString(" ")
	b.WriteString(strings.Repeat("─", dashes))
	b.WriteString("╮")
}

func (a *App) boxBottom(b *strings.Builder) {
	b.WriteString(fgSeq(a.theme.Border))
	b.WriteString("╰" + strings.Repeat("─", max(0, a.w-2)) + "╯")
	b.WriteString(reset)
	if a.theme.BG != "" {
		b.WriteString(bgSeq(a.theme.BG))
	}
}

// hintKey is one binding shown on the bottom status line; the key gets the
// accent color so it stands out from its description.
type hintKey struct{ key, desc string }

// hint renders the bottom status line: an optional momentary message (error,
// scan counter) followed by key bindings. Everything is truncated to the
// terminal width, keys dropped from the right first when space runs out.
func (a *App) hint(b *strings.Builder, msg string, keys []hintKey) {
	b.WriteString("  ")
	left := max(0, a.w-2)
	write := func(s, fg string) {
		s = trunc(s, left)
		a.text(b, s, fg, "")
		left -= utf8.RuneCountInString(s)
	}
	if msg != "" {
		write(msg, a.theme.Secondary)
		write("    ", a.theme.Secondary)
	}
	for i, k := range keys {
		if i > 0 {
			write("    ", a.theme.Secondary)
		}
		write(k.key, a.theme.Accent)
		write(" "+k.desc, a.theme.Secondary)
		if left <= 0 {
			break
		}
	}
}

// detailLine is one content row of the per-mount deep-dive box.
type detailLine struct{ l, fg string }

// detailLines builds the rows for the selected device's deep-dive box:
// an inode usage bar, live read/write rates, and biggest-directory
// warnings (background scan, restarted when the selection changes).
// Returns nil when nothing is worth showing.
func (a *App) detailLines(inner int) []detailLine {
	t := a.theme
	if a.sel < 0 || a.sel >= len(a.mounts) {
		return nil
	}
	m := a.mounts[a.sel]
	if !m.Mounted {
		return nil
	}
	// keep one background biggest-dirs scan per selected mountpoint
	// ponytail: walks the whole mount once per selection — same cost as the
	// browse view's Enter, but automatic. If selections churn on huge roots,
	// gate it behind a keypress instead.
	if a.detailPath != m.Mountpoint {
		a.detailPath = m.Mountpoint
		a.detailScan = StartScan(m.Mountpoint)
	}
	entries, done, count := a.detailScan.Snapshot()

	var lines []detailLine

	// btrfs and some other fses report f_files = 0; skip the inode bar then,
	// the r/w and biggest-dir rows are still worth showing
	if m.InoTotal > 0 {
		inodeFrac := float64(m.InoUsed) / float64(m.InoTotal)
		inoHi := t.Accent
		if inodeFrac >= 0.9 {
			inoHi = warnColor
		}
		var lb strings.Builder
		lb.WriteString(fgSeq(t.Secondary))
		lb.WriteString("inodes ")
		a.bar(&lb, inodeFrac, 12, inoHi)
		lb.WriteString(fgSeq(t.Text))
		lb.WriteString(fmt.Sprintf(" %4.1f%%  %s", inodeFrac*100, formatSize(m.InoUsed)))
		lines = append(lines, detailLine{lb.String(), ""})
	}

	var rb strings.Builder
	rb.WriteString(fgSeq(t.Secondary))
	rb.WriteString("r/w    ")
	rb.WriteString(fgSeq(t.Text))
	rb.WriteString("read " + formatRate(m.Read))
	rb.WriteString(fgSeq(t.Secondary))
	rb.WriteString("  ")
	rb.WriteString(fgSeq(t.Text))
	rb.WriteString("write " + formatRate(m.Write))
	lines = append(lines, detailLine{rb.String(), ""})

	// top-3 biggest dirs as "largest dir warnings", like the doc's footer
	for i, e := range entries {
		if i >= 3 {
			break
		}
		name := e.Name
		if e.IsDir {
			name += "/"
		}
		lines = append(lines, detailLine{
			"  " + trunc(name, 24) + strings.Repeat(" ", 25-utf8.RuneCountInString(trunc(name, 24))) + padR(formatSize(e.Size), 8),
			t.Secondary})
	}
	if !done {
		lines = append(lines, detailLine{fmt.Sprintf("scanning %d files…", count), t.Secondary})
	}
	return lines
}

// visRunes returns the visible (non-ANSI) rune count of s.
var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

func visRunes(s string) int {
	return utf8.RuneCountInString(ansiRE.ReplaceAllString(s, ""))
}

// drawDetail renders the deep-dive box built by detailLines.
func (a *App) drawDetail(b *strings.Builder, inner int, lines []detailLine) {
	t := a.theme
	m := a.mounts[a.sel]
	a.boxTop(b, inner, "detail · "+truncHead(m.Device, max(12, inner-12)))
	for _, ln := range lines {
		b.WriteString(fgSeq(t.Border))
		b.WriteString("│ ")
		if vis := visRunes(ln.l); vis < inner-1 {
			ln.l += strings.Repeat(" ", inner-1-vis)
		}
		if ln.fg != "" {
			b.WriteString(fgSeq(ln.fg))
		}
		b.WriteString(ln.l)
		b.WriteString(reset)
		if t.BG != "" {
			b.WriteString(bgSeq(t.BG))
		}
		b.WriteString(fgSeq(t.Border))
		b.WriteString("│")
		b.WriteString(reset)
		if t.BG != "" {
			b.WriteString(bgSeq(t.BG))
		}
	}
	a.boxBottom(b)
}

func (a *App) drawDevices(b *strings.Builder) {
	t := a.theme
	n := len(a.mounts)
	inner := max(0, a.w-2)

	// used-space pie in its own box on top, growing into the rows the device
	// list, detail box, and hint don't need (tall terminals get a bigger
	// chart instead of blank space under the table)
	pieBox := 0
	var detail []detailLine
	detailBox := 0
	if a.h >= 15 && a.w >= 70 {
		if lines := a.detailLines(inner); lines != nil {
			detail = lines
		}
		// budget the pie against the detail box's max height (inode + r/w +
		// 3 dirs + scanning, plus borders) so it doesn't rescale every tick
		// while the "scanning N files…" row comes and goes
		detailMax := 8
		if len(detail) == 0 {
			detailMax = 0
		}
		var vals []float64
		var names []string
		for _, m := range a.mounts {
			if m.Mounted && m.Used > 0 {
				vals = append(vals, float64(m.Used))
				names = append(names, m.Mountpoint)
			}
		}
		// table keeps at least all n mounts: hint(1) + borders(2) + n +
		// detailMax + pie(R+3) <= a.h, so R = a.h - 6 - n - detailMax.
		// The pie can't be wider than the name column allows: legend needs
		// maxName >= 3, and maxName = inner - (2R+1) - 2 - 10, so
		// R <= (inner-16)/2. Let both constraints bind — tall/wide terminals
		// get the full free-lines chart, not a fixed mid-size.
		R := a.h - 6 - len(a.mounts) - detailMax
		if maxR := (inner - 16) / 2; R > maxR {
			R = maxR
		}
		if R < 7 {
			R = 7
		}
		if lines := a.buildPie(vals, names, inner, R); lines != nil {
			pieBox = len(lines) + 2 // rows inside, plus top/bottom borders
			a.boxTop(b, inner, "used by device")
			for _, l := range lines {
				b.WriteString(fgSeq(t.Border))
				b.WriteString("│ ")
				b.WriteString(l)
				b.WriteString(fgSeq(t.Border))
				b.WriteString("│")
				b.WriteString(reset)
				if t.BG != "" {
					b.WriteString(bgSeq(t.BG))
				}
			}
			a.boxBottom(b)
		}
	}

	// deep-dive box for the selected device, when the terminal has room
	// for it above the hint (table keeps at least one full row)
	if len(detail) > 0 && a.h-3-pieBox-(len(detail)+2) >= 1 {
		detailBox = len(detail) + 2
	} else {
		detail = nil
	}

	title := fmt.Sprintf("%d local devices", n)
	if a.all {
		title = fmt.Sprintf("%d filesystems", n)
	}
	a.boxTop(b, inner, title)

	sizeW, pctW, barW, devW, mountW := devLayout(inner)

	rows := max(1, a.h-3-pieBox-detailBox)
	top := 0
	if a.sel > top+rows-1 {
		top = a.sel - rows + 1
	} else if a.sel < top {
		top = a.sel
	}
	for i := top; i < min(n, top+rows); i++ {
		m := a.mounts[i]
		sel := i == a.sel
		b.WriteString(fgSeq(t.Border))
		b.WriteString("│ ")
		rowBG, rowFG := "", t.FG
		if sel {
			rowBG, rowFG = t.SelBG, t.SelFG
		}
		use := 0.0
		if m.Total > 0 {
			use = float64(m.Used) / float64(m.Total)
		}
		full := use >= 0.9 // warn color on pct and bar
		dev := truncHead(m.Device, devW)
		a.text(b, dev+strings.Repeat(" ", max(0, devW-utf8.RuneCountInString(dev))), t.Secondary, rowBG)
		mount := trunc(m.Mountpoint, mountW)
		a.text(b, mount+strings.Repeat(" ", max(0, mountW-utf8.RuneCountInString(mount))), rowFG, rowBG)
		if !m.Mounted {
			rest := 3*sizeW + pctW + barW
			a.text(b, trunc("not mounted", rest)+strings.Repeat(" ", max(0, rest-utf8.RuneCountInString("not mounted"))), t.Secondary, rowBG)
			b.WriteString(fgSeq(t.Border))
			b.WriteString("│")
			b.WriteString(reset)
			if t.BG != "" {
				b.WriteString(bgSeq(t.BG))
			}
			continue
		}
		a.text(b, padR(formatSize(m.Total), sizeW), t.Text, rowBG)
		a.text(b, padR(formatSize(m.Used), sizeW), t.Text, rowBG)
		a.text(b, padR(formatSize(m.Avail), sizeW), t.Text, rowBG)
		pctFg := rowFG
		if full {
			pctFg = warnColor
		}
		a.text(b, padR(fmt.Sprintf("%.1f%%", use*100), pctW), pctFg, rowBG)
		hi := t.Accent
		if full {
			hi = warnColor
		}
		a.bar(b, use, barW, hi)
		b.WriteString(fgSeq(t.Border))
		b.WriteString("│")
		b.WriteString(reset)
		if t.BG != "" {
			b.WriteString(bgSeq(t.BG))
		}
	}
	a.boxBottom(b)
	if len(detail) > 0 {
		a.drawDetail(b, inner, detail)
	}
	a.hint(b, a.err, []hintKey{
		{"q", "quit"},
		{"r", "refresh"},
		{"↑/↓", "move"},
		{"Enter", "open"},
		{"u", "unmount"},
		{"M", "mount"},
	})
}

func (a *App) drawDir(b *strings.Builder) {
	t := a.theme
	entries, done, count := a.scan.Snapshot()
	inner := max(0, a.w-2)
	a.boxTop(b, inner, a.dir)

	sizeW, barW, nameW := dirLayout(inner)

	rows := max(1, a.h-3)
	top := 0
	if a.dirSel > top+rows-1 {
		top = a.dirSel - rows + 1
	} else if a.dirSel < top {
		top = a.dirSel
	}
	biggest := uint64(0)
	if len(entries) > 0 {
		biggest = entries[0].Size
	}
	for i := top; i < min(len(entries)+1, top+rows); i++ {
		b.WriteString(fgSeq(t.Border))
		b.WriteString("│ ")
		sel := i == a.dirSel
		rowBG, rowFG := "", t.FG
		if sel {
			rowBG, rowFG = t.SelBG, t.SelFG
		}
		if i == 0 { // virtual ".." row
			a.text(b, ".."+strings.Repeat(" ", max(0, nameW-2)), rowFG, rowBG)
			a.text(b, strings.Repeat(" ", sizeW), t.Text, rowBG)
			a.bar(b, 0, barW, t.Accent)
			b.WriteString(fgSeq(t.Border))
			b.WriteString("│")
			b.WriteString(reset)
			if t.BG != "" {
				b.WriteString(bgSeq(t.BG))
			}
			continue
		}
		e := entries[i-1]
		name := e.Name
		if e.IsDir {
			name += "/"
		}
		name = trunc(name, nameW)
		a.text(b, name+strings.Repeat(" ", nameW-utf8.RuneCountInString(name)), rowFG, rowBG)
		a.text(b, padR(formatSize(e.Size), sizeW), t.Text, rowBG)
		frac := 0.0
		if biggest > 0 {
			frac = float64(e.Size) / float64(biggest)
		}
		a.bar(b, frac, barW, t.Accent)
		b.WriteString(fgSeq(t.Border))
		b.WriteString("│")
		b.WriteString(reset)
		if t.BG != "" {
			b.WriteString(bgSeq(t.BG))
		}
	}
	a.boxBottom(b)
	msg := ""
	if !done {
		msg = fmt.Sprintf("scanning %d files…", count)
	}
	a.hint(b, msg, []hintKey{
		{"q", "quit"},
		{"r", "rescan"},
		{"↑/↓", "move"},
		{"Enter/→", "enter"},
		{"←/Esc", "back"},
	})
}

// devLayout splits a devices-row width into columns: device, mount name, three
// sizes, percent, bar. The mount name flexes; fixed columns shrink in order
// (bar, %, device, then sizes) as the terminal narrows, and at very narrow
// widths the bar then % then device columns are dropped entirely, so a row
// fills exactly the full frame (no mid-line wrap). The mount cell is padded
// out to absorb the "│ " and "│" border cells, so every row is exactly w
// runes wide.
func devLayout(inner int) (sizeW, pctW, barW, devW, mountW int) {
	sizeW, pctW, devW = 7, 5, 12
	barW = min(24, max(4, inner/5))
	mountW = inner - 3*sizeW - pctW - barW - devW - 1
	for mountW < 6 && (barW > 4 || pctW > 4 || devW > 4 || sizeW > 4) {
		if barW > 4 {
			barW--
		} else if pctW > 4 {
			pctW--
		} else if devW > 4 {
			devW--
		} else {
			sizeW--
		}
		mountW = inner - 3*sizeW - pctW - barW - devW - 1
	}
	// still cramped? drop the bar, then the %, then the device column
	// (bar() no-ops at w<=0, padR(pct, 0) renders nothing, so the row just
	// has one fewer column).
	for mountW < 6 && (barW > 0 || pctW > 0 || devW > 0) {
		if barW > 0 {
			barW--
		} else if pctW > 0 {
			pctW--
		} else {
			devW--
		}
		mountW = inner - 3*sizeW - pctW - barW - devW - 1
	}
	mountW = max(0, mountW)
	return
}

// dirLayout splits a dir-row width into columns; the name flexes, the bar
// then the size column shrink as the terminal narrows. Same as devLayout,
// the name cell is padded out so every row is exactly w runes wide.
func dirLayout(inner int) (sizeW, barW, nameW int) {
	sizeW, barW = 8, min(20, max(6, inner/4))
	nameW = inner - sizeW - barW - 1
	for nameW < 6 && (barW > 4 || sizeW > 5) {
		if barW > 4 {
			barW--
		} else {
			sizeW--
		}
		nameW = inner - sizeW - barW - 1
	}
	for nameW < 6 && (barW > 0 || sizeW > 4) {
		if barW > 0 {
			barW--
		} else {
			sizeW--
		}
		nameW = inner - sizeW - barW - 1
	}
	nameW = max(0, nameW)
	return
}

// padR left-pads s with spaces to width w (runes), truncating if longer.
func padR(s string, w int) string {
	if w <= 0 {
		return ""
	}
	n := utf8.RuneCountInString(s)
	if n > w {
		return trunc(s, w)
	}
	return strings.Repeat(" ", w-n) + s
}

func trunc(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string([]rune(s)[:w-1]) + "…"
}

// truncHead keeps the tail of s — device paths lose their start, not the
// partition number at the end: /dev/nvme0n1p2 → …p2.
func truncHead(s string, w int) string {
	if w <= 0 {
		return ""
	}
	n := utf8.RuneCountInString(s)
	if n <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return "…" + string([]rune(s)[n-w+1:])
}
