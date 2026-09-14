package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// typeText feeds characters one at a time, the way the user would.
func typeText(m Model, s string) Model {
	for _, r := range s {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	return m
}

func TestFlashOpensGuardNotAWrite(t *testing.T) {
	m := testModel()
	m = press(m, "f")
	if m.mode != modeGuard {
		t.Fatalf("mode=%v, f must open the guard rather than write", m.mode)
	}
	if m.flashing {
		t.Fatal("nothing may be written before the guard is satisfied")
	}
}

func TestFlashNeedsBothSelections(t *testing.T) {
	m := testModel()
	m.images = nil
	m = press(m, "f")
	if m.mode == modeGuard {
		t.Error("guard opened with no image selected")
	}

	m = testModel()
	m.disks = nil
	m = press(m, "f")
	if m.mode == modeGuard {
		t.Error("guard opened with no target selected")
	}
}

// The whole point of the guard: a wrong name must not write.
func TestGuardRejectsWrongName(t *testing.T) {
	m := testModel()
	m = press(m, "f")
	m = typeText(m, "sdb")
	if m.guard.confirmed() {
		t.Fatal("typing the wrong device name must not confirm")
	}
	m = press(m, "enter")
	if m.flashing {
		t.Fatal("a wrong name started a write")
	}
	if m.mode != modeGuard {
		t.Errorf("mode=%v, the guard should stay open", m.mode)
	}
	if !strings.Contains(m.status, "sda") {
		t.Errorf("status should say what was expected: %q", m.status)
	}
}

func TestGuardAcceptsExactName(t *testing.T) {
	m := testModel()
	m = press(m, "f")
	m = typeText(m, "sda")
	if !m.guard.confirmed() {
		t.Fatal("the exact device name should confirm")
	}
}

// Case-insensitive matching would weaken the check for no benefit: /dev names
// are lowercase.
func TestGuardIsCaseSensitive(t *testing.T) {
	m := testModel()
	m = press(m, "f")
	m = typeText(m, "SDA")
	if m.guard.confirmed() {
		t.Fatal("uppercase must not confirm")
	}
}

// A path, rather than a bare name, is a plausible mistype and must not pass.
func TestGuardRejectsFullPath(t *testing.T) {
	m := testModel()
	m = press(m, "f")
	m = typeText(m, "/dev/sda")
	if m.guard.confirmed() {
		t.Fatal("the full path must not confirm; the bare name is required")
	}
}

// The system disk cannot be confirmed at all, no matter what is typed.
func TestGuardBlocksSystemDisk(t *testing.T) {
	m := testModel()
	m.diskList.cursor = 1 // nvme0n1, flagged System
	m = press(m, "f")
	if m.guard.blocked == "" {
		t.Fatal("the system disk must be blocked outright")
	}
	m = typeText(m, "nvme0n1")
	if m.guard.confirmed() {
		t.Fatal("a blocked target must never confirm")
	}
	m = press(m, "enter")
	if m.flashing {
		t.Fatal("enter started a write on the system disk")
	}
}

func TestGuardBlocksReadOnlyDevice(t *testing.T) {
	m := testModel()
	m.disks[0].ReadOnly = true
	m = press(m, "f")
	if !strings.Contains(m.guard.blocked, "read-only") {
		t.Fatalf("blocked=%q, want it to name the write-protect switch", m.guard.blocked)
	}
}

func TestGuardEscapeCancels(t *testing.T) {
	m := testModel()
	m = press(m, "f", "esc")
	if m.mode != modeBrowse {
		t.Errorf("mode=%v after esc", m.mode)
	}
	if m.flashing {
		t.Fatal("esc started a write")
	}
}

func TestGuardWarnings(t *testing.T) {
	m := testModel()
	m = press(m, "f")
	ws := strings.Join(m.guard.warnings(), "\n")

	if !strings.Contains(ws, "mounted") {
		t.Errorf("mounted partitions should be warned about: %q", ws)
	}
	if !strings.Contains(ws, "destroyed") {
		t.Errorf("existing partitions should be warned about: %q", ws)
	}
}

// An internal, non-removable drive that is not the system disk is still worth
// shouting about.
func TestGuardWarnsOnNonRemovable(t *testing.T) {
	m := testModel()
	m.disks[0].Removable = false
	m.disks[0].Hotplug = false
	m.disks[0].Transport = "sata"
	m = press(m, "f")
	ws := strings.Join(m.guard.warnings(), "\n")
	if !strings.Contains(ws, "NOT removable") {
		t.Errorf("want a non-removable warning, got %q", ws)
	}
	if !strings.Contains(ws, "SATA") {
		t.Errorf("want the transport named, got %q", ws)
	}
}

func TestGuardWarnsWhenImageDoesNotFit(t *testing.T) {
	m := testModel()
	m.images[0].Expanded = 1 << 60
	m = press(m, "f")
	ws := strings.Join(m.guard.warnings(), "\n")
	if !strings.Contains(ws, "device holds") {
		t.Errorf("an oversized image should be warned about: %q", ws)
	}
}

// Powering off the system disk would take the machine down.
func TestPowerOffRefusesSystemDisk(t *testing.T) {
	m := testModel()
	m.diskList.cursor = 1
	next, cmd := m.Update(key("p"))
	m = next.(Model)
	if cmd != nil {
		t.Fatal("no command should be issued for the system disk")
	}
	if !strings.Contains(m.status, "refusing") {
		t.Errorf("status=%q", m.status)
	}
}
