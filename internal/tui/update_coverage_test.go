package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/susamn/flashtui/internal/blockdev"
	"github.com/susamn/flashtui/internal/flash"
	"github.com/susamn/flashtui/internal/imagefile"
	"github.com/susamn/flashtui/internal/seed"
)

func TestAsyncResults(t *testing.T) {
	m := testModel()
	m = m.onImages(imagesMsg{dir: "/new", images: []imagefile.Image{{Name: "new.img"}}})
	if m.imageDir != "/new" || len(m.images) != 1 {
		t.Fatal(m.imageDir, m.images)
	}
	m = m.onImages(imagesMsg{dir: "/missing", err: errors.New("missing")})
	if len(m.images) != 0 || m.statusLevel != levelError {
		t.Fatal(m.status)
	}
	m = m.onImages(imagesMsg{dir: "/empty"})
	if m.statusLevel != levelWarn {
		t.Fatal(m.status)
	}
	m = testModel()
	m.diskList.cursor = 1
	m = m.onDisks(disksMsg{disks: []blockdev.Disk{m.disks[1], m.disks[0]}})
	if m.diskList.cursor != 0 {
		t.Fatal("selected disk lost")
	}
	m = m.onDisks(disksMsg{err: errors.New("lsblk")})
	if m.statusLevel != levelError {
		t.Fatal(m.status)
	}
	m.progressCh = make(chan flash.Progress, 1)
	next, cmd := m.onProgress(progressMsg(flash.Progress{Phase: flash.PhaseWriting, Bytes: 123}))
	m = next.(Model)
	if cmd == nil || m.progress.Bytes != 123 {
		t.Fatal(m.progress)
	}
	m.progressCh = nil
	_, cmd = m.onProgress(progressMsg(flash.Progress{Bytes: 200}))
	if cmd != nil {
		t.Fatal("unexpected listener")
	}
	m.seedAfter = false
	next, cmd = m.onFlashDone(flashDoneMsg{target: "/dev/sda"})
	m = next.(Model)
	if m.mode != modeReport || cmd == nil || m.lastOK == "" {
		t.Fatal(m)
	}
	m.seedAfter = true
	next, cmd = m.onFlashDone(flashDoneMsg{target: "/dev/sda"})
	m = next.(Model)
	if cmd == nil || !strings.Contains(m.status, "inspecting") {
		t.Fatal(m.status)
	}
	next, cmd = m.onFlashDone(flashDoneMsg{target: "/dev/sda", err: errors.New("failed")})
	m = next.(Model)
	if cmd == nil || m.lastErr == nil || m.statusLevel != levelError {
		t.Fatal(m.status)
	}
	m = m.onDetect(detectMsg{err: errors.New("no partitions")})
	if m.statusLevel != levelWarn {
		t.Fatal(m.status)
	}
	m = m.onDetect(detectMsg{detection: seed.Detection{Reason: "no hook"}})
	if m.statusLevel != levelWarn {
		t.Fatal(m.status)
	}
	m = m.onDetect(detectMsg{detection: seed.Detection{Family: seed.CloudInit}})
	if m.mode != modeSeed || m.detection.Family != seed.CloudInit {
		t.Fatal(m.mode)
	}
	m = m.onSeedDone(seedDoneMsg{err: errors.New("seed failed")})
	if m.mode != modeReport || m.lastErr == nil {
		t.Fatal(m.status)
	}
	m = m.onSeedDone(seedDoneMsg{})
	if m.lastErr != nil || m.statusLevel != levelOK {
		t.Fatal(m.status)
	}
	m = m.onMount(mountMsg{action: "mount", err: errors.New("failed")})
	if m.statusLevel != levelError {
		t.Fatal(m.status)
	}
	m = m.onMount(mountMsg{action: "mount", detail: "done"})
	if m.statusLevel != levelOK {
		t.Fatal(m.status)
	}
}

func TestUpdateMessageRouting(t *testing.T) {
	cases := []tea.Msg{
		tea.WindowSizeMsg{Width: 90, Height: 24},
		imagesMsg{dir: "/empty"}, disksMsg{disks: []blockdev.Disk{}},
		progressMsg(flash.Progress{Bytes: 1}), flashDoneMsg{err: errors.New("failed")},
		detectMsg{detection: seed.Detection{Reason: "unsupported"}},
		seedDoneMsg{}, mountMsg{action: "mount"}, tickMsg{}, key("?"), struct{}{},
	}
	for _, msg := range cases {
		m := testModel()
		next, _ := m.Update(msg)
		if next == nil {
			t.Fatalf("no model for %T", msg)
		}
	}
	m := testModel()
	m.flashing = true
	next, cmd := m.Update(tickMsg{})
	if next.(Model).tick != 1 || cmd == nil {
		t.Fatal("animation stopped")
	}
	m = testModel()
	m.progressCh = make(chan flash.Progress, 1)
	_, cmd = m.Update(progressMsg(flash.Progress{Bytes: 1}))
	if cmd == nil {
		t.Fatal("progress listener stopped")
	}
}

func TestModalKeyHandling(t *testing.T) {
	m := testModel()
	m = press(m, "?", "x", "o")
	if m.mode != modeDirInput {
		t.Fatal(m.mode)
	}
	m = press(m, "esc", "f")
	if m.mode != modeGuard {
		t.Fatal(m.mode)
	}
	m = press(m, "esc", "f")
	m.guard.input.SetValue("wrong")
	m = press(m, "enter")
	if m.mode != modeGuard || m.statusLevel != levelError {
		t.Fatal(m.status)
	}
	m = press(m, "esc")
	m.mode = modeFlashing
	m = press(m, "j", "q")
	if m.mode != modeFlashing || !strings.Contains(m.status, "configuration") {
		t.Fatal(m.status)
	}
	m.mode = modeReport
	m = press(m, "q")
	if m.mode != modeBrowse {
		t.Fatal(m.mode)
	}
	m = press(m, "o")
	m.dirInput.SetValue("~/Downloads")
	next, cmd := m.Update(key("enter"))
	if next.(Model).mode != modeBrowse || cmd == nil {
		t.Fatal("directory scan not started")
	}
}

func TestMoreBrowseAndModalBranches(t *testing.T) {
	m := testModel()
	m.focus = paneTargets
	m = press(m, "ctrl+d", "ctrl+u", "G", "g")
	if m.diskList.cursor != 0 {
		t.Fatal(m.diskList.cursor)
	}
	m = press(m, "m", "u", "p")
	m = press(m, "o")
	m = press(m, "x", "esc")
	if m.mode != modeBrowse {
		t.Fatal(m.mode)
	}
	m = press(m, "f")
	m.guard.input.SetValue("sda")
	next, cmd := m.Update(key("enter"))
	if next.(Model).mode != modeFlashing || cmd == nil {
		t.Fatal("confirmed flash not scheduled")
	}
	// A blocked guard ignores the typed confirmation.
	m = testModel()
	m.diskList.cursor = 1
	m = press(m, "f")
	if got := press(m, "enter"); got.mode != modeGuard {
		t.Fatal(got.mode)
	}
	m = testModel()
	m.mode = modeSeed
	m.form = newSeedForm()
	m.form.inputs[fieldUser].SetValue("pi")
	m.form.inputs[fieldPassword].SetValue("pw")
	m = press(m, "ctrl+w", "ctrl+k", "tab")
	if m.mode != modeSeed {
		t.Fatal(m.mode)
	}
	m.width = 32
	m.height = 8
	if out := m.View(); !strings.Contains(out, "needs") {
		t.Fatal(out)
	}
}

func TestBrowseActionsAndCursor(t *testing.T) {
	m := testModel()
	for _, k := range []string{"1", "2", "3", "tab", "shift+tab", "h", "l", "g", "G", "ctrl+d", "ctrl+u", "j", "k", "r", "m", "u", "p"} {
		next, _ := m.Update(key(k))
		m = next.(Model)
	}
	m = testModel()
	m.diskList.cursor = 1
	next, cmd := m.Update(key("p"))
	if cmd != nil || next.(Model).statusLevel != levelError {
		t.Fatal("system disk powered off")
	}
	m = testModel()
	m.images = nil
	if press(m, "f").mode != modeBrowse {
		t.Fatal("flashed without image")
	}
	m = testModel()
	m.disks = nil
	if press(m, "f").mode != modeBrowse {
		t.Fatal("flashed without disk")
	}
	m = testModel()
	m.mode = modeSeed
	m.form = newSeedForm()
	for _, k := range []string{"tab", "shift+tab", "ctrl+w", "down", "up", "ctrl+k", "esc"} {
		m = press(m, k)
	}
	if m.mode != modeReport {
		t.Fatal(m.mode)
	}
	m = testModel()
	m.mode = modeSeed
	m.form = newSeedForm()
	m = press(m, "enter")
	if m.form.err == "" {
		t.Fatal("invalid form submitted")
	}
}
