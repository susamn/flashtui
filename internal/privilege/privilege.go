// Package privilege runs individual commands as root via pkexec.
//
// The TUI itself stays unprivileged: only the operations that genuinely need
// root (writing to a raw device, reading it back to verify, mounting, powering
// the drive off) are escalated, one at a time.
package privilege

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Escalator builds privileged commands.
type Escalator struct {
	// pkexecPath is resolved once at construction. Empty when the process is
	// already root, in which case commands run directly.
	pkexecPath string
	amRoot     bool
}

// New returns an Escalator, reporting an error only when escalation is both
// needed and impossible.
func New() (*Escalator, error) {
	e := &Escalator{amRoot: os.Geteuid() == 0}
	if e.amRoot {
		return e, nil
	}
	p, err := exec.LookPath("pkexec")
	if err != nil {
		return nil, fmt.Errorf("pkexec not found and not running as root: %w", err)
	}
	e.pkexecPath = p
	return e, nil
}

// Direct returns an Escalator that never escalates and runs every command as
// the current user. It is what New already returns when the process is root,
// and it is how a caller opts out of escalation for a target that does not
// need it, such as writing to a regular file.
func Direct() *Escalator { return &Escalator{amRoot: true} }

// Root reports whether commands will run without escalation.
func (e *Escalator) Root() bool { return e.amRoot }

// Command returns an *exec.Cmd that runs name with root privileges. Stdin,
// stdout and stderr are left for the caller to wire; pkexec passes all three
// through to the child, which is what makes streaming an image into dd's stdin
// work.
func (e *Escalator) Command(ctx context.Context, name string, args ...string) (*exec.Cmd, error) {
	target, err := exec.LookPath(name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if e.amRoot {
		return exec.CommandContext(ctx, target, args...), nil
	}
	// pkexec prompts through a polkit agent when one is running and through its
	// own text agent on the controlling tty when one is not. Either way the
	// caller must release the terminal first, or a raw-mode TUI will eat the
	// prompt. See tui.releaseTerminal.
	full := append([]string{target}, args...)
	return exec.CommandContext(ctx, e.pkexecPath, full...), nil
}

// Describe renders the command line that would run, for display in a
// confirmation prompt.
func (e *Escalator) Describe(name string, args ...string) string {
	parts := make([]string, 0, len(args)+2)
	if !e.amRoot {
		parts = append(parts, "pkexec")
	}
	parts = append(parts, name)
	parts = append(parts, args...)
	return strings.Join(parts, " ")
}

// ErrDenied reports whether err is the user dismissing or failing the
// authentication prompt. pkexec exits 126 when authorisation could not be
// obtained, and 127 when the target program could not be run at all.
func ErrDenied(err error) bool {
	var ee *exec.ExitError
	return errors.As(err, &ee) && ee.ExitCode() == 126
}
