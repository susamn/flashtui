// Package seed configures a freshly flashed image for headless first boot:
// a user, a password, an SSH key, a hostname and optionally wifi, so the
// machine can be reached over the network without ever attaching a screen.
//
// Only images that expose a first-boot mechanism can be seeded. Which ones
// those are is decided by Detect, not assumed from the file name.
package seed

import (
	"os"
	"path/filepath"

	"github.com/susamn/flashtui/internal/blockdev"
)

// Family is the first-boot mechanism an image exposes.
type Family int

const (
	// Unseedable covers images with no writable first-boot hook: live ISOs
	// whose root is a read-only squashfs unpacked into RAM, and anything
	// unrecognised.
	Unseedable Family = iota
	// RaspberryPiOS reads /boot/firmware/userconf.txt and an ssh marker on
	// first boot, and has a real ext4 root to drop an authorized_keys into.
	RaspberryPiOS
	// CloudInit reads a NoCloud seed from /var/lib/cloud/seed/nocloud-net on
	// the root filesystem.
	CloudInit
)

func (f Family) String() string {
	switch f {
	case RaspberryPiOS:
		return "Raspberry Pi OS"
	case CloudInit:
		return "cloud-init"
	default:
		return "not seedable"
	}
}

// Detection is the result of inspecting a flashed disk.
type Detection struct {
	Family Family
	// Boot is the partition holding the firmware/boot files, empty when the
	// family does not use one.
	Boot string
	// Root is the partition holding the root filesystem.
	Root string
	// Reason explains an Unseedable result in terms the UI can show verbatim.
	Reason string
}

// Seedable reports whether Apply can do anything with this detection.
func (d Detection) Seedable() bool { return d.Family != Unseedable }

// Detect classifies a flashed disk. Mount is called for each candidate
// partition, and the returned cleanup runs before Detect returns; it is a
// parameter so the caller controls how mounting happens and so this stays
// testable against plain directories.
func Detect(disk blockdev.Disk, mount func(part string) (string, func(), error)) Detection {
	// A live ISO writes an iso9660 filesystem straight onto the device. There
	// is no writable location the installer or live system will read back.
	for _, p := range disk.Partitions {
		if p.FSType == "iso9660" {
			return Detection{
				Reason: "live ISO (iso9660): its root is a read-only squashfs " +
					"unpacked into RAM, so there is nowhere to seed",
			}
		}
	}
	if len(disk.Partitions) == 0 {
		return Detection{Reason: "no partitions found on the device"}
	}

	var bootPart, rootPart string
	for _, p := range disk.Partitions {
		mp, done, err := mount(p.Path)
		if err != nil {
			continue
		}
		if bootPart == "" && looksLikeRaspiBoot(mp) {
			bootPart = p.Path
		}
		if rootPart == "" && looksLikeLinuxRoot(mp) {
			rootPart = p.Path
		}
		done()
	}

	switch {
	case bootPart != "" && rootPart != "":
		return Detection{Family: RaspberryPiOS, Boot: bootPart, Root: rootPart}
	case rootPart != "":
		return Detection{Family: CloudInit, Root: rootPart}
	default:
		return Detection{
			Reason: "no Linux root filesystem found; the image exposes no " +
				"first-boot hook to seed",
		}
	}
}

// looksLikeRaspiBoot identifies the Pi firmware partition by the files the
// bootloader itself requires, rather than by the "bootfs" label, which a user
// can change and some derivatives do not set.
func looksLikeRaspiBoot(mp string) bool {
	return exists(filepath.Join(mp, "config.txt")) && exists(filepath.Join(mp, "cmdline.txt"))
}

// looksLikeLinuxRoot identifies a real root filesystem. All three must be
// present so that a boot partition carrying a stray /etc is not mistaken for
// the root.
func looksLikeLinuxRoot(mp string) bool {
	return isDir(filepath.Join(mp, "etc")) &&
		isDir(filepath.Join(mp, "home")) &&
		isDir(filepath.Join(mp, "var"))
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
