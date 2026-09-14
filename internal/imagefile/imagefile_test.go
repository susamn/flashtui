package imagefile

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// payload is deliberately incompressible-ish but repetitive enough that every
// codec shrinks it, so a wrapped gzip ISIZE would be caught.
func payload(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*7 + i/251)
	}
	return b
}

func writeFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// compressWith shells out so the fixtures come from the same encoders users
// will actually feed in. The test skips when the tool is absent.
func compressWith(t *testing.T, tool string, args []string, dir, name string, data []byte) string {
	t.Helper()
	if _, err := exec.LookPath(tool); err != nil {
		t.Skipf("%s not installed", tool)
	}
	cmd := exec.Command(tool, args...)
	cmd.Stdin = bytes.NewReader(data)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	return writeFile(t, dir, name, out)
}

// compressFile is compressWith for encoders that need a real input file to
// record the uncompressed length in their header.
func compressFile(t *testing.T, tool string, args []string, dir, name string) string {
	t.Helper()
	if _, err := exec.LookPath(tool); err != nil {
		t.Skipf("%s not installed", tool)
	}
	out, err := exec.Command(tool, args...).Output()
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	return writeFile(t, dir, name, out)
}

func TestDetectAndExpand(t *testing.T) {
	dir := t.TempDir()
	const n = 3 << 20
	raw := payload(n)

	t.Run("plain", func(t *testing.T) {
		img, err := Inspect(writeFile(t, dir, "a.img", raw))
		if err != nil {
			t.Fatal(err)
		}
		if img.Kind != Plain || img.Expanded != n {
			t.Fatalf("kind=%v expanded=%d", img.Kind, img.Expanded)
		}
	})

	t.Run("gzip", func(t *testing.T) {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		zw.Write(raw)
		zw.Close()
		img, err := Inspect(writeFile(t, dir, "b.img.gz", buf.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		if img.Kind != Gzip {
			t.Fatalf("kind=%v", img.Kind)
		}
		if img.Expanded != n {
			t.Errorf("expanded=%d want %d", img.Expanded, n)
		}
		assertStream(t, img, raw)
	})

	t.Run("xz", func(t *testing.T) {
		p := compressWith(t, "xz", []string{"-T0", "-1", "-c"}, dir, "c.img.xz", raw)
		img, err := Inspect(p)
		if err != nil {
			t.Fatal(err)
		}
		if img.Kind != Xz {
			t.Fatalf("kind=%v", img.Kind)
		}
		if img.Expanded != n {
			t.Errorf("xz index expanded=%d want %d", img.Expanded, n)
		}
		assertStream(t, img, raw)
	})

	t.Run("zstd", func(t *testing.T) {
		// zstd only records Frame_Content_Size when it knows the input length,
		// which means compressing a file rather than a stdin stream.
		src := writeFile(t, dir, "d-src.bin", raw)
		p := compressFile(t, "zstd", []string{"-q", "-c", src}, dir, "d.img.zst")
		img, err := Inspect(p)
		if err != nil {
			t.Fatal(err)
		}
		if img.Kind != Zstd {
			t.Fatalf("kind=%v", img.Kind)
		}
		if img.Expanded != n {
			t.Errorf("zstd expanded=%d want %d", img.Expanded, n)
		}
		assertStream(t, img, raw)
	})

	t.Run("bzip2 size is unknown", func(t *testing.T) {
		p := compressWith(t, "bzip2", []string{"-c"}, dir, "e.img.bz2", raw)
		img, err := Inspect(p)
		if err != nil {
			t.Fatal(err)
		}
		if img.Kind != Bzip2 {
			t.Fatalf("kind=%v", img.Kind)
		}
		if img.ExpandedKnown() {
			t.Error("bzip2 records no length; must report unknown")
		}
		if img.WriteSize() != 0 {
			t.Errorf("WriteSize=%d want 0 for indeterminate", img.WriteSize())
		}
		assertStream(t, img, raw)
	})
}

func assertStream(t *testing.T, img Image, want []byte) {
	t.Helper()
	rc, err := img.Reader()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("stream mismatch: got %d bytes want %d", len(got), len(want))
	}
}

// Magic bytes win over the extension, so a mislabelled download still flashes.
func TestDetectIgnoresExtension(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte("hello"))
	zw.Close()
	img, err := Inspect(writeFile(t, dir, "lying.img", buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if img.Kind != Gzip {
		t.Fatalf("kind=%v, want Gzip despite .img name", img.Kind)
	}
}

func TestScanFiltersAndSorts(t *testing.T) {
	dir := t.TempDir()
	raw := payload(1024)
	for _, n := range []string{"b.img", "a.iso", "notes.pdf", "movie.mp4", "c.img.xz"} {
		writeFile(t, dir, n, raw)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	imgs, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, i := range imgs {
		names = append(names, i.Name)
	}
	want := []string{"a.iso", "b.img", "c.img.xz"}
	if len(names) != len(want) {
		t.Fatalf("got %v want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("got %v want %v", names, want)
		}
	}
}

func TestScanMissingDir(t *testing.T) {
	if _, err := Scan(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("want error for missing dir")
	}
}
