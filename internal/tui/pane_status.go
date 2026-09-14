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

	// Ordered most useful first, because a narrow terminal drops from the end.
	type hint struct {
		rendered string
		plain    string
	}
	hints := []hint{
		{k("j/k") + mu(" move"), "j/k move"},
		{k("tab") + mu(" pane"), "tab pane"},
		{k("f") + mu(" flash"), "f flash"},
		{k("m") + mu("/") + k("u") + mu(" mount"), "m/u mount"},
		{k("p") + mu(" power off"), "p power off"},
		{k("v") + mu(" verify:") + stateText(m, m.verify), "v verify:" + onOff(m.verify)},
		{k("s") + mu(" seed:") + stateText(m, m.seedAfter), "s seed:" + onOff(m.seedAfter)},
		{k("o") + mu(" dir"), "o dir"},
		{k("r") + mu(" refresh"), "r refresh"},
		{k("?") + mu(" help"), "? help"},
		{k("q") + mu(" quit"), "q quit"},
	}

	// The separator is measured in plain characters, since the rendered form
	// carries escape sequences that occupy no cells.
	const sep = "  ·  "
	budget := m.width - 2 // the status bar's own padding
	var rendered []string
	used := 0
	for _, h := range hints {
		cost := len(h.plain)
		if len(rendered) > 0 {
			cost += len(sep)
		}
		if used+cost > budget {
			break
		}
		used += cost
		rendered = append(rendered, h.rendered)
	}
	return m.styles.statusBar.Render(strings.Join(rendered, mu(sep)))
}

func stateText(m Model, on bool) string {
	if on {
		return m.styles.ok.Render("on")
	}
	return m.styles.muted.Render("off")
}
