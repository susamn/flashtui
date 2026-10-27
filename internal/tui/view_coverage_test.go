package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/susamn/flashtui/internal/blockdev"
	"github.com/susamn/flashtui/internal/flash"
	"github.com/susamn/flashtui/internal/imagefile"
)

func TestNarrowViewsAndScrollBounds(t *testing.T) {
	m := testModel()
	m.width = 60
	m.height = 18
	for _, mode := range []mode{modeBrowse, modeGuard, modeSeed, modeHelp, modeDirInput, modeFlashing, modeReport} {
		m.mode = mode
		if mode == modeSeed {
			m.form = newSeedForm()
		}
		if out := m.View(); out == "" {
			t.Fatal(mode)
		}
	}
	l := listState{cursor: 12, offset: 12}
	l.clamp(15, 0)
	if l.offset != 0 {
		t.Fatal(l)
	}
	l = listState{cursor: 0, offset: 14}
	l.clamp(15, 3)
	if l.offset != 0 {
		t.Fatal(l)
	}
	m = testModel()
	m.width = 100
	m.height = 28
	m.imgList.offset = 10
	_ = m.imagesRows()
	_ = m.targetRows()
}

func TestWritePaneStates(t *testing.T) {
	m := testModel()
	cases := []struct {
		name  string
		setup func(*Model)
		want  string
	}{
		{"unknown", func(m *Model) { m.flashing = true; m.progress = flash.Progress{Phase: flash.PhaseWriting, Bytes: 10} }, "written"},
		{"verify", func(m *Model) {
			m.flashing = true
			m.progress = flash.Progress{Phase: flash.PhaseVerifying, Bytes: 10, Total: 20, Rate: 10, Elapsed: time.Second}
		}, "eta"},
		{"done", func(m *Model) { m.lastOK = "write complete" }, "write complete"},
		{"failed", func(m *Model) { m.lastErr = errors.New("disk removed") }, "disk removed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n := m
			c.setup(&n)
			if got := n.writePane(80); !strings.Contains(got, c.want) {
				t.Fatalf("missing %q in %q", c.want, got)
			}
		})
	}
}

func TestInfoPaneEdgeCases(t *testing.T) {
	m := testModel()
	m.disks = nil
	if got := m.infoPane(60, 12); !strings.Contains(got, "select a device") {
		t.Fatal(got)
	}
	m = testModel()
	m.focus = paneInfo
	m.disks[0].Vendor = "Acme"
	m.disks[0].ReadOnly = true
	m.disks[0].Partitions[0].FSUsed = 99
	m.disks[0].Partitions[0].FSAvail = 1
	if got := m.infoPane(60, 9); !strings.Contains(got, "sda") {
		t.Fatal(got)
	}
	if got := strings.Join(m.diskSummary(m.disks[0], 60), " "); !strings.Contains(got, "read-only") {
		t.Fatal(got)
	}
	m.disks[0].Partitions[0].FSUsed = 85
	m.disks[0].Partitions[0].FSAvail = 15
	_ = m.partitionLines(m.disks[0], 45)
	m.disks[0].Partitions = nil
	if got := strings.Join(m.partitionLines(m.disks[0], 60), " "); !strings.Contains(got, "no partition") {
		t.Fatal(got)
	}
	m.disks[0].Size = 0
	if got := m.fitLines(m.disks[0], 60); len(got) < 2 {
		t.Fatal(got)
	}
	m.images[0] = imagefile.Image{Name: "unknown.img.bz2", Kind: imagefile.Bzip2, Size: 100}
	if got := strings.Join(m.fitLines(m.disks[0], 60), " "); !strings.Contains(got, "unknown") {
		t.Fatal(got)
	}
	m.images = nil
	if got := m.fitLines(m.disks[0], 60); got != nil {
		t.Fatal(got)
	}
	if upperOrDash("") != "-" || upperOrDash("usb") != "USB" {
		t.Fatal("transport formatting")
	}
	m.disks = []blockdev.Disk{{Path: "/dev/sdz", Size: 200, PTType: ""}}
	if got := strings.Join(m.diskSummary(m.disks[0], 60), " "); !strings.Contains(got, "none") {
		t.Fatal(got)
	}
	for _, lev := range []level{levelInfo, levelOK, levelWarn, levelError} {
		m.statusLevel = lev
		_ = m.statusLine()
	}
}

func TestDefaultImageDirAndExpandHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	home := defaultImageDir()
	if home == "" {
		t.Fatal(home)
	}
	if got := expandHome("~/images"); !strings.HasPrefix(got, home) {
		t.Fatal(got)
	}
}
