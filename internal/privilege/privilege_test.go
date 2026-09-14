package privilege

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestCommandWrapsInPkexec(t *testing.T) {
	e := &Escalator{pkexecPath: "/usr/bin/pkexec"}
	cmd, err := e.Command(context.Background(), "dd", "of=/dev/null")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path != "/usr/bin/pkexec" {
		t.Fatalf("path=%q want pkexec", cmd.Path)
	}
	// Args[1] must be the absolute target, not the bare name: pkexec refuses
	// to resolve relative programs.
	if !strings.HasSuffix(cmd.Args[1], "/dd") {
		t.Fatalf("args=%v want absolute dd at [1]", cmd.Args)
	}
	if cmd.Args[len(cmd.Args)-1] != "of=/dev/null" {
		t.Fatalf("args=%v lost trailing arg", cmd.Args)
	}
}

func TestCommandSkipsPkexecWhenRoot(t *testing.T) {
	e := &Escalator{amRoot: true}
	cmd, err := e.Command(context.Background(), "dd")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(cmd.Path, "pkexec") {
		t.Fatalf("path=%q must not escalate when already root", cmd.Path)
	}
}

func TestCommandRejectsMissingProgram(t *testing.T) {
	e := &Escalator{pkexecPath: "/usr/bin/pkexec"}
	if _, err := e.Command(context.Background(), "definitely-not-a-real-program-xyz"); err == nil {
		t.Fatal("want error for unresolvable program")
	}
}

func TestDescribe(t *testing.T) {
	e := &Escalator{pkexecPath: "/usr/bin/pkexec"}
	if got, want := e.Describe("dd", "of=/dev/sda"), "pkexec dd of=/dev/sda"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
	r := &Escalator{amRoot: true}
	if got, want := r.Describe("dd", "of=/dev/sda"), "dd of=/dev/sda"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestErrDenied(t *testing.T) {
	// Exit 126 is pkexec's "not authorised"; anything else is a real failure
	// and must not be reported to the user as a cancelled prompt.
	denied := exec.Command("sh", "-c", "exit 126").Run()
	if !ErrDenied(denied) {
		t.Error("exit 126 should read as denied")
	}
	other := exec.Command("sh", "-c", "exit 1").Run()
	if ErrDenied(other) {
		t.Error("exit 1 is a command failure, not a denial")
	}
	if ErrDenied(errors.New("plain")) || ErrDenied(nil) {
		t.Error("non-exit errors are not denials")
	}
}
