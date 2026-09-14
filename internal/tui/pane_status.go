package tui

import "strings"

func (m Model) statusLine() string {
	style := m.styles.muted
	switch m.statusLevel {
	case levelOK:
		style = m.styles.ok
	case levelWarn:
		style = m.styles.warn
	case levelError:
		style = m.styles.danger
	}
	return m.styles.statusBar.Render(style.Render(truncate(m.status, m.width-2)))
}

// hintLine is the always-visible key legend, lazygit style. Toggles show their
// current state so the user does not have to remember whether verify is on.
func (m Model) hintLine() string {
	k := m.styles.key.Render
	mu := m.styles.muted.Render

	parts := []string{
		k("j/k") + mu(" move"),
		k("tab") + mu(" pane"),
		k("f") + mu(" flash"),
		k("m") + mu("/") + k("u") + mu(" mount"),
		k("p") + mu(" power off"),
		k("o") + mu(" dir"),
		k("r") + mu(" refresh"),
		k("v") + mu(" verify:") + stateText(m, m.verify),
		k("s") + mu(" seed:") + stateText(m, m.seedAfter),
		k("?") + mu(" help"),
		k("q") + mu(" quit"),
	}
	line := strings.Join(parts, mu("  ·  "))
	return m.styles.statusBar.Render(line)
}

func stateText(m Model, on bool) string {
	if on {
		return m.styles.ok.Render("on")
	}
	return m.styles.muted.Render("off")
}
