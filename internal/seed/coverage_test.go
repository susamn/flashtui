package seed

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/susamn/flashtui/internal/privilege"
)

func TestHashPasswordFallbacks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if _, err := hashPassword(context.Background(), "secret"); err == nil {
		t.Fatal("missing hasher accepted")
	}
	tool := filepath.Join(dir, "openssl")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nread input\n[ \"$input\" = secret ] || exit 1\necho '$6$fixture$hash'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if got, err := hashPassword(context.Background(), "secret"); err != nil || got != "$6$fixture$hash" {
		t.Fatal(got, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mkpasswd"), []byte("#!/bin/sh\necho invalid\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if got, err := hashPassword(context.Background(), "secret"); err != nil || got != "$6$fixture$hash" {
		t.Fatal(got, err)
	}
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := hashPassword(context.Background(), "secret"); err == nil {
		t.Fatal("bad hash accepted")
	}
}

func TestApplyRefusesInvalidAndMissingMounts(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{Username: "pi", Password: "secret"}
	det := Detection{Family: RaspberryPiOS}
	if err := Apply(context.Background(), privilege.Direct(), det, cfg, Mounted{}, dir); err == nil {
		t.Fatal("missing root accepted")
	}
	if err := Apply(context.Background(), privilege.Direct(), det, cfg, Mounted{Root: dir}, dir); err == nil {
		t.Fatal("missing boot accepted")
	}
	cfg.Password = ""
	if err := Apply(context.Background(), privilege.Direct(), det, cfg, Mounted{Boot: dir, Root: dir}, dir); err == nil {
		t.Fatal("invalid config accepted")
	}
	for _, f := range []Family{RaspberryPiOS, CloudInit, Unseedable} {
		if f.String() == "" {
			t.Fatal(f)
		}
	}
	if _, err := buildScript(Detection{Family: Unseedable}, cfg, Mounted{Root: dir}, ""); err == nil {
		t.Fatal("unsupported family accepted")
	}
	if !strings.Contains(Detection{Reason: "unknown"}.Reason, "unknown") {
		t.Fatal("reason lost")
	}
}
