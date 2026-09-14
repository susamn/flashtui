// Package mountctl drives udisks2 for mounting, unmounting and powering down
// removable drives.
//
// udisksctl carries its own polkit integration, so these operations do not go
// through the pkexec escalator; on a desktop session they are already permitted
// for removable media and run without a prompt. Re-reading a partition table
// after a write is the exception and does need root.
package mountctl

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// run is swapped out in tests.
var run = func(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(out), err
}

// mountedAt extracts the path from udisksctl's "Mounted /dev/sda1 at /run/..."
// success line.
var mountedAt = regexp.MustCompile(`at (\S+?)\.?$`)

// Mount mounts a single partition and returns where it landed.
func Mount(ctx context.Context, partition string) (string, error) {
	out, err := run(ctx, "udisksctl", "mount", "-b", partition, "--no-user-interaction")
	if err != nil {
		// Already mounted is the desired end state, not a failure.
		if strings.Contains(out, "AlreadyMounted") {
			return existingMount(ctx, partition)
		}
		return "", fmt.Errorf("mount %s: %s", partition, firstLine(out))
	}
	if m := mountedAt.FindStringSubmatch(strings.TrimSpace(firstLine(out))); len(m) == 2 {
		return m[1], nil
	}
	return existingMount(ctx, partition)
}

// Unmount unmounts a single partition. A partition that is not mounted is
// treated as success so callers can unmount unconditionally.
func Unmount(ctx context.Context, partition string) error {
	out, err := run(ctx, "udisksctl", "unmount", "-b", partition, "--no-user-interaction")
	if err == nil {
		return nil
	}
	if strings.Contains(out, "NotMounted") {
		return nil
	}
	return fmt.Errorf("unmount %s: %s", partition, firstLine(out))
}

// UnmountAll unmounts every given partition, continuing past failures so that
// one stuck mount does not leave the rest mounted. It returns the first error.
//
// This must succeed before a flash: writing to a disk whose filesystems are
// mounted corrupts the page cache's view of them and can hang the unmount
// afterwards.
func UnmountAll(ctx context.Context, partitions []string) error {
	var first error
	for _, p := range partitions {
		if err := Unmount(ctx, p); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// PowerOff cuts power to the drive so it is safe to physically remove.
func PowerOff(ctx context.Context, disk string) error {
	out, err := run(ctx, "udisksctl", "power-off", "-b", disk, "--no-user-interaction")
	if err != nil {
		return fmt.Errorf("power-off %s: %s", disk, firstLine(out))
	}
	return nil
}

// Settle waits for udev to finish creating the device nodes for a partition
// table that was just rewritten. It needs no privileges, which matters: the
// flash script already ran partprobe as root, and escalating again here would
// cost the user a second password prompt for nothing.
func Settle(ctx context.Context) {
	_, _ = run(ctx, "udevadm", "settle", "--timeout=10")
}

// existingMount asks lsblk where a partition is mounted, for the paths where
// udisksctl's output does not carry it.
func existingMount(ctx context.Context, partition string) (string, error) {
	out, err := run(ctx, "lsblk", "-no", "MOUNTPOINT", partition)
	if err != nil {
		return "", fmt.Errorf("locate mount of %s: %w", partition, err)
	}
	mp := strings.TrimSpace(firstLine(out))
	if mp == "" {
		return "", fmt.Errorf("%s reports no mountpoint", partition)
	}
	return mp, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
