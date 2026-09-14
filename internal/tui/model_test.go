package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/susamn/flashtui/internal/blockdev"
	"github.com/susamn/flashtui/internal/imagefile"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "ctrl+d":
		return tea.KeyMsg{Type: tea.KeyCtrlD}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// press feeds keys in order and returns the resulting model.
func press(m Model, keys ...string) Model {
	for _, k := range keys {
		next, _ := m.Update(key(k))
		m = next.(Model)
	}
	return m
}

func testModel() Model {
	m := New(nil, "/tmp/images", "/tmp/scratch")
	m.width, m.height = 120, 40
	m.images = []imagefile.Image{
		{Name: "a.img", Path: "/tmp/images/a.img", Size: 100, Expanded: 100},
		{Name: "b.img.xz", Path: "/tmp/images/b.img.xz", Size: 50, Kind: imagefile.Xz, Expanded: 500},
		{Name: "c.iso", Path: "/tmp/images/c.iso", Size: 700, Expanded: 700},
	}
	m.disks = []blockdev.Disk{
		{
			Name: "sda", Path: "/dev/sda", Size: 31266996224, Model: "STORAGE DEVICE",
			Transport: "usb", Removable: true, PTType: "dos",
			Partitions: []blockdev.Partition{
				{Name: "sda1", Path: "/dev/sda1", Size: 536870912, FSType: "vfat", Label: "bootfs",
					MountPoint: "/run/media/susamn/bootfs", FSUsed: 68157440, FSAvail: 460519424},
				{Name: "sda2", Path: "/dev/sda2", Size: 30712791040, FSType: "ext4", Label: "rootfs"},
			},
		},
		{
			Name: "nvme0n1", Path: "/dev/nvme0n1", Size: 2000398934016, Model: "CT2000P5PSSD8",
			Transport: "nvme", System: true, PTType: "gpt",
			Partitions: []blockdev.Partition{{Name: "nvme0n1p1", Path: "/dev/nvme0n1p1", Size: 1 << 31, FSType: "vfat", MountPoint: "/boot"}},
		},
	}
	return m
}

func TestListStateClamp(t *testing.T) {
	var l listState

	l.cursor = 99
	l.clamp(3, 10)
	if l.cursor != 2 {
		t.Errorf("cursor=%d want clamped to 2", l.cursor)
	}

	l.cursor = -5
	l.clamp(3, 10)
	if l.cursor != 0 {
		t.Errorf("cursor=%d want clamped to 0", l.cursor)
	}

	// An empty list resets both, so a stale offset cannot survive a refresh
	// that removed every row.
	l.cursor, l.offset = 4, 4
	l.clamp(0, 10)
	if l.cursor != 0 || l.offset != 0 {
		t.Errorf("cursor=%d offset=%d want 0/0 for an empty list", l.cursor, l.offset)
	}
}

// The window has to follow the cursor in both directions or rows scroll out of
// sight.
func TestListStateScrolls(t *testing.T) {
	var l listState
	l.cursor = 12
	l.clamp(20, 5)
	if l.cursor < l.offset || l.cursor >= l.offset+5 {
		t.Fatalf("cursor %d outside window [%d,%d)", l.cursor, l.offset, l.offset+5)
	}

	l.cursor = 1
	l.clamp(20, 5)
	if l.offset > l.cursor {
		t.Fatalf("offset %d did not follow cursor back up to %d", l.offset, l.cursor)
	}

	// The window never scrolls past the end of the list.
	l.cursor = 19
	l.clamp(20, 5)
	if l.offset+5 > 20 {
		t.Fatalf("offset %d shows rows past the end", l.offset)
	}
}

func TestVimMotions(t *testing.T) {
	m := testModel()
	m.focus = paneImages

	m = press(m, "j")
	if m.imgList.cursor != 1 {
		t.Errorf("j: cursor=%d want 1", m.imgList.cursor)
	}
	m = press(m, "k")
	if m.imgList.cursor != 0 {
		t.Errorf("k: cursor=%d want 0", m.imgList.cursor)
	}
	m = press(m, "G")
	if m.imgList.cursor != len(m.images)-1 {
		t.Errorf("G: cursor=%d want last", m.imgList.cursor)
	}
	m = press(m, "g")
	if m.imgList.cursor != 0 {
		t.Errorf("g: cursor=%d want first", m.imgList.cursor)
	}
	// Moving up from the top must stay put rather than wrap.
	m = press(m, "k", "k")
	if m.imgList.cursor != 0 {
		t.Errorf("cursor=%d, want no wrap past the top", m.imgList.cursor)
	}
}

func TestPaneNavigation(t *testing.T) {
	m := testModel()
	if m.focus != paneImages {
		t.Fatalf("focus starts at %v", m.focus)
	}
	m = press(m, "tab")
	if m.focus != paneTargets {
		t.Errorf("tab: focus=%v", m.focus)
	}
	m = press(m, "shift+tab")
	if m.focus != paneImages {
		t.Errorf("shift+tab: focus=%v", m.focus)
	}
	// h at the leftmost pane stays put rather than wrapping round.
	m = press(m, "h")
	if m.focus != paneImages {
		t.Errorf("h at edge: focus=%v", m.focus)
	}
	m = press(m, "3")
	if m.focus != paneInfo {
		t.Errorf("3: focus=%v", m.focus)
	}
	m = press(m, "l")
	if m.focus != paneInfo {
		t.Errorf("l at edge: focus=%v", m.focus)
	}
}

// Moving the cursor in the info pane must do nothing: it is a readout.
func TestInfoPaneHasNoCursor(t *testing.T) {
	m := testModel()
	m.focus = paneInfo
	before := m.imgList.cursor
	m = press(m, "j", "j", "G")
	if m.imgList.cursor != before {
		t.Errorf("image cursor moved to %d while the info pane had focus", m.imgList.cursor)
	}
}

func TestToggles(t *testing.T) {
	m := testModel()
	if !m.verify {
		t.Error("verification should default to on")
	}
	m = press(m, "v")
	if m.verify {
		t.Error("v did not toggle verify off")
	}
	if m.seedAfter {
		t.Error("headless setup should default to off")
	}
	m = press(m, "s")
	if !m.seedAfter {
		t.Error("s did not toggle seeding on")
	}
}
