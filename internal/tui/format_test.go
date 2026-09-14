package tui

import (
	"testing"
	"time"
)

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KiB"},
		{31266996224, "29.1 GiB"},      // the USB drive from the session
		{2000398934016, "1.8 TiB"},     // the nvme
		{2977955840, "2.8 GiB"},        // the raspios image expanded
		{1024 * 1024 * 150, "150 MiB"}, // >=100 drops the decimal
		{-1, "-"},
	}
	for _, c := range cases {
		if got := humanBytes(c.in); got != c.want {
			t.Errorf("humanBytes(%d) = %q want %q", c.in, got, c.want)
		}
	}
}

func TestHumanRate(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "--"},
		{-5, "--"},
		{512, "512 B/s"},
		{48_000_000, "48.0 MB/s"},
		{1_500_000_000, "1.5 GB/s"},
	}
	for _, c := range cases {
		if got := humanRate(c.in); got != c.want {
			t.Errorf("humanRate(%v) = %q want %q", c.in, got, c.want)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{23 * time.Second, "0:23"},
		{95 * time.Second, "1:35"},
		{3725 * time.Second, "1:02:05"},
		{-time.Second, "--"},
	}
	for _, c := range cases {
		if got := humanDuration(c.in); got != c.want {
			t.Errorf("humanDuration(%v) = %q want %q", c.in, got, c.want)
		}
	}
}

func TestTruncateAndPad(t *testing.T) {
	if got := truncate("raspios-trixie.img.xz", 10); got != "raspios-t…" {
		t.Errorf("got %q", got)
	}
	if got := truncate("short", 10); got != "short" {
		t.Errorf("got %q", got)
	}
	if got := truncate("abc", 0); got != "" {
		t.Errorf("got %q", got)
	}
	if got := truncate("abc", 1); got != "…" {
		t.Errorf("got %q", got)
	}
	if got := pad("ab", 5); got != "ab   " {
		t.Errorf("got %q", got)
	}
	// Padding must never widen a column beyond its budget.
	if got := pad("abcdefgh", 4); len([]rune(got)) != 4 {
		t.Errorf("got %q (%d runes)", got, len([]rune(got)))
	}
}
