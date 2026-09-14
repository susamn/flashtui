package flash

import (
	"fmt"
	"strings"
)

// buildScript renders the privileged half of a flash as a single shell script.
//
// It exists because polkit's org.freedesktop.policykit.exec action defaults to
// auth_admin, not auth_admin_keep: nothing is cached, so every pkexec call
// prompts again. Writing, verifying and re-reading the partition table as
// three separate escalations would put two extra password prompts in the
// middle of a running flash, one of them while the TUI holds the terminal in
// raw mode. One script means one prompt, before anything is written.
//
// The unprivileged side still owns both data streams: the image goes in on
// stdin and the read-back comes out on stdout, so progress and the digest are
// computed here rather than trusted from the child.
// readyMarker is echoed by the script once it is running as root.
const readyMarker = "__flashtui_authenticated__"

func buildScript(target string, verify, directRead bool, blockSize int) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("set -eu\n")
	// Printed the instant the script starts, which is the instant pkexec has
	// finished authenticating. The caller watches for it so the terminal is
	// released only for the password prompt, not for the whole write.
	fmt.Fprintf(&b, "echo %s >&2\n", readyMarker)
	fmt.Fprintf(&b, "TARGET=%s\n", shellQuote(target))
	fmt.Fprintf(&b, "BS=%d\n", blockSize)
	b.WriteString(`
# conv=fsync makes dd flush before exiting, so a clean exit means the bytes
# reached the device rather than the page cache.
ERR=$(mktemp)
trap 'rm -f "$ERR"' EXIT
dd of="$TARGET" bs="$BS" conv=fsync 2>"$ERR"
`)
	if verify {
		b.WriteString(`
# dd reports the byte count on stderr; the read-back has to cover exactly the
# range that was written, rounded up to whole blocks.
N=$(awk '/bytes/ {print $1; exit}' "$ERR")
[ -n "$N" ] || { echo "could not determine how many bytes were written" >&2; exit 1; }
BLOCKS=$(( (N + BS - 1) / BS ))
`)
		flags := "status=none"
		if directRead {
			// O_DIRECT bypasses the page cache, so the comparison sees what is
			// actually on the device rather than what was written through it.
			flags += " iflag=direct"
		}
		fmt.Fprintf(&b, "dd if=\"$TARGET\" bs=\"$BS\" count=\"$BLOCKS\" %s\n", flags)
	}
	b.WriteString(`
# Make the kernel pick up the partition table the image just laid down, so the
# new partitions can be mounted for seeding. Harmless if the target is not a
# block device.
partprobe "$TARGET" >/dev/null 2>&1 || true
`)
	return b.String()
}

// shellQuote renders s as a single-quoted shell word.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
