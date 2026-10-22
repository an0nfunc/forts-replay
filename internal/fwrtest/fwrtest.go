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

// State is the content of a snapshot payload with its binary state, as
// Payload lays it out.
type State struct {
	NextNode, NextDevice uint32
	Lua                  []string // section texts, each starting with "data = "
	Teams                []Team   // team-list slots in order; the rest are unused
	DamageTaken          [2]float32
	Slots                [9]Slot
	Nodes                []Node
	Gap                  []byte // the item gap, 0 and 1 bytes for a valid one
	Devices              []Device
	Materials            []string
	Links                []Link
}

// Team is one team-list slot and its counters.
type Team struct {
	ID                                 int32
	Metal, Energy, MaxMetal, MaxEnergy float32
	FloatedMetal, FloatedEnergy        float32
	DamageDealt                        float32
}

// Slot is one header player slot.
type Slot struct {
	Team        int32
	APM, Damage float32
}

// Node is one node record. Kind is the byte at +156 that sets its length:
// 0xff for a plain record, 0 to 3 for an extended one (1 is plain on the
// last node).
type Node struct {
	ID   uint32
	Team int32
	X, Y float32
	Kind byte
}

// Device is one device; a weapon gets Beams 24-byte entries and A and B
// 9-byte entries in its weapon block.
type Device struct {
	ID           uint32
	Name         string
	Team         int32
	Type         uint16
	X, Y         float32
	NodeA, NodeB uint32
	Health       float32
	Weapon       bool
	Beams, A, B  int
}

// Link is one strut from node index A to node index B. Device is the index
// of a device on it, or -1; List is the strut's n list; Segments is its
// fire segment count, and a negative count writes no segments.
type Link struct {
	A, B     int
	Material uint32
	HP       float32
	State    byte
	Device   int32
	List     []uint32
	Segments int
}

// Header offsets of the snapshot's binary state, as package state reads
// them.
const (
	stNextNode  = 61
	stTeams     = 81
	stCurrent   = 229
	stMaximum   = 457
	stFloated   = 1141
	stDealt     = 1449
	stTaken     = 1636
	stSlotTeams = 1652
	stSlotBlock = 1688
	stHeaderEnd = stSlotBlock + 9*148
)

// Payload returns the snapshot payload: the header tables, the Lua
// sections, then the world state with a short preamble before the node
// table and a minimal tail after the links.
func (s State) Payload() []byte {
	le := binary.LittleEndian
	f32 := func(b []byte, off int, v float32) { le.PutUint32(b[off:], math.Float32bits(v)) }
	p := make([]byte, stHeaderEnd)
	p[0] = 0xe1
	le.PutUint32(p[stNextNode:], s.NextNode)
	le.PutUint32(p[stNextNode+4:], s.NextDevice)
	for i := range 19 {
		le.PutUint32(p[stTeams+4*i:], 0xffffffff)
	}
	for i, t := range s.Teams {
		le.PutUint32(p[stTeams+4*i:], uint32(t.ID))
		f32(p, stCurrent+12*i+4, t.Metal)
		f32(p, stCurrent+12*i+8, t.Energy)
		f32(p, stMaximum+12*i+4, t.MaxMetal)
		f32(p, stMaximum+12*i+8, t.MaxEnergy)
		f32(p, stFloated+12*i+4, t.FloatedMetal)
		f32(p, stFloated+12*i+8, t.FloatedEnergy)
		f32(p, stDealt+4*i, t.DamageDealt)
	}
	f32(p, stTaken, s.DamageTaken[0])
	f32(p, stTaken+4, s.DamageTaken[1])
	for k, sl := range s.Slots {
		blk := stSlotBlock + 148*k
		le.PutUint32(p[stSlotTeams+4*k:], uint32(sl.Team))
		f32(p, blk+132, sl.APM)
		le.PutUint32(p[blk+140:], uint32(sl.Team))
		f32(p, blk+144, sl.Damage)
	}
	return s.rest(p)
}

// rest appends the Lua sections and the world state to the header tables
// p.
func (s State) rest(p []byte) []byte {
	le := binary.LittleEndian
	f32 := func(v float32) { p = le.AppendUint32(p, math.Float32bits(v)) }
	for _, text := range s.Lua {
		p = le.AppendUint32(p, uint32(len(text)))
		p = append(p, text...)
	}
	p = append(p, 0x16, 0, 0, 0, 0xcd, 0xcc, 0xcc, 0x3e) // preamble, not decoded
	p = le.AppendUint32(p, uint32(len(s.Nodes)))
	p = append(p, 0xff)
	for i, n := range s.Nodes {
		p = append(p, "    "...)
		size := 157
		switch {
		case n.Kind == 1 && i == len(s.Nodes)-1:
		case n.Kind <= 1:
			size = 298
		case n.Kind == 2:
			size = 371
		case n.Kind == 3:
			size = 302
		}
		rec := make([]byte, size)
		le.PutUint32(rec, n.ID)
		le.PutUint32(rec[4:], uint32(n.Team))
		le.PutUint32(rec[20:], math.Float32bits(n.X))
		le.PutUint32(rec[24:], math.Float32bits(n.Y))
		rec[156] = n.Kind
		if size > 157 {
			copy(rec[162:], "0000")
		}
		p = append(p, rec...)
	}
	p = append(p, s.Gap...)
	p = le.AppendUint32(p, uint32(len(s.Devices)))
	for _, d := range s.Devices {
		p = le.AppendUint32(p, uint32(len(d.Name)))
		p = append(p, d.Name...)
		p = le.AppendUint32(p, uint32(d.Team))
		p = le.AppendUint32(p, 0)
		f32(1)
		p = le.AppendUint32(p, 0xdededede)
		body := make([]byte, 168)
		le.PutUint32(body, d.ID)
		le.PutUint32(body[4:], uint32(d.Team))
		le.PutUint16(body[16:], d.Type)
		le.PutUint32(body[20:], math.Float32bits(d.X))
		le.PutUint32(body[24:], math.Float32bits(d.Y))
		le.PutUint32(body[32:], d.NodeA)
		le.PutUint32(body[36:], d.NodeB)
		le.PutUint32(body[44:], math.Float32bits(d.Health))
		p = append(p, body...)
		if d.Weapon {
			w := make([]byte, 376-168)
			copy(w, []byte{0xf1, 0xf1, 0xf1, 0xf1})
			p = append(p, w...)
			p = le.AppendUint32(p, uint32(d.Beams))
			p = append(p, make([]byte, 24*d.Beams)...)
			p = le.AppendUint32(p, uint32(d.A))
			p = append(p, make([]byte, 9*d.A)...)
			p = le.AppendUint32(p, uint32(d.B))
			p = append(p, make([]byte, 9*d.B)...)
		}
	}
	p = le.AppendUint32(p, uint32(len(s.Materials)))
	for _, m := range s.Materials {
		p = le.AppendUint32(p, uint32(len(m)))
		p = append(p, m...)
		p = append(p, 1, 0, 0, 0)
	}
	byNode := make([][]Link, len(s.Nodes))
	for _, l := range s.Links {
		byNode[l.A] = append(byNode[l.A], l)
	}
	for _, out := range byNode {
		p = le.AppendUint32(p, uint32(len(out)))
		for j, l := range out {
			p = append(p, byte(j))
			p = le.AppendUint32(p, uint32(l.B))
			p = le.AppendUint32(p, l.Material)
			p = le.AppendUint32(p, uint32(len(l.List)))
			for _, v := range l.List {
				p = le.AppendUint32(p, v)
			}
			p = le.AppendUint32(p, uint32(l.Device))
			if l.Device >= 0 {
				p = append(p, 0)
				f32(0.5)
			}
			p = le.AppendUint32(p, uint32(l.Segments))
			p = append(p, make([]byte, 9*max(l.Segments, 0))...)
			body := make([]byte, 85)
			le.PutUint32(body, 0x01010101)
			le.PutUint32(body[4:], l.Material)
			le.PutUint32(body[8:], math.Float32bits(l.HP))
			body[41] = l.State
			p = append(p, body...)
		}
	}
	// The tail: no area effects or players, winner -1, no music state or
	// named floats.
	p = append(p, make([]byte, 4+4+4+6)...)
	p = le.AppendUint32(p, 0xffffffff)
	return append(p, make([]byte, 4+4+4)...)
}
