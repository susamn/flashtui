package seed

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/susamn/flashtui/internal/privilege"
)

// These run the generated script for real, against directories standing in for
// the mounted partitions, so the shell quoting and file layout are exercised
// rather than just pattern-matched.
//
// The script chowns seeded files to uid 1000. That is permitted without root
// only when the test itself runs as uid 1000, so the test skips elsewhere
// rather than reporting a failure that says nothing about the code.
func requireSeedableUID(t *testing.T) {
	t.Helper()
	if uid := os.Getuid(); uid != 0 && uid != 1000 {
		t.Skipf("script chowns to uid 1000; running as %d", uid)
	}
}

func fakeHasher(t *testing.T) {
	t.Helper()
	orig := hasher
	hasher = func(context.Context, string) (string, error) {
		return "$6$testsalt$testhashvalue", nil
	}
	t.Cleanup(func() { hasher = orig })
}

func TestApplyRaspiosWritesEverything(t *testing.T) {
	requireSeedableUID(t)
	fakeHasher(t)

	boot, root := makeRaspiBoot(t), makeLinuxRoot(t)
	cfg := Config{
		Username:      "susamn",
		Password:      "correct horse battery staple",
		AuthorizedKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIexample susamn@arch",
		Hostname:      "pi5",
		EnableSSH:     true,
		WifiSSID:      "Home Net",
		WifiPSK:       "supersecret",
		WifiCountry:   "in",
	}
	det := Detection{Family: RaspberryPiOS, Boot: "/dev/sda1", Root: "/dev/sda2"}

	err := Apply(context.Background(), privilege.Direct(), det, cfg,
		Mounted{Boot: boot, Root: root}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(boot, "ssh")); err != nil {
		t.Error("ssh marker not created; sshd would stay off")
	}

	uc := readFile(t, filepath.Join(boot, "userconf.txt"))
	if want := "susamn:$6$testsalt$testhashvalue\n"; uc != want {
		t.Errorf("userconf.txt = %q want %q", uc, want)
	}

	// The stock home is /home/pi even though the user is susamn: userconf's
	// 'usermod -m -d' moves it during the rename on first boot.
	ak := readFile(t, filepath.Join(root, "home", "pi", ".ssh", "authorized_keys"))
	if !strings.Contains(ak, "AAAAC3NzaC1lZDI1NTE5AAAAIexample") {
		t.Errorf("authorized_keys = %q", ak)
	}
	assertMode(t, filepath.Join(root, "home", "pi", ".ssh"), 0o700)
	assertMode(t, filepath.Join(root, "home", "pi", ".ssh", "authorized_keys"), 0o600)

	if got := readFile(t, filepath.Join(root, "etc", "hostname")); got != "pi5\n" {
		t.Errorf("hostname = %q", got)
	}
	if hosts := readFile(t, filepath.Join(root, "etc", "hosts")); !strings.Contains(hosts, "pi5") {
		t.Errorf("/etc/hosts not updated, sudo will stall: %q", hosts)
	}

	nm := filepath.Join(root, "etc", "NetworkManager", "system-connections", "Home_Net.nmconnection")
	prof := readFile(t, nm)
	if !strings.Contains(prof, "ssid=Home Net") || !strings.Contains(prof, "psk=supersecret") {
		t.Errorf("wifi profile = %q", prof)
	}
	// NetworkManager silently ignores a keyfile that is not 0600.
	assertMode(t, nm, 0o600)
}

func TestApplyCloudInitWritesSeed(t *testing.T) {
	requireSeedableUID(t)
	fakeHasher(t)

	root := makeLinuxRoot(t)
	cfg := Config{
		Username:      "ubuntu",
		Password:      "pw",
		AuthorizedKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIexample susamn@arch",
		Hostname:      "cloudbox",
		EnableSSH:     true,
	}
	err := Apply(context.Background(), privilege.Direct(),
		Detection{Family: CloudInit, Root: "/dev/sda1"}, cfg, Mounted{Root: root}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	seedDir := filepath.Join(root, "var", "lib", "cloud", "seed", "nocloud-net")
	ud := readFile(t, filepath.Join(seedDir, "user-data"))
	for _, want := range []string{"#cloud-config", "name: \"ubuntu\"", "hostname: \"cloudbox\"", "ssh_authorized_keys"} {
		if !strings.Contains(ud, want) {
			t.Errorf("user-data missing %q:\n%s", want, ud)
		}
	}
	// meta-data must exist or the NoCloud datasource skips the directory.
	if md := readFile(t, filepath.Join(seedDir, "meta-data")); !strings.Contains(md, "instance-id") {
		t.Errorf("meta-data = %q", md)
	}
	assertMode(t, filepath.Join(seedDir, "user-data"), 0o600)
}

// A password or SSID containing shell metacharacters must land verbatim, not
// be executed.
func TestApplySurvivesHostileInput(t *testing.T) {
	requireSeedableUID(t)
	fakeHasher(t)

	root := makeLinuxRoot(t)
	canary := filepath.Join(t.TempDir(), "pwned")
	cfg := Config{
		Username:      "ubuntu",
		Password:      "pw",
		AuthorizedKey: "ssh-ed25519 AAAA'; touch " + canary + "; echo '",
		Hostname:      "box",
	}
	// Validation rejects this before quoting even matters: the payload is not
	// valid base64 once the injected quote is in it.
	if err := Apply(context.Background(), privilege.Direct(),
		Detection{Family: CloudInit, Root: "/dev/sda1"}, cfg, Mounted{Root: root}, t.TempDir()); err == nil {
		t.Fatal("malformed key should have been rejected")
	}
	if _, err := os.Stat(canary); err == nil {
		t.Fatal("injected command executed")
	}

	// Now the same metacharacters somewhere validation does allow them.
	cfg.AuthorizedKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIexample x"
	cfg.WifiSSID = "net'; touch " + canary + "; echo '"
	cfg.WifiPSK = "abcdefghij"
	cfg.WifiCountry = "IN"
	if err := Apply(context.Background(), privilege.Direct(),
		Detection{Family: CloudInit, Root: "/dev/sda1"}, cfg, Mounted{Root: root}, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(canary); err == nil {
		t.Fatal("injected command executed via SSID")
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func assertMode(t *testing.T, p string, want os.FileMode) {
	t.Helper()
	st, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat %s: %v", p, err)
	}
	if got := st.Mode().Perm(); got != want {
		t.Errorf("%s mode = %04o want %04o", p, got, want)
	}
}
