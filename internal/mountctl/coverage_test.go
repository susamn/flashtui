package mountctl

import (
	"context"
	"testing"
)

func TestDefaultRunner(t *testing.T) {
	out, err := run(context.Background(), "sh", "-c", "printf fixture")
	if err != nil || out != "fixture" {
		t.Fatal(out, err)
	}
}

func TestMountFallbackAndSettle(t *testing.T) {
	calls := stub(t, func(name string, _ []string) (string, error) {
		if name == "lsblk" {
			return "/mnt/test\n", nil
		}
		return "unexpected but successful output", nil
	})
	got, err := Mount(context.Background(), "/dev/fake1")
	if err != nil || got != "/mnt/test" || len(*calls) != 2 {
		t.Fatal(got, err, *calls)
	}
	Settle(context.Background())
	if (*calls)[2].name != "udevadm" {
		t.Fatal(*calls)
	}
}

func TestMissingExistingMount(t *testing.T) {
	stub(t, func(name string, _ []string) (string, error) {
		if name == "lsblk" {
			return "", failed()
		}
		return "AlreadyMounted", failed()
	})
	if _, err := Mount(context.Background(), "/dev/fake1"); err == nil {
		t.Fatal("ignored lsblk failure")
	}
	stub(t, func(name string, _ []string) (string, error) {
		if name == "lsblk" {
			return "\n", nil
		}
		return "AlreadyMounted", failed()
	})
	if _, err := Mount(context.Background(), "/dev/fake1"); err == nil {
		t.Fatal("ignored missing mountpoint")
	}
	if got := firstLine(" first\nsecond"); got != "first" {
		t.Fatal(got)
	}
}
