package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestProgressBarFill(t *testing.T) {
	plain := lipgloss.NewStyle()
	cases := []struct {
		frac  float64
		want  int
		label string
	}{
		{0, 0, "empty"},
		{0.5, 10, "half"},
		{1, 20, "full"},
		{1.5, 20, "clamped above"},
		{-0.2, 0, "clamped below"},
	}
	for _, c := range cases {
		got := progressBar(c.frac, 20, plain)
		if n := strings.Count(got, "█"); n != c.want {
			t.Errorf("%s: %d filled cells, want %d", c.label, n, c.want)
		}
		if total := len([]rune(got)); total != 20 {
			t.Errorf("%s: bar is %d cells, want 20", c.label, total)
		}
	}
}

func TestProgressBarTooNarrow(t *testing.T) {
	if got := progressBar(0.5, 2, lipgloss.NewStyle()); got != "" {
		t.Errorf("got %q, want empty when there is no room", got)
	}
}

// The indeterminate bar bounces rather than wrapping, so it never looks like
// progress restarting from zero.
func TestIndeterminateBarBounces(t *testing.T) {
	plain := lipgloss.NewStyle()
	var positions []int
	for tick := 0; tick < 40; tick++ {
		bar := indeterminateBar(tick, 20, plain)
		if len([]rune(bar)) != 20 {
			t.Fatalf("tick %d: width %d", tick, len([]rune(bar)))
		}
		if n := strings.Count(bar, "█"); n != 6 {
			t.Fatalf("tick %d: %d filled, want a 6-cell block", tick, n)
		}
		positions = append(positions, strings.Index(bar, "█"))
	}
	// It must both advance and come back.
	sawForward, sawBack := false, false
	for i := 1; i < len(positions); i++ {
		if positions[i] > positions[i-1] {
			sawForward = true
		}
		if positions[i] < positions[i-1] {
			sawBack = true
		}
	}
	if !sawForward || !sawBack {
		t.Errorf("forward=%v back=%v, want a bounce", sawForward, sawBack)
	}
}
