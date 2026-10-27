package main

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"testing"
)

func TestVersionAndUsage(t *testing.T) {
	oldArgs, oldFlags, oldStdout := os.Args, flag.CommandLine, os.Stdout
	t.Cleanup(func() { os.Args, flag.CommandLine, os.Stdout = oldArgs, oldFlags, oldStdout })
	os.Args = []string{"flashtui", "-version"}
	flag.CommandLine = flag.NewFlagSet("flashtui", flag.ContinueOnError)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	main()
	_ = w.Close()
	output, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil || !strings.Contains(string(output), "flashtui ") {
		t.Fatalf("version: %s, %v", output, err)
	}
	if flag.CommandLine.Usage == nil {
		t.Fatal("usage not set")
	}
	flag.Usage()
}

func TestRunAndErrorReporting(t *testing.T) {
	oldArgs, oldFlags, oldStderr, oldRunner, oldExit := os.Args, flag.CommandLine, os.Stderr, runTUI, exit
	t.Cleanup(func() {
		os.Args, flag.CommandLine, os.Stderr, runTUI, exit = oldArgs, oldFlags, oldStderr, oldRunner, oldExit
	})
	os.Args = []string{"flashtui", "-dir", "/fixture"}
	flag.CommandLine = flag.NewFlagSet("flashtui", flag.ContinueOnError)
	var chosen string
	runTUI = func(dir string) error { chosen = dir; return nil }
	main()
	if chosen != "/fixture" {
		t.Fatal(chosen)
	}
	os.Args = []string{"flashtui", "-dir", "/missing"}
	flag.CommandLine = flag.NewFlagSet("flashtui", flag.ContinueOnError)
	runTUI = func(string) error { return errors.New("cannot launch") }
	code := 0
	exit = func(n int) { code = n }
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	main()
	_ = w.Close()
	var out bytes.Buffer
	_, err = out.ReadFrom(r)
	_ = r.Close()
	if err != nil || code != 1 || !strings.Contains(out.String(), "cannot launch") {
		t.Fatal(code, out.String(), err)
	}
}
