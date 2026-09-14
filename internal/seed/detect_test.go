package seed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/susamn/flashtui/internal/blockdev"
)

func fakeDisk(parts ...blockdev.Partition) blockdev.Disk {
	return blockdev.Disk{Name: "sda", Path: "/dev/sda", Partitions: parts}
}

func mounter(m map[string]string) func(string) (string, func(), error) {
	return func(part string) (string, func(), error) {
		mp, ok := m[part]
		if !ok {
			return "", nil, os.ErrNotExist
		}
		return mp, func() {}, nil
	}
}

func makeRaspiBoot(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	for _, f := range []string{"config.txt", "cmdline.txt"} {
		if err := os.WriteFile(filepath.Join(d, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

func makeLinuxRoot(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	for _, sub := range []string{"etc", "home", "var"} {
		if err := os.MkdirAll(filepath.Join(d, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A stock Raspberry Pi OS passwd: uid 1000 exists but cannot log in.
	passwd := "root:x:0:0::/root:/bin/bash\npi:x:1000:1000::/home/pi:/usr/sbin/nologin\n"
	if err := os.WriteFile(filepath.Join(d, "etc", "passwd"), []byte(passwd), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "etc", "hosts"), []byte("127.0.0.1\tlocalhost\n127.0.1.1\traspberrypi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDetectRaspberryPiOS(t *testing.T) {
	boot, root := makeRaspiBoot(t), makeLinuxRoot(t)
	disk := fakeDisk(
		blockdev.Partition{Path: "/dev/sda1", FSType: "vfat"},
		blockdev.Partition{Path: "/dev/sda2", FSType: "ext4"},
	)
	got := Detect(disk, mounter(map[string]string{"/dev/sda1": boot, "/dev/sda2": root}))
	if got.Family != RaspberryPiOS {
		t.Fatalf("family=%v reason=%q", got.Family, got.Reason)
	}
	if got.Boot != "/dev/sda1" || got.Root != "/dev/sda2" {
		t.Errorf("boot=%q root=%q", got.Boot, got.Root)
	}
}

func TestDetectCloudInit(t *testing.T) {
	root := makeLinuxRoot(t)
	disk := fakeDisk(blockdev.Partition{Path: "/dev/sda1", FSType: "ext4"})
	got := Detect(disk, mounter(map[string]string{"/dev/sda1": root}))
	if got.Family != CloudInit || got.Root != "/dev/sda1" {
		t.Fatalf("family=%v root=%q", got.Family, got.Root)
	}
	if got.Boot != "" {
		t.Errorf("cloud images have no seedable boot partition, got %q", got.Boot)
	}
}

// The case the user asked about: an Arch or Ubuntu live ISO cannot be seeded,
func TestDetectLiveISOIsUnseedable(t *testing.T) {
	disk := fakeDisk(blockdev.Partition{Path: "/dev/sda1", FSType: "iso9660"})
	got := Detect(disk, mounter(nil))
	if got.Seedable() {
		t.Fatal("iso9660 must not be reported as seedable")
	}
	if !strings.Contains(got.Reason, "squashfs") {
		t.Errorf("reason should explain why: %q", got.Reason)
	}
}

func TestDetectNoPartitions(t *testing.T) {
	if got := Detect(fakeDisk(), mounter(nil)); got.Seedable() {
		t.Fatal("a disk with no partitions is not seedable")
	}
}

func TestDetectUnrecognisedFilesystem(t *testing.T) {
	d := t.TempDir() // empty: no etc/home/var, no config.txt
	disk := fakeDisk(blockdev.Partition{Path: "/dev/sda1", FSType: "ext4"})
	got := Detect(disk, mounter(map[string]string{"/dev/sda1": d}))
	if got.Seedable() {
		t.Fatalf("empty filesystem should not be seedable, got %v", got.Family)
	}
}
