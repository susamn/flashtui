package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/susamn/flashtui/internal/blockdev"
	"github.com/susamn/flashtui/internal/flash"
	"github.com/susamn/flashtui/internal/imagefile"
	"github.com/susamn/flashtui/internal/privilege"
	"github.com/susamn/flashtui/internal/seed"
)

// pane identifies a focusable column.
type pane int

const (
	paneImages pane = iota
	paneTargets
	paneInfo
	paneCount
)

func (p pane) title() string {
	switch p {
	case paneImages:
		return "IMAGES"
	case paneTargets:
		return "TARGETS"
	default:
		return "DRIVE INFO"
	}
}

// mode is the active interaction. Anything other than modeBrowse means a modal
// owns the keyboard.
type mode int

const (
	modeBrowse   mode = iota
	modeGuard         // confirming a destructive write
	modeSeed          // filling in headless first-boot details
	modeDirInput      // typing a new image directory
	modeHelp
	modeFlashing
	modeReport // result of the last flash, dismissed with any key
)

// level classifies a status line so the view can colour it.
type level int

const (
	levelInfo level = iota
	levelOK
	levelWarn
	levelError
)

// listState is the cursor and scroll offset of one scrollable pane.
type listState struct {
	cursor int
	offset int
}

// clamp keeps the cursor inside the list and scrolls the window to follow it.
func (l *listState) clamp(n, visible int) {
	if n == 0 {
		l.cursor, l.offset = 0, 0
		return
	}
	if l.cursor >= n {
		l.cursor = n - 1
	}
	if l.cursor < 0 {
		l.cursor = 0
	}
	if visible <= 0 {
		l.offset = 0
		return
	}
	if l.cursor < l.offset {
		l.offset = l.cursor
	}
	if l.cursor >= l.offset+visible {
		l.offset = l.cursor - visible + 1
	}
	if max := n - visible; l.offset > max {
		if max < 0 {
			max = 0
		}
		l.offset = max
	}
	if l.offset < 0 {
		l.offset = 0
	}
}

// Model is the whole application state.
type Model struct {
	styles styles
	keys   keymap
	width  int
	height int

	esc     *privilege.Escalator
	scratch string

	imageDir string
	images   []imagefile.Image
	imgList  listState

	disks    []blockdev.Disk
	diskList listState

	focus pane
	mode  mode

	// verify re-reads the device after writing; seedAfter opens the headless
	// configuration form once the write succeeds.
	verify    bool
	seedAfter bool

	// Flash progress and outcome.
	progress flash.Progress
	flashing bool
	// progressCh is the live sample channel; Update re-arms a listener on it
	// after every sample until it closes.
	progressCh chan flash.Progress
	cancel     context.CancelFunc
	tick       int
	lastErr    error
	lastOK     string
	flashedTo  string

	guard    guardState
	form     seedForm
	dirInput textinput.Model

	detection seed.Detection

	status      string
	statusLevel level
}

// New builds the initial model. The image directory defaults to the user's
// Downloads, which is where images are almost always sitting.
func New(esc *privilege.Escalator, imageDir, scratch string) Model {
	di := textinput.New()
	di.Prompt = "  directory: "
	di.CharLimit = 4096

	return Model{
		styles:   newStyles(),
		keys:     defaultKeys(),
		esc:      esc,
		scratch:  scratch,
		imageDir: imageDir,
		verify:   true,
		dirInput: di,
		status:   "loading…",
	}
}

// Init loads the two lists before the first paint.
func (m Model) Init() tea.Cmd {
	return tea.Batch(scanImages(m.imageDir), listDisks())
}

// selectedImage returns the highlighted image, if any.
func (m Model) selectedImage() (imagefile.Image, bool) {
	if m.imgList.cursor < 0 || m.imgList.cursor >= len(m.images) {
		return imagefile.Image{}, false
	}
	return m.images[m.imgList.cursor], true
}

// selectedDisk returns the highlighted target disk, if any.
func (m Model) selectedDisk() (blockdev.Disk, bool) {
	if m.diskList.cursor < 0 || m.diskList.cursor >= len(m.disks) {
		return blockdev.Disk{}, false
	}
	return m.disks[m.diskList.cursor], true
}

// busy reports whether a long-running operation owns the model.
func (m Model) busy() bool { return m.flashing }

func (m *Model) setStatus(l level, format string, args ...any) {
	m.statusLevel = l
	m.status = sprintf(format, args...)
}
