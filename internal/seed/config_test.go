package seed

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// fakeDisk builds a Disk whose partitions mount to the given directories.
// A root filesystem with no Pi firmware partition is a cloud image.
// and must say why rather than silently appearing to work.
func TestValidate(t *testing.T) {
	base := Config{Username: "susamn", Password: "hunter2"}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	cases := []struct {
		name string
		mut  func(*Config)
		want string
	}{
		{"empty user", func(c *Config) { c.Username = "" }, "username is required"},
		{"uppercase user", func(c *Config) { c.Username = "Susamn" }, "lowercase"},
		{"leading digit", func(c *Config) { c.Username = "1pi" }, "lowercase"},
		{"root", func(c *Config) { c.Username = "root" }, "cannot be root"},
		{"no credentials", func(c *Config) { c.Password = "" }, "cannot be logged into"},
		{"bad hostname", func(c *Config) { c.Hostname = "-nope-" }, "DNS label"},
		{"short psk", func(c *Config) { c.WifiSSID = "net"; c.WifiPSK = "abc"; c.WifiCountry = "IN" }, "8 and 63"},
		{"no country", func(c *Config) { c.WifiSSID = "net"; c.WifiPSK = "abcdefgh"; c.WifiCountry = "" }, "country code"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.mut(&c)
			err := c.Validate()
			if err == nil {
				t.Fatalf("want error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("got %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// A pasted private key is the mistake worth catching by name.
func TestValidateRejectsPrivateKey(t *testing.T) {
	c := Config{Username: "pi", AuthorizedKey: "-----BEGIN OPENSSH PRIVATE KEY-----"}
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "private key") {
		t.Fatalf("got %v, want a private-key warning", err)
	}
}

func TestValidateKeyShapes(t *testing.T) {
	good := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIexample susamn@host"
	if err := validateKey(good); err != nil {
		t.Errorf("rejected a valid key: %v", err)
	}
	if err := validateKey("ssh-ed25519"); err == nil {
		t.Error("a type with no payload is not a key")
	}
	if err := validateKey("ssh-dss AAAA"); err == nil {
		t.Error("unsupported type should be rejected")
	}
	if err := validateKey("ssh-ed25519 AAAA\nssh-ed25519 BBBB"); err == nil {
		t.Error("multi-line input should be rejected")
	}
}

func TestShellQuoteNeutralisesInjection(t *testing.T) {
	got := shellQuote(`a'; rm -rf /; echo '`)
	if strings.Contains(got, "rm -rf /") && !strings.Contains(got, `'\''`) {
		t.Fatalf("quote did not escape: %s", got)
	}
	// Round-trip through a real shell to prove the value survives intact.
	want := `a'; rm -rf /; echo '`
	out := shOutput(t, "printf '%s' "+got)
	if out != want {
		t.Errorf("round trip got %q want %q", out, want)
	}
}

func TestYamlStringEscapes(t *testing.T) {
	if got := yamlString(`he said "hi"`); got != `"he said \"hi\""` {
		t.Errorf("got %s", got)
	}
	if got := yamlString("a\nb"); strings.Contains(got, "\n") {
		t.Errorf("newline must be escaped, got %q", got)
	}
}

func TestSanitizeFilename(t *testing.T) {
	if got := sanitizeFilename("My Wifi/../etc"); strings.ContainsAny(got, "/.") {
		t.Errorf("got %q, must not allow path traversal", got)
	}
	if got := sanitizeFilename("///"); got != "___" {
		t.Errorf("got %q, want separators replaced not dropped", got)
	}
	if got := sanitizeFilename(""); got != "wifi" {
		t.Errorf("got %q want the empty-name fallback", got)
	}
}

func TestBuildScriptRaspios(t *testing.T) {
	cfg := Config{
		Username: "susamn", AuthorizedKey: "ssh-ed25519 AAAAC3Nz susamn@host",
		Hostname: "pi5", EnableSSH: true,
	}
	det := Detection{Family: RaspberryPiOS, Boot: "/dev/sda1", Root: "/dev/sda2"}
	got, err := buildScript(det, cfg, Mounted{Boot: "/mnt/boot", Root: "/mnt/root"}, "$6$abc$def")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`touch "$BOOT/ssh"`,
		`userconf.txt`,
		`'susamn:$6$abc$def'`,
		`authorized_keys`,
		`/etc/hostname`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("script missing %q:\n%s", want, got)
		}
	}
	// The hash contains $ and must not be interpolated by the shell.
	if strings.Contains(got, `"susamn:$6$`) {
		t.Error("password hash must be single-quoted, not double")
	}
}

func TestBuildScriptCloudInit(t *testing.T) {
	cfg := Config{Username: "ubuntu", AuthorizedKey: "ssh-ed25519 AAAA x", Hostname: "box", EnableSSH: true}
	got, err := buildScript(Detection{Family: CloudInit, Root: "/dev/sda1"}, cfg, Mounted{Root: "/mnt/root"}, "$6$x$y")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"nocloud-net", "#cloud-config", "ssh_authorized_keys", "meta-data",
		`rm -rf "$ROOT/var/lib/cloud/instance"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("script missing %q:\n%s", want, got)
		}
	}
}

func TestBuildScriptRejectsUnseedable(t *testing.T) {
	_, err := buildScript(Detection{}, Config{Username: "x", Password: "y"}, Mounted{Root: "/r"}, "")
	if err == nil {
		t.Fatal("want error for an unseedable family")
	}
}

func TestApplyRefusesUnseedable(t *testing.T) {
	err := Apply(context.Background(), nil, Detection{Reason: "live ISO"},
		Config{Username: "a", Password: "b"}, Mounted{Root: "/r"}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "live ISO") {
		t.Fatalf("got %v, want the detection reason surfaced", err)
	}
}

func TestApplyValidatesBeforeTouchingDisk(t *testing.T) {
	err := Apply(context.Background(), nil, Detection{Family: CloudInit, Root: "/dev/sda1"},
		Config{Username: "ROOT"}, Mounted{Root: "/r"}, t.TempDir())
	if err == nil {
		t.Fatal("want validation error")
	}
}

// shOutput runs a shell fragment and returns its stdout, for proving that
// generated quoting behaves the way a real shell reads it.
func shOutput(t *testing.T, script string) string {
	t.Helper()
	out, err := exec.Command("sh", "-c", script).Output()
	if err != nil {
		t.Fatalf("sh: %v", err)
	}
	return string(out)
}
