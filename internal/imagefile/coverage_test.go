package imagefile

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

func TestReadersWithoutExternalEncoders(t *testing.T) {
	dir := t.TempDir()
	const text = "fixture bytes"
	var xzData bytes.Buffer
	xzWriter, err := xz.NewWriter(&xzData)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := xzWriter.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
	if err := xzWriter.Close(); err != nil {
		t.Fatal(err)
	}
	zstdWriter, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	zstdData := zstdWriter.EncodeAll([]byte(text), nil)
	zstdWriter.Close()
	bzData, err := base64.StdEncoding.DecodeString("QlpoOTFBWSZTWRkxZT0AAACBAAJEoAAhmmgzTQczi7kinChIDJiynoA=")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"a.img.xz", xzData.Bytes(), text}, {"b.img.zst", zstdData, text}, {"c.img.bz2", bzData, "hello"},
	} {
		img, err := Inspect(writeFile(t, dir, tc.name, tc.data))
		if err != nil {
			t.Fatal(err)
		}
		reader, err := img.Reader()
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if err := reader.Close(); err != nil {
			t.Fatal(err)
		}
		if string(got) != tc.want {
			t.Fatalf("%s: %q", tc.name, got)
		}
	}
}

func TestKindAndReaderFailures(t *testing.T) {
	for _, tc := range []struct {
		kind Kind
		want string
	}{{Plain, "raw"}, {Gzip, "gzip"}, {Xz, "xz"}, {Zstd, "zstd"}, {Bzip2, "bzip2"}, {Kind(100), "raw"}} {
		if got := tc.kind.String(); got != tc.want {
			t.Fatal(got)
		}
	}
	dir := t.TempDir()
	for _, tc := range []struct {
		kind Kind
		data string
	}{{Gzip, "\x1f\x8bgarbage"}, {Xz, "\xfd7zXZ\x00garbage"}, {Zstd, "\x28\xb5\x2f\xfdgarbage"}, {Kind(99), "arbitrary"}} {
		path := writeFile(t, dir, "bad.img", []byte(tc.data))
		r, err := (Image{Path: path, Kind: tc.kind}).Reader()
		if err == nil {
			if r != nil {
				_ = r.Close()
			}
			if tc.kind != Zstd {
				t.Fatalf("invalid %v accepted", tc.kind)
			}
		}
	}
	if _, err := (Image{Path: filepath.Join(dir, "gone"), Kind: Plain}).Reader(); err == nil {
		t.Fatal("missing file opened")
	}
	if _, err := Inspect(filepath.Join(dir, "gone")); err == nil {
		t.Fatal("inspected missing file")
	}
	if _, err := Inspect(dir); err == nil {
		t.Fatal("inspected directory")
	}
	if got := (Image{Kind: Plain, Size: 10}).WriteSize(); got != 10 {
		t.Fatal(got)
	}
	if got := (Image{Kind: Gzip, Expanded: 42}).WriteSize(); got != 42 {
		t.Fatal(got)
	}
	for _, name := range []string{"disk.img", "disk.iso", "disk.img.gz", "disk.raw.zst"} {
		if !looksLikeImage(name) {
			t.Fatal(name)
		}
	}
	if looksLikeImage("README.txt") {
		t.Fatal("non-image accepted")
	}
	c := chain{r: strings.NewReader("x"), closers: []io.Closer{failingCloser{}, failingCloser{}}}
	if err := c.Close(); err == nil {
		t.Fatal("close failure hidden")
	}
}

type failingCloser struct{}

func (failingCloser) Close() error { return io.ErrClosedPipe }

func TestContainerHeaderLimits(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "short", make([]byte, 20))
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if gzipISize(f, 4) != 0 {
		t.Fatal("short gzip length")
	}
	buf := make([]byte, 18)
	copy(buf[0:4], []byte{0x28, 0xb5, 0x2f, 0xfd})
	for _, h := range [][]byte{nil, buf[:5], append([]byte{}, buf...)} {
		_ = zstdContentSize(h)
	}
	buf[4] = 0x20
	buf[5] = 7
	if got := zstdContentSize(buf); got != 7 {
		t.Fatal(got)
	}
	buf[4] = 0x21
	if got := zstdContentSize(buf); got != 0 {
		t.Fatal(got)
	}
	buf[4] = 0x40
	buf[5] = 0
	binary.LittleEndian.PutUint16(buf[6:], 12)
	if got := zstdContentSize(buf); got != 268 {
		t.Fatal(got)
	}
	for _, raw := range [][]byte{{}, {0x80}, bytes.Repeat([]byte{0x80}, 9), {0xff, 0x01}} {
		_, _ = readMultibyte(bytes.NewReader(raw))
	}
}

func TestScanSkipsDanglingImageAndInvalidGzipTrailer(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, "bad.img")); err != nil {
		t.Fatal(err)
	}
	imgs, err := Scan(dir)
	if err != nil || len(imgs) != 0 {
		t.Fatal(imgs, err)
	}
	path := writeFile(t, dir, "small.gz", make([]byte, 18))
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if got := gzipISize(f, 100); got != 0 {
		t.Fatal(got)
	}
}

func TestZstdHeaderBranches(t *testing.T) {
	buf := make([]byte, 18)
	copy(buf, []byte{0x28, 0xb5, 0x2f, 0xfd})
	buf[4] = 0x20
	buf[5] = 5
	if got := zstdContentSize(buf); got != 5 {
		t.Fatal(got)
	}
	buf[4] = 0x80
	buf[5] = 0
	binary.LittleEndian.PutUint32(buf[6:], 5000)
	if got := zstdContentSize(buf); got != 5000 {
		t.Fatal(got)
	}
	buf[4] = 0xe0
	binary.LittleEndian.PutUint64(buf[5:], 12345)
	if got := zstdContentSize(buf); got != 12345 {
		t.Fatal(got)
	}
	binary.LittleEndian.PutUint64(buf[5:], 1<<63)
	if got := zstdContentSize(buf); got != 0 {
		t.Fatal(got)
	}
	buf[4] = 0xe0
	if got := zstdContentSize(buf[:6]); got != 0 {
		t.Fatal(got)
	}
}

func TestXZIndexParsingBranches(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name  string
		index []byte
		want  int64
	}{
		{"one record", []byte{0, 1, 1, 5, 0, 0, 0, 0}, 5},
		{"wrong indicator", []byte{1, 1, 1, 5, 0, 0, 0, 0}, 0},
		{"zero records", []byte{0, 0, 1, 5, 0, 0, 0, 0}, 0},
		{"missing size", []byte{0, 1, 0x80, 0x80}, 0},
		{"missing length", []byte{0, 1, 1, 0x80}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			footer := make([]byte, 12)
			binary.LittleEndian.PutUint32(footer[4:], uint32(len(tc.index)/4-1))
			copy(footer[10:], "YZ")
			data := append(append([]byte{}, tc.index...), footer...)
			path := writeFile(t, dir, "index.xz", data)
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if got := xzIndexSize(f, int64(len(data))); got != tc.want {
				t.Fatalf("got %d want %d", got, tc.want)
			}
		})
	}
}

func TestXZInvalidIndexes(t *testing.T) {
	dir := t.TempDir()
	for _, b := range [][]byte{make([]byte, 10), make([]byte, 30), append(make([]byte, 20), []byte("YZ")...)} {
		path := writeFile(t, dir, "bad.xz", b)
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := xzIndexSize(f, int64(len(b))); got != 0 {
			t.Fatal(got)
		}
		_ = f.Close()
	}
}
