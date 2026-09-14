package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Layout constants. The left column holds the two lists, the right column the
// drive readout, and a write pane spans the bottom while a flash is running or
// its result is still on screen.
const (
	minWidth      = 60
	minHeight     = 18
	leftMinWidth  = 26
	writePaneRows = 6
)

// leftWidth splits the columns. The lists need less room than the readout, but
// not so little that an image filename becomes unreadable.
func (m Model) leftWidth() int {
	w := m.width * 38 / 100
	if w < leftMinWidth {
		w = leftMinWidth
	}
	if max := m.width - 30; w > max && max > leftMinWidth {
		w = max
	}
	return w
}

// bodyHeight is the vertical space the panes share, after the status and hint
// lines and any write pane are taken out.
func (m Model) bodyHeight() int {
	h := m.height - 2 // status line + hint line
	if m.showWritePane() {
		h -= writePaneRows
	}
	if h < 6 {
		h = 6
	}
	return h
}

// listHeight is the number of rows one list pane can show. Both lists get the
// same budget so the cursor maths is identical for each.
func (m Model) listHeight() int {
	// Two panes, each spending 2 rows on its border and 1 on its title.
	per := (m.bodyHeight() / 2) - 3
	if per < 1 {
		per = 1
	}
	return per
}

func (m Model) showWritePane() bool {
	return m.flashing || m.mode == modeFlashing || m.mode == modeReport
}

// View renders the whole screen.
func (m Model) View() string {
	if m.width < minWidth || m.height < minHeight {
		// Wrapped, because the terminal that triggers this message is by
		// definition too narrow to print it on one line.
		return m.styles.warn.Render(wrap(
			sprintf("terminal is %dx%d; flashtui needs at least %dx%d",
				m.width, m.height, minWidth, minHeight), m.width, 0))
	}

	switch m.mode {
	case modeHelp:
		return m.overlay(m.helpView())
	case modeGuard:
		return m.overlay(m.guardView())
	case modeSeed:
		return m.overlay(m.seedView())
	}

	left := m.leftWidth()
	right := m.width - left

	lists := lipgloss.JoinVertical(lipgloss.Left,
		m.listPane(paneImages, left, m.imagesRows()),
		m.listPane(paneTargets, left, m.targetRows()),
	)
	info := m.infoPane(right, m.bodyHeight())

	body := lipgloss.JoinHorizontal(lipgloss.Top, lists, info)

	parts := []string{body}
	if m.showWritePane() {
		parts = append(parts, m.writePane(m.width))
	}
	parts = append(parts, m.statusLine(), m.hintLine())

	out := lipgloss.JoinVertical(lipgloss.Left, parts...)
	if m.mode == modeDirInput {
		return m.overlay(m.dirView())
	}
	return out
}

// listPane frames one scrollable list.
func (m Model) listPane(p pane, width int, rows []string) string {
	focused := m.focus == p && m.mode == modeBrowse
	st := m.styles.pane
	title := m.styles.title
	if focused {
		st = m.styles.paneFocused
		title = m.styles.titleFocus
	}

	h := m.listHeight()

	var b strings.Builder
	b.WriteString(title.Render(p.title()))
	b.WriteString("\n")
	for i := 0; i < h; i++ {
		if i < len(rows) {
			// Rows arrive already padded and styled. Truncating here would
			// count the ANSI escape bytes against the visible width and cut
			// the text to a few characters.
			b.WriteString(rows[i])
		}
		if i < h-1 {
			b.WriteString("\n")
		}
	}
	return st.Width(width - 2).Render(b.String())
}

// imagesRows renders the visible slice of the image list.
func (m Model) imagesRows() []string {
	if len(m.images) == 0 {
		return []string{m.styles.muted.Render("(no images here — press o)")}
	}
	inner := m.leftWidth() - 4
	var rows []string
	end := m.imgList.offset + m.listHeight()
	if end > len(m.images) {
		end = len(m.images)
	}
	for i := m.imgList.offset; i < end; i++ {
		img := m.images[i]
		// Size column on the right: the expanded size is what matters, since
		// that is what has to fit on the card.
		size := humanBytes(img.WriteSize())
		if !img.ExpandedKnown() && img.Kind != 0 {
			size = "? " + img.Kind.String()
		}
		nameW := inner - len(size) - 3
		if nameW < 6 {
			nameW = 6
		}
		line := " " + pad(img.Name, nameW) + " " + size
		rows = append(rows, m.rowStyle(paneImages, i == m.imgList.cursor).Render(pad(line, inner)))
	}
	return rows
}

// targetRows renders the visible slice of the device list.
func (m Model) targetRows() []string {
	if len(m.disks) == 0 {
		return []string{m.styles.muted.Render("(no block devices found)")}
	}
	inner := m.leftWidth() - 4
	var rows []string
	end := m.diskList.offset + m.listHeight()
	if end > len(m.disks) {
		end = len(m.disks)
	}
	for i := m.diskList.offset; i < end; i++ {
		d := m.disks[i]
		// The system disk is marked in the list itself, not only in the guard,
		// so it reads as untouchable before the user ever selects it.
		marker := " "
		if d.System {
			marker = "!"
		} else if d.Removable || d.Transport == "usb" {
			marker = "+"
		}
		size := humanBytes(int64(d.Size))
		nameW := inner - len(size) - 4
		if nameW < 6 {
			nameW = 6
		}
		line := marker + " " + pad(d.Name+"  "+d.Description(), nameW) + " " + size
		base := m.rowStyle(paneTargets, i == m.diskList.cursor)
		if d.System && i != m.diskList.cursor {
			base = m.styles.danger
		}
		rows = append(rows, base.Render(pad(line, inner)))
	}
	return rows
}

// rowStyle picks the highlight for a row, keeping a dimmer marker on the
// selection of an unfocused pane so the reader does not lose their place.
func (m Model) rowStyle(p pane, selected bool) lipgloss.Style {
	if !selected {
		return m.styles.item
	}
	if m.focus == p && m.mode == modeBrowse {
		return m.styles.itemSel
	}
	return m.styles.itemSelBlur
}
