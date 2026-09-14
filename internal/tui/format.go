package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// humanBytes renders a byte count the way disk tooling does, in binary units,
// with enough precision to tell two similar drives apart.
func humanBytes(n int64) string {
	const unit = 1024
	if n < 0 {
		return "-"
	}
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit && exp < 5; m /= unit {
		div *= unit
		exp++
	}
	v := float64(n) / float64(div)
	suffix := [...]string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}[exp]
	// One decimal below 100 keeps "29.1 GiB" distinct from "29.9 GiB"; above
	// that the fraction is noise.
	if v >= 100 {
		return fmt.Sprintf("%.0f %s", v, suffix)
	}
	return fmt.Sprintf("%.1f %s", v, suffix)
}

// humanRate renders a transfer rate in decimal units, matching how drive and
// bus speeds are quoted.
func humanRate(bytesPerSec float64) string {
	if bytesPerSec <= 0 {
		return "--"
	}
	units := []string{"B/s", "kB/s", "MB/s", "GB/s"}
	v, i := bytesPerSec, 0
	for v >= 1000 && i < len(units)-1 {
		v /= 1000
		i++
	}
	if v >= 100 || i == 0 {
		return fmt.Sprintf("%.0f %s", v, units[i])
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}

// humanDuration renders a short clock, since an ETA is read at a glance.
func humanDuration(d time.Duration) string {
	if d < 0 {
		return "--"
	}
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// truncate shortens s to width, marking the cut with an ellipsis so the reader
// knows something was removed.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return string(r[:width-1]) + "…"
}

// pad right-pads s to width so columns line up, truncating when it overflows.
func pad(s string, width int) string {
	s = truncate(s, width)
	for len([]rune(s)) < width {
		s += " "
	}
	return s
}

// expandHome resolves a leading ~ so the directory prompt accepts the form
// people actually type.
func expandHome(p string) string {
	p = strings.TrimSpace(p)
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
		}
	}
	return p
}
