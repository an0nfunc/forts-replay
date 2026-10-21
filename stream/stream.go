// SPDX-License-Identifier: AGPL-3.0-or-later

// Package stream splits one decompressed block into records. It has no
// knowledge of what record types mean.
package stream

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	recordMarker   = 0xabababab
	keyframeMarker = 0xc0c0c0c0
)

// ErrCorrupt reports record framing that does not parse.
var ErrCorrupt = errors.New("stream: corrupt record framing")

// Record is one record. Payload aliases the block data.
type Record struct {
	Type     byte // Payload[0]
	Keyframe bool // the record has the keyframe prefix
	Offset   int  // offset of the record's first byte in the block, prefix included
	Payload  []byte
}

// Scanner iterates over the records of one block.
type Scanner struct {
	data []byte
	off  int
	rec  Record
	err  error
}

// New returns a Scanner over the records of one block.
func New(data []byte) *Scanner { return &Scanner{data: data} }

// Record returns the record read by the last successful Next.
func (s *Scanner) Record() Record { return s.rec }

// Err returns the framing error that stopped the scan, or nil.
func (s *Scanner) Err() error { return s.err }

// Next reads the next record and reports whether it succeeded.
func (s *Scanner) Next() bool {
	if s.err != nil || s.off >= len(s.data) {
		return false
	}
	start, off := s.off, s.off
	keyframe := false
	if s.u32(off) == keyframeMarker {
		keyframe = true
		off += 4
	}
	if len(s.data)-off < 8 {
		return s.failf(start, "short record header")
	}
	if m := s.u32(off); m != recordMarker {
		return s.failf(off, "record marker %#08x", m)
	}
	n := binary.LittleEndian.Uint32(s.data[off+4:])
	if n == 0 {
		return s.failf(off, "empty record")
	}
	if int64(n) > int64(len(s.data)-off-8) {
		return s.failf(off, "record length %d past end of block", n)
	}
	end := off + 8 + int(n)
	s.rec = Record{Type: s.data[off+8], Keyframe: keyframe, Offset: start, Payload: s.data[off+8 : end : end]}
	s.off = end
	return true
}

// u32 returns the value at off, or 0 when fewer than 4 bytes remain.
func (s *Scanner) u32(off int) uint32 {
	if len(s.data)-off < 4 {
		return 0
	}
	return binary.LittleEndian.Uint32(s.data[off:])
}

func (s *Scanner) failf(off int, format string, args ...any) bool {
	s.err = fmt.Errorf("%w at block offset %d: %s", ErrCorrupt, off, fmt.Sprintf(format, args...))
	return false
}
