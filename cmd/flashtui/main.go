// Command flashtui writes OS images to removable drives from a terminal UI.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/susamn/flashtui/internal/tui"
)

func main() {
	dir := flag.String("dir", "", "directory to scan for images (default: ~/Downloads)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: flashtui [-dir PATH]\n\n")
		fmt.Fprintf(os.Stderr, "Writes an OS image to a removable drive, with a guard on the\n")
		fmt.Fprintf(os.Stderr, "target device and optional headless first-boot setup.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if err := tui.Run(*dir); err != nil {
		fmt.Fprintf(os.Stderr, "flashtui: %v\n", err)
		os.Exit(1)
	}
}
