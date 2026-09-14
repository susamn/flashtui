package tui

import (
	"strings"
	"testing"
)

// The view must not panic or overflow at any size the user might have.
func TestViewRendersAtManySizes(t *testing.T) {
	sizes := [][2]int{
		{120, 40}, {100, 30}, {80, 24}, {60, 18}, {200, 60}, {59, 17}, {40, 10},
	}
	for _, s := range sizes {
		m := testModel()
		m.width, m.height = s[0], s[1]
		out := m.View()
		if out == "" {
			t.Errorf("%dx%d rendered nothing", s[0], s[1])
		}
		for _, line := range strings.Split(out, "\n") {
			if w := lineWidth(line); w > s[0] {
				t.Errorf("%dx%d: a line is %d cells wide", s[0], s[1], w)
			}
		}
	}
}

func TestViewTooSmallSaysSo(t *testing.T) {
	m := testModel()
	m.width, m.height = 30, 8
	if out := m.View(); !strings.Contains(out, "needs at least") {
		t.Errorf("a cramped terminal should say so, got %q", out)
	}
}

func TestViewShowsBothLists(t *testing.T) {
	m := testModel()
	out := m.View()
	for _, want := range []string{"IMAGES", "TARGETS", "DRIVE INFO", "a.img", "sda", "nvme0n1"} {
		if !strings.Contains(out, want) {
			t.Errorf("view is missing %q", want)
		}
	}
}

// The system disk has to be recognisable before the user ever selects it.
func TestViewMarksSystemDisk(t *testing.T) {
	m := testModel()
	m.diskList.cursor = 1
	out := m.View()
	if !strings.Contains(out, "SYSTEM DISK") {
		t.Error("the info pane should label the system disk")
	}
}

// The readout is the part that improves on a plain image writer: it has to show
// what is already on the card and whether the image fits.
func TestInfoPaneShowsContentsAndFit(t *testing.T) {
	m := testModel()
	out := m.View()
	for _, want := range []string{"bootfs", "rootfs", "vfat", "ext4", "PARTITIONS", "fits"} {
		if !strings.Contains(out, want) {
			t.Errorf("info pane is missing %q", want)
		}
	}
}

func TestInfoPaneFlagsAnImageThatDoesNotFit(t *testing.T) {
	m := testModel()
	m.images[0].Expanded = 1 << 60
	if out := m.View(); !strings.Contains(out, "DOES NOT FIT") {
		t.Error("an oversized image should be called out in the readout")
	}
}

func TestModalsRender(t *testing.T) {
	cases := []struct {
		name string
		keys []string
		want string
	}{
		{"guard", []string{"f"}, "OVERWRITE THIS DEVICE?"},
		{"help", []string{"?"}, "KEYS"},
		{"dir", []string{"o"}, "IMAGE DIRECTORY"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := press(testModel(), c.keys...)
			if out := m.View(); !strings.Contains(out, c.want) {
				t.Errorf("modal missing %q", c.want)
			}
		})
	}
}

// lineWidth counts printable cells, ignoring the ANSI colour sequences lipgloss
// emits.
func lineWidth(s string) int {
	n, inEsc := 0, false
	for _, r := range s {
		switch {
		case r == 0x1b:
			inEsc = true
		case inEsc && (r == 'm' || r == 'K' || r == 'H'):
			inEsc = false
		case inEsc:
		default:
			n++
		}
	}
	return n
}
