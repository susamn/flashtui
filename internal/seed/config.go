package seed

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// Config is what the user wants seeded into the flashed image.
type Config struct {
	Username string
	// Password is plaintext as typed; it is hashed before it is written and
	// never stored anywhere in clear.
	Password string
	// AuthorizedKey is one SSH public key line.
	AuthorizedKey string
	Hostname      string
	EnableSSH     bool

	WifiSSID    string
	WifiPSK     string
	WifiCountry string // ISO 3166-1 alpha-2, required by the Pi's wifi regulatory domain
}

// userRE matches the same shape userconf-pi validates, so a name accepted here
// is not rejected on the device at first boot.
var userRE = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// hostRE is the RFC 1123 label rule.
var hostRE = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$`)

// Validate reports the first problem that would make the seeded image
// unreachable, which is the whole point of seeding it.
func (c Config) Validate() error {
	switch {
	case c.Username == "":
		return errors.New("username is required")
	case len(c.Username) > 32:
		return errors.New("username must be at most 32 characters")
	case !userRE.MatchString(c.Username):
		return errors.New("username must start with a lowercase letter and contain only lowercase letters, digits and hyphens")
	case c.Username == "root":
		return errors.New("username cannot be root")
	}
	if c.Password == "" && c.AuthorizedKey == "" {
		return errors.New("set a password or an SSH key, otherwise the account cannot be logged into")
	}
	if c.AuthorizedKey != "" {
		if err := validateKey(c.AuthorizedKey); err != nil {
			return err
		}
	}
	if c.Hostname != "" {
		if len(c.Hostname) > 63 || !hostRE.MatchString(c.Hostname) {
			return errors.New("hostname must be a single DNS label: letters, digits and hyphens, not starting or ending with a hyphen")
		}
	}
	if c.WifiSSID != "" {
		if len(c.WifiPSK) < 8 || len(c.WifiPSK) > 63 {
			return errors.New("wifi passphrase must be between 8 and 63 characters")
		}
		if len(c.WifiCountry) != 2 {
			return errors.New("wifi needs a two-letter country code to set the regulatory domain")
		}
	}
	return nil
}

// keyTypes are the public key algorithms OpenSSH will accept in an
// authorized_keys file and that are still considered current.
var keyTypes = []string{
	"ssh-ed25519", "ssh-rsa", "ecdsa-sha2-nistp256",
	"ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521",
	"sk-ssh-ed25519@openssh.com", "sk-ecdsa-sha2-nistp256@openssh.com",
}

func validateKey(k string) error {
	k = strings.TrimSpace(k)
	if strings.ContainsAny(k, "\n\r") {
		return errors.New("SSH key must be a single line")
	}
	fields := strings.Fields(k)
	if len(fields) < 2 {
		return errors.New("SSH key must look like '<type> <base64> [comment]'")
	}
	known := false
	for _, t := range keyTypes {
		if fields[0] == t {
			known = true
			break
		}
	}
	if !known {
		// A private key pasted by mistake is the failure worth naming.
		if strings.Contains(k, "PRIVATE KEY") {
			return errors.New("that is a private key; use the matching .pub file")
		}
		return fmt.Errorf("unrecognised key type %q", fields[0])
	}
	// The payload must be real base64, which catches a truncated copy-paste
	// before it becomes an authorized_keys line sshd silently ignores.
	if _, err := base64.StdEncoding.DecodeString(fields[1]); err != nil {
		return errors.New("SSH key payload is not valid base64; the key looks truncated or mangled")
	}
	return nil
}

// hasher is swapped out in tests.
var hasher = hashPassword

// hashPassword produces a SHA-512 crypt hash, the format both
// /etc/shadow and cloud-init's passwd field expect.
//
// Go has no crypt(3) in the standard library and pulling a third-party
// implementation in for one call is a poor trade, so this shells out to
// whichever of the two standard tools is installed. The plaintext goes over
// stdin, never in argv, so it does not appear in the process list.
func hashPassword(ctx context.Context, plain string) (string, error) {
	type attempt struct {
		bin  string
		args []string
	}
	for _, a := range []attempt{
		{"mkpasswd", []string{"--method=sha512crypt", "--stdin"}},
		{"openssl", []string{"passwd", "-6", "-stdin"}},
	} {
		if _, err := exec.LookPath(a.bin); err != nil {
			continue
		}
		cmd := exec.CommandContext(ctx, a.bin, a.args...)
		cmd.Stdin = strings.NewReader(plain + "\n")
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		h := strings.TrimSpace(string(out))
		if strings.HasPrefix(h, "$6$") {
			return h, nil
		}
	}
	return "", errors.New("no password hasher available: install mkpasswd (whois) or openssl")
}
