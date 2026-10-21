// SPDX-License-Identifier: AGPL-3.0-or-later

// Package fwrtest builds synthetic replay files for tests.
package fwrtest

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"unicode/utf16"

	"github.com/ulikunitz/xz/lzma"
)

// Header holds the fixed header fields; Extra follows them and the first
// block starts right after Extra.
type Header struct {
	Version  uint32
	Frames   uint32
	Unknown9 uint32
	Extra    []byte
}

// File returns a replay with one block per element of blocks, each given
// as decompressed bytes.
func File(h Header, blocks ...[]byte) []byte {
	var b bytes.Buffer
	put32(&b, h.Version)
	b.WriteByte(0)
	put32(&b, h.Frames)
	put32(&b, h.Unknown9)
	put32(&b, uint32(17+len(h.Extra)))
	b.Write(h.Extra)
	for _, data := range blocks {
		b.Write(Block(data))
	}
	return b.Bytes()
}

// Block compresses data and frames it as one block. Like the game, it
// writes no end marker: the stream stops at the declared size.
func Block(data []byte) []byte {
	var lz bytes.Buffer
	cfg := lzma.WriterConfig{DictCap: 64 << 10, SizeInHeader: true, Size: int64(len(data))}
	w, err := cfg.NewWriter(&lz)
	if err != nil {
		panic(err)
	}
	if _, err := w.Write(data); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	enc := lz.Bytes() // classic header: props (1), dict size (4), size (8); then the raw stream
	var payload bytes.Buffer
	put32(&payload, uint32(len(data)))
	payload.Write(enc[:5])
	payload.Write(enc[13:])
	var b bytes.Buffer
	put32(&b, 0xd1edd1ed)
	put32(&b, uint32(len(data)))
	put32(&b, uint32(payload.Len()))
	b.Write(payload.Bytes())
	return b.Bytes()
}

// Record frames payload as one record, with the keyframe prefix if asked.
func Record(payload []byte, keyframe bool) []byte {
	var b bytes.Buffer
	if keyframe {
		put32(&b, 0xc0c0c0c0)
	}
	put32(&b, 0xabababab)
	put32(&b, uint32(len(payload)))
	b.Write(payload)
	return b.Bytes()
}

func put32(b *bytes.Buffer, v uint32) {
	b.Write(binary.LittleEndian.AppendUint32(nil, v))
}

// Setup holds the header fields of version 1383 after the fixed ones.
// Extra lays them out as Header.Extra.
type Setup struct {
	Ranked       bool
	Options      []string
	Map          string // .fwe file; the .spr file is derived from it
	Recorder     uint32
	RecorderTeam int32
	Players      []Player
	AI           []AI
	Images       int       // embedded images, each with a small payload
	CustomTeams  bool      // team records with custom names
	Commanders   [2]string // save-names of side 1's and side 2's commanders; "" for none
}

// Player is one entry of the header's player table.
type Player struct {
	Slot, ID uint32
	Team     int32
	Name     string // up to 15 UTF-16 code units
	SteamID  uint64 // 0 writes an entry without one
}

// AI is one entry of the header's AI fort list.
type AI struct {
	Team   int32
	Script string
	Value  uint32
}

// Extra returns the header bytes from offset 17 to the first block.
func (s Setup) Extra() []byte {
	var b bytes.Buffer
	b.Write(make([]byte, 0x33a-17))
	if s.Ranked {
		b.WriteByte(1)
		b.Write(bytes.Repeat([]byte{0x5a}, 0x80))
	} else {
		b.WriteByte(0)
	}
	b.Write(s.region()) // save name, draws, commander names
	put32(&b, uint32(len(s.Options)))
	for _, o := range s.Options {
		str8(&b, o)
	}
	str8(&b, s.Map)
	str8(&b, strings.TrimSuffix(s.Map, ".fwe")+".spr")
	put32(&b, 0)
	put32(&b, 7)
	b.Write([]byte{0, 1, 0})
	put32(&b, 1)
	put32(&b, s.Recorder)
	put32(&b, uint32(s.RecorderTeam))
	put32(&b, uint32(len(s.Players)))
	for _, p := range s.Players {
		put32(&b, p.Slot)
		put32(&b, p.ID)
		put32(&b, uint32(p.Team))
		units := utf16.Encode([]rune(p.Name))
		if len(units) > 15 {
			panic("fwrtest: name above 15 UTF-16 units")
		}
		name := make([]byte, 32)
		for i, u := range units {
			binary.LittleEndian.PutUint16(name[2*i:], u)
		}
		b.Write(name)
		entry := make([]byte, 53)
		if p.SteamID != 0 {
			const a = 0x0123456789abcdef
			binary.LittleEndian.PutUint64(entry[21:], a)
			binary.LittleEndian.PutUint64(entry[29:], a^p.SteamID^steamIDMagic)
		}
		b.Write(entry)
		put32(&b, 0xfffffffd)
	}
	put32(&b, 0xa1a1a1a1)
	put32(&b, math.Float32bits(0.5))
	put32(&b, math.Float32bits(0.5))
	put32(&b, uint32(len(s.AI)))
	for _, a := range s.AI {
		put32(&b, uint32(a.Team))
		str8(&b, a.Script)
		put32(&b, a.Value)
	}
	put32(&b, 0xb7b7b7b7)
	for range s.Images {
		put32(&b, 0x3c3c3c3c)
		put32(&b, 311+8)
		b.Write(make([]byte, 311+8))
	}
	for i := range 2 {
		if s.CustomTeams {
			str8(&b, "")
			str16(&b, fmt.Sprintf("Team %c", 'A'+i))
			str8(&b, "")
			str16(&b, string(rune('A'+i)))
			str8(&b, "custom/avatars/team.png")
			put32(&b, 0)
		} else {
			str8(&b, fmt.Sprintf("Team%d", i+1))
			str16(&b, "")
			str8(&b, fmt.Sprintf("T%d", i+1))
			str16(&b, "")
			str8(&b, "ui/textures/hud/observer/avatars/forts.dds")
			put32(&b, 0xffffffff)
		}
	}
	str8(&b, "ui/textures/hud/observer/badges/forts.dds")
	return b.Bytes()
}

// Header bytes 0x33b to 0x570, after the ranked flag; header.Parse holds
// the same constants and mask, which this package cannot import.
const (
	drawsAt      = 0x37c - 0x33b
	namesAt      = 0x50c - 0x33b
	seedXOR      = 0x67e36aed
	steamIDMagic = 0x314e62818b5f1701
)

// region returns the bytes from 0x33b to the options: 100 distinct
// fifteen-bit draws, then the commander names masked with the stream that
// draws[73] seeds. Each name and its NUL are masked; the rest of each
// 50-byte block is zero.
func (s Setup) region() []byte {
	b := make([]byte, 0x570-0x33b)
	draw := func(i int) uint32 { return 0x1000 + 97*uint32(i) }
	for i := range 100 {
		binary.LittleEndian.PutUint32(b[drawsAt+4*i:], draw(i))
	}
	state := draw(73) ^ seedXOR
	mask := func() byte {
		state = state*1103515245 + 12345
		t := state
		if t&0x80000000 != 0 {
			t += 0xffff
		}
		return byte((510*(t>>16&0x7fff) + 32767) / 65534)
	}
	for side, name := range s.Commanders {
		if len(name) > 49 || strings.Contains(name, "\x00") {
			panic("fwrtest: commander name above 49 bytes or with a NUL")
		}
		for i, c := range append([]byte(name), 0) {
			b[namesAt+50*side+i] = c ^ mask()
		}
	}
	return b
}

func str8(b *bytes.Buffer, s string) {
	put32(b, uint32(len(s)))
	b.WriteString(s)
}

func str16(b *bytes.Buffer, s string) {
	units := utf16.Encode([]rune(s))
	put32(b, uint32(2*len(units)))
	for _, u := range units {
		b.Write(binary.LittleEndian.AppendUint16(nil, u))
	}
}

// Tick returns a tick payload advancing to target.
func Tick(target uint32) []byte { return step(0xbe, 28, target) }

// Sync returns a sync payload for frame.
func Sync(frame uint32) []byte { return step(0xc0, 164, frame) }

func step(t byte, n int, v uint32) []byte {
	p := make([]byte, n)
	p[0], p[17] = t, t
	binary.LittleEndian.PutUint32(p[24:], v)
	return p
}

// Snapshot returns a snapshot payload with one Lua section per text; each
// text starts with "data = ".
func Snapshot(texts ...string) []byte {
	p := []byte{0xe1}
	for _, s := range texts {
		p = binary.LittleEndian.AppendUint32(p, uint32(len(s)))
		p = append(p, s...)
	}
	return p
}

// Result returns a ranked result payload (0xbb) from sender.
func Result(sender, winner uint32, team int32, steamID uint64) []byte {
	p := make([]byte, 85)
	p[0], p[17] = 0xbb, 0xbb
	le := binary.LittleEndian
	le.PutUint32(p[20:], sender)
	le.PutUint32(p[24:], winner)
	le.PutUint32(p[28:], uint32(team))
	le.PutUint64(p[32:], steamID)
	le.PutUint64(p[40:], steamID)
	return p
}

// Ability returns a commander ability payload (0xda) from player.
func Ability(player uint32) []byte {
	p := make([]byte, 29)
	p[0], p[17], p[28] = 0xda, 0xda, 1
	binary.LittleEndian.PutUint32(p[20:], player)
	return p
}
