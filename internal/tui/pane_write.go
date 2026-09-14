package tui

import (
	"strings"

	"github.com/susamn/flashtui/internal/flash"
)

// writePane spans the bottom while a write is running and afterwards while its
// result is still on screen.
func (m Model) writePane(width int) string {
	inner := width - 4
	var b strings.Builder
	b.WriteString(m.styles.titleFocus.Render("WRITE"))
	b.WriteString("\n")

	switch {
	case m.lastErr != nil:
		b.WriteString(m.styles.danger.Render("failed"))
		b.WriteString("\n")
		b.WriteString(wrap(m.lastErr.Error(), inner, 2))
	case !m.flashing && m.lastOK != "":
		b.WriteString(m.styles.ok.Render("done"))
		b.WriteString("\n")
		b.WriteString(wrap(m.lastOK, inner, 2))
	default:
		b.WriteString(m.progressLines(inner))
	}

	b.WriteString("\n")
	if m.flashing {
		b.WriteString(m.styles.muted.Render("esc cancel"))
	} else {
		b.WriteString(m.styles.muted.Render("any key to dismiss"))
	}
	return m.styles.paneFocused.Width(width - 2).Render(b.String())
}

func (m Model) progressLines(inner int) string {
	p := m.progress
	barW := inner - 34
	if barW < 10 {
		barW = 10
	}

	var bar, right string
	if frac, ok := p.Fraction(); ok {
		bar = progressBar(frac, barW, m.styles.accent)
		right = sprintf(" %3.0f%%  %s / %s", frac*100,
			humanBytes(p.Bytes), humanBytes(p.Total))
	} else {
		// bzip2 and truncated containers record no length, so an honest moving
		// bar beats a fabricated percentage.
		bar = indeterminateBar(m.tick, barW, m.styles.accent)
		right = sprintf("  %s written", humanBytes(p.Bytes))
	}

	phase := m.styles.warn.Render(pad(p.Phase.String(), 10))
	if p.Phase == flash.PhaseVerifying {
		phase = m.styles.accent.Render(pad(p.Phase.String(), 10))
	}

	line2 := sprintf("%s  elapsed %s", pad(humanRate(p.Rate), 12), humanDuration(p.Elapsed))
	if eta, ok := p.ETA(); ok {
		line2 += sprintf("  eta %s", humanDuration(eta))
	}

	return phase + bar + right + "\n" + m.styles.muted.Render("          "+line2)
}

// wrap breaks s to width, indenting every line, so a long error message stays
// inside its pane.
func wrap(s string, width, indent int) string {
	pre := strings.Repeat(" ", indent)
	width -= indent
	if width < 10 {
		width = 10
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		line := pre
		for _, word := range strings.Fields(para) {
			if len(line)+len(word)+1 > width+indent && strings.TrimSpace(line) != "" {
				out = append(out, line)
				line = pre
			}
			if strings.TrimSpace(line) != "" {
				line += " "
			}
			line += word
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
