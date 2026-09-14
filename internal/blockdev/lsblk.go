package blockdev

import (
	"os/exec"
	"sync"
)

// systemPaths are checked before PATH when locating util-linux tools.
//
// This is not premature: a Homebrew/Linuxbrew lsblk earlier on PATH is built
// without libudev, and silently reports null for pttype, fstype, label and
// uuid on every device. The drive readout and the guard both depend on those
// fields, so picking the wrong binary turns an informative confirmation into a
// blank one. Verified on this machine: `ldd $(which lsblk) | grep udev` is
// empty for the brew build and matches for /usr/bin/lsblk.
var systemPaths = []string{"/usr/bin/", "/bin/", "/usr/sbin/", "/sbin/"}

var (
	lsblkOnce sync.Once
	lsblkPath string
)

// lsblkBinary returns the best available lsblk, preferring the distribution's
// own build over anything shadowing it on PATH.
func lsblkBinary() string {
	lsblkOnce.Do(func() {
		lsblkPath = findTool("lsblk")
	})
	return lsblkPath
}

func findTool(name string) string {
	for _, dir := range systemPaths {
		p := dir + name
		if isExecutable(p) {
			return p
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return name
}

func isExecutable(p string) bool {
	info, err := exec.Command(p, "--version").Output()
	return err == nil && len(info) > 0
}
