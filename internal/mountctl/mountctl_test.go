package mountctl

import (
	"context"
	"os/exec"
	"testing"
)

type call struct {
	name string
	args []string
}

// stub replaces run with a scripted responder and records what was invoked.
func stub(t *testing.T, fn func(name string, args []string) (string, error)) *[]call {
	t.Helper()
	var calls []call
	orig := run
	run = func(_ context.Context, name string, args ...string) (string, error) {
		calls = append(calls, call{name, args})
		return fn(name, args)
	}
	t.Cleanup(func() { run = orig })
	return &calls
}

func failed() error { return exec.Command("sh", "-c", "exit 1").Run() }

func TestMountParsesPath(t *testing.T) {
	stub(t, func(string, []string) (string, error) {
		return "Mounted /dev/sda1 at /run/media/susamn/bootfs.\n", nil
	})
	got, err := Mount(context.Background(), "/dev/sda1")
	if err != nil {
		t.Fatal(err)
	}
	if want := "/run/media/susamn/bootfs"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// Mounting something already mounted is the desired end state, so it must
// resolve to the existing path rather than erroring.
func TestMountAlreadyMountedFallsBackToLsblk(t *testing.T) {
	calls := stub(t, func(name string, _ []string) (string, error) {
		if name == "udisksctl" {
			return "Error mounting: GDBus.Error:...AlreadyMounted: Device is already mounted", failed()
		}
		return "/run/media/susamn/bootfs\n", nil
	})
	got, err := Mount(context.Background(), "/dev/sda1")
	if err != nil {
		t.Fatal(err)
	}
	if want := "/run/media/susamn/bootfs"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if len(*calls) != 2 || (*calls)[1].name != "lsblk" {
		t.Errorf("expected lsblk fallback, calls=%v", *calls)
	}
}

func TestMountRealFailurePropagates(t *testing.T) {
	stub(t, func(string, []string) (string, error) {
		return "Error mounting /dev/sda1: unknown filesystem", failed()
	})
	if _, err := Mount(context.Background(), "/dev/sda1"); err == nil {
		t.Fatal("want error")
	}
}

// Unmounting an unmounted partition is success: callers unmount
// unconditionally before a flash.
func TestUnmountNotMountedIsSuccess(t *testing.T) {
	stub(t, func(string, []string) (string, error) {
		return "Error unmounting: GDBus.Error:...NotMounted: Device is not mounted", failed()
	})
	if err := Unmount(context.Background(), "/dev/sda1"); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

// One stuck mount must not stop the others from being released, or the flash
// guard leaves the disk half-mounted.
func TestUnmountAllContinuesPastFailure(t *testing.T) {
	calls := stub(t, func(_ string, args []string) (string, error) {
		for _, a := range args {
			if a == "/dev/sdb2" {
				return "Error unmounting: target is busy", failed()
			}
		}
		return "", nil
	})
	err := UnmountAll(context.Background(), []string{"/dev/sdb1", "/dev/sdb2", "/dev/sdb3"})
	if err == nil {
		t.Fatal("want the first error reported")
	}
	if len(*calls) != 3 {
		t.Errorf("want all 3 attempted, got %d", len(*calls))
	}
}

func TestUnmountAllClean(t *testing.T) {
	stub(t, func(string, []string) (string, error) { return "", nil })
	if err := UnmountAll(context.Background(), []string{"/dev/sdb1", "/dev/sdb2"}); err != nil {
		t.Fatal(err)
	}
}

func TestPowerOff(t *testing.T) {
	calls := stub(t, func(string, []string) (string, error) { return "", nil })
	if err := PowerOff(context.Background(), "/dev/sda"); err != nil {
		t.Fatal(err)
	}
	got := (*calls)[0]
	if got.name != "udisksctl" || got.args[0] != "power-off" {
		t.Errorf("unexpected call %v", got)
	}
}

func TestPowerOffError(t *testing.T) {
	stub(t, func(string, []string) (string, error) { return "Error: busy", failed() })
	if err := PowerOff(context.Background(), "/dev/sda"); err == nil {
		t.Fatal("want error")
	}
}
