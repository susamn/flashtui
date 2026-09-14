package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/susamn/flashtui/internal/blockdev"
	"github.com/susamn/flashtui/internal/flash"
	"github.com/susamn/flashtui/internal/imagefile"
	"github.com/susamn/flashtui/internal/mountctl"
	"github.com/susamn/flashtui/internal/privilege"
	"github.com/susamn/flashtui/internal/seed"
)

// opTimeout bounds the short udisks operations. A flash has no timeout: a slow
// card can legitimately take a very long time.
const opTimeout = 60 * time.Second

func scanImages(dir string) tea.Cmd {
	return func() tea.Msg {
		imgs, err := imagefile.Scan(dir)
		return imagesMsg{dir: dir, images: imgs, err: err}
	}
}

func listDisks() tea.Cmd {
	return func() tea.Msg {
		disks, err := blockdev.List()
		return disksMsg{disks: disks, err: err}
	}
}

func mountDisk(disk blockdev.Disk) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
		defer cancel()
		var mounted int
		var last string
		for _, p := range disk.Partitions {
			mp, err := mountctl.Mount(ctx, p.Path)
			if err != nil {
				// A partition with no mountable filesystem is normal on a
				// flashed card; only report if nothing mounted at all.
				continue
			}
			mounted++
			last = mp
		}
		if mounted == 0 {
			return mountMsg{action: "mount", err: errNothingMounted}
		}
		return mountMsg{action: "mount", detail: sprintf("%d partition(s), last at %s", mounted, last)}
	}
}

func unmountDisk(disk blockdev.Disk) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
		defer cancel()
		err := mountctl.UnmountAll(ctx, partitionPaths(disk))
		return mountMsg{action: "unmount", detail: disk.Path, err: err}
	}
}

func powerOffDisk(disk blockdev.Disk) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
		defer cancel()
		// Powering off a disk with mounted filesystems loses whatever is still
		// in the page cache, so release them first.
		if err := mountctl.UnmountAll(ctx, partitionPaths(disk)); err != nil {
			return mountMsg{action: "power off", err: err}
		}
		err := mountctl.PowerOff(ctx, disk.Path)
		return mountMsg{action: "power off", detail: disk.Path, err: err}
	}
}

func partitionPaths(d blockdev.Disk) []string {
	out := make([]string, 0, len(d.Partitions))
	for _, p := range d.Partitions {
		out = append(out, p.Path)
	}
	return out
}

// startFlash unmounts the target, then runs the write. Progress samples arrive
// as progressMsg and the outcome as flashDoneMsg.
//
// The returned cancel stops the write; the caller keeps it so the user can
// abort.
func startFlash(esc *privilege.Escalator, job flash.Job, disk blockdev.Disk, term *terminal) (tea.Cmd, chan flash.Progress, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	updates := make(chan flash.Progress, 64)

	// pkexec prompts on /dev/tty, which bubbletea is holding in raw mode. The
	// terminal goes back to the shell for the prompt and is reclaimed as soon
	// as the privileged script reports that it is running.
	job.OnAuthenticated = term.restore

	run := func() tea.Msg {
		term.release()
		// Writing to a disk whose filesystems are mounted corrupts the
		// kernel's cached view of them, so this has to succeed first.
		uctx, ucancel := context.WithTimeout(ctx, opTimeout)
		err := mountctl.UnmountAll(uctx, partitionPaths(disk))
		ucancel()
		if err != nil {
			close(updates)
			return flashDoneMsg{target: job.Target, err: err}
		}
		err = flash.Run(ctx, esc, job, updates)
		return flashDoneMsg{target: job.Target, err: err}
	}

	return run, updates, cancel
}

// awaitProgress yields the next sample and is re-issued by Update after each
// one, which is how a channel is bridged into the Elm loop. A closed channel
// yields nil, which tea discards.
func awaitProgress(ch chan flash.Progress) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return nil
		}
		return progressMsg(p)
	}
}

// detectAfterFlash re-reads the partition table the write just created, then
// classifies what first-boot mechanism the image exposes.
func detectAfterFlash(target string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
		defer cancel()

		// The flash script already ran partprobe as root; this only waits for
		// udev to catch up, so it needs no second escalation.
		mountctl.Settle(ctx)

		disks, err := blockdev.List()
		if err != nil {
			return detectMsg{err: err}
		}
		var disk blockdev.Disk
		for _, d := range disks {
			if d.Path == target {
				disk = d
			}
		}
		if disk.Path == "" {
			return detectMsg{err: errTargetGone}
		}
		det := seed.Detect(disk, func(part string) (string, func(), error) {
			mp, err := mountctl.Mount(ctx, part)
			if err != nil {
				return "", nil, err
			}
			return mp, func() {
				uctx, ucancel := context.WithTimeout(context.Background(), opTimeout)
				defer ucancel()
				_ = mountctl.Unmount(uctx, part)
			}, nil
		})
		return detectMsg{detection: det}
	}
}

// applySeed mounts what the detection named, writes the configuration, then
// unmounts again so the card is safe to pull.
func applySeed(esc *privilege.Escalator, det seed.Detection, cfg seed.Config, scratch string, term *terminal) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		// Seeding is one short privileged script, so the terminal is simply
		// lent out for its whole duration rather than handed back mid-run.
		term.release()
		defer term.restore()

		var m seed.Mounted
		var toUnmount []string
		mount := func(part string) (string, error) {
			mp, err := mountctl.Mount(ctx, part)
			if err != nil {
				return "", err
			}
			toUnmount = append(toUnmount, part)
			return mp, nil
		}
		defer func() {
			uctx, ucancel := context.WithTimeout(context.Background(), opTimeout)
			defer ucancel()
			_ = mountctl.UnmountAll(uctx, toUnmount)
		}()

		if det.Boot != "" {
			mp, err := mount(det.Boot)
			if err != nil {
				return seedDoneMsg{err: err}
			}
			m.Boot = mp
		}
		mp, err := mount(det.Root)
		if err != nil {
			return seedDoneMsg{err: err}
		}
		m.Root = mp

		return seedDoneMsg{err: seed.Apply(ctx, esc, det, cfg, m, scratch)}
	}
}

// animate drives the indeterminate bar.
func animate() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}
