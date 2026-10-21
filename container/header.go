// SPDX-License-Identifier: AGPL-3.0-or-later

package container

import (
	"bytes"
	"encoding/binary"
	"io"
)

const fixedHeaderLen = 17

// Header is the part of the file before the first block. Fields named
// Unknown are kept until their meaning is known; Raw holds every header
// byte, including the fixed fields.
type Header struct {
	Version    uint32 // 1383 in game v1.38.2; candidate format or build version
	Unknown4   uint8
	Frames     uint32 // total physics frames at 25 Hz
	Unknown9   uint32
	FirstBlock uint32 // offset of the first block, which is also the header length
	Raw        []byte
}

func readHeader(r io.Reader, lim Limits) (Header, error) {
	fixed := make([]byte, fixedHeaderLen)
	if _, err := io.ReadFull(r, fixed); err != nil {
		return Header{}, readErr(err, 0, "header")
	}
	le := binary.LittleEndian
	h := Header{
		Version:    le.Uint32(fixed[0:]),
		Unknown4:   fixed[4],
		Frames:     le.Uint32(fixed[5:]),
		Unknown9:   le.Uint32(fixed[9:]),
		FirstBlock: le.Uint32(fixed[13:]),
	}
	switch {
	case h.FirstBlock < fixedHeaderLen:
		return Header{}, errAt(ErrCorrupt, 13, "first block offset %d", h.FirstBlock)
	case int64(h.FirstBlock) > int64(lim.MaxHeader):
		return Header{}, errAt(ErrLimit, 13, "header length %d > %d", h.FirstBlock, lim.MaxHeader)
	}
	// CopyN grows the buffer with the bytes actually present, as for block
	// payloads.
	raw := bytes.NewBuffer(fixed)
	if _, err := io.CopyN(raw, r, int64(h.FirstBlock)-fixedHeaderLen); err != nil {
		return Header{}, readErr(err, fixedHeaderLen, "header")
	}
	h.Raw = raw.Bytes()
	return h, nil
}
