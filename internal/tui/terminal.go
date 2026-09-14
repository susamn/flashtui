package tui

import (
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// terminal hands the tty back and forth between the TUI and pkexec.
//
// pkexec has no polkit agent to delegate to on a plain Wayland or console
// session, so it registers its own text agent and reads the password from
// /dev/tty. That cannot work while bubbletea holds the terminal in raw mode
// with the alternate screen active, so the terminal is released for the
// prompt and taken back the moment the privileged script confirms it is
// running.
type terminal struct {
	mu       sync.Mutex
	prog     *tea.Program
	released bool
}

func newTerminal() *terminal { return &terminal{} }

// attach records the program once it exists. Until then release and restore
// are no-ops, which is correct: nothing owns the terminal yet.
func (t *terminal) attach(p *tea.Program) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.prog = p
}

// release gives the terminal back to the shell so a password prompt is visible
// and echoes correctly.
func (t *terminal) release() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.prog == nil || t.released {
		return
	}
	// A failure here means the terminal was never ours to release, so there is
	// nothing to undo and nothing useful to report.
	if err := t.prog.ReleaseTerminal(); err == nil {
		t.released = true
	}
}

// restore takes the terminal back and repaints. It is safe to call when
// nothing was released.
func (t *terminal) restore() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.prog == nil || !t.released {
		return
	}
	if err := t.prog.RestoreTerminal(); err == nil {
		t.released = false
	}
}
