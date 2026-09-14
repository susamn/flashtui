package tui

import (
	"strings"

	"github.com/susamn/flashtui/internal/blockdev"
)

// infoPane is the readout balenaEtcher only hints at: what is physically on the
// selected drive, how full each filesystem is, and whether the selected image
// actually fits.
func (m Model) infoPane(width, height int) string {
	focused := m.focus == paneInfo && m.mode == modeBrowse
	st := m.styles.pane
	title := m.styles.title
	if focused {
		st = m.styles.paneFocused
		title = m.styles.titleFocus
	}
	inner := width - 4

	var b strings.Builder
	b.WriteString(title.Render(paneInfo.title()))
	b.WriteString("\n")

	d, ok := m.selectedDisk()
	if !ok {
		b.WriteString(m.styles.muted.Render("select a device in TARGETS"))
		return st.Width(width - 2).Height(height - 2).Render(b.String())
	}

	lines := m.diskSummary(d, inner)
	lines = append(lines, "")
	lines = append(lines, m.partitionLines(d, inner)...)
	if fit := m.fitLines(d, inner); len(fit) > 0 {
		lines = append(lines, "")
		lines = append(lines, fit...)
	}

	// The pane is a readout, so it clips rather than scrolls; the most
	// important facts are first.
	maxRows := height - 3
	if len(lines) > maxRows && maxRows > 0 {
		lines = lines[:maxRows]
	}
	b.WriteString(strings.Join(lines, "\n"))
	return st.Width(width - 2).Height(height - 2).Render(b.String())
}

func (m Model) diskSummary(d blockdev.Disk, inner int) []string {
	kv := func(k, v string) string {
		return m.styles.muted.Render(pad(k, 14)) + v
	}

	header := m.styles.accent.Render(d.Path)
	if d.System {
		header += "  " + m.styles.danger.Render("SYSTEM DISK")
	}

	desc := d.Description()
	if d.Vendor != "" && d.Vendor != desc {
		desc = d.Vendor + " " + desc
	}

	removable := "no"
	if d.Removable || d.Hotplug {
		removable = "yes"
	}
	table := d.PTType
	if table == "" {
		table = m.styles.muted.Render("none")
	}

	lines := []string{
		header,
		m.styles.muted.Render(truncate(desc, inner)),
		"",
		kv("capacity", humanBytes(int64(d.Size))),
		kv("transport", upperOrDash(d.Transport)),
		kv("removable", removable),
		kv("partition tbl", table),
		kv("partitions", sprintf("%d", len(d.Partitions))),
		kv("mounted", sprintf("%d of %d", d.MountedCount(), len(d.Partitions))),
	}
	if d.ReadOnly {
		lines = append(lines, m.styles.warn.Render("device is read-only"))
	}
	return lines
}

// partitionLines renders each partition with a usage bar, which is the quickest
// way to recognise a card by what is already on it.
func (m Model) partitionLines(d blockdev.Disk, inner int) []string {
	if len(d.Partitions) == 0 {
		return []string{m.styles.muted.Render("no partition table — the drive is blank or unrecognised")}
	}
	lines := []string{m.styles.title.Render("PARTITIONS")}
	for _, p := range d.Partitions {
		fs := p.FSType
		if fs == "" {
			fs = "-"
		}
		head := sprintf(" %s  %s  %s", pad(p.Name, 12), pad(humanBytes(int64(p.Size)), 10), fs)
		if p.Label != "" {
			head += "  " + m.styles.accent.Render(p.Label)
		}
		lines = append(lines, truncate(head, inner))

		if p.Mounted() {
			lines = append(lines, m.styles.muted.Render(truncate("   "+p.MountPoint, inner)))
		}
		// Usage is only known for a mounted filesystem; showing an empty bar
		// for an unmounted one would read as "empty" rather than "unknown".
		if used, avail := int64(p.FSUsed), int64(p.FSAvail); used+avail > 0 {
			total := used + avail
			frac := float64(used) / float64(total)
			// Capped rather than filling the pane: a usage bar is a glance,
			// and a very wide one just pushes the numbers off to the right.
			barW := inner - 26
			if barW > 24 {
				barW = 24
			}
			if barW < 6 {
				barW = 6
			}
			style := m.styles.ok
			switch {
			case frac > 0.95:
				style = m.styles.danger
			case frac > 0.80:
				style = m.styles.warn
			}
			bar := progressBar(frac, barW, style)
			lines = append(lines, truncate(sprintf("   %s %3.0f%% · %s free",
				bar, frac*100, humanBytes(avail)), inner))
		}
	}
	return lines
}

// fitLines answers the question the user actually has before pressing f: will
// this image go on this drive, and what happens to what is already there.
func (m Model) fitLines(d blockdev.Disk, inner int) []string {
	img, ok := m.selectedImage()
	if !ok {
		return nil
	}
	lines := []string{m.styles.title.Render("SELECTED IMAGE")}
	lines = append(lines, truncate(" "+img.Name, inner))

	detail := sprintf(" %s · %s on disk", img.Kind, humanBytes(img.Size))
	if img.ExpandedKnown() && img.Kind != 0 {
		detail += sprintf(" → %s written", humanBytes(img.Expanded))
	}
	lines = append(lines, m.styles.muted.Render(truncate(detail, inner)))

	need := img.WriteSize()
	switch {
	case need == 0:
		lines = append(lines, m.styles.warn.Render(
			truncate(" size unknown until written — progress cannot show a percentage", inner)))
	case d.Size == 0:
		// Nothing useful to compare against.
	case need > int64(d.Size):
		lines = append(lines, m.styles.danger.Render(
			truncate(sprintf(" DOES NOT FIT — needs %s more", humanBytes(need-int64(d.Size))), inner)))
	default:
		lines = append(lines, m.styles.ok.Render(
			truncate(sprintf(" fits, %s to spare", humanBytes(int64(d.Size)-need)), inner)))
	}
	if d.System {
		lines = append(lines, m.styles.danger.Render(truncate(" target is the system disk; writing is blocked", inner)))
	}
	return lines
}

func upperOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return strings.ToUpper(s)
}
