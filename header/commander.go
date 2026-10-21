// SPDX-License-Identifier: AGPL-3.0-or-later

package header

import "encoding/binary"

const (
	drawsOffset = 0x37c // 100 × u32
	seedDraw    = 73    // the draw that seeds the mask
	seedXOR     = 0x67e36aed
	namesOffset = 0x50c // side 1's block, then side 2's
	nameBlock   = 50

	steamIDXOR  = 0x314e62818b5f1701
	steamIDHigh = 0x01100001 // top 32 bits of a valid SteamID64
)

// mask is the byte stream the commander names are XOR-masked with.
type mask struct{ s uint32 }

func (m *mask) next() byte {
	m.s = m.s*1103515245 + 12345
	t := m.s
	if t&0x80000000 != 0 {
		t += 0xffff
	}
	return scale(t >> 16 & 0x7fff)
}

// scale narrows a fifteen-bit output to a byte. The game rounds 255·out/32767
// in float32; this integer form gives the same byte for every output, and no
// fused multiply-add can change it.
func scale(out uint32) byte {
	return byte((510*out + 32767) / 65534) //nolint:gosec // G115: at most 255
}

// commanders decodes side 1's and side 2's commander names from raw, a
// header whose fields up to the options are in bounds; shift is 0x80 in a
// ranked header. It returns nil when a block has no NUL or a name is not
// printable UTF-8. The stream runs on across blocks, so a damaged side 1
// leaves side 2 undecodable too.
func commanders(raw []byte, shift int) []string {
	m := mask{s: binary.LittleEndian.Uint32(raw[drawsOffset+shift+4*seedDraw:]) ^ seedXOR}
	names := make([]string, 2)
	for side := range names {
		block := raw[namesOffset+shift+nameBlock*side:][:nameBlock]
		var name []byte
		for _, c := range block {
			c ^= m.next()
			if c == 0 {
				break
			}
			name = append(name, c)
		}
		if len(name) == nameBlock || !isText(name) { // a full block has no NUL
			return nil
		}
		names[side] = string(name)
	}
	return names
}

// steamID returns the SteamID64 that a player entry's two u64 hold, or 0
// when they hold none.
func steamID(a, b uint64) uint64 {
	if id := a ^ b ^ steamIDXOR; id>>32 == steamIDHigh {
		return id
	}
	return 0
}
