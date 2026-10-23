// SPDX-License-Identifier: AGPL-3.0-or-later

package msg

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Input state record types. The game writes each one when its body changes.
const (
	TypeHover   byte = 0x92
	TypeAim     byte = 0x94
	TypePointer byte = 0x95
	TypeAPM     byte = 0x97
)

// Input holds the field every input record shares.
type Input struct {
	Player uint32 `json:"player"` // u32@20: the sender's player ID from the header's player table; 0 in records the game writes
}

// Sender returns the record's player field.
func (in Input) Sender() uint32 { return in.Player }

// Pointer is the mouse pointer in world coordinates, y growing downward.
type Pointer struct {
	Input

	X float32 `json:"x"`
	Y float32 `json:"y"`
}

// MsgType returns TypePointer.
func (*Pointer) MsgType() byte { return TypePointer }

// Hover is what the pointer is over and what the player has selected.
type Hover struct {
	Input

	NodeA     uint16 `json:"node_a"`     // first node of the hovered or selected link, or the pressed node
	NodeB     uint16 `json:"node_b"`     // second node of that link, 0 when NodeA is a single node
	Device    uint32 `json:"device"`     // selected device instance, 0 for none
	Terrain   uint16 `json:"terrain"`    // hovered terrain block, 0xffff for none
	HoverKind uint8  `json:"hover_kind"` // 0 nothing, 1 node, 5 terrain, 6 link, 7 device
	Material  uint8  `json:"material"`   // selected material, 0xff for none
	BuildType uint8  `json:"build_type"` // selected device or weapon type, 0xff for none
	Buttons   uint8  `json:"buttons"`    // mouse buttons held: 1 left, 2 right
	Feedback  uint8  `json:"feedback"`   // candidate cursor feedback, such as 13 reloading or 14 line of fire blocked
}

// MsgType returns TypeHover.
func (*Hover) MsgType() byte { return TypeHover }

// APM is the game's actions-per-minute figure: ten times the actions in the
// last 150 frames.
type APM struct {
	Input

	Rate float32 `json:"rate"`
}

// MsgType returns TypeAPM.
func (*APM) MsgType() byte { return TypeAPM }

// Aim is written as the pointer moves while a swarm launcher is selected,
// and on cancel. Candidate: the selected weapon's aim point.
type Aim struct {
	Input

	X float32 `json:"x"`
	Y float32 `json:"y"`
}

// MsgType returns TypeAim.
func (*Aim) MsgType() byte { return TypeAim }

// body reads the fields of a record whose length is checked. It keeps the
// first error.
type body struct {
	p   []byte
	err error
}

func (b *body) u8(off int) uint8   { return b.p[off] }
func (b *body) u16(off int) uint16 { return binary.LittleEndian.Uint16(b.p[off:]) }
func (b *body) u32(off int) uint32 { return binary.LittleEndian.Uint32(b.p[off:]) }
func (b *body) u64(off int) uint64 { return binary.LittleEndian.Uint64(b.p[off:]) }

func (b *body) i32(off int) int32 {
	return int32(b.u32(off)) //nolint:gosec // G115: the field is a two's-complement i32
}

// f32 reads a float that must be finite.
func (b *body) f32(off int) float32 {
	v := math.Float32frombits(b.u32(off))
	if f := float64(v); (math.IsNaN(f) || math.IsInf(f, 0)) && b.err == nil {
		b.err = fmt.Errorf("f32 at offset %d is %v", off, v)
	}
	return v
}

// flag reads a byte that must be 0 or 1.
func (b *body) flag(off int) bool {
	v := b.u8(off)
	if v > 1 && b.err == nil {
		b.err = fmt.Errorf("byte %d is %d, want 0 or 1", off, v)
	}
	return v == 1
}

// inputDecoder returns the decoder for an input record of n bytes whose
// byte 17 repeats its type. read builds the message from the record's
// fields.
func inputDecoder(n int, read func(in Input, b *body) Message) DecodeFunc {
	return inputDecoderAlt(n, 0, read)
}

// inputDecoderAlt is inputDecoder that also accepts the type minus dec at
// offset 17.
func inputDecoderAlt(n int, dec byte, read func(in Input, b *body) Message) DecodeFunc {
	return func(p []byte) (Message, error) {
		if err := checkStep(p, n, dec); err != nil {
			return nil, err
		}
		b := &body{p: p}
		m := read(Input{Player: b.u32(20)}, b)
		if b.err != nil {
			return nil, b.err
		}
		return m, nil
	}
}

// registerInputs adds the input state decoders for version v.
func registerInputs(r *Registry, v uint32) {
	// Player 1 writes the type minus 1 at byte 17 in single-player games and
	// in some multiplayer replays; every other player writes the type.
	register(r, v, TypePointer, inputDecoderAlt(37, 1, func(in Input, b *body) Message {
		return &Pointer{Input: in, X: b.f32(28), Y: b.f32(32)}
	}))
	register(r, v, TypeHover, inputDecoderAlt(53, 1, func(in Input, b *body) Message {
		return &Hover{
			Input: in, NodeA: b.u16(28), NodeB: b.u16(30), Device: b.u32(32), Terrain: b.u16(40),
			HoverKind: b.u8(42), Material: b.u8(43), BuildType: b.u8(44), Buttons: b.u8(45), Feedback: b.u8(46),
		}
	}))
	register(r, v, TypeAPM, inputDecoder(36, func(in Input, b *body) Message {
		return &APM{Input: in, Rate: b.f32(28)}
	}))
	// u32@24 repeats the player.
	register(r, v, TypeAim, inputDecoder(37, func(in Input, b *body) Message {
		return &Aim{Input: in, X: b.f32(28), Y: b.f32(32)}
	}))
}
