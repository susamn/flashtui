// Package blockdev enumerates block devices and classifies which of them are
// safe to overwrite.
package blockdev

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Partition is one partition of a Disk.
type Partition struct {
	Name       string // "sda1"
	Path       string // "/dev/sda1"
	Size       uint64 // bytes
	FSType     string // "vfat", "ext4", "" when unformatted
	Label      string
	UUID       string
	FSAvail    uint64 // bytes free, 0 when unknown
	FSUsed     uint64 // bytes used, 0 when unknown
	MountPoint string // "" when not mounted
}

// Mounted reports whether the partition is currently mounted.
func (p Partition) Mounted() bool { return p.MountPoint != "" }

// Disk is a whole block device.
type Disk struct {
	Name       string // "sda"
	Path       string // "/dev/sda"
	Size       uint64 // bytes
	Model      string
	Vendor     string
	Serial     string
	Transport  string // "usb", "nvme", "sata", ""
	Removable  bool
	Hotplug    bool
	ReadOnly   bool
	PTType     string // "gpt", "dos", "" when no partition table
	Partitions []Partition

	// System is true when this disk carries a mounted filesystem that the
	// running OS depends on (/, /boot, /home, swap...). Writing to it would
	// destroy the machine, so the UI must refuse outright.
	System bool
}

// MountedCount returns how many of the disk's partitions are mounted.
func (d Disk) MountedCount() int {
	n := 0
	for _, p := range d.Partitions {
		if p.Mounted() {
			n++
		}
	}
	return n
}

// Description returns a human label for the disk, falling back through the
// identifying fields lsblk may or may not populate.
func (d Disk) Description() string {
	for _, s := range []string{d.Model, d.Vendor, d.Name} {
		if t := strings.TrimSpace(s); t != "" {
			return t
		}
	}
	return d.Name
}

// lsblkNode mirrors the subset of `lsblk --json` we consume. lsblk emits sizes
// as JSON numbers under -b but as strings without it, and several fields are
// null for devices that do not carry them, so every field is a json.Number or
// pointer-friendly type rather than a plain uint64.
type lsblkNode struct {
	Name        string      `json:"name"`
	Path        string      `json:"path"`
	Type        string      `json:"type"`
	Size        json.Number `json:"size"`
	Model       string      `json:"model"`
	Vendor      string      `json:"vendor"`
	Serial      string      `json:"serial"`
	Tran        string      `json:"tran"`
	RM          bool        `json:"rm"`
	Hotplug     bool        `json:"hotplug"`
	RO          bool        `json:"ro"`
	PTType      string      `json:"pttype"`
	FSType      string      `json:"fstype"`
	Label       string      `json:"label"`
	UUID        string      `json:"uuid"`
	FSAvail     json.Number `json:"fsavail"`
	FSUsed      json.Number `json:"fsused"`
	MountPoint  string      `json:"mountpoint"`
	MountPoints []*string   `json:"mountpoints"`
	Children    []lsblkNode `json:"children"`
}

func (n lsblkNode) allMountPoints() []string {
	var mps []string
	if n.MountPoint != "" {
		mps = append(mps, n.MountPoint)
	}
	for _, p := range n.MountPoints {
		if p != nil && *p != "" {
			found := false
			for _, m := range mps {
				if m == *p {
					found = true
					break
				}
			}
			if !found {
				mps = append(mps, *p)
			}
		}
	}
	return mps
}

func (n lsblkNode) bytes(v json.Number) uint64 {
	if v == "" {
		return 0
	}
	i, err := v.Int64()
	if err != nil || i < 0 {
		return 0
	}
	return uint64(i)
}

// runner is swapped out in tests.
var runner = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// lsblkFields is the explicit column set. Asking for the columns we use instead
// of -O keeps the output small and stable across util-linux versions.
// We request MOUNTPOINTS (plural) to capture bind mounts and multiple mounts.
const lsblkFields = "NAME,PATH,TYPE,SIZE,MODEL,VENDOR,SERIAL,TRAN,RM,HOTPLUG,RO,PTTYPE,FSTYPE,LABEL,UUID,FSAVAIL,FSUSED,MOUNTPOINT,MOUNTPOINTS"
const lsblkFieldsFallback = "NAME,PATH,TYPE,SIZE,MODEL,VENDOR,SERIAL,TRAN,RM,HOTPLUG,RO,PTTYPE,FSTYPE,LABEL,UUID,FSAVAIL,FSUSED,MOUNTPOINT"

// List returns every whole disk on the system, partitions attached, with
// System already resolved.
func List() ([]Disk, error) {
	out, err := runner(lsblkBinary(), "--json", "--bytes", "--paths", "-o", lsblkFields)
	if err != nil {
		out, err = runner(lsblkBinary(), "--json", "--bytes", "--paths", "-o", lsblkFieldsFallback)
		if err != nil {
			return nil, fmt.Errorf("lsblk: %w", err)
		}
	}
	var doc struct {
		BlockDevices []lsblkNode `json:"blockdevices"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, fmt.Errorf("parse lsblk output: %w", err)
	}

	var disks []Disk
	for _, n := range doc.BlockDevices {
		if n.Type != "disk" {
			continue
		}
		// zram, loop and device-mapper nodes are not flashable targets and
		// only clutter the list. With --paths, lsblk reports name as the full
		// /dev path, so match on the base name.
		base := baseName(n.Name)
		if strings.HasPrefix(base, "zram") || strings.HasPrefix(base, "loop") {
			continue
		}
		d := Disk{
			Name:      base,
			Path:      pathOf(n),
			Size:      n.bytes(n.Size),
			Model:     strings.TrimSpace(n.Model),
			Vendor:    strings.TrimSpace(n.Vendor),
			Serial:    strings.TrimSpace(n.Serial),
			Transport: n.Tran,
			Removable: n.RM,
			Hotplug:   n.Hotplug,
			ReadOnly:  n.RO,
			PTType:    n.PTType,
		}
		d.Partitions = collectPartitions(n)
		d.System = holdsSystemMount(n)
		disks = append(disks, d)
	}
	return disks, nil
}

// collectPartitions walks children recursively so that partitions hidden under
// a crypt or LVM node still surface with the mountpoint their mapper carries.
func collectPartitions(n lsblkNode) []Partition {
	var parts []Partition
	for _, c := range n.Children {
		if c.Type == "part" {
			p := Partition{
				Name:       baseName(c.Name),
				Path:       pathOf(c),
				Size:       c.bytes(c.Size),
				FSType:     c.FSType,
				Label:      c.Label,
				UUID:       c.UUID,
				FSAvail:    c.bytes(c.FSAvail),
				FSUsed:     c.bytes(c.FSUsed),
				MountPoint: c.MountPoint,
			}
			if p.MountPoint == "" {
				if mps := c.allMountPoints(); len(mps) > 0 {
					p.MountPoint = mps[0]
				}
			}
			// A LUKS or LVM partition reports its own fstype as "crypto_LUKS"
			// and carries the real filesystem on a child mapper node. Show the
			// mapper's filesystem so the pane is not a wall of crypto_LUKS.
			if p.MountPoint == "" && len(c.Children) > 0 {
				m := c.Children[0]
				if mps := m.allMountPoints(); len(mps) > 0 {
					p.MountPoint = mps[0]
					p.FSType = m.FSType
					p.FSAvail = c.bytes(m.FSAvail)
					p.FSUsed = c.bytes(m.FSUsed)
				}
			}
			parts = append(parts, p)
		}
		parts = append(parts, collectPartitions(c)...)
	}
	return parts
}

// systemMountPrefixes are filesystem hierarchies whose disk must never be offered as a flash
// target. "/" and "/boot" are fatal on their own; the rest mean the machine is
// actively using the disk for OS components or user data.
var systemMountPrefixes = []string{
	"/",
	"/boot",
	"/boot/efi",
	"/efi",
	"/home",
	"/var",
	"/usr",
	"/nix",
	"/etc",
	"/opt",
	"/srv",
	"/root",
}

// holdsSystemMount reports whether any node in the subtree is mounted at a
// path the running system depends on.
func holdsSystemMount(n lsblkNode) bool {
	for _, mp := range n.allMountPoints() {
		if isSystemMount(mp) {
			return true
		}
	}
	if n.FSType == "swap" && (n.MountPoint != "" || len(n.MountPoints) > 0) {
		return true
	}
	for _, c := range n.Children {
		if holdsSystemMount(c) {
			return true
		}
	}
	return false
}

func isSystemMount(mp string) bool {
	if mp == "" {
		return false
	}
	if mp == "[SWAP]" || strings.HasPrefix(mp, "[SWAP") {
		return true
	}
	for _, s := range systemMountPrefixes {
		if mp == s {
			return true
		}
	}
	for _, s := range systemMountPrefixes {
		if s == "/" {
			continue
		}
		if strings.HasPrefix(mp, s+"/") {
			return true
		}
	}
	return false
}

func pathOf(n lsblkNode) string {
	if n.Path != "" {
		return n.Path
	}
	return "/dev/" + baseName(n.Name)
}

func baseName(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}
