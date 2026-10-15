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
		return zstdSize(f, size)
	case Xz:
		return xzIndexSize(f, size)
	default:
		// bzip2 stores no total length anywhere in the stream.
		return 0
	}
}

// gzipISize reads the trailing ISIZE field. It verifies the stream is not
// concatenated and that ISIZE has not wrapped at 4 GiB; otherwise it returns
// 0 (unknown).
func gzipISize(f *os.File, size int64) int64 {
	if size < 18 || size >= 4<<30 {
		return 0
	}
	buf := make([]byte, 4)
	if _, err := f.ReadAt(buf, size-4); err != nil {
		return 0
	}
	v := int64(binary.LittleEndian.Uint32(buf))
	if v < size || v > size*1032 {
		return 0
	}
	// Check if there are concatenated gzip members. Each member begins with
	// 0x1F, 0x8B, 0x08 (Deflate). If any other member header exists past offset 0,
	// the trailer only reflects the last member, so total size is unknown.
	if gzipHasMultipleMembers(f, size) {
		return 0
	}
	return v
}

func gzipHasMultipleMembers(f *os.File, size int64) bool {
	chunk := make([]byte, 64<<10)
	var off int64
	for off < size {
		n, err := f.ReadAt(chunk, off)
		if n <= 0 {
			break
		}
		data := chunk[:n]
		start := 0
		if off == 0 {
			start = 2 // skip the leading 0x1F, 0x8B
		}
		for i := start; i+2 < len(data); i++ {
			if data[i] == 0x1F && data[i+1] == 0x8B && data[i+2] == 0x08 {
				return true
			}
		}
		off += int64(n) - 2 // overlap window in case magic spans boundary
		if err != nil {
			break
		}
	}
	return false
}

// zstdSize scans all frames in a zstd stream to compute the total uncompressed
// size. If any frame lacks Frame_Content_Size or is corrupt, it returns 0.
func zstdSize(f *os.File, size int64) int64 {
	var pos int64
	var totalUncompressed uint64

	buf := make([]byte, 32)
	for pos < size {
		if pos+4 > size {
			return 0
		}
		if _, err := f.ReadAt(buf[:4], pos); err != nil {
			return 0
		}
		magic := binary.LittleEndian.Uint32(buf[:4])
		// Handle skippable frames: 0x184D2A50 - 0x184D2A5F
		if magic >= 0x184D2A50 && magic <= 0x184D2A5F {
			if pos+8 > size {
				return 0
			}
			if _, err := f.ReadAt(buf[:4], pos+4); err != nil {
				return 0
			}
			skipLen := int64(binary.LittleEndian.Uint32(buf[:4]))
			pos += 8 + skipLen
			continue
		}
		if magic != 0xFD2FB528 { // 0x28, 0xB5, 0x2F, 0xFD
			return 0
		}
		if pos+5 > size {
			return 0
		}
		if _, err := f.ReadAt(buf[:1], pos+4); err != nil {
			return 0
		}
		fhd := buf[0]
		if fhd&0x08 != 0 { // reserved bit must be 0
			return 0
		}
		singleSegment := fhd&0x20 != 0
		fcsFlag := fhd >> 6
		hasChecksum := fhd&0x04 != 0
		dictIDFlag := fhd & 0x03

		if fcsFlag == 0 && !singleSegment {
			return 0 // Frame_Content_Size absent
		}

		headerLen := int64(5)
		if !singleSegment {
			headerLen++ // Window_Descriptor
		}
		dictSizes := []int64{0, 1, 2, 4}
		headerLen += dictSizes[dictIDFlag]

		var fcsBytes int64
		if fcsFlag == 0 && singleSegment {
			fcsBytes = 1
		} else {
			fcsBytes = int64([]int{0, 2, 4, 8}[fcsFlag])
		}

		if pos+headerLen+fcsBytes > size {
			return 0
		}
		fcsBuf := make([]byte, fcsBytes)
		if _, err := f.ReadAt(fcsBuf, pos+headerLen); err != nil {
			return 0
		}
		var fcs uint64
		switch fcsBytes {
		case 1:
			fcs = uint64(fcsBuf[0])
		case 2:
			fcs = uint64(binary.LittleEndian.Uint16(fcsBuf)) + 256
		case 4:
			fcs = uint64(binary.LittleEndian.Uint32(fcsBuf))
		case 8:
			fcs = binary.LittleEndian.Uint64(fcsBuf)
		}
		totalUncompressed += fcs
		headerLen += fcsBytes

		// Walk blocks until last block
		blockPos := pos + headerLen
		lastBlock := false
		for !lastBlock {
			if blockPos+3 > size {
				return 0
			}
			var b [3]byte
			if _, err := f.ReadAt(b[:], blockPos); err != nil {
				return 0
			}
			lastBlock = (b[0] & 1) != 0
			blockType := (b[0] >> 1) & 0x03
			if blockType == 3 { // reserved
				return 0
			}
			blockSize := int64(b[0]>>3) | (int64(b[1]) << 5) | (int64(b[2]) << 13)
			blockPos += 3 + blockSize
		}
		if hasChecksum {
			blockPos += 4
		}
		pos = blockPos
	}

	if pos != size || totalUncompressed > 1<<62 {
		return 0
	}
	return int64(totalUncompressed)
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
