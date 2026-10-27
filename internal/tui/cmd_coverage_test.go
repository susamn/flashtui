package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/susamn/flashtui/internal/blockdev"
	"github.com/susamn/flashtui/internal/flash"
	"github.com/susamn/flashtui/internal/imagefile"
	"github.com/susamn/flashtui/internal/privilege"
	"github.com/susamn/flashtui/internal/seed"
)

func fakeCommand(t *testing.T, name, contents string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+contents+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
}

func TestCommandResultsWithoutDeviceWrites(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.img"), []byte("image"), 0600); err != nil {
		t.Fatal(err)
	}
	msg := scanImages(dir)().(imagesMsg)
	if msg.err != nil || len(msg.images) != 1 {
		t.Fatal(msg)
	}
	// A list result (including a failed lsblk call) must be delivered as a message.
	if _, ok := listDisks()().(disksMsg); !ok {
		t.Fatal("missing disks message")
	}
	disk := blockdev.Disk{Path: "/dev/fake", Partitions: []blockdev.Partition{{Path: "/dev/fake1"}}}
	if got := partitionPaths(disk); len(got) != 1 || got[0] != "/dev/fake1" {
		t.Fatal(got)
	}
	fakeCommand(t, "udisksctl", `case "$1" in
 mount) echo 'Mounted /dev/fake1 at /run/media/test/fake.' ;;
 unmount) echo 'Unmounted /dev/fake1.' ;;
 power-off) echo 'Powered off.' ;;
 esac`)
	for _, tc := range []struct {
		name string
		cmd  func(blockdev.Disk) any
	}{
		{"mount", func(d blockdev.Disk) any { return mountDisk(d)() }},
		{"unmount", func(d blockdev.Disk) any { return unmountDisk(d)() }},
		{"power off", func(d blockdev.Disk) any { return powerOffDisk(d)() }},
	} {
		got := tc.cmd(disk).(mountMsg)
		if got.err != nil || got.action != tc.name {
			t.Fatalf("%s: %+v", tc.name, got)
		}
	}
	disk.Partitions = nil
	if got := mountDisk(disk)().(mountMsg); got.err != errNothingMounted {
		t.Fatal(got)
	}
	fakeCommand(t, "udisksctl", "echo 'busy' >&2; exit 1")
	disk.Partitions = []blockdev.Partition{{Path: "/dev/fake1"}}
	if got := powerOffDisk(disk)().(mountMsg); got.err == nil {
		t.Fatal("power off after failed unmount")
	}
	if got := unmountDisk(disk)().(mountMsg); got.err == nil {
		t.Fatal("unmount failure hidden")
	}
}

func TestSuccessfulFixtureFlashAndErrorCommands(t *testing.T) {
	fakeCommand(t, "udisksctl", "echo 'Unmounted.'")
	fakeCommand(t, "partprobe", "exit 0")
	dir := t.TempDir()
	source := filepath.Join(dir, "source.img")
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(source, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	img, err := imagefile.Inspect(source)
	if err != nil {
		t.Fatal(err)
	}
	disk := blockdev.Disk{Path: target, Size: 4096}
	run, ch, cancel := startFlash(privilege.Direct(), flash.Job{Image: img, Target: target, ScratchDir: dir}, disk, newTerminal())
	defer cancel()
	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()
	if got := run().(flashDoneMsg); got.err != nil {
		t.Fatal(got.err)
	}
	<-done
	if b, err := os.ReadFile(target); err != nil || string(b) != "fixture" {
		t.Fatal(string(b), err)
	}
	m := testModel()
	m.esc = privilege.Direct()
	m.scratch = dir
	m.guard = newGuard(disk, img)
	next, cmd := m.runFlash()
	if next.(Model).mode != modeFlashing || cmd == nil {
		t.Fatal("flash did not start")
	}
}

func TestDetectAndSeedWithSyntheticDisk(t *testing.T) {
	old := enumerateDisks
	t.Cleanup(func() { enumerateDisks = old })
	fakeCommand(t, "udevadm", "exit 0")
	enumerateDisks = func() ([]blockdev.Disk, error) { return nil, errors.New("lsblk failed") }
	if got := detectAfterFlash("/dev/fake")().(detectMsg); got.err == nil {
		t.Fatal("lsblk failure ignored")
	}
	enumerateDisks = func() ([]blockdev.Disk, error) {
		return []blockdev.Disk{{Path: "/dev/fake", Partitions: []blockdev.Partition{{Path: "/dev/fake1"}}}}, nil
	}
	root := t.TempDir()
	for _, d := range []string{"etc", "home", "var"} {
		if err := os.Mkdir(filepath.Join(root, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	fakeCommand(t, "udisksctl", fmt.Sprintf(`case "$1" in
 mount) echo 'Mounted /dev/fake1 at %s.' ;;
 unmount) echo 'Unmounted.' ;;
 esac`, root))
	got := detectAfterFlash("/dev/fake")().(detectMsg)
	if got.err != nil || got.detection.Family != seed.CloudInit {
		t.Fatal(got)
	}
	cfg := seed.Config{Username: "ubuntu", AuthorizedKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIexample"}
	result := applySeed(privilege.Direct(), got.detection, cfg, t.TempDir(), newTerminal())().(seedDoneMsg)
	if result.err != nil {
		t.Fatal(result.err)
	}
	if _, err := os.Stat(filepath.Join(root, "var", "lib", "cloud", "seed", "nocloud-net", "user-data")); err != nil {
		t.Fatal(err)
	}
}

func TestDetectAndSeedCommandFailures(t *testing.T) {
	fakeCommand(t, "udevadm", "exit 0")
	got := detectAfterFlash("/dev/nonexistent-flashtui-test")().(detectMsg)
	if got.err == nil {
		t.Fatal("removed target detected")
	}
	fakeCommand(t, "udisksctl", "echo 'mount failed' >&2; exit 1")
	result := applySeed(privilege.Direct(), seed.Detection{Family: seed.CloudInit, Root: "/dev/nonexistent-flashtui-test"}, seed.Config{Username: "pi", Password: "secret"}, t.TempDir(), newTerminal())().(seedDoneMsg)
	if result.err == nil {
		t.Fatal("missing root mounted")
	}
}

func TestFlashPreUnmountFailureAndProgressChannel(t *testing.T) {
	fakeCommand(t, "udisksctl", "echo 'busy'; exit 1")
	disk := blockdev.Disk{Path: "/dev/fake", Partitions: []blockdev.Partition{{Path: "/dev/fake1"}}}
	job := flash.Job{Image: imagefile.Image{Path: "/missing/image.img", Kind: imagefile.Plain}, Target: disk.Path}
	run, ch, cancel := startFlash(nil, job, disk, newTerminal())
	defer cancel()
	result := run().(flashDoneMsg)
	if result.err == nil || !strings.Contains(result.err.Error(), "unmount") {
		t.Fatal(result.err)
	}
	// The failed operation closes its progress stream; an awaiting command must terminate.
	if msg := awaitProgress(ch)(); msg != nil {
		t.Fatal(msg)
	}
	samples := make(chan flash.Progress, 1)
	samples <- flash.Progress{Bytes: 42}
	if got := awaitProgress(samples)().(progressMsg); got.Bytes != 42 {
		t.Fatal(got)
	}
	close(samples)
	if got := awaitProgress(samples)(); got != nil {
		t.Fatal(got)
	}
	if animate() == nil {
		t.Fatal("nil animation")
	}
	if _, ok := animate()().(tickMsg); !ok {
		t.Fatal("animation did not tick")
	}
}
