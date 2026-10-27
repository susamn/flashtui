package blockdev

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDescriptionAndFallbackFields(t *testing.T) {
	for _, c := range []struct {
		d    Disk
		want string
	}{
		{Disk{Model: "  Model  ", Name: "sda"}, "Model"},
		{Disk{Vendor: "  Vendor  ", Name: "sda"}, "Vendor"},
		{Disk{Name: "sda"}, "sda"},
	} {
		if got := c.d.Description(); got != c.want {
			t.Fatal(got, c.want)
		}
	}
	for _, s := range []string{"", "invalid", "-1", "18446744073709551616"} {
		if got := (lsblkNode{}).bytes(json.Number(s)); got != 0 {
			t.Fatal(s, got)
		}
	}
	if got := pathOf(lsblkNode{Name: "disk0"}); got != "/dev/disk0" {
		t.Fatal(got)
	}
	if got := baseName("sda"); got != "sda" {
		t.Fatal(got)
	}
}

func TestFindToolFallbacks(t *testing.T) {
	original := systemPaths
	systemPaths = []string{t.TempDir() + "/"}
	t.Cleanup(func() { systemPaths = original })
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if got := findTool("not-installed"); got != "not-installed" {
		t.Fatal(got)
	}
	bin := filepath.Join(dir, "my-tool")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho ok\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if got := findTool("my-tool"); got != bin {
		t.Fatal(got)
	}
	systemPaths = []string{dir + "/"}
	if got := findTool("my-tool"); got != bin {
		t.Fatal(got)
	}
}

func TestListEmptyAndFallbackPath(t *testing.T) {
	orig := runner
	t.Cleanup(func() { runner = orig })
	runner = func(string, ...string) ([]byte, error) {
		return []byte(`{"blockdevices":[{"name":"disk0","type":"disk","size":32,"children":[{"name":"disk0p1","type":"part","size":16}]}]}`), nil
	}
	ds, err := List()
	if err != nil || len(ds) != 1 || ds[0].Path != "/dev/disk0" || ds[0].Partitions[0].Path != "/dev/disk0p1" {
		t.Fatal(ds, err)
	}
	runner = func(string, ...string) ([]byte, error) { return nil, errors.New("failure") }
	if _, err := List(); err == nil {
		t.Fatal("missing runner error")
	}
	if strings.TrimSpace(ds[0].Description()) == "" {
		t.Fatal("missing description")
	}
}
