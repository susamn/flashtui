package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/susamn/flashtui/internal/seed"
)

func TestSeedFormFieldsAndKeyLoading(t *testing.T) {
	f := newSeedForm()
	if len(f.visible()) != 4 || f.focused != fieldUser {
		t.Fatal(f)
	}
	f.move(-1)
	if f.focused != fieldHostname {
		t.Fatal(f.focused)
	}
	f.move(1)
	if f.focused != fieldUser {
		t.Fatal(f.focused)
	}
	f.toggleWifi()
	if len(f.visible()) != 7 {
		t.Fatal(f.visible())
	}
	f.focus(fieldWifiPSK)
	f.inputs[fieldWifiSSID].SetValue("  Home  ")
	f.inputs[fieldWifiPSK].SetValue("password123")
	f.inputs[fieldWifiCountry].SetValue("us")
	f.inputs[fieldPassword].SetValue("secret")
	f.inputs[fieldKey].SetValue(" ssh-ed25519 AAAA comment ")
	f.inputs[fieldHostname].SetValue(" host ")
	cfg := f.config(true)
	if cfg.WifiSSID != "Home" || cfg.WifiCountry != "US" || cfg.Hostname != "host" || !cfg.EnableSSH {
		t.Fatal(cfg)
	}
	f.toggleWifi()
	if f.focused != fieldHostname || f.config(false).WifiSSID != "" {
		t.Fatal(f.focused)
	}
	_ = f.update(key("x"))

	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := f.loadPublicKey(); err != errNoPublicKey {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(home, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	pub := filepath.Join(home, ".ssh", "id_test.pub")
	if err := os.WriteFile(pub, []byte("ssh-ed25519 AAAA comment\nother\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := f.loadPublicKey(); err != nil || !strings.HasPrefix(f.config(false).AuthorizedKey, "ssh-ed25519 AAAA") {
		t.Fatal(err)
	}
	if err := os.Remove(pub); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(pub, 0700); err != nil {
		t.Fatal(err)
	}
	if err := f.loadPublicKey(); err == nil {
		t.Fatal("directory read as key")
	}
}

func TestSeedKeyModesAndViews(t *testing.T) {
	m := testModel()
	m.mode = modeSeed
	m.form = newSeedForm()
	m.form.inputs[fieldPassword].SetValue("secret")
	m.form.inputs[fieldUser].SetValue("user")
	m.form.toggleWifi()
	for _, k := range []string{"tab", "shift+tab", "down", "up", "ctrl+w", "ctrl+w"} {
		m = press(m, k)
	}
	if got := m.seedView(); got == "" {
		t.Fatal("empty form")
	}
	// A validated form moves into the busy state without running the command.
	next, cmd := m.Update(key("enter"))
	if next.(Model).mode != modeFlashing || cmd == nil {
		t.Fatal("valid form did not submit")
	}
	m.mode = modeFlashing
	m.cancel = func() {}
	m = press(m, "esc")
	if !strings.Contains(m.status, "partially written") {
		t.Fatal(m.status)
	}
	m.detection = seed.Detection{Family: seed.CloudInit}
}
