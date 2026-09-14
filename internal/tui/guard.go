package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"

	"github.com/susamn/flashtui/internal/blockdev"
	"github.com/susamn/flashtui/internal/imagefile"
)

// guardState is the confirmation standing between the user and an irreversible
// write.
//
// A yes/no prompt is too easy to answer reflexively, so the user has to type
// the target's device name. Picking the wrong drive is the mistake this tool
// exists to prevent, and typing "sdb" is a check that fails when the user had
// "sda" in mind.
type guardState struct {
	disk  blockdev.Disk
	image imagefile.Image
	input textinput.Model
	// blocked is set when the target can never be written to, in which case
	// there is no input to fill in at all.
	blocked string
}

func newGuard(disk blockdev.Disk, img imagefile.Image) guardState {
	in := textinput.New()
	in.Prompt = "  type the device name: "
	in.CharLimit = 32
	in.Focus()

	g := guardState{disk: disk, image: img, input: in}
	switch {
	case disk.System:
		g.blocked = "this disk holds the running system (/, /boot or /home). " +
			"flashtui will not write to it."
	case disk.ReadOnly:
		g.blocked = "the device is read-only; check its write-protect switch."
	}
	return g
}

// confirmed reports whether what was typed matches the target exactly. The
// comparison is case sensitive and ignores only surrounding whitespace: /dev
// names are lowercase, and accepting "SDA" would weaken the check for no gain.
func (g guardState) confirmed() bool {
	if g.blocked != "" {
		return false
	}
	return strings.TrimSpace(g.input.Value()) == g.disk.Name
}

// warnings lists everything about this target the user should see before
// confirming, most alarming first.
func (g guardState) warnings() []string {
	var w []string
	d := g.disk
	if !d.Removable && !d.Hotplug {
		w = append(w, "device is NOT removable — it looks like an internal disk")
	}
	if d.Transport != "usb" && d.Transport != "" {
		w = append(w, "connected over "+strings.ToUpper(d.Transport)+", not USB")
	}
	if n := d.MountedCount(); n > 0 {
		w = append(w, sprintf("%d partition(s) mounted; they will be unmounted first", n))
	}
	if len(d.Partitions) > 0 {
		w = append(w, sprintf("%d existing partition(s) will be destroyed", len(d.Partitions)))
	}
	if need := g.image.WriteSize(); need > 0 && d.Size > 0 {
		if need > int64(d.Size) {
			w = append(w, sprintf("image needs %s but the device holds %s",
				humanBytes(need), humanBytes(int64(d.Size))))
		}
	}
	return w
}

// labels returns the filesystem labels already on the disk, which is usually
// how a user recognises the card they meant to pick.
func (g guardState) labels() []string {
	var out []string
	for _, p := range g.disk.Partitions {
		if p.Label != "" {
			out = append(out, p.Label)
		}
	}
	return out
}
