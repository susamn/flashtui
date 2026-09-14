package blockdev

import (
	"errors"
	"os"
	"testing"
)

func useFixture(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	orig := runner
	runner = func(string, ...string) ([]byte, error) { return data, nil }
	t.Cleanup(func() { runner = orig })
}

func find(t *testing.T, disks []Disk, name string) Disk {
	t.Helper()
	for _, d := range disks {
		if d.Name == name {
			return d
		}
	}
	t.Fatalf("disk %q not in %v", name, disks)
	return Disk{}
}

func TestListSkipsPseudoDevices(t *testing.T) {
	useFixture(t, "testdata/lsblk.json")
	disks, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(disks) != 2 {
		t.Fatalf("want 2 disks (zram dropped), got %d", len(disks))
	}
	for _, d := range disks {
		if d.Name == "zram0" {
			t.Error("zram0 should be filtered out")
		}
	}
}

func TestUSBDiskIsNotSystem(t *testing.T) {
	useFixture(t, "testdata/lsblk.json")
	disks, _ := List()
	sda := find(t, disks, "sda")

	if sda.System {
		t.Error("removable usb disk must not be flagged System")
	}
	if !sda.Removable || sda.Transport != "usb" {
		t.Errorf("removable=%v tran=%q", sda.Removable, sda.Transport)
	}
	if got := len(sda.Partitions); got != 2 {
		t.Fatalf("want 2 partitions, got %d", got)
	}
	if got := sda.MountedCount(); got != 2 {
		t.Errorf("want 2 mounted, got %d", got)
	}
	if got := sda.Partitions[0].Label; got != "bootfs" {
		t.Errorf("label = %q", got)
	}
	if got := sda.Description(); got != "STORAGE DEVICE" {
		t.Errorf("description = %q", got)
	}
}

// The system disk is the one the guard exists for: /boot lives on p1 and /home
// on a mapper under p2, so detection has to walk past a crypt node.
func TestSystemDiskDetectedThroughCryptChild(t *testing.T) {
	useFixture(t, "testdata/lsblk.json")
	disks, _ := List()
	nvme := find(t, disks, "nvme0n1")

	if !nvme.System {
		t.Fatal("disk holding /boot and /home must be flagged System")
	}
	// The LUKS partition must display the mapper's ext4 + mountpoint, not
	// a bare crypto_LUKS row.
	var p2 Partition
	for _, p := range nvme.Partitions {
		if p.Name == "nvme0n1p2" {
			p2 = p
		}
	}
	if p2.FSType != "ext4" || p2.MountPoint != "/home" {
		t.Errorf("crypt child not surfaced: fstype=%q mount=%q", p2.FSType, p2.MountPoint)
	}
}

func TestListPropagatesCommandError(t *testing.T) {
	orig := runner
	runner = func(string, ...string) ([]byte, error) { return nil, errors.New("boom") }
	t.Cleanup(func() { runner = orig })

	if _, err := List(); err == nil {
		t.Fatal("want error when lsblk fails")
	}
}

func TestListRejectsGarbage(t *testing.T) {
	orig := runner
	runner = func(string, ...string) ([]byte, error) { return []byte("not json"), nil }
	t.Cleanup(func() { runner = orig })

	if _, err := List(); err == nil {
		t.Fatal("want error on unparseable output")
	}
}
