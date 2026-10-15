package imagefile

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
)

// xzIndexSize reads the uncompressed length out of an .xz stream's index.
// It iterates backwards through every stream in the file so that concatenated
// .xz files have their uncompressed lengths summed accurately.
// If any stream is corrupt, truncated, or if the streams do not account for
// the full file, it returns 0 (unknown).
func xzIndexSize(f *os.File, size int64) int64 {
	const footerLen = 12
	pos := size
	var totalUncompressed uint64

	for pos > 0 {
		// 1. Skip 4-byte null padding between streams or at end of file.
		for pos >= 4 {
			var pad [4]byte
			if _, err := f.ReadAt(pad[:], pos-4); err != nil {
				return 0
			}
			if pad != [4]byte{0, 0, 0, 0} {
				break
			}
			pos -= 4
		}
		if pos < footerLen*2 { // Must fit at least stream header and footer
			return 0
		}

		// 2. Read Stream Footer (12 bytes)
		footer := make([]byte, footerLen)
		if _, err := f.ReadAt(footer, pos-footerLen); err != nil {
			return 0
		}
		if !bytes.Equal(footer[10:12], []byte("YZ")) {
			return 0
		}

		backward := (int64(binary.LittleEndian.Uint32(footer[4:8])) + 1) * 4
		indexStart := pos - footerLen - backward
		if indexStart < footerLen || backward > 1<<20 {
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

		var streamUncompressed uint64
		var streamBlocksPaddedSize uint64
		for i := uint64(0); i < count; i++ {
			unpadded, err := readMultibyte(r)
			if err != nil {
				return 0
			}
			uncompressed, err := readMultibyte(r)
			if err != nil {
				return 0
			}
			streamUncompressed += uncompressed
			padded := (unpadded + 3) & ^uint64(3)
			streamBlocksPaddedSize += padded
		}

		if streamUncompressed > 1<<62 || totalUncompressed+streamUncompressed > 1<<62 {
			return 0
		}
		totalUncompressed += streamUncompressed

		// 3. Verify stream header at streamStart
		streamStart := indexStart - int64(streamBlocksPaddedSize) - footerLen
		if streamStart < 0 {
			return 0
		}
		header := make([]byte, footerLen)
		if _, err := f.ReadAt(header, streamStart); err != nil {
			return 0
		}
		if !bytes.Equal(header[:6], []byte{0xFD, '7', 'z', 'X', 'Z', 0x00}) {
			return 0
		}
		if !bytes.Equal(header[6:8], footer[8:10]) {
			return 0
		}

		pos = streamStart
	}

	if pos != 0 || totalUncompressed > 1<<62 {
		return 0
	}
	return int64(totalUncompressed)
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
