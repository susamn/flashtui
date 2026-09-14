package tui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/susamn/flashtui/internal/seed"
)

// field indexes the seed form's inputs.
type field int

const (
	fieldUser field = iota
	fieldPassword
	fieldKey
	fieldHostname
	fieldWifiSSID
	fieldWifiPSK
	fieldWifiCountry
	fieldCount
)

var fieldLabels = [fieldCount]string{
	"username", "password", "ssh public key", "hostname",
	"wifi ssid", "wifi passphrase", "wifi country",
}

var fieldHints = [fieldCount]string{
	"the account you will ssh into",
	"required: it is what unlocks the account",
	"paste, or leave blank to load ~/.ssh/*.pub with ctrl+k",
	"optional",
	"optional",
	"8-63 characters",
	"two letters, e.g. IN — sets the radio's regulatory domain",
}

// seedForm collects headless first-boot details.
type seedForm struct {
	inputs  [fieldCount]textinput.Model
	focused field
	err     string
	// wifi collapses the last three fields until the user opts in, so the
	// common case is four fields rather than seven.
	wifi bool
}

func newSeedForm() seedForm {
	var f seedForm
	for i := range f.inputs {
		in := textinput.New()
		in.CharLimit = 4096
		in.Width = 48
		if field(i) == fieldPassword || field(i) == fieldWifiPSK {
			in.EchoMode = textinput.EchoPassword
			in.EchoCharacter = '•'
		}
		f.inputs[i] = in
	}
	f.inputs[fieldUser].SetValue("pi")
	f.inputs[fieldWifiCountry].CharLimit = 2
	f.focused = fieldUser
	f.inputs[fieldUser].Focus()
	return f
}

// visible returns the fields currently shown, which depends on whether wifi is
// expanded.
func (f seedForm) visible() []field {
	out := []field{fieldUser, fieldPassword, fieldKey, fieldHostname}
	if f.wifi {
		out = append(out, fieldWifiSSID, fieldWifiPSK, fieldWifiCountry)
	}
	return out
}

func (f *seedForm) focus(target field) {
	for i := range f.inputs {
		f.inputs[i].Blur()
	}
	f.focused = target
	f.inputs[target].Focus()
}

// move steps focus by delta through the visible fields, wrapping.
func (f *seedForm) move(delta int) {
	vis := f.visible()
	idx := 0
	for i, fd := range vis {
		if fd == f.focused {
			idx = i
		}
	}
	idx = (idx + delta + len(vis)) % len(vis)
	f.focus(vis[idx])
}

func (f *seedForm) toggleWifi() {
	f.wifi = !f.wifi
	if !f.wifi {
		// Clear the collapsed fields so a hidden half-filled wifi block cannot
		// fail validation invisibly.
		for _, fd := range []field{fieldWifiSSID, fieldWifiPSK, fieldWifiCountry} {
			f.inputs[fd].SetValue("")
		}
		if f.focused >= fieldWifiSSID {
			f.focus(fieldHostname)
		}
	}
}

// config assembles what the user typed.
func (f seedForm) config(enableSSH bool) seed.Config {
	return seed.Config{
		Username:      strings.TrimSpace(f.inputs[fieldUser].Value()),
		Password:      f.inputs[fieldPassword].Value(),
		AuthorizedKey: strings.TrimSpace(f.inputs[fieldKey].Value()),
		Hostname:      strings.TrimSpace(f.inputs[fieldHostname].Value()),
		EnableSSH:     enableSSH,
		WifiSSID:      strings.TrimSpace(f.inputs[fieldWifiSSID].Value()),
		WifiPSK:       f.inputs[fieldWifiPSK].Value(),
		WifiCountry:   strings.ToUpper(strings.TrimSpace(f.inputs[fieldWifiCountry].Value())),
	}
}

// update forwards a key press to the focused input.
func (f *seedForm) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	f.inputs[f.focused], cmd = f.inputs[f.focused].Update(msg)
	return cmd
}

// loadPublicKey fills the key field from the user's SSH directory.
//
// It reads only *.pub files, which are public by definition, and never touches
// a private key. If more than one exists the first by name is taken and the
// user can edit the field.
func (f *seedForm) loadPublicKey() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	matches, err := filepath.Glob(filepath.Join(home, ".ssh", "*.pub"))
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return errNoPublicKey
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		return err
	}
	line := strings.TrimSpace(string(data))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	f.inputs[fieldKey].SetValue(line)
	return nil
}
