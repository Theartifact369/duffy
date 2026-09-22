package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	theme := flag.String("theme", "", "theme name (default \"current\": btop's themes dir) or a path to a .theme file")
	all := flag.Bool("all", false, "include pseudo filesystems (proc, tmpfs, ...)")
	dir := flag.String("dir", "", "start browsing this directory instead of the device list")
	flag.Parse()

	name := *theme
	if name == "" {
		name = os.Getenv("DUT_THEME")
	}
	if name == "" {
		name = readConfigTheme()
	}
	load := name
	if load == "" || load == "system" {
		load = "current"
	}
	t, found := LoadTheme(load)
	if !found && load != "current" {
		fmt.Fprintf(os.Stderr, "duffy: theme %q not found (looked in ~/.config/{duffy,btop}/themes), using built-in palette\n", load)
	}

	a := NewApp(t, Mounts(*all), *all, *dir, name)
	if err := a.run(); err != nil {
		fmt.Fprintln(os.Stderr, "duffy:", err)
		os.Exit(1)
	}
}
