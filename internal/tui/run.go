package tui

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/susamn/flashtui/internal/privilege"
)

// Run starts the interactive program and blocks until the user quits.
func Run(imageDir string) error {
	esc, err := privilege.New()
	if err != nil {
		return err
	}
	if imageDir == "" {
		imageDir = defaultImageDir()
	}
	scratch, err := os.MkdirTemp("", "flashtui-")
	if err != nil {
		return fmt.Errorf("create scratch directory: %w", err)
	}
	defer os.RemoveAll(scratch)

	m := New(esc, imageDir, scratch)
	p := tea.NewProgram(m, tea.WithAltScreen())
	// The model needs the program to lend the terminal to pkexec, and the
	// program needs the model to exist first, so the handle is attached here.
	m.term.attach(p)

	_, err = p.Run()
	return err
}

// defaultImageDir is the user's Downloads folder, which is where a downloaded
// image is sitting.
func defaultImageDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	dl := filepath.Join(home, "Downloads")
	if st, err := os.Stat(dl); err == nil && st.IsDir() {
		return dl
	}
	return home
}
