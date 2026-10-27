package tui

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/susamn/flashtui/internal/blockdev"
)

func TestRunWithSyntheticTerminal(t *testing.T) {
	originalProgram, originalList := programFactory, enumerateDisks
	t.Cleanup(func() { programFactory, enumerateDisks = originalProgram, originalList })
	enumerateDisks = func() ([]blockdev.Disk, error) { return nil, nil }
	programFactory = func(m tea.Model) *tea.Program {
		return tea.NewProgram(m, tea.WithInput(strings.NewReader("q")), tea.WithOutput(io.Discard), tea.WithoutRenderer())
	}
	dir := t.TempDir()
	fakeCommand(t, "pkexec", "exit 1")
	t.Setenv("TMPDIR", t.TempDir())
	if err := Run(dir); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)
	if got := defaultImageDir(); got != dir {
		t.Fatal(got)
	}
	if err := os.Mkdir(filepath.Join(dir, "Downloads"), 0755); err != nil {
		t.Fatal(err)
	}
	if got := defaultImageDir(); got != filepath.Join(dir, "Downloads") {
		t.Fatal(got)
	}
}

func TestModelInit(t *testing.T) {
	m := testModel()
	if m.Init() == nil || m.busy() {
		t.Fatal("incorrect initial state")
	}
	m.flashing = true
	if !m.busy() {
		t.Fatal("flash not marked busy")
	}
	m.term.attach(nil)
	m.term.release()
	m.term.restore()
}
