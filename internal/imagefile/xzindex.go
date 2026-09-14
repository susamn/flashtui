package imagefile

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
)

// xzIndexSize reads the uncompressed length out of an .xz stream's index.
//
// The layout walked here is, from the end of the file:
//
//	Stream Footer  = CRC32(4) | Backward Size(4) | Stream Flags(2) | "YZ"(2)
//	Index          = 0x00 | record count | { unpadded size, uncompressed size }*
//
// Backward Size stores (real size / 4) - 1, and both counters are LZMA
// multibyte integers. Only the final stream is read: concatenated .xz streams
// would undercount, so the result is discarded unless it looks sane, and the
// caller falls back to an indeterminate progress bar.
func xzIndexSize(f *os.File, size int64) int64 {
	const footerLen = 12
	if size < footerLen+2 {
		return 0
	}
	footer := make([]byte, footerLen)
	if _, err := f.ReadAt(footer, size-footerLen); err != nil {
		return 0
	}
	if !bytes.Equal(footer[10:12], []byte("YZ")) {
		return 0
	}
	backward := (int64(binary.LittleEndian.Uint32(footer[4:8])) + 1) * 4
	indexStart := size - footerLen - backward
	if indexStart < 0 || backward > 1<<20 {
		return 0
	}
	index := make([]byte, backward)
	if _, err := f.ReadAt(index, indexStart); err != nil {
		return 0
	}
	if len(index) == 0 || index[0] != 0x00 {
		return 0
	}

	r := bytes.NewReader(index[1:])
	count, err := readMultibyte(r)
	if err != nil || count == 0 || count > 1<<20 {
		return 0
	}
	var total uint64
	for i := uint64(0); i < count; i++ {
		if _, err := readMultibyte(r); err != nil { // unpadded size, unused
			return 0
		}
		uncompressed, err := readMultibyte(r)
		if err != nil {
			return 0
		}
		total += uncompressed
	}
	if total > 1<<62 {
		return 0
	}
	return int64(total)
}

// readMultibyte decodes an xz multibyte integer: seven bits per byte, little
// endian, high bit set on every byte but the last.
func readMultibyte(r io.ByteReader) (uint64, error) {
	var v uint64
	for i := 0; i < 9; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		v |= uint64(b&0x7F) << (7 * i)
		if b&0x80 == 0 {
			return v, nil
		}
	}
	return 0, io.ErrUnexpectedEOF
}
