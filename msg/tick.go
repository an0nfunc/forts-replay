// SPDX-License-Identifier: AGPL-3.0-or-later

package msg

import (
	"encoding/binary"
	"fmt"
)

const (
	// TypeTick starts a two-frame step.
	TypeTick byte = 0xbe
	// TypeSync follows each tick and carries the step's frame.
	TypeSync byte = 0xc0

	tickLen = 28
	syncLen = 164
)

// Tick starts a two-frame step.
type Tick struct {
	Target uint32 // frame the step advances to: the following sync's frame plus 2
}

// MsgType returns TypeTick.
func (*Tick) MsgType() byte { return TypeTick }

// Sync carries the frame of a step. The decoder assigns that frame to every
// message up to the next sync.
type Sync struct {
	Frame  uint32
	Values [17]uint64 // u64 at offsets 28 to 156; candidate state checksums
}

// MsgType returns TypeSync.
func (*Sync) MsgType() byte { return TypeSync }

// NextFrame returns the sync's frame.
func (s *Sync) NextFrame(int64) int64 { return int64(s.Frame) }

func decodeTick(p []byte) (Message, error) {
	if err := checkStep(p, TypeTick, tickLen); err != nil {
		return nil, err
	}
	return &Tick{Target: binary.LittleEndian.Uint32(p[24:])}, nil
}

func decodeSync(p []byte) (Message, error) {
	if err := checkStep(p, TypeSync, syncLen); err != nil {
		return nil, err
	}
	s := &Sync{Frame: binary.LittleEndian.Uint32(p[24:])}
	for i := range s.Values {
		s.Values[i] = binary.LittleEndian.Uint64(p[28+8*i:])
	}
	return s, nil
}

// checkStep checks a record's length and the byte at offset 17.
func checkStep(p []byte, b17 byte, n int) error {
	if len(p) != n {
		return fmt.Errorf("length %d, want %d", len(p), n)
	}
	if p[17] != b17 {
		return fmt.Errorf("byte 17 is %#02x, want %#02x", p[17], b17)
	}
	return nil
}
