package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/susamn/flashtui/internal/flash"
)

// Update routes a message. Modal modes get first refusal on key presses so a
// global binding such as "q" cannot fire while the user is typing a password
// into a form.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case imagesMsg:
		return m.onImages(msg), nil

	case disksMsg:
		return m.onDisks(msg), nil

	case progressMsg:
		return m.onProgress(msg)

	case flashDoneMsg:
		return m.onFlashDone(msg)

	case detectMsg:
		return m.onDetect(msg), nil

	case seedDoneMsg:
		return m.onSeedDone(msg), listDisks()

	case mountMsg:
		return m.onMount(msg), listDisks()

	case tickMsg:
		m.tick++
		if m.flashing {
			return m, animate()
		}
		return m, nil

	case tea.KeyMsg:
		return m.onKey(msg)
	}
	return m, nil
}

func (m Model) onImages(msg imagesMsg) Model {
	if msg.err != nil {
		m.setStatus(levelError, "cannot read %s: %v", msg.dir, msg.err)
		m.images = nil
		m.imgList = listState{}
		return m
	}
	m.images = msg.images
	m.imageDir = msg.dir
	m.imgList.clamp(len(m.images), m.listHeight())
	if len(m.images) == 0 {
		m.setStatus(levelWarn, "no images in %s — press o to choose another directory", msg.dir)
	} else {
		m.setStatus(levelInfo, "%d image(s) in %s", len(m.images), msg.dir)
	}
	return m
}

func (m Model) onDisks(msg disksMsg) Model {
	if msg.err != nil {
		m.setStatus(levelError, "cannot list devices: %v", msg.err)
		return m
	}
	// Keep the cursor on the same device across a refresh; the list reorders
	// when a drive is plugged in or removed.
	var wantPath string
	if d, ok := m.selectedDisk(); ok {
		wantPath = d.Path
	}
	m.disks = msg.disks
	if wantPath != "" {
		for i, d := range m.disks {
			if d.Path == wantPath {
				m.diskList.cursor = i
			}
		}
	}
	m.diskList.clamp(len(m.disks), m.listHeight())
	return m
}

func (m Model) onProgress(msg progressMsg) (tea.Model, tea.Cmd) {
	m.progress = flash.Progress(msg)
	if m.progressCh == nil {
		return m, nil
	}
	return m, awaitProgress(m.progressCh)
}

func (m Model) onFlashDone(msg flashDoneMsg) (tea.Model, tea.Cmd) {
	m.flashing = false
	m.cancel = nil
	m.progressCh = nil
	m.mode = modeReport
	m.flashedTo = msg.target

	if msg.err != nil {
		m.lastErr = msg.err
		m.lastOK = ""
		m.setStatus(levelError, "flash failed")
		return m, listDisks()
	}
	m.lastErr = nil
	m.lastOK = sprintf("wrote %s to %s", humanBytes(m.progress.Bytes), msg.target)
	m.setStatus(levelOK, "flash complete")

	if m.seedAfter {
		m.setStatus(levelInfo, "inspecting the written image…")
		return m, detectAfterFlash(msg.target)
	}
	return m, listDisks()
}

func (m Model) onDetect(msg detectMsg) Model {
	if msg.err != nil {
		m.setStatus(levelWarn, "flashed, but could not inspect the result: %v", msg.err)
		return m
	}
	m.detection = msg.detection
	if !msg.detection.Seedable() {
		m.setStatus(levelWarn, "flashed. not seedable: %s", msg.detection.Reason)
		return m
	}
	m.form = newSeedForm()
	m.mode = modeSeed
	m.setStatus(levelInfo, "%s detected — fill in headless access", msg.detection.Family)
	return m
}

func (m Model) onSeedDone(msg seedDoneMsg) Model {
	m.mode = modeReport
	if msg.err != nil {
		m.lastErr = msg.err
		m.setStatus(levelError, "seeding failed")
		return m
	}
	m.lastErr = nil
	m.lastOK += "\nheadless access configured; the card is ready to boot"
	m.setStatus(levelOK, "seeded")
	return m
}

func (m Model) onMount(msg mountMsg) Model {
	if msg.err != nil {
		m.setStatus(levelError, "%s failed: %v", msg.action, msg.err)
		return m
	}
	m.setStatus(levelOK, "%s: %s", msg.action, msg.detail)
	return m
}

// onKey dispatches by mode. Each modal handler returns the model unchanged for
// keys it does not claim, which keeps the modal from leaking presses to the
// browse bindings underneath it.
func (m Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeGuard:
		return m.keyGuard(msg)
	case modeSeed:
		return m.keySeed(msg)
	case modeDirInput:
		return m.keyDirInput(msg)
	case modeHelp:
		// Any key closes help.
		m.mode = modeBrowse
		return m, nil
	case modeReport:
		m.mode = modeBrowse
		return m, nil
	case modeFlashing:
		return m.keyFlashing(msg)
	default:
		return m.keyBrowse(msg)
	}
}

func (m Model) keyFlashing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Only cancelling is offered mid-write; every other binding would act on a
	// device that is being overwritten.
	if !matches(msg, m.keys.Cancel) && !matches(msg, m.keys.Quit) {
		return m, nil
	}
	if m.cancel == nil {
		// Seeding reuses this view and is a short script; interrupting it
		// halfway would leave a partly configured image.
		m.setStatus(levelWarn, "configuration is already running; it will finish shortly")
		return m, nil
	}
	m.cancel()
	m.setStatus(levelWarn, "cancelling — the target is now partially written")
	return m, nil
}

func (m Model) keyBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case matches(msg, m.keys.Quit):
		return m, tea.Quit

	case matches(msg, m.keys.Help):
		m.mode = modeHelp
		return m, nil

	case matches(msg, m.keys.Refresh):
		m.setStatus(levelInfo, "refreshing…")
		return m, tea.Batch(scanImages(m.imageDir), listDisks())

	case matches(msg, m.keys.PaneNext):
		m.focus = (m.focus + 1) % paneCount
		return m, nil
	case matches(msg, m.keys.PanePrev):
		m.focus = (m.focus + paneCount - 1) % paneCount
		return m, nil
	case matches(msg, m.keys.PaneLeft):
		if m.focus > 0 {
			m.focus--
		}
		return m, nil
	case matches(msg, m.keys.PaneRight):
		if m.focus < paneCount-1 {
			m.focus++
		}
		return m, nil

	case msg.String() == "1":
		m.focus = paneImages
		return m, nil
	case msg.String() == "2":
		m.focus = paneTargets
		return m, nil
	case msg.String() == "3":
		m.focus = paneInfo
		return m, nil

	case matches(msg, m.keys.ToggleVerify):
		m.verify = !m.verify
		m.setStatus(levelInfo, "verify after write: %s", onOff(m.verify))
		return m, nil

	case matches(msg, m.keys.ToggleSeed):
		m.seedAfter = !m.seedAfter
		m.setStatus(levelInfo, "configure headless access after write: %s", onOff(m.seedAfter))
		return m, nil

	case matches(msg, m.keys.ChangeDir):
		m.mode = modeDirInput
		m.dirInput.SetValue(m.imageDir)
		m.dirInput.CursorEnd()
		m.dirInput.Focus()
		return m, nil

	case matches(msg, m.keys.Flash):
		return m.beginFlash()

	case matches(msg, m.keys.Mount):
		d, ok := m.selectedDisk()
		if !ok {
			return m, nil
		}
		m.setStatus(levelInfo, "mounting %s…", d.Path)
		return m, mountDisk(d)

	case matches(msg, m.keys.Unmount):
		d, ok := m.selectedDisk()
		if !ok {
			return m, nil
		}
		m.setStatus(levelInfo, "unmounting %s…", d.Path)
		return m, unmountDisk(d)

	case matches(msg, m.keys.PowerOff):
		d, ok := m.selectedDisk()
		if !ok {
			return m, nil
		}
		if d.System {
			m.setStatus(levelError, "refusing to power off the system disk")
			return m, nil
		}
		m.setStatus(levelInfo, "powering off %s…", d.Path)
		return m, powerOffDisk(d)
	}

	m.moveCursor(msg)
	return m, nil
}

// moveCursor applies vim motions to whichever list has focus.
func (m *Model) moveCursor(msg tea.KeyMsg) {
	var st *listState
	var n int
	switch m.focus {
	case paneImages:
		st, n = &m.imgList, len(m.images)
	case paneTargets:
		st, n = &m.diskList, len(m.disks)
	default:
		return // the info pane is a readout, not a list
	}
	if n == 0 {
		return
	}
	visible := m.listHeight()
	half := visible / 2
	if half < 1 {
		half = 1
	}

	switch {
	case matches(msg, m.keys.Down):
		st.cursor++
	case matches(msg, m.keys.Up):
		st.cursor--
	case matches(msg, m.keys.Top):
		st.cursor = 0
	case matches(msg, m.keys.Bottom):
		st.cursor = n - 1
	case matches(msg, m.keys.HalfDown):
		st.cursor += half
	case matches(msg, m.keys.HalfUp):
		st.cursor -= half
	default:
		return
	}
	st.clamp(n, visible)
}

// beginFlash opens the guard. Nothing is written until it is satisfied.
func (m Model) beginFlash() (tea.Model, tea.Cmd) {
	img, okImg := m.selectedImage()
	disk, okDisk := m.selectedDisk()
	if !okImg {
		m.setStatus(levelWarn, "select an image first")
		return m, nil
	}
	if !okDisk {
		m.setStatus(levelWarn, "select a target device first")
		return m, nil
	}
	m.guard = newGuard(disk, img)
	m.mode = modeGuard
	return m, nil
}

func (m Model) keyGuard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case matches(msg, m.keys.Cancel):
		m.mode = modeBrowse
		m.setStatus(levelInfo, "cancelled; nothing was written")
		return m, nil

	case matches(msg, m.keys.Confirm):
		if m.guard.blocked != "" {
			return m, nil
		}
		if !m.guard.confirmed() {
			m.setStatus(levelError, "that does not match %s", m.guard.disk.Name)
			return m, nil
		}
		return m.runFlash()
	}

	var cmd tea.Cmd
	m.guard.input, cmd = m.guard.input.Update(msg)
	return m, cmd
}

func (m Model) runFlash() (tea.Model, tea.Cmd) {
	job := flash.Job{
		Image:      m.guard.image,
		Target:     m.guard.disk.Path,
		TargetSize: int64(m.guard.disk.Size),
		Verify:     m.verify,
		ScratchDir: m.scratch,
	}
	run, ch, cancel := startFlash(m.esc, job, m.guard.disk, m.term)

	m.progressCh = ch
	m.cancel = cancel
	m.flashing = true
	m.mode = modeFlashing
	m.progress = flash.Progress{Phase: flash.PhaseWriting, Total: job.Image.WriteSize()}
	m.lastErr, m.lastOK = nil, ""
	m.setStatus(levelWarn, "writing to %s — do not unplug", job.Target)

	return m, tea.Batch(run, awaitProgress(ch), animate())
}

func (m Model) keyDirInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case matches(msg, m.keys.Cancel):
		m.mode = modeBrowse
		return m, nil
	case matches(msg, m.keys.Confirm):
		dir := expandHome(m.dirInput.Value())
		m.mode = modeBrowse
		m.setStatus(levelInfo, "scanning %s…", dir)
		return m, scanImages(dir)
	}
	var cmd tea.Cmd
	m.dirInput, cmd = m.dirInput.Update(msg)
	return m, cmd
}

func (m Model) keySeed(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case matches(msg, m.keys.Cancel):
		m.mode = modeReport
		m.setStatus(levelWarn, "skipped headless setup; the image is flashed but unconfigured")
		return m, nil

	case msg.String() == "tab" || msg.String() == "down":
		m.form.move(1)
		return m, nil
	case msg.String() == "shift+tab" || msg.String() == "up":
		m.form.move(-1)
		return m, nil

	case msg.String() == "ctrl+w":
		m.form.toggleWifi()
		return m, nil

	case msg.String() == "ctrl+k":
		if err := m.form.loadPublicKey(); err != nil {
			m.form.err = err.Error()
		} else {
			m.form.err = ""
		}
		return m, nil

	case matches(msg, m.keys.Confirm):
		cfg := m.form.config(true)
		if err := cfg.Validate(); err != nil {
			m.form.err = err.Error()
			return m, nil
		}
		m.form.err = ""
		m.mode = modeFlashing // reuse the busy view while the script runs
		m.setStatus(levelInfo, "writing configuration…")
		return m, applySeed(m.esc, m.detection, cfg, m.scratch, m.term)
	}

	return m, m.form.update(msg)
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
