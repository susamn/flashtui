// Package imagefile discovers OS image files and opens them as a plain byte
// stream regardless of how they are compressed.
package imagefile

import (
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// Kind is the container a disk image arrives in.
type Kind int

const (
	Plain Kind = iota
	Gzip
	Xz
	Zstd
	Bzip2
)

func (k Kind) String() string {
	switch k {
	case Gzip:
		return "gzip"
	case Xz:
		return "xz"
	case Zstd:
		return "zstd"
	case Bzip2:
		return "bzip2"
	default:
		return "raw"
	}
}

// Image is one candidate image file on disk.
type Image struct {
	Path string
	Name string
	// Size is the on-disk (possibly compressed) size in bytes.
	Size int64
	Kind Kind
	// Expanded is the number of bytes that will actually be written to the
	// target. Zero means the container does not record it (bzip2 never does,
	// and a truncated or streamed file may not either) and progress must run
	// without a percentage.
	Expanded int64
}

// ExpandedKnown reports whether a percentage can be shown for this image.
func (i Image) ExpandedKnown() bool { return i.Expanded > 0 }

// WriteSize returns the number of bytes the flash will write, falling back to
// the on-disk size for uncompressed images.
func (i Image) WriteSize() int64 {
	if i.Expanded > 0 {
		return i.Expanded
	}
	if i.Kind == Plain {
		return i.Size
	}
	return 0
}

// imageExts are the suffixes worth showing, after any compression suffix is
// stripped.
var imageExts = map[string]bool{
	".img": true, ".iso": true, ".raw": true, ".bin": true, ".wic": true,
}

// Scan lists the image files directly inside dir, newest first. Directories are
// not descended into: image folders are flat in practice and recursing into a
// Downloads tree is slow and noisy.
func Scan(dir string) ([]Image, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []Image
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !looksLikeImage(e.Name()) {
			continue
		}
		full := filepath.Join(dir, e.Name())
		img, err := Inspect(full)
		if err != nil {
			// An unreadable or truncated file should not take down the whole
			// listing; skip it.
			continue
		}
		out = append(out, img)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out, nil
}

// looksLikeImage filters by name only. Inspect confirms the real kind by magic
// bytes, so a misnamed file is still handled correctly once selected.
func looksLikeImage(name string) bool {
	lower := strings.ToLower(name)
	for _, c := range []string{".xz", ".gz", ".zst", ".zstd", ".bz2"} {
		lower = strings.TrimSuffix(lower, c)
	}
	return imageExts[filepath.Ext(lower)]
}

// Inspect reads the file's magic bytes and, where the container records it, the
// uncompressed size.
func Inspect(path string) (Image, error) {
	st, err := os.Stat(path)
	if err != nil {
		return Image{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return Image{}, err
	}
	defer f.Close()

	head := make([]byte, 18)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return Image{}, err
	}
	head = head[:n]

	img := Image{
		Path: path,
		Name: filepath.Base(path),
		Size: st.Size(),
		Kind: detect(head),
	}
	img.Expanded = expandedSize(f, img.Kind, st.Size(), head)
	return img, nil
}

func detect(head []byte) Kind {
	switch {
	case bytes.HasPrefix(head, []byte{0xFD, '7', 'z', 'X', 'Z', 0x00}):
		return Xz
	case bytes.HasPrefix(head, []byte{0x1F, 0x8B}):
		return Gzip
	case bytes.HasPrefix(head, []byte{0x28, 0xB5, 0x2F, 0xFD}):
		return Zstd
	case bytes.HasPrefix(head, []byte("BZh")):
		return Bzip2
	default:
		return Plain
	}
}

// expandedSize returns the uncompressed byte count, or 0 when the container
// does not record one. f's offset is not preserved.
func expandedSize(f *os.File, k Kind, size int64, head []byte) int64 {
	switch k {
	case Plain:
		return size
	case Gzip:
		return gzipISize(f, size)
	case Zstd:
		return zstdContentSize(head)
	case Xz:
		return xzIndexSize(f, size)
	default:
		// bzip2 stores no total length anywhere in the stream.
		return 0
	}
}

// gzipISize reads the trailing ISIZE field. It is the uncompressed length mod
// 2^32, so it is only trustworthy below 4 GiB; larger images report a wrapped
// value and are treated as unknown.
func gzipISize(f *os.File, size int64) int64 {
	if size < 18 {
		return 0
	}
	buf := make([]byte, 4)
	if _, err := f.ReadAt(buf, size-4); err != nil {
		return 0
	}
	v := int64(binary.LittleEndian.Uint32(buf))
	// A plausible image compresses to less than its own size; if the recorded
	// value is smaller than the compressed file, ISIZE almost certainly wrapped.
	if v < size {
		return 0
	}
	return v
}

// zstdContentSize reads Frame_Content_Size out of the frame header when the
// producer chose to record it.
func zstdContentSize(head []byte) int64 {
	if len(head) < 6 {
		return 0
	}
	fhd := head[4]
	fcsFlag := fhd >> 6
	singleSegment := fhd&0x20 != 0
	if fcsFlag == 0 && !singleSegment {
		return 0 // field absent
	}
	off := 5
	if !singleSegment {
		off++ // skip Window_Descriptor
	}
	// Dictionary_ID_Flag widens the header; those frames are not produced by
	// image tooling, so bail rather than guess.
	if fhd&0x03 != 0 {
		return 0
	}
	sizes := []int{1, 2, 4, 8}
	n := sizes[fcsFlag]
	if off+n > len(head) {
		return 0
	}
	b := head[off : off+n]
	switch n {
	case 1:
		return int64(b[0])
	case 2:
		return int64(binary.LittleEndian.Uint16(b)) + 256
	case 4:
		return int64(binary.LittleEndian.Uint32(b))
	default:
		v := binary.LittleEndian.Uint64(b)
		if v > 1<<62 {
			return 0
		}
		return int64(v)
	}
}

// Reader opens the image and returns a stream of uncompressed bytes. The
// returned closer also closes the underlying file.
func (i Image) Reader() (io.ReadCloser, error) {
	f, err := os.Open(i.Path)
	if err != nil {
		return nil, err
	}
	switch i.Kind {
	case Plain:
		return f, nil
	case Gzip:
		zr, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("gzip: %w", err)
		}
		return chain{zr, []io.Closer{zr, f}}, nil
	case Xz:
		xr, err := xz.NewReader(f)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("xz: %w", err)
		}
		return chain{xr, []io.Closer{f}}, nil
	case Zstd:
		zr, err := zstd.NewReader(f)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("zstd: %w", err)
		}
		return chain{zr.IOReadCloser(), []io.Closer{zr.IOReadCloser(), f}}, nil
	case Bzip2:
		return chain{bzip2.NewReader(f), []io.Closer{f}}, nil
	}
	f.Close()
	return nil, fmt.Errorf("unsupported image kind %v", i.Kind)
}

// chain pairs a reader with every closer that has to run when it is done.
type chain struct {
	r       io.Reader
	closers []io.Closer
}

func (c chain) Read(p []byte) (int, error) { return c.r.Read(p) }

func (c chain) Close() error {
	var first error
	for _, cl := range c.closers {
		if err := cl.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
