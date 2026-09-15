// Command flashtui writes OS images to removable drives from a terminal UI.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/susamn/flashtui/internal/tui"
)

// version is the release this binary was built from. GoReleaser sets it
// via -ldflags -X on a tagged build; a plain `go build` leaves it "dev",
// which is the honest answer for an untagged tree.
var version = "dev"

func main() {
	dir := flag.String("dir", "", "directory to scan for images (default: ~/Downloads)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: flashtui [-dir PATH] [-version]\n\n")
		fmt.Fprintf(os.Stderr, "Writes an OS image to a removable drive, with a guard on the\n")
		fmt.Fprintf(os.Stderr, "target device and optional headless first-boot setup.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Println("flashtui", version)
		return
	}

	if err := tui.Run(*dir); err != nil {
		fmt.Fprintf(os.Stderr, "flashtui: %v\n", err)
		os.Exit(1)
	}
}
