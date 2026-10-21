// SPDX-License-Identifier: AGPL-3.0-or-later

package container

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/ulikunitz/xz/lzma"
)

const (
	blockMarker    = 0xd1edd1ed
	blockHeaderLen = 12
	lzmaPrefixLen  = 9 // u32 decompressed size + 5 LZMA properties bytes
)

// maxCompressed bounds the compressed size of a block declaring usz
// decompressed bytes. LZMA output for data that does not compress is
// barely larger than the input.
func maxCompressed(usz uint32) int64 {
	return lzmaPrefixLen + int64(usz) + int64(usz)/16 + 64
}

// errEnd marks a clean end of file. io.EOF cannot serve: the LZMA decoder
// returns it for a damaged stream, and wrapped errors keep it.
var errEnd = errors.New("end of replay")

// Block is one decompressed block.
type Block struct {
	Index          int
	Offset         int64  // file offset of the block marker
	CompressedSize uint32 // bytes after the 12-byte block header
	Data           []byte
}

// Reader reads a replay sequentially. Wrap unbuffered sources in a
// bufio.Reader; the reader issues small reads for block headers.
type Reader struct {
	r     io.Reader
	lim   Limits
	hdr   Header
	off   int64
	total int64
	block Block
	n     int
	err   error
	done  bool
}

// NewReader reads the header. Blocks are read by NextBlock.
func NewReader(r io.Reader, lim Limits) (*Reader, error) {
	lim = lim.withDefaults()
	h, err := readHeader(r, lim)
	if err != nil {
		return nil, err
	}
	return &Reader{r: r, lim: lim, hdr: h, off: int64(h.FirstBlock)}, nil
}

// Header returns the header read by NewReader.
func (r *Reader) Header() Header { return r.hdr }

// Block returns the block read by the last successful NextBlock.
func (r *Reader) Block() Block { return r.block }

// Err returns the error that ended iteration, or nil at a clean end of file.
// A file without any block is truncated.
func (r *Reader) Err() error { return r.err }

// NextBlock reads the next block and reports whether it succeeded.
func (r *Reader) NextBlock() bool {
	if r.done {
		return false
	}
	b, err := r.readBlock()
	if err != nil {
		r.done = true
		switch {
		case !errors.Is(err, errEnd):
			r.err = err
		case r.n == 0:
			r.err = errAt(ErrTruncated, r.off, "no blocks")
		}
		return false
	}
	r.block = b
	r.n++
	return true
}

// readBlock returns errEnd at a clean end of file.
func (r *Reader) readBlock() (Block, error) {
	start := r.off
	var h [blockHeaderLen]byte
	n, err := io.ReadFull(r.r, h[:])
	if n == 0 && errors.Is(err, io.EOF) {
		return Block{}, errEnd
	}
	if err != nil {
		return Block{}, readErr(err, start, "block header")
	}
	le := binary.LittleEndian
	marker, usz, csz := le.Uint32(h[0:]), le.Uint32(h[4:]), le.Uint32(h[8:])
	switch {
	case marker != blockMarker:
		return Block{}, errAt(ErrCorrupt, start, "block marker %#08x", marker)
	case int64(usz) > int64(r.lim.MaxBlock):
		return Block{}, errAt(ErrLimit, start, "block size %d > %d", usz, r.lim.MaxBlock)
	case r.total+int64(usz) > r.lim.MaxTotal:
		return Block{}, errAt(ErrLimit, start, "decompressed total above %d", r.lim.MaxTotal)
	case csz < lzmaPrefixLen:
		return Block{}, errAt(ErrCorrupt, start, "compressed size %d", csz)
	case int64(csz) > maxCompressed(usz):
		return Block{}, errAt(ErrLimit, start, "compressed size %d for %d bytes", csz, usz)
	}
	// CopyN grows the buffer with the bytes actually present, so a large
	// declared size on a short file costs nothing.
	var payload bytes.Buffer
	if _, err = io.CopyN(&payload, r.r, int64(csz)); err != nil {
		return Block{}, readErr(err, start, "block payload")
	}
	data, err := decompress(payload.Bytes(), usz, r.lim.MaxBlock)
	if err != nil {
		return Block{}, fmt.Errorf("%w at offset %d: %w", ErrCorrupt, start, err)
	}
	r.off += blockHeaderLen + int64(csz)
	r.total += int64(usz)
	return Block{Index: r.n, Offset: start, CompressedSize: csz, Data: data}, nil
}

func decompress(p []byte, usz uint32, maxBlock int) ([]byte, error) {
	if inner := binary.LittleEndian.Uint32(p); inner != usz {
		return nil, fmt.Errorf("inner size %d, block size %d", inner, usz)
	}
	// Like liblzma, refuse lc+lp above 4: the literal tables grow as
	// 0x300<<(lc+lp) and are built before any data is decoded.
	if props, err := lzma.PropertiesForCode(p[4]); err == nil && props.LC+props.LP > 4 {
		return nil, fmt.Errorf("LZMA properties %#02x: lc+lp above 4", p[4])
	}
	// Rebuild the 13-byte header of the classic .lzma format around the
	// raw stream: properties, dictionary size, decompressed size.
	var hdr [13]byte
	copy(hdr[:5], p[4:9])
	binary.LittleEndian.PutUint64(hdr[5:], uint64(usz))
	src := bufio.NewReader(io.MultiReader(bytes.NewReader(hdr[:]), bytes.NewReader(p[lzmaPrefixLen:])))
	zr, err := lzma.ReaderConfig{DictCap: max(maxBlock, lzma.MinDictCap)}.NewReader(src)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	n, err := io.Copy(&out, io.LimitReader(zr, int64(usz)+1))
	if err != nil {
		return nil, err
	}
	if n != int64(usz) {
		return nil, fmt.Errorf("decompressed %d bytes, block declares %d", n, usz)
	}
	return out.Bytes(), nil
}
