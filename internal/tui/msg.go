package tui

import (
	"fmt"

	"github.com/susamn/flashtui/internal/blockdev"
	"github.com/susamn/flashtui/internal/flash"
	"github.com/susamn/flashtui/internal/imagefile"
	"github.com/susamn/flashtui/internal/seed"
)

// imagesMsg carries the result of rescanning the image directory.
type imagesMsg struct {
	dir    string
	images []imagefile.Image
	err    error
}

// disksMsg carries the result of re-enumerating block devices.
type disksMsg struct {
	disks []blockdev.Disk
	err   error
}

// progressMsg is one sample from an in-flight flash.
type progressMsg flash.Progress

// flashDoneMsg ends a flash, successfully or not.
type flashDoneMsg struct {
	target string
	err    error
}

// mountMsg reports the outcome of a mount, unmount or power-off.
type mountMsg struct {
	action string
	detail string
	err    error
}

// detectMsg carries the post-flash classification of the written disk.
type detectMsg struct {
	detection seed.Detection
	err       error
}

// seedDoneMsg ends a seeding run.
type seedDoneMsg struct{ err error }

// tickMsg drives the indeterminate bar's animation.
type tickMsg struct{}

func sprintf(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}
