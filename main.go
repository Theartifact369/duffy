package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	theme := flag.String("theme", "", "theme name or a path to a .theme file (default: the live system palette)")
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
	var t Theme
	if name == "" || name == "system" {
		// system colors: the live Omarchy palette, else btop's export
		t, _ = SystemTheme()
	} else {
		var found bool
		t, found = LoadTheme(name)
		if !found {
			fmt.Fprintf(os.Stderr, "duffy: theme %q not found (looked in ~/.config/{duffy,btop}/themes), using built-in palette\n", name)
		}
	}

	a := NewApp(t, Mounts(*all), *all, *dir, name)
	if err := a.run(); err != nil {
		fmt.Fprintln(os.Stderr, "duffy:", err)
		os.Exit(1)
	}
}
