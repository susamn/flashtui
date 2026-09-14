package flash

import (
	"os/exec"
	"strings"
	"testing"
)

// The script is the only thing that runs as root, so it has to be syntactically
// valid before it is ever handed to a privileged shell.
func TestScriptIsValidShell(t *testing.T) {
	for _, verify := range []bool{true, false} {
		for _, direct := range []bool{true, false} {
			s := buildScript("/dev/sda", verify, direct, blockSize)
			cmd := exec.Command("sh", "-n")
			cmd.Stdin = strings.NewReader(s)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("verify=%v direct=%v: %v\n%s\n%s", verify, direct, err, out, s)
			}
		}
	}
}

func TestScriptVerifyToggle(t *testing.T) {
	with := buildScript("/dev/sda", true, true, blockSize)
	if !strings.Contains(with, `if="$TARGET"`) {
		t.Error("verify script must read the device back")
	}
	if !strings.Contains(with, "iflag=direct") {
		t.Error("verify read must bypass the page cache")
	}

	without := buildScript("/dev/sda", false, true, blockSize)
	if strings.Contains(without, `if="$TARGET"`) {
		t.Error("no read-back should be emitted when verification is off")
	}
}

// partprobe runs regardless, so the partitions the image laid down can be
// mounted for seeding without a second escalation.
func TestScriptAlwaysRescans(t *testing.T) {
	for _, verify := range []bool{true, false} {
		if !strings.Contains(buildScript("/dev/sda", verify, true, blockSize), "partprobe") {
			t.Errorf("verify=%v: partprobe missing", verify)
		}
	}
}

// A device path is attacker-controlled only in the sense that it comes from
// lsblk, but quoting it costs nothing and removes the question entirely.
func TestScriptQuotesTarget(t *testing.T) {
	s := buildScript("/dev/sd'a; rm -rf /", true, true, blockSize)
	if strings.Contains(s, "TARGET='/dev/sd'; rm -rf /") {
		t.Fatalf("target escaped its quotes:\n%s", s)
	}
	cmd := exec.Command("sh", "-n")
	cmd.Stdin = strings.NewReader(s)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hostile target produced invalid shell: %v\n%s", err, out)
	}
}
