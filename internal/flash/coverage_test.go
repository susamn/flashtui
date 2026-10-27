package flash

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/susamn/flashtui/internal/imagefile"
	"github.com/susamn/flashtui/internal/privilege"
)

func TestStageScriptUsesTemporaryDirectory(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	path, cleanup, err := stageScript(Job{Target: "/dev/fake"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestPhaseNamesAndProgressBounds(t *testing.T) {
	for _, p := range []Phase{PhaseIdle, PhaseWriting, PhaseFlushing, PhaseVerifying, PhaseDone, Phase(99)} {
		if p.String() == "" {
			t.Fatal(p)
		}
	}
	for _, p := range []Progress{{Bytes: 1, Total: 2}, {Bytes: 2, Total: 2, Rate: 1}, {Bytes: 2, Total: 2, Rate: 0}} {
		_, _ = p.ETA()
	}
}

func TestWriterProcessStartFailure(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "sh")
	if err := os.WriteFile(bin, []byte("not an executable format"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	img := imageOf(t, dir, "source.img", []byte("fixture"))
	ch := make(chan Progress, 64)
	err := Run(context.Background(), privilege.Direct(), Job{Image: img, Target: filepath.Join(dir, "target"), ScratchDir: dir}, ch)
	if err == nil || !strings.Contains(err.Error(), "start writer") {
		t.Fatal(err)
	}
	for range ch {
	}
}

func TestFlashEarlyAndWriterErrors(t *testing.T) {
	dir := t.TempDir()
	img := imageOf(t, dir, "source.img", []byte("safe fixture"))
	for _, tc := range []struct {
		name string
		job  Job
		want string
	}{
		{"missing source", Job{Image: imagefile.Image{Path: filepath.Join(dir, "missing")}, Target: filepath.Join(dir, "device")}, "missing"},
		{"unwritable scratch", Job{Image: img, Target: filepath.Join(dir, "device"), ScratchDir: filepath.Join(dir, "missing")}, "stage write script"},
		{"bad target", Job{Image: img, Target: dir, ScratchDir: dir}, "write"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ch := make(chan Progress, 64)
			err := Run(context.Background(), privilege.Direct(), tc.job, ch)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v want %q", err, tc.want)
			}
			for range ch {
			}
		})
	}
	// Reader can fail after opening (truncated compressed source); the writer
	// must be reaped and the progress channel closed.
	bad := imageOf(t, dir, "bad.img.gz", []byte{0x1f, 0x8b, 0x08})
	ch := make(chan Progress, 64)
	if err := Run(context.Background(), privilege.Direct(), Job{Image: bad, Target: filepath.Join(dir, "bad.dev"), ScratchDir: dir}, ch); err == nil {
		t.Fatal("corrupt gzip accepted")
	}
	for range ch {
	}
}

func TestPumpErrorsAndReadBackShort(t *testing.T) {
	ch := make(chan Progress, 64)
	h := newHashOf(t, []byte("x"))
	if err := readBack(context.Background(), strings.NewReader("short"), 20, h, ch); err == nil || !strings.Contains(err.Error(), "expected") {
		t.Fatal(err)
	}
	if err := readBack(context.Background(), failingReader{}, 1, h, ch); err == nil {
		t.Fatal("read error hidden")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := pump(ctx, io.Discard, strings.NewReader("x"), sha256.New(), 0, PhaseWriting, ch); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := pump(context.Background(), failingWriter{}, strings.NewReader("x"), sha256.New(), 0, PhaseWriting, ch); err == nil {
		t.Fatal("write error hidden")
	}
	if _, err := pump(context.Background(), io.Discard, failingReader{}, sha256.New(), 0, PhaseWriting, ch); err == nil {
		t.Fatal("read error hidden")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestAuthenticationWatcherPartialMarker(t *testing.T) {
	count := 0
	w := &authWatcher{sink: &tailBuffer{limit: 256}, notify: func() { count++ }}
	mid := len(readyMarker) / 2
	_, _ = w.Write([]byte(readyMarker[:mid]))
	_, _ = w.Write([]byte(readyMarker[mid:]))
	w.fire()
	_, _ = w.Write([]byte("late"))
	if count != 1 || !bytes.Contains([]byte(w.sink.String()), []byte("late")) {
		t.Fatal(count, w.sink.String())
	}
	other := &authWatcher{sink: &tailBuffer{limit: 4}}
	other.fire()
	// Script file is removed after a completed job.
	dir := t.TempDir()
	img := imageOf(t, dir, "empty.img", nil)
	ch := make(chan Progress, 64)
	_ = Run(context.Background(), privilege.Direct(), Job{Image: img, Target: filepath.Join(dir, "device"), ScratchDir: dir}, ch)
	for range ch {
	}
	if _, err := os.Stat(filepath.Join(dir, "flashtui-write.sh")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
