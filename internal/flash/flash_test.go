package flash

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/susamn/flashtui/internal/imagefile"
	"github.com/susamn/flashtui/internal/privilege"
)

// These tests run the real dd pipeline against a regular file standing in for
// the block device, so the command construction, streaming, digest and verify
// comparison are all exercised end to end without root.
func init() { directRead = false }

func payload(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*31 + i/97)
	}
	return b
}

func imageOf(t *testing.T, dir, name string, data []byte) imagefile.Image {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	img, err := imagefile.Inspect(p)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func runJob(t *testing.T, job Job) (error, []Progress) {
	t.Helper()
	ch := make(chan Progress, 256)
	var got []Progress
	done := make(chan struct{})
	go func() {
		for p := range ch {
			got = append(got, p)
		}
		close(done)
	}()
	err := Run(context.Background(), privilege.Direct(), job, ch)
	<-done
	return err, got
}

func TestWriteAndVerifyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	raw := payload(9 << 20) // not a multiple of the 4 MiB block size
	img := imageOf(t, dir, "src.img", raw)
	target := filepath.Join(dir, "target.dev")

	err, prog := runJob(t, Job{Image: img, Target: target, TargetSize: 64 << 20, Verify: true, ScratchDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	on, err2 := os.ReadFile(target)
	if err2 != nil {
		t.Fatal(err2)
	}
	if !bytes.Equal(on, raw) {
		t.Fatalf("device holds %d bytes, image was %d", len(on), len(raw))
	}

	var sawWrite, sawVerify, sawDone bool
	for _, p := range prog {
		switch p.Phase {
		case PhaseWriting:
			sawWrite = true
		case PhaseVerifying:
			sawVerify = true
		case PhaseDone:
			sawDone = true
		}
	}
	if !sawWrite || !sawVerify || !sawDone {
		t.Errorf("phases: write=%v verify=%v done=%v", sawWrite, sawVerify, sawDone)
	}
}

// The compressed path is the one that actually gets used; progress must be
// measured in uncompressed bytes, not bytes read from the file.
func TestWriteDecompresses(t *testing.T) {
	dir := t.TempDir()
	raw := payload(5 << 20)
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write(raw)
	zw.Close()

	img := imageOf(t, dir, "src.img.gz", buf.Bytes())
	if img.Kind != imagefile.Gzip {
		t.Fatalf("kind=%v", img.Kind)
	}
	target := filepath.Join(dir, "target.dev")

	err, prog := runJob(t, Job{Image: img, Target: target, TargetSize: 64 << 20, Verify: true, ScratchDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	on, _ := os.ReadFile(target)
	if !bytes.Equal(on, raw) {
		t.Fatalf("decompressed badly: %d bytes want %d", len(on), len(raw))
	}
	for _, p := range prog {
		if p.Phase == PhaseWriting && p.Total != int64(len(raw)) {
			t.Fatalf("progress total=%d, want uncompressed %d", p.Total, len(raw))
		}
	}
}

// Verification exists to catch a device that did not keep what it was given.
func TestVerifyDetectsCorruption(t *testing.T) {
	dir := t.TempDir()
	raw := payload(5 << 20)
	img := imageOf(t, dir, "src.img", raw)
	target := filepath.Join(dir, "target.dev")

	if err, _ := runJob(t, Job{Image: img, Target: target, TargetSize: 64 << 20, ScratchDir: dir}); err != nil {
		t.Fatal(err)
	}
	// Flip one byte behind flash's back, the way a failing card would.
	f, err := os.OpenFile(target, os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte{raw[1234] ^ 0xFF}, 1234); err != nil {
		t.Fatal(err)
	}
	f.Close()

	// readBack compares what the device returns against the digest the write
	// accumulated from the pristine image.
	on, err := os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	defer on.Close()

	ch := make(chan Progress, 256)
	go func() {
		for range ch {
		}
	}()
	defer close(ch)

	err = readBack(context.Background(), on, int64(len(raw)), newHashOf(t, raw), ch)
	if err == nil {
		t.Fatal("verify must reject a device whose contents were altered")
	}
	if !strings.Contains(err.Error(), "differ") {
		t.Errorf("error should name the mismatch, got %v", err)
	}
}

// A clean read-back must pass, or the check above proves nothing.
func TestVerifyAcceptsIntactDevice(t *testing.T) {
	dir := t.TempDir()
	raw := payload(3 << 20)
	img := imageOf(t, dir, "src.img", raw)
	target := filepath.Join(dir, "target.dev")

	if err, _ := runJob(t, Job{Image: img, Target: target, TargetSize: 64 << 20, ScratchDir: dir}); err != nil {
		t.Fatal(err)
	}
	on, err := os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	defer on.Close()

	ch := make(chan Progress, 256)
	go func() {
		for range ch {
		}
	}()
	defer close(ch)

	if err := readBack(context.Background(), on, int64(len(raw)), newHashOf(t, raw), ch); err != nil {
		t.Fatalf("intact device rejected: %v", err)
	}
}

func TestRefusesTargetTooSmall(t *testing.T) {
	dir := t.TempDir()
	img := imageOf(t, dir, "src.img", payload(8<<20))
	target := filepath.Join(dir, "target.dev")

	err, _ := runJob(t, Job{Image: img, Target: target, TargetSize: 1 << 20, ScratchDir: dir})
	if !errors.Is(err, ErrTargetTooSmall) {
		t.Fatalf("got %v, want ErrTargetTooSmall", err)
	}
	if _, statErr := os.Stat(target); statErr == nil {
		t.Error("nothing should have been written")
	}
}

func TestCancellationStopsWrite(t *testing.T) {
	dir := t.TempDir()
	img := imageOf(t, dir, "src.img", payload(64<<20))
	target := filepath.Join(dir, "target.dev")

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan Progress, 256)
	go func() {
		for range ch {
			cancel() // abort as soon as the first sample lands
		}
	}()
	err := Run(ctx, privilege.Direct(), Job{Image: img, Target: target, TargetSize: 1 << 30, ScratchDir: dir}, ch)
	if err == nil {
		t.Fatal("want an error after cancellation")
	}
}

func TestProgressFractionAndETA(t *testing.T) {
	p := Progress{Bytes: 50, Total: 200, Rate: 10}
	if f, ok := p.Fraction(); !ok || f != 0.25 {
		t.Errorf("fraction=%v ok=%v", f, ok)
	}
	if eta, ok := p.ETA(); !ok || eta != 15*time.Second {
		t.Errorf("eta=%v ok=%v", eta, ok)
	}

	// An unknown total is the bzip2 case: the UI must fall back to an
	// indeterminate bar rather than show a bogus 0%.
	u := Progress{Bytes: 50, Total: 0, Rate: 10}
	if _, ok := u.Fraction(); ok {
		t.Error("unknown total must report ok=false")
	}
	if _, ok := u.ETA(); ok {
		t.Error("unknown total cannot yield an ETA")
	}

	// Fraction is clamped: a compressed stream can exceed a stale estimate.
	over := Progress{Bytes: 300, Total: 200}
	if f, _ := over.Fraction(); f != 1 {
		t.Errorf("fraction=%v want clamp to 1", f)
	}
}

func TestSmoothIgnoresFirstSampleBias(t *testing.T) {
	if got := smooth(0, 100); got != 100 {
		t.Errorf("first sample should pass through, got %v", got)
	}
	if got := smooth(100, 200); got <= 100 || got >= 200 {
		t.Errorf("smoothed value %v should sit between", got)
	}
}

func TestTailBufferKeepsTail(t *testing.T) {
	b := &tailBuffer{limit: 8}
	b.Write([]byte("0123456789abcdef"))
	if got := b.String(); got != "89abcdef" {
		t.Errorf("got %q want last 8 bytes", got)
	}
}

// newHashOf returns the SHA-256 of data, matching what write() accumulates.
func newHashOf(t *testing.T, data []byte) []byte {
	t.Helper()
	h := sha256.New()
	h.Write(data)
	return h.Sum(nil)
}
