# duffy

A small disk-usage TUI that shares btop's look and theme files.

- **Devices view** — df-style table of mounts with a device column, usage
  bars (like duf), a used-space pie chart in its own box above the table
  (five largest slices + "other") when the terminal is wide and tall enough,
  and nearly-full mounts (≥ 90%) flagged in red. The pie's key shares the
  box with a btop-style disk-activity readout: each slice gets two rows —
  swatch, name, share, and live read/write bars and rates — in pie order,
  so every mount's name appears exactly once, next to both its share and
  its I/O. The device list auto-refreshes every 2 s, so plugged-in drives
  appear on their own. Selecting a mount shows a deep-dive box below the
  list: inode usage bar, live read/write rates (from `/proc/diskstats`),
  and the biggest directories under that mount, scanned in the background.
- **Browse view** — enter any mount or directory, entries sized and sorted
  biggest-first (like ncdu), with live progress while scanning.
- **btop theming** — reads btop `.theme` files directly, so `duffy` and `btop`
  match out of the box. Edit `~/.config/btop/themes/*.theme` and both change.
  Usage bars use btop's dotted braille meter glyphs (`⣿⢸⣀`).
- **Theme menu** — press `m` to pick from your themes plus the Omarchy system
  theme; the choice persists in `~/.config/duffy/config`.
- Read-only browsing; mount/unmount on the devices view via udisks2
  (`u` unmount, `M` mount — unmounted block devices with a filesystem are
  listed too, marked "not mounted"). Mounting skips `/` by design.
- **Adaptive layout** — columns resize with the terminal: bars shrink first,
  then percentages, then sizes; at very narrow widths the bars drop instead
  of clipping. Works down to ~20 columns.

## Install

Requires Go (it's a single module, no other deps besides `golang.org/x/term`):

```sh
go build -ldflags "-s -w" -o ~/.local/bin/duffy .
```

## Run

```sh
duffy                # devices view, theme "current" from ~/.config/btop/themes
duffy -dir /home     # start browsing a directory
duffy --all          # include pseudo filesystems (proc, tmpfs, ...)
duffy --theme tokyo-night   # theme name or a path to a .theme file
```

Theme resolution: `--theme <name|path|system>`, else `$DUT_THEME`, else the
`color_theme` saved in `~/.config/duffy/config`, else `system` (the current
Omarchy/btop theme). `system` resolves to btop's `current.theme`. Names look
in `~/.config/duffy/themes/`, then `~/.config/btop/themes/` (so btop's theme
files work as-is), then `/usr/share/omarchy/themes/<name>/btop.theme`.
Missing theme -> built-in Aether palette.

Theme keys used: `main_bg`, `main_fg`, `title`, `hi_fg` (bars, accent, and
box borders), `selected_bg`, `selected_fg`, `inactive_fg`
(secondary text), `meter_bg` (bar tray), `graph_text` (numbers).

## Keys

| Key | Action |
|-----|--------|
| `↑/↓` `j` `k` | move |
| `Enter` `→` `l` | open mount / enter directory |
| `←` `h` `Esc` `Backspace` | up a directory (back to devices at root) |
| `r` | refresh mounts / rescan directory |
| `u` | unmount selected device (skips `/`) |
| `M` | mount selected device (requires udisks2) |
| `g` `G` `PgUp` `PgDn` | top / bottom / page |
| `m` | open theme menu (`Enter` applies, `Esc`/`←` closes) |
| `q` | quit |