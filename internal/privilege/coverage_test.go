package privilege

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConstructorAndDirect(t *testing.T) {
	d := Direct()
	if !d.Root() {
		t.Fatal("direct escalator must not use pkexec")
	}
	if os.Geteuid() == 0 {
		got, err := New()
		if err != nil || !got.Root() {
			t.Fatal(got, err)
		}
		return
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "pkexec")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	got, err := New()
	if err != nil || got.Root() || got.pkexecPath != path {
		t.Fatal(got, err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := New(); err == nil || !strings.Contains(err.Error(), "pkexec") {
		t.Fatal(err)
	}
}
