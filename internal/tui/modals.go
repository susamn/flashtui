package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// overlay centres a modal on an otherwise empty screen. The panes underneath
// are not drawn: a confirmation that matters should not compete with the list
// the user might have mis-read in the first place.
func (m Model) overlay(content string) string {
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m Model) modalWidth() int {
	w := m.width * 3 / 4
	if w > 78 {
		w = 78
	}
	if w < 40 {
		w = 40
	}
	return w
}

// guardView is the last thing between the user and an irreversible write.
func (m Model) guardView() string {
	w := m.modalWidth()
	g := m.guard
	var b strings.Builder

	b.WriteString(m.styles.danger.Render("OVERWRITE THIS DEVICE?"))
	b.WriteString("\n\n")

	b.WriteString(m.styles.muted.Render(pad("device", 12)) + m.styles.accent.Render(g.disk.Path))
	b.WriteString("\n")
	b.WriteString(m.styles.muted.Render(pad("model", 12)) + truncate(g.disk.Description(), w-14))
	b.WriteString("\n")
	b.WriteString(m.styles.muted.Render(pad("capacity", 12)) + humanBytes(int64(g.disk.Size)))
	b.WriteString("\n")
	b.WriteString(m.styles.muted.Render(pad("transport", 12)) + upperOrDash(g.disk.Transport))
	b.WriteString("\n")
	if labels := g.labels(); len(labels) > 0 {
		// The labels are usually how the user recognises which card this is.
		b.WriteString(m.styles.muted.Render(pad("contains", 12)) +
			truncate(strings.Join(labels, ", "), w-14))
		b.WriteString("\n")
	}
	b.WriteString(m.styles.muted.Render(pad("image", 12)) + truncate(g.image.Name, w-14))
	b.WriteString("\n")

	if ws := g.warnings(); len(ws) > 0 {
		b.WriteString("\n")
		for _, warn := range ws {
			b.WriteString(m.styles.warn.Render("  ! " + truncate(warn, w-6)))
			b.WriteString("\n")
		}
	}

	if g.blocked != "" {
		b.WriteString("\n")
		b.WriteString(m.styles.danger.Render(wrap(g.blocked, w-4, 2)))
		b.WriteString("\n\n")
		b.WriteString(m.styles.muted.Render("  esc  back"))
		return m.styles.modalDanger.Width(w).Render(b.String())
	}

	b.WriteString("\n")
	b.WriteString(wrap("Everything on this device will be destroyed. Type its name to confirm.", w-4, 0))
	b.WriteString("\n\n")
	b.WriteString(m.styles.danger.Render(sprintf("  expected: %s", g.disk.Name)))
	b.WriteString("\n")
	b.WriteString(g.input.View())
	b.WriteString("\n\n")
	b.WriteString(m.styles.muted.Render("  enter  write   ·   esc  cancel"))

	return m.styles.modalDanger.Width(w).Render(b.String())
}

// seedView collects headless first-boot details after a successful flash.
func (m Model) seedView() string {
	w := m.modalWidth()
	var b strings.Builder

	b.WriteString(m.styles.titleFocus.Render("HEADLESS ACCESS"))
	b.WriteString("\n")
	b.WriteString(m.styles.muted.Render(sprintf("%s detected on %s", m.detection.Family, m.flashedTo)))
	b.WriteString("\n\n")

	for _, f := range m.form.visible() {
		label := fieldLabels[f]
		if m.form.focused == f {
			b.WriteString(m.styles.key.Render("▸ " + pad(label, 16)))
		} else {
			b.WriteString(m.styles.muted.Render("  " + pad(label, 16)))
		}
		b.WriteString("\n    ")
		b.WriteString(m.form.inputs[f].View())
		b.WriteString("\n")
		if m.form.focused == f {
			b.WriteString(m.styles.muted.Render("    " + fieldHints[f]))
			b.WriteString("\n")
		}
	}

	if m.form.err != "" {
		b.WriteString("\n")
		b.WriteString(m.styles.danger.Render(wrap(m.form.err, w-4, 2)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	wifi := "ctrl+w  add wifi"
	if m.form.wifi {
		wifi = "ctrl+w  drop wifi"
	}
	b.WriteString(m.styles.muted.Render(
		"  tab next  ·  ctrl+k load ~/.ssh/*.pub  ·  " + wifi))
	b.WriteString("\n")
	b.WriteString(m.styles.muted.Render("  enter apply  ·  esc skip"))

	return m.styles.modal.Width(w).Render(b.String())
}

func (m Model) dirView() string {
	w := m.modalWidth()
	var b strings.Builder
	b.WriteString(m.styles.titleFocus.Render("IMAGE DIRECTORY"))
	b.WriteString("\n\n")
	b.WriteString(m.dirInput.View())
	b.WriteString("\n\n")
	b.WriteString(m.styles.muted.Render("  enter scan  ·  esc cancel"))
	return m.styles.modal.Width(w).Render(b.String())
}

func (m Model) helpView() string {
	w := m.modalWidth()
	k := m.styles.key.Render
	mu := m.styles.muted.Render

	rows := [][2]string{
		{"j / k, ↓ / ↑", "move within a pane"},
		{"g / G", "first / last"},
		{"ctrl+d / ctrl+u", "half page down / up"},
		{"tab / shift+tab", "cycle panes"},
		{"h / l, ← / →", "previous / next pane"},
		{"1 / 2 / 3", "jump to images / targets / info"},
		{"", ""},
		{"f", "flash the selected image to the selected device"},
		{"m", "mount every partition of the selected device"},
		{"u", "unmount every partition"},
		{"p", "unmount and power off, so it is safe to unplug"},
		{"", ""},
		{"o", "change the image directory"},
		{"r", "rescan images and devices"},
		{"v", "toggle read-back verification"},
		{"s", "toggle headless setup after a successful write"},
		{"", ""},
		{"?", "this help"},
		{"q / ctrl+c", "quit"},
	}

	var b strings.Builder
	b.WriteString(m.styles.titleFocus.Render("KEYS"))
	b.WriteString("\n\n")
	for _, r := range rows {
		if r[0] == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString("  " + k(pad(r[0], 18)) + mu(r[1]) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(mu("  any key to close"))
	return m.styles.modal.Width(w).Render(b.String())
}
