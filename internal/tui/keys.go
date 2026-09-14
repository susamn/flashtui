package tui

import tea "github.com/charmbracelet/bubbletea"

// Navigation follows vim and lazygit: j/k to move within a pane, h/l and tab
// to move between panes, number keys to jump straight to one.
type keymap struct {
	Up, Down         []string
	Top, Bottom      []string
	HalfUp, HalfDown []string
	PaneLeft         []string
	PaneRight        []string
	PaneNext         []string
	PanePrev         []string

	Flash    []string
	Mount    []string
	Unmount  []string
	PowerOff []string
	Refresh  []string
	ChangeDir,
	ToggleVerify,
	ToggleSeed []string

	Help    []string
	Quit    []string
	Cancel  []string
	Confirm []string
}

func defaultKeys() keymap {
	return keymap{
		Up:       []string{"k", "up"},
		Down:     []string{"j", "down"},
		Top:      []string{"g", "home"},
		Bottom:   []string{"G", "end"},
		HalfUp:   []string{"ctrl+u", "pgup"},
		HalfDown: []string{"ctrl+d", "pgdown"},

		PaneLeft:  []string{"h", "left"},
		PaneRight: []string{"l", "right"},
		PaneNext:  []string{"tab"},
		PanePrev:  []string{"shift+tab"},

		Flash:        []string{"f"},
		Mount:        []string{"m"},
		Unmount:      []string{"u"},
		PowerOff:     []string{"p"},
		Refresh:      []string{"r"},
		ChangeDir:    []string{"o"},
		ToggleVerify: []string{"v"},
		ToggleSeed:   []string{"s"},

		Help:    []string{"?"},
		Quit:    []string{"q", "ctrl+c"},
		Cancel:  []string{"esc"},
		Confirm: []string{"enter"},
	}
}

// matches reports whether a key press is one of the bindings.
func matches(msg tea.KeyMsg, binding []string) bool {
	s := msg.String()
	for _, b := range binding {
		if s == b {
			return true
		}
	}
	return false
}
