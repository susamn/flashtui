package tui

import "github.com/charmbracelet/lipgloss"

// Colours are adaptive so the same build is readable on a light or dark
// terminal without a theme setting.
var (
	colText     = lipgloss.AdaptiveColor{Light: "#1f2328", Dark: "#d8dee9"}
	colMuted    = lipgloss.AdaptiveColor{Light: "#6a737d", Dark: "#7d879c"}
	colBorder   = lipgloss.AdaptiveColor{Light: "#d0d7de", Dark: "#3b4252"}
	colAccent   = lipgloss.AdaptiveColor{Light: "#0969da", Dark: "#88c0d0"}
	colOK       = lipgloss.AdaptiveColor{Light: "#1a7f37", Dark: "#a3be8c"}
	colWarn     = lipgloss.AdaptiveColor{Light: "#9a6700", Dark: "#ebcb8b"}
	colDanger   = lipgloss.AdaptiveColor{Light: "#cf222e", Dark: "#bf616a"}
	colSelBG    = lipgloss.AdaptiveColor{Light: "#ddf4ff", Dark: "#434c5e"}
	colFocusSel = lipgloss.AdaptiveColor{Light: "#0969da", Dark: "#5e81ac"}
)

type styles struct {
	pane        lipgloss.Style
	paneFocused lipgloss.Style
	title       lipgloss.Style
	titleFocus  lipgloss.Style
	item        lipgloss.Style
	itemSel     lipgloss.Style
	itemSelBlur lipgloss.Style
	muted       lipgloss.Style
	key         lipgloss.Style
	ok          lipgloss.Style
	warn        lipgloss.Style
	danger      lipgloss.Style
	accent      lipgloss.Style
	statusBar   lipgloss.Style
	modal       lipgloss.Style
	modalDanger lipgloss.Style
}

func newStyles() styles {
	pane := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colBorder).
		Padding(0, 1)

	return styles{
		pane: pane,
		// Only the border colour changes with focus, so the layout never
		// reflows when focus moves.
		paneFocused: pane.BorderForeground(colAccent),

		title:      lipgloss.NewStyle().Foreground(colMuted).Bold(true),
		titleFocus: lipgloss.NewStyle().Foreground(colAccent).Bold(true),

		item:    lipgloss.NewStyle().Foreground(colText),
		itemSel: lipgloss.NewStyle().Foreground(colText).Background(colFocusSel).Bold(true),
		// A blurred pane keeps a dimmer marker on its selection so the reader
		// does not lose their place when focus moves elsewhere.
		itemSelBlur: lipgloss.NewStyle().Foreground(colText).Background(colSelBG),

		muted:  lipgloss.NewStyle().Foreground(colMuted),
		key:    lipgloss.NewStyle().Foreground(colAccent).Bold(true),
		ok:     lipgloss.NewStyle().Foreground(colOK),
		warn:   lipgloss.NewStyle().Foreground(colWarn),
		danger: lipgloss.NewStyle().Foreground(colDanger).Bold(true),
		accent: lipgloss.NewStyle().Foreground(colAccent),

		statusBar: lipgloss.NewStyle().Foreground(colMuted).Padding(0, 1),

		modal: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colAccent).
			Padding(1, 2),
		modalDanger: lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(colDanger).
			Padding(1, 2),
	}
}

// progressBar renders a determinate bar. frac is clamped by the caller.
func progressBar(frac float64, width int, style lipgloss.Style) string {
	if width < 4 {
		return ""
	}
	filled := int(frac * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	bar := ""
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	return style.Render(bar)
}

// indeterminateBar renders a moving block for the case where the image records
// no uncompressed length, so a percentage would be a lie.
func indeterminateBar(tick int, width int, style lipgloss.Style) string {
	if width < 4 {
		return ""
	}
	const blockLen = 6
	// Bounce rather than wrap, so the motion reads as activity rather than as
	// repeated progress from zero.
	span := width - blockLen
	if span < 1 {
		span = 1
	}
	pos := tick % (2 * span)
	if pos >= span {
		pos = 2*span - pos
	}
	bar := ""
	for i := 0; i < width; i++ {
		if i >= pos && i < pos+blockLen {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	return style.Render(bar)
}
